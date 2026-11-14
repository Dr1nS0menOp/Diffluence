-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 074, wave GT1: guided templates. A template becomes a list of entries, and each
-- save makes a numbered version whose manifest says what the template makes. Using it
-- makes a whole set of pages in one transaction, or nothing.
--
-- Expand only (TPL-1100). The previous release reads and writes templates as before: it
-- knows none of these tables and columns, and every new column has a default. A template
-- it saves gets no new version; the new release notices that on first read (TPL-1103).
-- It may run twice, as every migration since 0049 may.

-- Who owns a template, whether it was approved, and whether it uses anything of this
-- feature. Every template that exists is approved and owned by its creator (TPL-1101).
ALTER TABLE templates ADD COLUMN IF NOT EXISTS owner_id uuid;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS state text NOT NULL DEFAULT 'approved'
    CHECK (state IN ('draft', 'waiting', 'approved', 'sent_back'));
ALTER TABLE templates ADD COLUMN IF NOT EXISTS approved_version integer;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS guided boolean NOT NULL DEFAULT false;
UPDATE templates SET owner_id = created_by WHERE owner_id IS NULL;

-- A version is never changed while its template exists (TPL-601). The manifest is checked
-- in Go on save; the bound here is the last line of defence (TPL-906).
CREATE TABLE IF NOT EXISTS template_versions (
    template_id  uuid NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
    number       integer NOT NULL CHECK (number > 0),
    manifest     jsonb NOT NULL CHECK (octet_length(manifest::text) <= 65536),
    author_id    uuid NOT NULL,
    state        text NOT NULL DEFAULT 'approved'
        CHECK (state IN ('draft', 'waiting', 'approved', 'sent_back', 'superseded')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (template_id, number)
);

-- The hidden pages behind the entries of a template other than its first, which is
-- templates.page_id. The file route of templates accepts the files of any of them.
CREATE TABLE IF NOT EXISTS template_entry_pages (
    page_id      uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    template_id  uuid NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
    entry_key    text NOT NULL,
    UNIQUE (template_id, entry_key)
);

-- One use of a template. It keeps no answers (TPL-1409). request_key makes a resent
-- request make one set (TPL-123). The root page is set before the transaction commits.
CREATE TABLE IF NOT EXISTS template_sets (
    id            uuid PRIMARY KEY,
    template_id   uuid REFERENCES templates (id) ON DELETE SET NULL,
    version       integer NOT NULL,
    space_id      uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    root_page_id  uuid REFERENCES pages (id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED,
    created_by    uuid NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    page_count    integer NOT NULL CHECK (page_count BETWEEN 0 AND 100),
    request_key   text CHECK (length(request_key) <= 100)
);
CREATE UNIQUE INDEX IF NOT EXISTS template_sets_request ON template_sets (created_by, request_key)
    WHERE request_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS template_sets_template ON template_sets (template_id, version);

-- Where a page came from (TPL-610). A deleted template leaves the record (TPL-612).
CREATE TABLE IF NOT EXISTS page_template_origin (
    page_id      uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    set_id       uuid NOT NULL REFERENCES template_sets (id) ON DELETE CASCADE,
    template_id  uuid REFERENCES templates (id) ON DELETE SET NULL,
    version      integer NOT NULL,
    entry_key    text NOT NULL,
    created_by   uuid NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS page_template_origin_set ON page_template_origin (set_id);

-- A stored copy of a file, written before the transaction that refers to it commits
-- (TPL-821). That transaction removes the row; a row left behind belongs to a copy that
-- rolled back, and a periodic task hands it to storage_deletions.
CREATE TABLE IF NOT EXISTS pending_file_copies (
    object_key  text PRIMARY KEY,
    queued_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pending_file_copies_age ON pending_file_copies (queued_at);

-- Version 1 of every template: its row and the current version of its hidden page
-- (TPL-620, TPL-1101). A template the starters add later gets its version in Go.
INSERT INTO template_versions (template_id, number, manifest, author_id, state)
SELECT t.id, 1, jsonb_build_object(
        'format', 1,
        'entries', jsonb_build_array(jsonb_build_object(
            'key', 'root', 'name', '', 'kind', 'page', 'parent', '',
            'page_id', t.page_id, 'page_version', p.current_version,
            'title_pattern', t.title_pattern, 'labels', to_jsonb(t.labels))),
        'steps', t.steps, 'publish', false, 'default_parent', t.default_parent),
    t.created_by, 'approved'
FROM templates t JOIN pages p ON p.id = t.page_id
ON CONFLICT DO NOTHING;
UPDATE templates t SET approved_version = 1
WHERE t.approved_version IS NULL AND EXISTS (SELECT 1 FROM template_versions v WHERE v.template_id = t.id);
