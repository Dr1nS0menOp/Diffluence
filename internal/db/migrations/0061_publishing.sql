-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 073, wave PB1: draft and published spaces. Two spaces are linked as a pair: writers
-- work in the draft space, readers have the published space, and content crosses only by a
-- release (PUB-200). The published space is managed: the permission rule refuses every write
-- of content there, and the trigger below holds that in the database too, for a node of the
-- previous release and for a write path that forgets the rule (PUB-220, PUB-224, PUB-1306).
--
-- Expand only (PUB-1300): new tables, one nullable column on spaces that the previous release
-- never reads, a trigger that does nothing while no space is published, and space.release
-- split off space.administer as specs 036, 040, 045, 049 and 064 split theirs, so nobody
-- gains or loses anything. It may run twice, as every migration since 0049 may.

-- Which side of a pair a space is on. The write paths read it with the space row they load
-- already, and the trigger reads it by key (PUB-340).
ALTER TABLE spaces ADD COLUMN IF NOT EXISTS pair_role text CHECK (pair_role IN ('draft', 'published'));

-- A pair, from a draft space to a published space. Its settings are JSON checked in Go
-- (internal/publishing). An ended pair stays on record (PUB-245).
CREATE TABLE IF NOT EXISTS space_pairs (
    id                  uuid PRIMARY KEY,
    draft_space_id      uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    published_space_id  uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    settings            jsonb NOT NULL DEFAULT '{}',
    created_by          uuid NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    ended_at            timestamptz,
    ended_by            uuid,
    CHECK (draft_space_id <> published_space_id)
);
-- A space is in at most one active pair on either side (PUB-200, PUB-1050, PUB-1051).
CREATE UNIQUE INDEX IF NOT EXISTS space_pairs_active_draft ON space_pairs (draft_space_id) WHERE ended_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS space_pairs_active_published ON space_pairs (published_space_id) WHERE ended_at IS NULL;

-- One row per draft page with a twin: the version last released and the facts of each aspect
-- at that release, and the aspects changed since, kept by the triggers below in the
-- transaction of every change (PUB-201, PUB-340, decision D3). A home page is paired when the
-- link is made, with version 0, which reads as not released yet (PUB-213).
CREATE TABLE IF NOT EXISTS page_pairs (
    draft_page_id       uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    pair_id             uuid NOT NULL REFERENCES space_pairs (id) ON DELETE CASCADE,
    twin_page_id        uuid NOT NULL UNIQUE REFERENCES pages (id) ON DELETE CASCADE,
    released_version    integer NOT NULL DEFAULT 0,
    twin_version        integer NOT NULL DEFAULT 0,
    released_at         timestamptz,
    released_by         uuid,
    run_id              uuid,
    released_title      text NOT NULL DEFAULT '',
    released_parent_id  uuid,
    released_look       jsonb NOT NULL DEFAULT '{}',
    released_labels     text[] NOT NULL DEFAULT '{}',
    released_hash       text NOT NULL DEFAULT '',
    -- Draft pages this page linked to that had no twin at its release: their links are text.
    text_links          uuid[] NOT NULL DEFAULT '{}',
    changed             text[] NOT NULL DEFAULT '{}',
    changed_since       timestamptz
);
CREATE INDEX IF NOT EXISTS page_pairs_pair ON page_pairs (pair_id);

-- A file of a draft page and its stored copy on the twin, with the version copied (PUB-202).
CREATE TABLE IF NOT EXISTS file_pairs (
    draft_attachment_id  uuid NOT NULL REFERENCES attachments (id) ON DELETE CASCADE,
    twin_page_id         uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    copy_attachment_id   uuid NOT NULL REFERENCES attachments (id) ON DELETE CASCADE,
    copied_version       integer NOT NULL,
    PRIMARY KEY (draft_attachment_id, twin_page_id)
);
CREATE INDEX IF NOT EXISTS file_pairs_copy ON file_pairs (copy_attachment_id);

-- A run starts as a preview, which holds the versions it will release (PUB-430, PUB-431).
-- Its times come from the database server (PUB-1064).
CREATE TABLE IF NOT EXISTS release_runs (
    id            uuid PRIMARY KEY,
    pair_id       uuid NOT NULL REFERENCES space_pairs (id) ON DELETE CASCADE,
    scope         text NOT NULL CHECK (scope IN ('page', 'branch', 'space')),
    root_page_id  uuid,
    status        text NOT NULL CHECK (status IN ('previewed', 'done', 'failed', 'expired')),
    note          text NOT NULL DEFAULT '',
    silent        boolean NOT NULL DEFAULT false,
    actor_type    text NOT NULL DEFAULT 'user',
    actor_id      uuid,
    caused_by     uuid,
    previewed_at  timestamptz NOT NULL DEFAULT now(),
    started_at    timestamptz,
    finished_at   timestamptz,
    counts        jsonb NOT NULL DEFAULT '{}',
    report        jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS release_runs_pair ON release_runs (pair_id, previewed_at);

-- Each page of a run: the draft version it takes, the twin version it became, what it did,
-- the content hash of the draft version and the link mapping, for verification (PUB-732, D9).
CREATE TABLE IF NOT EXISTS release_run_pages (
    run_id         uuid NOT NULL REFERENCES release_runs (id) ON DELETE CASCADE,
    draft_page_id  uuid NOT NULL,
    draft_version  integer NOT NULL,
    twin_page_id   uuid,
    twin_version   integer,
    action         text NOT NULL,
    outcome        text NOT NULL DEFAULT 'waiting',
    content_hash   text NOT NULL DEFAULT '',
    link_map       jsonb NOT NULL DEFAULT '{}',
    PRIMARY KEY (run_id, draft_page_id)
);

-- publishing_managed says whether a space is the published side of a pair.
CREATE OR REPLACE FUNCTION publishing_managed(space uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT EXISTS (SELECT 1 FROM spaces WHERE id = space AND pair_role = 'published')
$$;

-- publishing_refuse stops a write into a published space that is not a release (PUB-224). The
-- release path marks its transaction with the local setting diffluence.release; the
-- application role can set it too, so this protects against mistakes and older nodes, not
-- against a compromised application, which the permission rule and the audit cover (D6).
CREATE OR REPLACE FUNCTION publishing_refuse(space uuid) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    IF space IS NOT NULL AND current_setting('diffluence.release', true) IS DISTINCT FROM 'on'
       AND publishing_managed(space) THEN
        RAISE EXCEPTION 'This space is changed only by releases: it is the published space of a pair (spec 073).'
            USING ERRCODE = 'DF073';
    END IF;
END $$;

CREATE OR REPLACE FUNCTION publishing_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    CASE TG_TABLE_NAME
    WHEN 'pages' THEN
        PERFORM publishing_refuse(NEW.space_id);
        IF TG_OP = 'UPDATE' AND OLD.space_id IS DISTINCT FROM NEW.space_id THEN
            PERFORM publishing_refuse(OLD.space_id);
        END IF;
    WHEN 'attachments' THEN
        PERFORM publishing_refuse(NEW.space_id);
        IF TG_OP = 'UPDATE' AND OLD.space_id IS DISTINCT FROM NEW.space_id THEN
            PERFORM publishing_refuse(OLD.space_id);
        END IF;
    WHEN 'page_versions' THEN
        PERFORM publishing_refuse((SELECT p.space_id FROM pages p WHERE p.id = NEW.page_id));
    WHEN 'attachment_versions' THEN
        PERFORM publishing_refuse((SELECT a.space_id FROM attachments a WHERE a.id = NEW.attachment_id));
    END CASE;
    RETURN NEW;
END $$;

-- Only what a reader sees counts as a write: the owner, the review date or closing comments
-- of a page are not content, so they stay free (PUB-221).
DROP TRIGGER IF EXISTS pages_publishing_insert ON pages;
CREATE TRIGGER pages_publishing_insert BEFORE INSERT ON pages
    FOR EACH ROW EXECUTE FUNCTION publishing_guard();
DROP TRIGGER IF EXISTS pages_publishing_update ON pages;
CREATE TRIGGER pages_publishing_update BEFORE UPDATE ON pages
    FOR EACH ROW WHEN (OLD.space_id IS DISTINCT FROM NEW.space_id OR OLD.title IS DISTINCT FROM NEW.title
        OR OLD.current_version IS DISTINCT FROM NEW.current_version OR OLD.parent_id IS DISTINCT FROM NEW.parent_id
        OR OLD.position IS DISTINCT FROM NEW.position OR OLD.status IS DISTINCT FROM NEW.status
        OR OLD.archived_at IS DISTINCT FROM NEW.archived_at OR OLD.kind IS DISTINCT FROM NEW.kind
        OR OLD.link_target IS DISTINCT FROM NEW.link_target OR OLD.emoji IS DISTINCT FROM NEW.emoji
        OR OLD.full_width IS DISTINCT FROM NEW.full_width OR OLD.header_attachment_id IS DISTINCT FROM NEW.header_attachment_id
        OR OLD.header_position IS DISTINCT FROM NEW.header_position OR OLD.language IS DISTINCT FROM NEW.language)
    EXECUTE FUNCTION publishing_guard();
DROP TRIGGER IF EXISTS page_versions_publishing ON page_versions;
CREATE TRIGGER page_versions_publishing BEFORE INSERT ON page_versions
    FOR EACH ROW EXECUTE FUNCTION publishing_guard();
DROP TRIGGER IF EXISTS attachments_publishing_insert ON attachments;
CREATE TRIGGER attachments_publishing_insert BEFORE INSERT ON attachments
    FOR EACH ROW EXECUTE FUNCTION publishing_guard();
DROP TRIGGER IF EXISTS attachments_publishing_update ON attachments;
CREATE TRIGGER attachments_publishing_update BEFORE UPDATE ON attachments
    FOR EACH ROW WHEN (OLD.space_id IS DISTINCT FROM NEW.space_id OR OLD.page_id IS DISTINCT FROM NEW.page_id
        OR OLD.status IS DISTINCT FROM NEW.status OR OLD.current_version IS DISTINCT FROM NEW.current_version
        OR OLD.filename IS DISTINCT FROM NEW.filename OR OLD.comment IS DISTINCT FROM NEW.comment)
    EXECUTE FUNCTION publishing_guard();
DROP TRIGGER IF EXISTS attachment_versions_publishing ON attachment_versions;
CREATE TRIGGER attachment_versions_publishing BEFORE INSERT ON attachment_versions
    FOR EACH ROW EXECUTE FUNCTION publishing_guard();

-- page_pair_look is the look of a page as out of step compares it: the emoji, the header
-- image, the width and the language (PUB-303, PUB-1407).
CREATE OR REPLACE FUNCTION page_pair_look(emoji text, full_width boolean, header uuid, header_position integer, language text)
    RETURNS jsonb LANGUAGE sql IMMUTABLE AS $$
    SELECT jsonb_build_object('emoji', emoji, 'full_width', full_width, 'header', header,
        'header_position', header_position, 'language', language)
$$;

-- page_pair_content_hash is the hash of a stored document. jsonb prints its keys in one order,
-- so two documents that differ only in the order of their attributes hash the same (PUB-300).
CREATE OR REPLACE FUNCTION page_pair_content_hash(doc jsonb) RETURNS text LANGUAGE sql IMMUTABLE AS $$
    SELECT encode(sha256(convert_to(doc::text, 'UTF8')), 'hex')
$$;

-- page_pair_refresh compares a draft page with the facts of its last release and keeps the
-- aspects that differ (PUB-300 to PUB-304). A change that is undone matches the facts again,
-- so the page is released again (PUB-342). A page without a pair costs one look-up by key.
-- Place is the parent in PB1; the order among siblings comes with PUB-412 in PB2.
CREATE OR REPLACE FUNCTION page_pair_refresh(draft uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    pp page_pairs%ROWTYPE;
    aspects text[] := '{}';
    p record;
    labels text[];
BEGIN
    SELECT * INTO pp FROM page_pairs WHERE draft_page_id = draft;
    IF NOT FOUND OR pp.released_version = 0 THEN
        RETURN;
    END IF;
    SELECT pg.title, pg.current_version, pg.parent_id,
           page_pair_look(pg.emoji, pg.full_width, pg.header_attachment_id, pg.header_position, pg.language) AS look
        INTO p FROM pages pg WHERE pg.id = draft;
    IF p.current_version <> pp.released_version AND (SELECT page_pair_content_hash(v.document) FROM page_versions v
            WHERE v.page_id = draft AND v.number = p.current_version) IS DISTINCT FROM pp.released_hash THEN
        aspects := aspects || 'text'::text;
    END IF;
    IF p.title IS DISTINCT FROM pp.released_title THEN
        aspects := aspects || 'title'::text;
    END IF;
    IF p.parent_id IS DISTINCT FROM pp.released_parent_id THEN
        aspects := aspects || 'place'::text;
    END IF;
    IF p.look IS DISTINCT FROM pp.released_look THEN
        aspects := aspects || 'look'::text;
    END IF;
    SELECT COALESCE(array_agg(l.label ORDER BY l.label), '{}') INTO labels FROM page_labels l WHERE l.page_id = draft;
    IF labels IS DISTINCT FROM pp.released_labels THEN
        aspects := aspects || 'labels'::text;
    END IF;
    UPDATE page_pairs SET changed = aspects,
        changed_since = CASE WHEN cardinality(aspects) = 0 THEN NULL ELSE COALESCE(changed_since, now()) END
    WHERE draft_page_id = draft AND changed IS DISTINCT FROM aspects;
END $$;

CREATE OR REPLACE FUNCTION page_pair_touched() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_TABLE_NAME = 'page_labels' THEN
        PERFORM page_pair_refresh(CASE WHEN TG_OP = 'DELETE' THEN OLD.page_id ELSE NEW.page_id END);
    ELSE
        PERFORM page_pair_refresh(NEW.id);
    END IF;
    RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS pages_page_pair ON pages;
CREATE TRIGGER pages_page_pair AFTER UPDATE ON pages
    FOR EACH ROW WHEN (OLD.title IS DISTINCT FROM NEW.title OR OLD.current_version IS DISTINCT FROM NEW.current_version
        OR OLD.parent_id IS DISTINCT FROM NEW.parent_id OR OLD.emoji IS DISTINCT FROM NEW.emoji
        OR OLD.full_width IS DISTINCT FROM NEW.full_width OR OLD.header_attachment_id IS DISTINCT FROM NEW.header_attachment_id
        OR OLD.header_position IS DISTINCT FROM NEW.header_position OR OLD.language IS DISTINCT FROM NEW.language)
    EXECUTE FUNCTION page_pair_touched();
DROP TRIGGER IF EXISTS page_labels_page_pair ON page_labels;
CREATE TRIGGER page_labels_page_pair AFTER INSERT OR DELETE ON page_labels
    FOR EACH ROW EXECUTE FUNCTION page_pair_touched();

-- Pages a person may edit: as before, and never in a published space (PUB-012, PUB-220).
-- permissions.Evaluate is the same rule, and the matrix test holds the two equal.
CREATE OR REPLACE FUNCTION perm_editable_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT v.page_id
    FROM perm_visible_pages(u) v
    JOIN pages p ON p.id = v.page_id
    JOIN spaces s ON s.id = p.space_id
    WHERE s.status = 'active' AND p.archived_at IS NULL
      AND s.pair_role IS DISTINCT FROM 'published'
      AND CASE WHEN p.kind = 'database'
            THEN p.space_id IN (SELECT e.space_id FROM perm_spaces(u, 'database.edit') e)
            ELSE p.space_id IN (SELECT e.space_id FROM perm_spaces(u, 'page.edit') e) END
      AND p.id NOT IN (
        SELECT r.page_id
        FROM page_restrictions r
        WHERE r.kind = 'edit'
        GROUP BY r.page_id
        HAVING NOT bool_or(
               (r.subject_type = 'user' AND r.subject_id = u)
            OR (r.subject_type = 'group' AND r.subject_id IN
                   (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = u))))
$$;

-- A space that becomes managed, or stops being so, changes what its grants allow, so nodes
-- must read the facts of a permission check again (spec 071).
DROP TRIGGER IF EXISTS spaces_bump_permissions ON spaces;
CREATE CONSTRAINT TRIGGER spaces_bump_permissions AFTER INSERT OR DELETE OR UPDATE OF status, pair_role ON spaces
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION permission_generation_bump_once();

CREATE OR REPLACE FUNCTION rbac_split_off(broad text) RETURNS text[] LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE broad
        WHEN 'space.administer' THEN ARRAY['site.publish', 'space.automation', 'space.templates', 'space.audit',
            'space.labels', 'space.statuses', 'space.approvals', 'space.bundle', 'space.look', 'page.watchers',
            'space.workflows', 'space.release']
        WHEN 'page.edit' THEN ARRAY['page.move_out', 'database.edit', 'page.workflow']
        WHEN 'space.manage_access' THEN ARRAY['space.invite_guests']
        WHEN 'page.create' THEN ARRAY['post.create', 'database.create']
        ELSE '{}'::text[] END
$$;

CREATE OR REPLACE FUNCTION rbac_words(perms text[]) RETURNS text LANGUAGE sql IMMUTABLE AS $$
    SELECT string_agg(CASE p
        WHEN 'space.view' THEN 'View the space'
        WHEN 'page.create' THEN 'Create pages'
        WHEN 'post.create' THEN 'Create blog posts'
        WHEN 'database.create' THEN 'Create databases'
        WHEN 'page.edit' THEN 'Edit pages'
        WHEN 'database.edit' THEN 'Edit databases'
        WHEN 'page.workflow' THEN 'Set a workflow on a page'
        WHEN 'page.archive' THEN 'Archive pages'
        WHEN 'page.delete_own' THEN 'Delete own pages'
        WHEN 'page.delete_any' THEN 'Delete any page'
        WHEN 'page.move_out' THEN 'Move pages to another space'
        WHEN 'page.watchers' THEN 'Manage who watches'
        WHEN 'space.export' THEN 'Export'
        WHEN 'space.bundle' THEN 'Bundle the space'
        WHEN 'comment.add' THEN 'Add comments'
        WHEN 'comment.delete_own' THEN 'Delete own comments'
        WHEN 'comment.delete_any' THEN 'Delete any comment'
        WHEN 'attachment.add' THEN 'Add files'
        WHEN 'attachment.delete_own' THEN 'Delete own files'
        WHEN 'attachment.delete_any' THEN 'Delete any file'
        WHEN 'space.manage_access' THEN 'Manage access'
        WHEN 'space.invite_guests' THEN 'Ask for guests'
        WHEN 'restriction.manage' THEN 'Restrict pages'
        WHEN 'space.administer' THEN 'Run the space'
        WHEN 'site.publish' THEN 'Publish to the web'
        WHEN 'space.automation' THEN 'Automation'
        WHEN 'space.templates' THEN 'Templates'
        WHEN 'space.audit' THEN 'See the audit'
        WHEN 'space.labels' THEN 'Labels'
        WHEN 'space.statuses' THEN 'Statuses and reviews'
        WHEN 'space.approvals' THEN 'Approval rule'
        WHEN 'space.workflows' THEN 'Workflows'
        WHEN 'space.release' THEN 'Release to the published space'
        WHEN 'space.look' THEN 'Look and shortcuts'
        WHEN 'analytics.view' THEN 'View analytics'
        WHEN 'use_product' THEN 'Use the product'
        WHEN 'view_profiles' THEN 'See people'
        WHEN 'invite_people' THEN 'Invite people'
        WHEN 'create_space' THEN 'Create spaces'
        WHEN 'create_personal_space' THEN 'Have a personal space'
        WHEN 'administrator' THEN 'Administrator'
        WHEN 'system_administrator' THEN 'System administrator'
        ELSE p END, ', ' ORDER BY p)
    FROM unnest(perms) AS p
$$;

SELECT rbac_split_roles_from('space.administer', ARRAY['space.release']);
