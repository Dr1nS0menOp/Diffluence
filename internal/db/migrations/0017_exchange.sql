-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Everything can leave and come in (spec 014). Exports live 24 hours for the person
-- who made them, imports can be undone, and public sites are copies outside every
-- rule, so what goes in is decided by PRM-096 to PRM-099.

-- A version from a bundle keeps its author and date, which nobody here can vouch
-- for, so it never counts as proof of who wrote what (SEC-353).
ALTER TABLE page_versions
    ADD COLUMN imported  boolean NOT NULL DEFAULT false;

CREATE TABLE export_runs (
    id          uuid PRIMARY KEY,
    job_id      uuid NOT NULL,
    space_id    uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    root_id     uuid,
    format      text NOT NULL,
    options     jsonb NOT NULL DEFAULT '{}',
    state       text NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'done', 'failed', 'expired')),
    object_key  text NOT NULL DEFAULT '',
    filename    text NOT NULL DEFAULT '',
    size        bigint NOT NULL DEFAULT 0,
    report      jsonb NOT NULL DEFAULT '{}',
    created_by  uuid NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL
);
CREATE UNIQUE INDEX export_runs_one_running ON export_runs (created_by) WHERE state = 'running';
CREATE INDEX export_runs_expiry_idx ON export_runs (expires_at) WHERE state = 'done';

-- What a file holds, so the check at download can be made page by page (GAP-008).
-- No foreign key: a page that is gone must still refuse the file.
CREATE TABLE export_run_pages (
    run_id   uuid NOT NULL REFERENCES export_runs (id) ON DELETE CASCADE,
    page_id  uuid NOT NULL,
    PRIMARY KEY (run_id, page_id)
);

CREATE TABLE export_policy (
    format     text PRIMARY KEY,
    enabled    boolean NOT NULL DEFAULT true,
    group_ids  uuid[] NOT NULL DEFAULT '{}'
);

CREATE TABLE space_pdf_design (
    space_id        uuid PRIMARY KEY REFERENCES spaces (id) ON DELETE CASCADE,
    cover_title     text NOT NULL DEFAULT '',
    cover_subtitle  text NOT NULL DEFAULT '',
    logo_id         uuid,
    header          text NOT NULL DEFAULT '',
    footer          text NOT NULL DEFAULT ''
);

CREATE TABLE import_runs (
    id           uuid PRIMARY KEY,
    space_id     uuid REFERENCES spaces (id) ON DELETE CASCADE,
    parent_id    uuid,
    kind         text NOT NULL,
    state        text NOT NULL DEFAULT 'reading'
                 CHECK (state IN ('reading', 'ready', 'running', 'done', 'failed', 'undone')),
    object_key   text NOT NULL DEFAULT '',
    filename     text NOT NULL,
    report       jsonb NOT NULL DEFAULT '{}',
    people_map   jsonb NOT NULL DEFAULT '{}',
    created_by   uuid NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz
);

CREATE TABLE import_items (
    run_id   uuid NOT NULL REFERENCES import_runs (id) ON DELETE CASCADE,
    page_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    source   text NOT NULL,
    version  integer NOT NULL,
    PRIMARY KEY (run_id, page_id)
);
CREATE INDEX import_items_page_idx ON import_items (page_id);

CREATE TABLE public_sites (
    id               uuid PRIMARY KEY,
    space_id         uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    name             text NOT NULL,
    address          text NOT NULL DEFAULT '',
    target           text NOT NULL,
    language         text NOT NULL DEFAULT 'en',
    accent           text NOT NULL DEFAULT 'blue',
    show_names       boolean NOT NULL DEFAULT false,
    search_engines   boolean NOT NULL DEFAULT false,
    update_mode      text NOT NULL DEFAULT 'manual' CHECK (update_mode IN ('publish', 'daily', 'manual')),
    live_build       integer NOT NULL DEFAULT 0,
    build_requested  timestamptz,
    withdrawn_at     timestamptz,
    created_by       uuid NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX public_sites_target_key ON public_sites (target);
CREATE INDEX public_sites_space_idx ON public_sites (space_id);

CREATE TABLE public_site_pages (
    site_id  uuid NOT NULL REFERENCES public_sites (id) ON DELETE CASCADE,
    page_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    PRIMARY KEY (site_id, page_id)
);
CREATE INDEX public_site_pages_page_idx ON public_site_pages (page_id);

CREATE TABLE public_site_builds (
    site_id      uuid NOT NULL REFERENCES public_sites (id) ON DELETE CASCADE,
    number       integer NOT NULL,
    state        text NOT NULL CHECK (state IN ('building', 'live', 'old', 'failed')),
    report       jsonb NOT NULL DEFAULT '{}',
    pages        uuid[] NOT NULL DEFAULT '{}',
    started_at   timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz,
    PRIMARY KEY (site_id, number)
);

CREATE TABLE public_links (
    id            uuid PRIMARY KEY,
    page_id       uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    version       integer NOT NULL,
    token         text NOT NULL UNIQUE,
    created_by    uuid NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    withdrawn_at  timestamptz
);
CREATE UNIQUE INDEX public_links_live_key ON public_links (page_id) WHERE withdrawn_at IS NULL;
