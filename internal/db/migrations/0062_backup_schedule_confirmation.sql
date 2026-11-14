-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 076. Keeping fewer backups, or backing up less often, takes effect only once the
-- backup service compared the code it wrote to its own log with the one typed into the
-- console. A system administrator asks here; the backup service keeps what it confirmed in
-- its own folder, which the server never reaches. The previous release ignores both. It may
-- run twice, as every migration since 0049 may.

-- backup_schedule_requests are the asks of the console. At most one is open at a time. The
-- interval and keep are as the backup section stored them: zero is the worker's default.
CREATE TABLE IF NOT EXISTS backup_schedule_requests (
    id             uuid PRIMARY KEY,
    asked_by       uuid REFERENCES users (id) ON DELETE SET NULL,
    asked_at       timestamptz NOT NULL DEFAULT now(),
    interval_hours integer NOT NULL CHECK (interval_hours >= 0),
    keep           integer NOT NULL CHECK (keep >= 0),
    code_shown_at  timestamptz,
    confirmation   text CHECK (length(confirmation) <= 40),
    wrong_codes    integer NOT NULL DEFAULT 0 CHECK (wrong_codes >= 0),
    outcome        text CHECK (outcome IN ('confirmed', 'refused', 'replaced', 'not_needed')),
    finished_at    timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS backup_schedule_requests_one_open ON backup_schedule_requests ((true)) WHERE finished_at IS NULL;

-- What the backup service holds to, as it reports each round, so the console can show it.
ALTER TABLE backup_worker
    ADD COLUMN IF NOT EXISTS held_keep integer CHECK (held_keep > 0),
    ADD COLUMN IF NOT EXISTS held_interval integer CHECK (held_interval > 0);
