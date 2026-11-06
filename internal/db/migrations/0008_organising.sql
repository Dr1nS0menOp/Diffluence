-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Organising (spec 011): kinds of pages, owners, statuses, labels, templates,
-- approvals, the archive, scheduling, and more of a space's own settings.

ALTER TABLE pages
    ADD COLUMN kind                text NOT NULL DEFAULT 'page' CHECK (kind IN ('page', 'post', 'folder', 'link', 'template')),
    ADD COLUMN owner_id            uuid,
    ADD COLUMN stage_text          text NOT NULL DEFAULT '',
    ADD COLUMN stage_color         text NOT NULL DEFAULT '',
    ADD COLUMN review_on           date,
    ADD COLUMN review_reminded_on  date,
    ADD COLUMN archived_at         timestamptz,
    ADD COLUMN archived_by         uuid,
    ADD COLUMN published_at        timestamptz,
    ADD COLUMN link_target         text NOT NULL DEFAULT '';

-- The previous release does not write these, so readers fall back to the creator.
UPDATE pages SET owner_id = created_by;
UPDATE pages p SET published_at = v.first_version
FROM (SELECT page_id, min(created_at) AS first_version FROM page_versions GROUP BY page_id) v
WHERE v.page_id = p.id;

CREATE INDEX pages_kind_idx ON pages (space_id, kind);
CREATE INDEX pages_archived_idx ON pages (space_id) WHERE archived_at IS NOT NULL;
CREATE INDEX pages_published_idx ON pages (published_at DESC) WHERE kind = 'post';

ALTER TABLE spaces
    ADD COLUMN system             boolean NOT NULL DEFAULT false,
    ADD COLUMN free_stage         boolean NOT NULL DEFAULT false,
    ADD COLUMN review_months      integer NOT NULL DEFAULT 0,
    ADD COLUMN approval_required  boolean NOT NULL DEFAULT false,
    ADD COLUMN fixed_labels       boolean NOT NULL DEFAULT false,
    ADD COLUMN title_align        text NOT NULL DEFAULT 'start' CHECK (title_align IN ('start', 'center')),
    ADD COLUMN icon_object        text NOT NULL DEFAULT '';

ALTER TABLE recent_pages ADD COLUMN seen_version integer;
ALTER TABLE drafts ADD COLUMN warned_at timestamptz;

CREATE TABLE page_labels (
    page_id     uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    label       text NOT NULL,
    created_by  uuid NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, label)
);
CREATE INDEX page_labels_label_idx ON page_labels (label);

CREATE TABLE space_fixed_labels (
    space_id  uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    label     text NOT NULL,
    PRIMARY KEY (space_id, label)
);

CREATE TABLE space_stages (
    space_id  uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    position  integer NOT NULL,
    text      text NOT NULL,
    color     text NOT NULL,
    PRIMARY KEY (space_id, position)
);

CREATE TABLE space_owners (
    space_id  uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    user_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (space_id, user_id)
);

CREATE TABLE space_approvers (
    space_id  uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    user_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (space_id, user_id)
);

-- An old key keeps working and stays taken, until the space releases it.
CREATE TABLE space_key_aliases (
    alias     text PRIMARY KEY,
    space_id  uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE
);

-- A missing row means the default, so existing spaces keep every feature.
CREATE TABLE space_features (
    space_id  uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    feature   text NOT NULL,
    enabled   boolean NOT NULL,
    PRIMARY KEY (space_id, feature)
);

CREATE TABLE page_verifications (
    page_id      uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    verified_by  uuid NOT NULL,
    verified_at  timestamptz NOT NULL DEFAULT now(),
    valid_until  date NOT NULL,
    version      integer NOT NULL
);

CREATE TABLE scheduled_publications (
    page_id     uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    user_id     uuid NOT NULL,
    publish_at  timestamptz NOT NULL,
    silent      boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX scheduled_publications_due_idx ON scheduled_publications (publish_at);

CREATE TABLE approval_rounds (
    id            uuid PRIMARY KEY,
    page_id       uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    asked_by      uuid NOT NULL,
    message       text NOT NULL DEFAULT '',
    due_on        date,
    title         text NOT NULL,
    document      jsonb NOT NULL,
    base_version  integer NOT NULL,
    approved_version integer,
    status        text NOT NULL CHECK (status IN ('open', 'approved', 'closed')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    closed_at     timestamptz
);
CREATE INDEX approval_rounds_page_idx ON approval_rounds (page_id, created_at DESC);

CREATE TABLE approval_answers (
    round_id     uuid NOT NULL REFERENCES approval_rounds (id) ON DELETE CASCADE,
    user_id      uuid NOT NULL,
    answer       text CHECK (answer IN ('approve', 'changes')),
    comment      text NOT NULL DEFAULT '',
    answered_at  timestamptz,
    PRIMARY KEY (round_id, user_id)
);

CREATE TABLE removed_versions (
    page_id     uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    number      integer NOT NULL,
    removed_by  uuid NOT NULL,
    reason      text NOT NULL,
    removed_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, number)
);

CREATE TABLE templates (
    id             uuid PRIMARY KEY,
    page_id        uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    space_id       uuid REFERENCES spaces (id) ON DELETE CASCADE,
    name           text NOT NULL,
    description    text NOT NULL DEFAULT '',
    category       text NOT NULL DEFAULT '',
    labels         text[] NOT NULL DEFAULT '{}',
    title_pattern  text NOT NULL DEFAULT '',
    steps          jsonb NOT NULL DEFAULT '[]',
    default_parent uuid,
    hidden         boolean NOT NULL DEFAULT false,
    promoted       boolean NOT NULL DEFAULT false,
    starter_key    text,
    created_by     uuid NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX templates_space_idx ON templates (space_id);
CREATE UNIQUE INDEX templates_starter_key ON templates (starter_key) WHERE starter_key IS NOT NULL;

CREATE TABLE template_favourites (
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    template_id  uuid NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, template_id)
);

CREATE TABLE template_uses (
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    template_id  uuid NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
    used_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, template_id)
);

CREATE TABLE space_templates (
    id           uuid PRIMARY KEY,
    name         text NOT NULL,
    description  text NOT NULL DEFAULT '',
    bundle       jsonb NOT NULL,
    starter_key  text,
    created_by   uuid NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX space_templates_starter_key ON space_templates (starter_key) WHERE starter_key IS NOT NULL;

-- A template is never a page to anyone, whichever list is written next.
CREATE OR REPLACE FUNCTION perm_visible_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT p.id
    FROM pages p
    WHERE p.space_id IN (SELECT ps.space_id FROM perm_spaces(u, 'space.view') ps)
      AND (p.status = 'published' OR (p.status = 'draft' AND p.created_by = u))
      AND p.kind <> 'template'
      AND p.id NOT IN (SELECT h.page_id FROM perm_hidden_pages(u) h)
$$;

-- What lists show: visible, and not in the archive. A list that used
-- perm_visible_pages would bring archived pages back.
CREATE FUNCTION perm_listed_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT v.page_id
    FROM perm_visible_pages(u) v
    JOIN pages p ON p.id = v.page_id
    WHERE p.archived_at IS NULL
$$;

-- One store for what is on in a space: what wave C kept in spaces.features moves here.
-- The column stays for the previous release, and goes in a contract migration.
INSERT INTO space_features (space_id, feature, enabled)
SELECT s.id, f.key, f.value::boolean
FROM spaces s, jsonb_each_text(s.features) f
WHERE f.key IN ('comments', 'reactions', 'access_requests')
ON CONFLICT DO NOTHING;

-- Pages a person may edit: the rule of page.edit for a list.
CREATE FUNCTION perm_editable_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT v.page_id
    FROM perm_visible_pages(u) v
    JOIN pages p ON p.id = v.page_id
    JOIN spaces s ON s.id = p.space_id
    WHERE s.status = 'active' AND p.archived_at IS NULL
      AND p.space_id IN (SELECT e.space_id FROM perm_spaces(u, 'page.edit') e)
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

-- Comments a person may see: view on the page, not in the trash, and for a thread for
-- editors also edit on the page. Everything that lists comments goes through this.
CREATE FUNCTION perm_visible_comments(u uuid) RETURNS TABLE (comment_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT c.id
    FROM comments c
    JOIN perm_visible_pages(u) v ON v.page_id = c.page_id
    WHERE c.trash_item_id IS NULL AND NOT c.purged
      AND (NOT c.editors_only OR c.page_id IN (SELECT e.page_id FROM perm_editable_pages(u) e))
$$;

CREATE INDEX comments_words_idx ON comments
    USING gin (jsonb_to_tsvector('simple', jsonb_path_query_array(body, '$.**.text'), '["string"]'));
