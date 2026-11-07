-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- What search needs to pick rows without touching the text (spec 028). A row of
-- search_documents holds kilobytes of index, and reading that for every match is what made a
-- common word slow. This table is small enough to walk. It is kept by triggers, so a write
-- cannot forget it, and the previous release keeps it correct without knowing it exists.

CREATE TABLE search_pages (
    page_id    uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    space_id   uuid NOT NULL,
    kind       text NOT NULL,
    title      text NOT NULL,
    published  boolean NOT NULL,
    archived   boolean NOT NULL,
    updated_at timestamptz NOT NULL,
    words      tsvector GENERATED ALWAYS AS (setweight(to_tsvector('simple', title), 'A')) STORED
);
CREATE INDEX search_pages_recent_idx ON search_pages (updated_at DESC, page_id DESC);
CREATE INDEX search_pages_space_idx ON search_pages (space_id, updated_at DESC, page_id DESC);
CREATE INDEX search_pages_words_idx ON search_pages USING gin (words);
CREATE INDEX search_pages_title_idx ON search_pages (lower(title));

INSERT INTO search_pages (page_id, space_id, kind, title, published, archived, updated_at)
SELECT id, space_id, kind, title, status = 'published', archived_at IS NOT NULL, updated_at FROM pages;

CREATE FUNCTION search_pages_sync() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO search_pages (page_id, space_id, kind, title, published, archived, updated_at)
    VALUES (NEW.id, NEW.space_id, NEW.kind, NEW.title, NEW.status = 'published', NEW.archived_at IS NOT NULL, NEW.updated_at)
    ON CONFLICT (page_id) DO UPDATE SET space_id = EXCLUDED.space_id, kind = EXCLUDED.kind, title = EXCLUDED.title,
        published = EXCLUDED.published, archived = EXCLUDED.archived, updated_at = EXCLUDED.updated_at;
    RETURN NULL;
END $$;

CREATE TRIGGER search_pages_sync AFTER INSERT OR UPDATE OF space_id, kind, title, status, archived_at, updated_at ON pages
    FOR EACH ROW EXECUTE FUNCTION search_pages_sync();

-- Labels are found like the names of files, in the same column of the index. Both are
-- rewritten together, so neither can wipe the other.
CREATE FUNCTION search_names(page uuid) RETURNS text LANGUAGE sql STABLE AS $$
    SELECT concat_ws(' ',
        (SELECT string_agg(a.filename, ' ' ORDER BY a.filename) FROM attachments a WHERE a.page_id = page AND a.status = 'active'),
        (SELECT string_agg(l.label, ' ' ORDER BY l.label) FROM page_labels l WHERE l.page_id = page))
$$;

CREATE FUNCTION search_labels_sync() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    page uuid := COALESCE(NEW.page_id, OLD.page_id);
BEGIN
    UPDATE search_documents SET attachment_names = search_names(page) WHERE page_id = page;
    RETURN NULL;
END $$;

CREATE TRIGGER search_labels_sync AFTER INSERT OR DELETE ON page_labels
    FOR EACH ROW EXECUTE FUNCTION search_labels_sync();

UPDATE search_documents d SET attachment_names = search_names(d.page_id)
WHERE d.page_id IN (SELECT l.page_id FROM page_labels l);

-- Children of these stand at the top of a tree. There are few, and a tree opens often.
CREATE INDEX pages_odd_parents_idx ON pages (space_id)
    WHERE archived_at IS NOT NULL OR status <> 'published' OR kind NOT IN ('page', 'folder', 'link', 'database');

-- The box runs on every pause in typing, so it may not read all files: by the words of a
-- name for a rare word, and from the newest for a common one.
CREATE INDEX attachments_name_words_idx ON attachments
    USING gin (to_tsvector('simple', translate(filename, '-_.()[]', '       ')));
CREATE INDEX attachments_recent_idx ON attachments (updated_at DESC, id);

-- Finding the people a page mentions starts from the name that was typed.
CREATE INDEX users_name_words_idx ON users USING gin (to_tsvector('simple', display_name));
