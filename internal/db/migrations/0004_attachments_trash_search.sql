-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Attachments, trash, full-text search, background jobs, settings (spec 003, 005).

CREATE TABLE attachments (
    id                  uuid PRIMARY KEY,
    origin_instance_id  uuid NOT NULL,
    space_id            uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    page_id             uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    filename            text NOT NULL,
    status              text NOT NULL CHECK (status IN ('active', 'trashed')),
    current_version     integer NOT NULL DEFAULT 1,
    trash_item_id       uuid,
    created_by          uuid NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX attachments_page_filename_key ON attachments (page_id, lower(filename)) WHERE status = 'active';
CREATE INDEX attachments_page_idx ON attachments (page_id);

CREATE TABLE attachment_versions (
    id                 uuid PRIMARY KEY,
    attachment_id      uuid NOT NULL REFERENCES attachments (id) ON DELETE CASCADE,
    number             integer NOT NULL,
    object_key         text NOT NULL,
    sha256             bytea NOT NULL,
    size               bigint NOT NULL,
    content_type       text NOT NULL,
    encryption_key_id  text NOT NULL DEFAULT '',
    uploaded_by        uuid NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (attachment_id, number)
);

-- Stored files to remove. Written in the same transaction as the purge, then
-- worked off by the worker, so a crash can never leave a file behind for good.
CREATE TABLE storage_deletions (
    object_key  text PRIMARY KEY,
    queued_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE trash_items (
    id           uuid PRIMARY KEY,
    space_id     uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    object_type  text NOT NULL CHECK (object_type IN ('page', 'attachment', 'space')),
    object_id    uuid NOT NULL,
    title        text NOT NULL,
    deleted_by   uuid NOT NULL,
    deleted_at   timestamptz NOT NULL DEFAULT now(),
    purge_after  timestamptz NOT NULL
);
CREATE INDEX trash_items_space_idx ON trash_items (space_id, deleted_at DESC);
CREATE INDEX trash_items_purge_idx ON trash_items (purge_after);

-- 'simple' on purpose: no stemming, so every language behaves the same.
CREATE TABLE search_documents (
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
CREATE INDEX search_documents_tsv_idx ON search_documents USING gin (tsv);

CREATE TABLE jobs (
    id           uuid PRIMARY KEY,
    kind         text NOT NULL,
    status       text NOT NULL CHECK (status IN ('queued', 'running', 'done', 'failed')),
    payload      jsonb NOT NULL DEFAULT '{}',
    result       jsonb NOT NULL DEFAULT '{}',
    created_by   uuid NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    started_at   timestamptz,
    finished_at  timestamptz
);
CREATE INDEX jobs_queued_idx ON jobs (created_at) WHERE status = 'queued';

-- Periodic work: a lease row makes sure only one worker runs a task at a time.
CREATE TABLE job_leases (
    name         text PRIMARY KEY,
    lease_until  timestamptz NOT NULL DEFAULT 'epoch',
    last_run     timestamptz NOT NULL DEFAULT 'epoch'
);

CREATE TABLE settings (
    key         text PRIMARY KEY,
    value       jsonb NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);
