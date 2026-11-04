-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Initial schema: instance, users, credentials, sessions, global grants, audit log, outbox.

CREATE TABLE instance (
    id          uuid PRIMARY KEY,
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    singleton   boolean NOT NULL DEFAULT true UNIQUE CHECK (singleton)
);

CREATE TABLE users (
    id                  uuid PRIMARY KEY,
    origin_instance_id  uuid NOT NULL,
    type                text NOT NULL CHECK (type IN ('member', 'guest', 'service')),
    status              text NOT NULL CHECK (status IN ('active', 'disabled')),
    email               text NOT NULL,
    display_name        text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TABLE local_credentials (
    user_id        uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    password_hash  text NOT NULL,
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id            uuid PRIMARY KEY,
    token_hash    bytea NOT NULL UNIQUE,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,
    ip            text NOT NULL DEFAULT '',
    user_agent    text NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- Global permissions (layer 1 of the permission model, docs/blueprint/05).
CREATE TABLE global_grants (
    subject_type  text NOT NULL CHECK (subject_type IN ('user', 'group')),
    subject_id    uuid NOT NULL,
    permission    text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subject_type, subject_id, permission)
);

-- Tamper-evident audit log (docs/blueprint/06). "payload" is the exact canonical
-- JSON that was hashed; the other columns are copies for filtering. No foreign
-- keys on purpose: audit rows outlive the objects they describe.
CREATE TABLE audit_events (
    seq          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id           uuid NOT NULL UNIQUE,
    occurred_at  timestamptz NOT NULL,
    actor_type   text NOT NULL,
    actor_id     text NOT NULL DEFAULT '',
    action       text NOT NULL,
    object_type  text NOT NULL DEFAULT '',
    object_id    text NOT NULL DEFAULT '',
    space_id     text NOT NULL DEFAULT '',
    result       text NOT NULL,
    ip           text NOT NULL DEFAULT '',
    request_id   text NOT NULL DEFAULT '',
    payload      text NOT NULL,
    prev_hash    bytea NOT NULL,
    hash         bytea NOT NULL
);
CREATE INDEX audit_events_occurred_at_idx ON audit_events (occurred_at);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_id, seq);
CREATE INDEX audit_events_action_idx ON audit_events (action, seq);

CREATE FUNCTION audit_events_append_only() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END;
$$;
CREATE TRIGGER audit_events_no_update_delete
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION audit_events_append_only();
CREATE TRIGGER audit_events_no_truncate
    BEFORE TRUNCATE ON audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION audit_events_append_only();

-- Transactional outbox (docs/blueprint/04).
CREATE TABLE outbox_events (
    id            uuid PRIMARY KEY,
    type          text NOT NULL,
    payload       jsonb NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    published_at  timestamptz
);
CREATE INDEX outbox_events_unpublished_idx ON outbox_events (created_at) WHERE published_at IS NULL;
