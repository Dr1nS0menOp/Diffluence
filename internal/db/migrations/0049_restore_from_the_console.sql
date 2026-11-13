-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 059. A full restore from the console rewinds every table in public, so what steers
-- it lives in a schema of its own that a restore never touches: the worker leaves ops out
-- of the restore, and backups hold its tables without their rows. Everything here may run
-- again: a restore of a backup from before this migration brings back a schema_migrations
-- without it, and the next start applies it once more on top of the ops that survived.
CREATE SCHEMA IF NOT EXISTS ops;

-- One row per restore. At most one is open: requested, on its way, or failed after it
-- changed something, which keeps every node on the waiting page until the way back ran.
-- No foreign key reaches into public, because a restore drops and makes those tables again.
CREATE TABLE IF NOT EXISTS ops.restores (
    id                uuid PRIMARY KEY,
    backup_name       text NOT NULL CHECK (backup_name ~ '^diffluence-[0-9]{8}T[0-9]{6}Z$'),
    requested_by      uuid,
    requested_by_name text NOT NULL CHECK (length(requested_by_name) <= 200),
    requested_at      timestamptz NOT NULL DEFAULT now(),
    state             text NOT NULL CHECK (state IN
        ('requested', 'preparing', 'stopping', 'backing_up', 'restoring', 'checking', 'done', 'refused', 'failed')),
    step              text NOT NULL DEFAULT '' CHECK (length(step) <= 300),
    safety_name       text CHECK (safety_name ~ '^diffluence-[0-9]{8}T[0-9]{6}Z$'),
    -- changed is set before the first step that alters the installation; from then on a
    -- failure keeps the nodes waiting.
    changed           boolean NOT NULL DEFAULT false,
    updated_at        timestamptz NOT NULL DEFAULT now(),
    finished_at       timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS restores_one_open ON ops.restores ((true)) WHERE finished_at IS NULL;
CREATE INDEX IF NOT EXISTS restores_newest ON ops.restores (requested_at DESC);

-- What each node is doing, refreshed every few seconds: the worker restores only once no
-- node has said it is starting or serving for a while.
CREATE TABLE IF NOT EXISTS ops.nodes (
    node_id uuid PRIMARY KEY,
    mode    text NOT NULL CHECK (mode IN ('starting', 'serving', 'waiting')),
    seen_at timestamptz NOT NULL DEFAULT now()
);

-- How a person last proved who they are in a session: a restore asks for a second factor
-- or a passkey, not only the password (spec 059).
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS reauth_method text;
