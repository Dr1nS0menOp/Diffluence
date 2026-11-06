-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Insight, automation and the platform (spec 015). A page view is personal data,
-- so the tables below say per column what they keep, and nothing else does.

-- Counts per page and day: nothing here points at a person.
CREATE TABLE page_view_days (
    page_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    day      date NOT NULL,
    views    integer NOT NULL DEFAULT 0,
    engaged  integer NOT NULL DEFAULT 0,
    readers  integer NOT NULL DEFAULT 0,
    PRIMARY KEY (page_id, day)
);
CREATE INDEX page_view_days_day_idx ON page_view_days (day);

-- Who was here today, as a hash with a salt of this day. Both go when the day closes.
CREATE TABLE analytics_salts (
    day   date PRIMARY KEY,
    salt  bytea NOT NULL
);

CREATE TABLE page_view_seen (
    day      date NOT NULL,
    reader   bytea NOT NULL,
    page_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    last_at  timestamptz NOT NULL,
    engaged  boolean NOT NULL DEFAULT false,
    PRIMARY KEY (day, reader, page_id)
);

-- Name mode only. Leaving name mode deletes every row, in the same transaction.
CREATE TABLE page_view_people (
    page_id       uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    first_at      timestamptz NOT NULL,
    last_at       timestamptz NOT NULL,
    views         integer NOT NULL DEFAULT 1,
    last_version  integer NOT NULL DEFAULT 0,
    PRIMARY KEY (page_id, user_id)
);

CREATE TABLE page_view_links (
    day      date NOT NULL,
    from_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    to_id    uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    count    integer NOT NULL DEFAULT 1,
    PRIMARY KEY (day, from_id, to_id)
);

CREATE TABLE analytics_days (
    day           date PRIMARY KEY,
    active_users  integer NOT NULL DEFAULT 0,
    views         integer NOT NULL DEFAULT 0,
    new_pages     integer NOT NULL DEFAULT 0,
    comments      integer NOT NULL DEFAULT 0,
    searches      integer NOT NULL DEFAULT 0
);

-- Search terms have no people. A term can hold a name, so rare ones never show.
CREATE TABLE search_terms (
    day       date NOT NULL,
    term      text NOT NULL,
    searches  integer NOT NULL DEFAULT 1,
    empty     integer NOT NULL DEFAULT 0,
    PRIMARY KEY (day, term)
);

CREATE TABLE automation_rules (
    id           uuid PRIMARY KEY,
    space_id     uuid REFERENCES spaces (id) ON DELETE CASCADE,
    name         text NOT NULL,
    enabled      boolean NOT NULL DEFAULT false,
    definition   jsonb NOT NULL,
    trigger      text NOT NULL,
    owner_id     uuid NOT NULL REFERENCES users (id),
    account_id   uuid NOT NULL REFERENCES users (id),
    next_run_at  timestamptz,
    paused_at    timestamptz,
    told_at      timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX automation_rules_trigger_idx ON automation_rules (trigger) WHERE enabled;
CREATE INDEX automation_rules_space_idx ON automation_rules (space_id);

CREATE TABLE automation_runs (
    id          uuid PRIMARY KEY,
    rule_id     uuid NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    event_id    uuid NOT NULL,
    state       text NOT NULL CHECK (state IN ('queued', 'running', 'done', 'failed', 'skipped')),
    context     jsonb NOT NULL DEFAULT '{}',
    steps       jsonb NOT NULL DEFAULT '[]',
    report      text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    UNIQUE (rule_id, event_id)
);
CREATE INDEX automation_runs_rule_idx ON automation_runs (rule_id, created_at DESC);

CREATE TABLE automation_hooks (
    rule_id     uuid PRIMARY KEY REFERENCES automation_rules (id) ON DELETE CASCADE,
    token_hash  bytea NOT NULL UNIQUE
);

CREATE TABLE outgoing_hosts (
    host      text PRIMARY KEY,
    added_by  uuid NOT NULL,
    added_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE webhook_endpoints (
    id             uuid PRIMARY KEY,
    space_id       uuid REFERENCES spaces (id) ON DELETE CASCADE,
    address        text NOT NULL,
    events         text[] NOT NULL,
    with_titles    boolean NOT NULL DEFAULT false,
    secret         bytea NOT NULL,
    old_secret     bytea,
    old_until      timestamptz,
    enabled        boolean NOT NULL DEFAULT true,
    failures       integer NOT NULL DEFAULT 0,
    owner_id       uuid NOT NULL REFERENCES users (id),
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX webhook_endpoints_space_idx ON webhook_endpoints (space_id);

CREATE TABLE webhook_deliveries (
    id            uuid PRIMARY KEY,
    endpoint_id   uuid NOT NULL REFERENCES webhook_endpoints (id) ON DELETE CASCADE,
    event_id      uuid NOT NULL,
    event_type    text NOT NULL,
    event_time    timestamptz NOT NULL,
    page_id       uuid,
    space_id      uuid,
    state         text NOT NULL CHECK (state IN ('waiting', 'delivered', 'failed', 'dropped')),
    attempts      integer NOT NULL DEFAULT 0,
    next_attempt  timestamptz NOT NULL DEFAULT now(),
    last_status   integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (endpoint_id, event_id)
);
CREATE INDEX webhook_deliveries_due_idx ON webhook_deliveries (next_attempt) WHERE state = 'waiting';
CREATE INDEX webhook_deliveries_endpoint_idx ON webhook_deliveries (endpoint_id, created_at DESC);

CREATE TABLE page_presence (
    page_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    user_id  uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    mode     text NOT NULL CHECK (mode IN ('reading', 'editing')),
    seen_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, user_id)
);
CREATE INDEX page_presence_seen_idx ON page_presence (seen_at);

-- A version names everyone who took part when people edit together (COL-092).
CREATE TABLE page_version_authors (
    page_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    number   integer NOT NULL,
    user_id  uuid NOT NULL,
    PRIMARY KEY (page_id, number, user_id)
);

-- The shared document of editing together (05.5, section 5.9). It goes with the page.
CREATE TABLE collab_documents (
    page_id     uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    state       bytea NOT NULL,
    title       text NOT NULL DEFAULT '',
    authors     uuid[] NOT NULL DEFAULT '{}',
    updated_at  timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE api_tokens ADD COLUMN scopes text[] NOT NULL DEFAULT '{read}';
UPDATE api_tokens SET scopes = '{read,write,export}' WHERE can_write;

-- Whoever administers a space may see its analytics. The previous release ignores
-- a permission it does not know.
INSERT INTO space_grants (space_id, subject_type, subject_id, permission)
SELECT space_id, subject_type, subject_id, 'analytics.view'
FROM space_grants WHERE permission = 'space.administer'
ON CONFLICT DO NOTHING;
