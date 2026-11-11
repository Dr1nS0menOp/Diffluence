-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 058. The console controls backups and a worker makes them: the backup service of
-- Compose, a Kubernetes CronJob or a systemd timer. They meet in these tables, so every
-- platform looks the same in the console. The previous release ignores them.

-- backup_requests are the "back up now" of the console. At most one is open at a time.
CREATE TABLE backup_requests (
    id           uuid PRIMARY KEY,
    requested_by uuid REFERENCES users (id) ON DELETE SET NULL,
    requested_at timestamptz NOT NULL DEFAULT now(),
    claimed_at   timestamptz,
    finished_at  timestamptz
);
CREATE UNIQUE INDEX backup_requests_one_open ON backup_requests ((true)) WHERE finished_at IS NULL;

-- backup_runs are what the worker reports: one row per backup it tried. The message names
-- the step that failed and never holds a path outside the backup folder, an address or a key.
CREATE TABLE backup_runs (
    id              uuid PRIMARY KEY,
    name            text NOT NULL,
    reason          text NOT NULL CHECK (reason IN ('schedule', 'request', 'manual')),
    started_at      timestamptz NOT NULL,
    finished_at     timestamptz NOT NULL DEFAULT now(),
    ok              boolean NOT NULL,
    size_bytes      bigint CHECK (size_bytes >= 0),
    message         text NOT NULL DEFAULT '' CHECK (length(message) <= 500),
    mailed_at       timestamptz
);
CREATE INDEX backup_runs_newest ON backup_runs (finished_at DESC);

-- backup_restore_checks are the results of the weekly restore into a throw-away copy.
CREATE TABLE backup_restore_checks (
    id         uuid PRIMARY KEY,
    name       text NOT NULL,
    checked_at timestamptz NOT NULL DEFAULT now(),
    ok         boolean NOT NULL,
    seconds    integer CHECK (seconds >= 0),
    message    text NOT NULL DEFAULT '' CHECK (length(message) <= 500),
    mailed_at  timestamptz
);
CREATE INDEX backup_restore_checks_newest ON backup_restore_checks (checked_at DESC);

-- backup_mail is when a mail about a failure of each kind last went to the administrators,
-- so a backup that fails every few hours mails at most twice a day. mailed_at in the two
-- tables above means the failure was handled: mailed, or folded into a recent mail.
CREATE TABLE backup_mail (
    kind    text PRIMARY KEY CHECK (kind IN ('backup_failed', 'restore_check_failed')),
    sent_at timestamptz NOT NULL
);

-- backup_worker is the one row the worker refreshes each time it asks what to do: when it
-- was last seen, the defaults it was started with, and whether it holds a key. The key
-- itself never leaves the worker; the fingerprint is the start of a SHA-256 of it.
CREATE TABLE backup_worker (
    id               smallint PRIMARY KEY CHECK (id = 1),
    seen_at          timestamptz NOT NULL DEFAULT now(),
    default_interval integer NOT NULL CHECK (default_interval > 0),
    default_keep     integer NOT NULL CHECK (default_keep > 0),
    key_fingerprint  text NOT NULL DEFAULT '' CHECK (key_fingerprint ~ '^[0-9a-f]{0,16}$')
);
