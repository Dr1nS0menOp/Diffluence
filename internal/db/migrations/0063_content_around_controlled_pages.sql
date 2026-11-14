-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 064, wave DC3: the content around a controlled page. Files, search, notifications,
-- templates and bundles follow the one rule of DOC-300: readers get the approved version,
-- and whoever may see the working version gets that. Every table and column is new and the
-- previous release reads none of them. Like every migration since 0049 it may run twice, so
-- each statement checks first.

-- The files a version of a controlled page shows, at the version and under the name they had
-- when it was written (DOC-510). Readers of the page get the pins of the version on the pages
-- row; a page without a pin row for that version pins nothing, and readers get its files as
-- they are. The three arrays run in step.
CREATE TABLE IF NOT EXISTS attachment_pins (
    page_id         uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    page_version    integer NOT NULL,
    attachment_ids  uuid[] NOT NULL,
    versions        integer[] NOT NULL,
    filenames       text[] NOT NULL,
    pinned_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, page_version)
);

-- When the version readers see of a page pinned its files, or NULL when it pins nothing.
CREATE OR REPLACE FUNCTION reader_pinned_at(page uuid) RETURNS timestamptz
    LANGUAGE sql STABLE AS $$
    SELECT pin.pinned_at FROM pages p JOIN attachment_pins pin ON pin.page_id = p.id AND pin.page_version = p.current_version
    WHERE p.id = page
$$;

-- The files readers of a page get, at the pinned version and name. Only meaningful when
-- reader_pinned_at is set.
CREATE OR REPLACE FUNCTION reader_pins(page uuid) RETURNS TABLE (attachment_id uuid, version integer, filename text)
    LANGUAGE sql STABLE AS $$
    SELECT f.attachment_id, f.version, f.filename
    FROM pages p
    JOIN attachment_pins pin ON pin.page_id = p.id AND pin.page_version = p.current_version
    CROSS JOIN LATERAL unnest(pin.attachment_ids, pin.versions, pin.filenames) AS f (attachment_id, version, filename)
    WHERE p.id = page
$$;

-- Files a person may not find in the lists of files (the Files screen, its filters and quick
-- search): on a page whose readers' version pins its files, a file that is not pinned as it
-- is now, for a person who may not edit that page (DOC-512, DOC-520). The changed files are
-- found first, and only their pages are asked about editing, so an instance with few
-- changes waiting pays little.
CREATE OR REPLACE FUNCTION perm_unpinned_files(u uuid) RETURNS TABLE (attachment_id uuid)
    LANGUAGE sql STABLE AS $$
    WITH changed AS MATERIALIZED (
        SELECT a.id, a.page_id
        FROM attachment_pins pin
        JOIN pages p ON p.id = pin.page_id AND p.current_version = pin.page_version
        JOIN attachments a ON a.page_id = pin.page_id AND a.status = 'active'
        WHERE NOT EXISTS (SELECT 1 FROM unnest(pin.attachment_ids, pin.versions, pin.filenames) AS f (id, version, filename)
                          WHERE f.id = a.id AND f.version = a.current_version AND f.filename = a.filename))
    SELECT c.id FROM changed c
    WHERE c.page_id NOT IN (SELECT e.page_id FROM perm_editable_pages(u) e
                            WHERE e.page_id = ANY (ARRAY(SELECT DISTINCT x.page_id FROM changed x)))
$$;

-- The names of the files and the labels of a page as its authors see them: the files as they
-- are now. This is what search_names was before this wave.
CREATE OR REPLACE FUNCTION search_working_names(page uuid) RETURNS text LANGUAGE sql STABLE AS $$
    SELECT concat_ws(' ',
        (SELECT string_agg(a.filename, ' ' ORDER BY a.filename) FROM attachments a WHERE a.page_id = page AND a.status = 'active'),
        (SELECT string_agg(l.label, ' ' ORDER BY l.label) FROM page_labels l WHERE l.page_id = page))
$$;

-- The names readers find a page by: on a page whose readers' version pins its files, the
-- pinned names, so a file added or renamed in a working version is never found by a reader
-- (DOC-520).
CREATE OR REPLACE FUNCTION search_names(page uuid) RETURNS text LANGUAGE sql STABLE AS $$
    SELECT CASE WHEN reader_pinned_at(page) IS NOT NULL THEN concat_ws(' ',
            (SELECT string_agg(r.filename, ' ' ORDER BY r.filename) FROM reader_pins(page) r),
            (SELECT string_agg(l.label, ' ' ORDER BY l.label) FROM page_labels l WHERE l.page_id = page))
        ELSE search_working_names(page) END
$$;

-- The second entry of search (DOC-520): the working version of a controlled page, for those
-- who may see it. A row exists only while a working version differs from the approved one
-- (DOC-1302), so the index grows only by the drafts in progress.
CREATE TABLE IF NOT EXISTS search_working (
    page_id           uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    title             text NOT NULL,
    body              text NOT NULL,
    attachment_names  text NOT NULL DEFAULT '',
    tsv               tsvector GENERATED ALWAYS AS (
                          setweight(to_tsvector('simple', title), 'A')
                       || setweight(to_tsvector('simple', attachment_names), 'B')
                       || setweight(to_tsvector('simple', body), 'C')) STORED,
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS search_working_tsv ON search_working USING gin (tsv);

-- A notification about a working version is shown and mailed only while its person may still
-- edit the page (DOC-603, DOC-604). Rows from before this wave keep the ordinary rule.
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS needs_edit boolean NOT NULL DEFAULT false;

-- People who asked to hear of every approval in a space, the approvals digest (DOC-607).
CREATE TABLE IF NOT EXISTS approval_subscriptions (
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    space_id    uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, space_id)
);
CREATE INDEX IF NOT EXISTS approval_subscriptions_space ON approval_subscriptions (space_id);

-- A template of a space may name a workflow of that space, which a page made from it enters
-- (DOC-560).
ALTER TABLE templates ADD COLUMN IF NOT EXISTS workflow_id uuid REFERENCES workflows (id) ON DELETE SET NULL;

-- Rounds that came with a bundle keep their record, marked with the installation they came
-- from (DOC-532). They never count here.
ALTER TABLE workflow_rounds ADD COLUMN IF NOT EXISTS imported_from uuid;

-- Pins for the pages under control before this wave: the version readers see and the head,
-- each with the files it shows at their versions now. Nothing was pinned before, so readers
-- keep what they had.
INSERT INTO attachment_pins (page_id, page_version, attachment_ids, versions, filenames)
SELECT v.page_id, v.number,
       COALESCE(array_agg(a.id ORDER BY a.id) FILTER (WHERE a.id IS NOT NULL), '{}'),
       COALESCE(array_agg(a.current_version ORDER BY a.id) FILTER (WHERE a.id IS NOT NULL), '{}'),
       COALESCE(array_agg(a.filename ORDER BY a.id) FILTER (WHERE a.id IS NOT NULL), '{}')
FROM (SELECT pc.page_id, p.current_version AS number FROM page_control pc JOIN pages p ON p.id = pc.page_id WHERE p.current_version > 0
      UNION
      SELECT pc.page_id, pc.head_version FROM page_control pc WHERE pc.head_version > 0) v
LEFT JOIN attachment_refs r ON r.page_id = v.page_id AND r.version = v.number
LEFT JOIN attachments a ON a.id = r.attachment_id AND a.page_id = v.page_id AND a.status = 'active'
GROUP BY v.page_id, v.number
ON CONFLICT (page_id, page_version) DO NOTHING;
