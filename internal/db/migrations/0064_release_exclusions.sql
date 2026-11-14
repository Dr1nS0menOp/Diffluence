-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 073, wave PB2: the whole space, leaving out, taking off and the release screen. Pages
-- can be left out of releases, removals in the draft are kept until a release mirrors them,
-- runs gain batches and a point to resume from, and out of step learns the order among
-- siblings, files and links waiting (PUB-302, PUB-305, PUB-306, PUB-420, PUB-451).
--
-- Expand only (PUB-1300): new tables, new nullable or defaulted columns the previous release
-- never reads, wider checks on release_runs, and functions and triggers that do nothing while
-- no space is paired. It may run twice.

-- A draft page left out of releases, alone or with everything below it (PUB-420). The mark
-- belongs to the page, not to a pair, so a bundle can carry it (PUB-1042). Pages below a mark
-- "with children" are left out by the tree itself, so a new child and a move out follow
-- without a write.
CREATE TABLE IF NOT EXISTS release_exclusions (
    draft_page_id  uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    with_children  boolean NOT NULL DEFAULT false,
    reason         text NOT NULL DEFAULT '',
    created_by     uuid NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- A twin whose draft page left the pair for good: purged, or moved to another space. Its page
-- pair is gone, so this row keeps it on the release screen as Removed in the draft until it is
-- taken off (PUB-331, PUB-1001, PUB-1003, PUB-1012). Deleted and archived pages keep their page
-- pair, because they can come back (PUB-1011, PUB-1013).
CREATE TABLE IF NOT EXISTS release_removals (
    twin_page_id   uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    pair_id        uuid NOT NULL REFERENCES space_pairs (id) ON DELETE CASCADE,
    draft_page_id  uuid NOT NULL,
    cause          text NOT NULL CHECK (cause IN ('moved', 'purged')),
    removed_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS release_removals_pair ON release_removals (pair_id);

-- The release screen reads the page pairs of a pair, the oldest change first (PUB-1103,
-- PUB-1220); pages that link to a page just released are found by their text links (PUB-512).
-- rewrite-ok: the table came with migration 0061 of this same release and holds rows only once the experimental part is on
CREATE INDEX IF NOT EXISTS page_pairs_pair_changed ON page_pairs (pair_id, changed_since);
-- rewrite-ok: the table came with migration 0061 of this same release and holds rows only once the experimental part is on
CREATE INDEX IF NOT EXISTS page_pairs_text_links ON page_pairs USING gin (text_links);

-- Runs of a branch, the whole space, a choice of pages, or a take-off, in the worker, in one
-- transaction or in batches with a point to resume from (PUB-451 to PUB-455, PUB-1061).
ALTER TABLE release_runs DROP CONSTRAINT IF EXISTS release_runs_scope_check;
-- rewrite-ok: the table came with migration 0061 of this same release and holds rows only once the experimental part is on
ALTER TABLE release_runs ADD CONSTRAINT release_runs_scope_check
    CHECK (scope IN ('page', 'branch', 'space', 'pages', 'take_off'));
ALTER TABLE release_runs DROP CONSTRAINT IF EXISTS release_runs_status_check;
-- rewrite-ok: the table came with migration 0061 of this same release and holds rows only once the experimental part is on
ALTER TABLE release_runs ADD CONSTRAINT release_runs_status_check
    CHECK (status IN ('previewed', 'queued', 'running', 'stopped', 'cancelled', 'done', 'failed', 'expired'));
ALTER TABLE release_runs ADD COLUMN IF NOT EXISTS options jsonb NOT NULL DEFAULT '{}';
ALTER TABLE release_runs ADD COLUMN IF NOT EXISTS total integer NOT NULL DEFAULT 0;
ALTER TABLE release_runs ADD COLUMN IF NOT EXISTS done integer NOT NULL DEFAULT 0;
ALTER TABLE release_runs ADD COLUMN IF NOT EXISTS batches integer NOT NULL DEFAULT 0;
ALTER TABLE release_runs ADD COLUMN IF NOT EXISTS batches_done integer NOT NULL DEFAULT 0;
ALTER TABLE release_runs ADD COLUMN IF NOT EXISTS heartbeat_at timestamptz;
ALTER TABLE release_runs ADD COLUMN IF NOT EXISTS cancel_asked boolean NOT NULL DEFAULT false;
ALTER TABLE release_runs ADD COLUMN IF NOT EXISTS stop_reason text NOT NULL DEFAULT '';
-- rewrite-ok: the table came with migration 0061 of this same release and holds rows only once the experimental part is on
CREATE INDEX IF NOT EXISTS release_runs_going ON release_runs (pair_id) WHERE status IN ('queued', 'running');

-- Each page of a run, in the order it is written (parents first), with its title for the
-- report and why it was skipped.
ALTER TABLE release_run_pages ADD COLUMN IF NOT EXISTS ord integer NOT NULL DEFAULT 0;
ALTER TABLE release_run_pages ADD COLUMN IF NOT EXISTS title text NOT NULL DEFAULT '';
ALTER TABLE release_run_pages ADD COLUMN IF NOT EXISTS reason text NOT NULL DEFAULT '';
ALTER TABLE release_run_pages ADD COLUMN IF NOT EXISTS aspects text[] NOT NULL DEFAULT '{}';
-- The feed shows one item per run, so it asks of a version whether a bulk run wrote it (PUB-810).
-- rewrite-ok: the table came with migration 0061 of this same release and holds rows only once the experimental part is on
CREATE INDEX IF NOT EXISTS release_run_pages_twin ON release_run_pages (twin_page_id, twin_version);

-- page_pair_refresh compares a draft page with the facts of its last release and keeps the
-- aspects that differ (PUB-300 to PUB-306). A change that is undone matches the facts again,
-- so the page is released again (PUB-342). A page without a pair costs one look-up by key.
--
-- Place is the parent, and the order among siblings (PUB-302, PUB-412): among the released
-- siblings that sit together on both sides, a page is out of place when the sibling before it
-- and the sibling after it both differ from those of its twin. A move changes both neighbours
-- of the page that moved and one of each page beside its old and new place, so only the page
-- that moved is marked, while every sibling's position is rewritten (decision D24).
CREATE OR REPLACE FUNCTION page_pair_refresh(draft uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    pp page_pairs%ROWTYPE;
    aspects text[] := '{}';
    p record;
    tw record;
    labels text[];
    dprev uuid;
    dnext uuid;
    tprev uuid;
    tnext uuid;
BEGIN
    SELECT * INTO pp FROM page_pairs WHERE draft_page_id = draft;
    IF NOT FOUND OR pp.released_version = 0 THEN
        RETURN;
    END IF;
    SELECT pg.title, pg.current_version, pg.parent_id, pg.position, pg.space_id, pg.archived_at,
           page_pair_look(pg.emoji, pg.full_width, pg.header_attachment_id, pg.header_position, pg.language) AS look
        INTO p FROM pages pg WHERE pg.id = draft;
    SELECT t.parent_id, t.position, t.space_id, t.archived_at INTO tw FROM pages t WHERE t.id = pp.twin_page_id;
    -- An empty hash marks a twin changed by hand while its pair was ended: the next release
    -- overwrites it, so the page is out of step until then (PUB-245).
    IF (p.current_version <> pp.released_version OR pp.released_hash = '') AND (SELECT page_pair_content_hash(v.document) FROM page_versions v
            WHERE v.page_id = draft AND v.number = p.current_version) IS DISTINCT FROM pp.released_hash THEN
        aspects := aspects || 'text'::text;
    END IF;
    IF p.title IS DISTINCT FROM pp.released_title THEN
        aspects := aspects || 'title'::text;
    END IF;
    IF p.parent_id IS DISTINCT FROM pp.released_parent_id THEN
        aspects := aspects || 'place'::text;
    ELSIF p.parent_id IS NOT NULL AND tw.parent_id IS NOT NULL THEN
        -- Each neighbour is read by the index on the children of a parent, from the page's
        -- own position outwards, so a large group of siblings costs no more than a small one.
        SELECT s.id INTO dprev FROM pages s
            JOIN page_pairs sp ON sp.draft_page_id = s.id AND sp.pair_id = pp.pair_id AND sp.released_version > 0
            JOIN pages st ON st.id = sp.twin_page_id AND st.parent_id = tw.parent_id AND st.status <> 'trashed'
            WHERE s.space_id = p.space_id AND s.parent_id = p.parent_id AND s.status <> 'trashed'
              AND s.position <= p.position AND (s.position < p.position OR s.id < draft)
            ORDER BY s.position DESC, s.id DESC LIMIT 1;
        SELECT s.id INTO dnext FROM pages s
            JOIN page_pairs sp ON sp.draft_page_id = s.id AND sp.pair_id = pp.pair_id AND sp.released_version > 0
            JOIN pages st ON st.id = sp.twin_page_id AND st.parent_id = tw.parent_id AND st.status <> 'trashed'
            WHERE s.space_id = p.space_id AND s.parent_id = p.parent_id AND s.status <> 'trashed'
              AND s.position >= p.position AND (s.position > p.position OR s.id > draft)
            ORDER BY s.position, s.id LIMIT 1;
        SELECT sp.draft_page_id INTO tprev FROM pages t
            JOIN page_pairs sp ON sp.twin_page_id = t.id AND sp.pair_id = pp.pair_id AND sp.released_version > 0
            JOIN pages sd ON sd.id = sp.draft_page_id AND sd.parent_id = p.parent_id AND sd.status <> 'trashed'
            WHERE t.space_id = tw.space_id AND t.parent_id = tw.parent_id AND t.status <> 'trashed'
              AND t.position <= tw.position AND (t.position < tw.position OR t.id < pp.twin_page_id)
            ORDER BY t.position DESC, t.id DESC LIMIT 1;
        SELECT sp.draft_page_id INTO tnext FROM pages t
            JOIN page_pairs sp ON sp.twin_page_id = t.id AND sp.pair_id = pp.pair_id AND sp.released_version > 0
            JOIN pages sd ON sd.id = sp.draft_page_id AND sd.parent_id = p.parent_id AND sd.status <> 'trashed'
            WHERE t.space_id = tw.space_id AND t.parent_id = tw.parent_id AND t.status <> 'trashed'
              AND t.position >= tw.position AND (t.position > tw.position OR t.id > pp.twin_page_id)
            ORDER BY t.position, t.id LIMIT 1;
        IF dprev IS DISTINCT FROM tprev AND dnext IS DISTINCT FROM tnext THEN
            aspects := aspects || 'place'::text;
        END IF;
    END IF;
    IF p.look IS DISTINCT FROM pp.released_look THEN
        aspects := aspects || 'look'::text;
    END IF;
    SELECT COALESCE(array_agg(l.label ORDER BY l.label), '{}') INTO labels FROM page_labels l WHERE l.page_id = draft;
    IF labels IS DISTINCT FROM pp.released_labels THEN
        aspects := aspects || 'labels'::text;
    END IF;
    -- A file the twin holds a copy of has a newer version, or went, in the draft (PUB-305).
    IF EXISTS (SELECT 1 FROM file_pairs fp JOIN attachments a ON a.id = fp.draft_attachment_id
               WHERE fp.twin_page_id = pp.twin_page_id AND (a.current_version <> fp.copied_version OR a.status <> 'active')) THEN
        aspects := aspects || 'files'::text;
    END IF;
    -- A link that became text has a twin to lead to now (PUB-306, PUB-512).
    IF cardinality(pp.text_links) > 0 AND EXISTS (SELECT 1 FROM page_pairs t WHERE t.draft_page_id = ANY (pp.text_links)
               AND t.pair_id = pp.pair_id AND t.released_version > 0) THEN
        aspects := aspects || 'links'::text;
    END IF;
    -- Unarchived in the draft, still archived for readers (PUB-1013).
    IF p.archived_at IS NULL AND tw.archived_at IS NOT NULL THEN
        aspects := aspects || 'archive'::text;
    END IF;
    UPDATE page_pairs SET changed = aspects,
        changed_since = CASE WHEN cardinality(aspects) = 0 THEN NULL ELSE COALESCE(changed_since, now()) END
    WHERE draft_page_id = draft AND changed IS DISTINCT FROM aspects;
END $$;

CREATE OR REPLACE FUNCTION page_pair_touched() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    CASE TG_TABLE_NAME
    WHEN 'page_labels' THEN
        PERFORM page_pair_refresh(CASE WHEN TG_OP = 'DELETE' THEN OLD.page_id ELSE NEW.page_id END);
    WHEN 'attachments' THEN
        -- Every twin that holds a copy of this file: the file may be shown on other pages.
        PERFORM page_pair_refresh(pp.draft_page_id) FROM file_pairs fp
            JOIN page_pairs pp ON pp.twin_page_id = fp.twin_page_id WHERE fp.draft_attachment_id = NEW.id;
    ELSE
        PERFORM page_pair_refresh(NEW.id);
    END CASE;
    RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS pages_page_pair ON pages;
CREATE TRIGGER pages_page_pair AFTER UPDATE ON pages
    FOR EACH ROW WHEN (OLD.title IS DISTINCT FROM NEW.title OR OLD.current_version IS DISTINCT FROM NEW.current_version
        OR OLD.parent_id IS DISTINCT FROM NEW.parent_id OR OLD.position IS DISTINCT FROM NEW.position
        OR OLD.archived_at IS DISTINCT FROM NEW.archived_at OR OLD.emoji IS DISTINCT FROM NEW.emoji
        OR OLD.full_width IS DISTINCT FROM NEW.full_width OR OLD.header_attachment_id IS DISTINCT FROM NEW.header_attachment_id
        OR OLD.header_position IS DISTINCT FROM NEW.header_position OR OLD.language IS DISTINCT FROM NEW.language)
    EXECUTE FUNCTION page_pair_touched();
DROP TRIGGER IF EXISTS attachments_page_pair ON attachments;
CREATE TRIGGER attachments_page_pair AFTER UPDATE ON attachments
    FOR EACH ROW WHEN (OLD.current_version IS DISTINCT FROM NEW.current_version OR OLD.status IS DISTINCT FROM NEW.status)
    EXECUTE FUNCTION page_pair_touched();

-- page_pair_gone keeps a twin whose draft page left the pair for good: purged, or moved to a
-- space that is not the draft space of the pair. The page pair ends; the twin stays for readers
-- until a release takes it off (PUB-1001, PUB-1003, PUB-1012).
CREATE OR REPLACE FUNCTION page_pair_gone() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    pp page_pairs%ROWTYPE;
BEGIN
    SELECT * INTO pp FROM page_pairs WHERE draft_page_id = OLD.id;
    IF NOT FOUND THEN
        RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NULL END;
    END IF;
    IF TG_OP = 'DELETE' THEN
        IF pp.released_version > 0 THEN
            INSERT INTO release_removals (twin_page_id, pair_id, draft_page_id, cause)
                VALUES (pp.twin_page_id, pp.pair_id, OLD.id, 'purged') ON CONFLICT (twin_page_id) DO NOTHING;
        END IF;
        RETURN OLD;
    END IF;
    IF EXISTS (SELECT 1 FROM space_pairs sp WHERE sp.id = pp.pair_id AND sp.draft_space_id = NEW.space_id) THEN
        RETURN NULL;
    END IF;
    IF pp.released_version > 0 THEN
        INSERT INTO release_removals (twin_page_id, pair_id, draft_page_id, cause)
            VALUES (pp.twin_page_id, pp.pair_id, OLD.id, 'moved') ON CONFLICT (twin_page_id) DO NOTHING;
    END IF;
    DELETE FROM page_pairs WHERE draft_page_id = OLD.id;
    RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS pages_page_pair_purged ON pages;
CREATE TRIGGER pages_page_pair_purged BEFORE DELETE ON pages
    FOR EACH ROW EXECUTE FUNCTION page_pair_gone();
DROP TRIGGER IF EXISTS pages_page_pair_moved ON pages;
CREATE TRIGGER pages_page_pair_moved AFTER UPDATE ON pages
    FOR EACH ROW WHEN (OLD.space_id IS DISTINCT FROM NEW.space_id)
    EXECUTE FUNCTION page_pair_gone();

-- space_pair_trashed ends the pair of a space sent to the trash: deleting either space ends it,
-- and the other side becomes an ordinary space (PUB-246, PUB-1046, PUB-1047). Restoring the
-- space does not bring the pair back (PUB-1034). The delete of a space writes no page, so it
-- never meets the trigger of a published space.
CREATE OR REPLACE FUNCTION space_pair_trashed() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    sp space_pairs%ROWTYPE;
BEGIN
    SELECT * INTO sp FROM space_pairs WHERE (draft_space_id = NEW.id OR published_space_id = NEW.id) AND ended_at IS NULL;
    IF NOT FOUND THEN
        RETURN NEW;
    END IF;
    UPDATE space_pairs SET ended_at = now() WHERE id = sp.id;
    UPDATE spaces SET pair_role = NULL
        WHERE id = CASE WHEN sp.draft_space_id = NEW.id THEN sp.published_space_id ELSE sp.draft_space_id END;
    NEW.pair_role := NULL;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS spaces_pair_trashed ON spaces;
CREATE TRIGGER spaces_pair_trashed BEFORE UPDATE ON spaces
    FOR EACH ROW WHEN (NEW.status = 'trashed' AND OLD.status IS DISTINCT FROM 'trashed' AND NEW.pair_role IS NOT NULL)
    EXECUTE FUNCTION space_pair_trashed();
