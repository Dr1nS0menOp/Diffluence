-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Wave K (spec 021).

-- An event a listener gave up on, so that it can be seen and replayed (ARC-050). The
-- payload holds ids only, like every event. The task purge-event-failures keeps it.
CREATE TABLE event_failures (
    id                  uuid PRIMARY KEY,
    consumer            text NOT NULL,
    payload             bytea NOT NULL,
    error               text NOT NULL DEFAULT '',
    failed_at           timestamptz NOT NULL DEFAULT now(),
    replay_requested_at timestamptz,
    replayed_at         timestamptz
);
CREATE INDEX event_failures_waiting_idx ON event_failures (failed_at) WHERE replayed_at IS NULL;

-- Presence is short-lived: a crash may empty it, and standbys need no copy (ARC-162).
ALTER TABLE page_presence SET UNLOGGED;
