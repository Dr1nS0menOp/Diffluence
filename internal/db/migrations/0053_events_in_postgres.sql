-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 068, wave EV1: the event path in PostgreSQL, built beside NATS. A sequencer moves
-- committed rows from outbox_events into event_log and gives each a gapless position in the
-- order they became visible (D1); every consumer reads event_log through one cursor per lane,
-- held by a lease (D3, D5); a failed event waits in event_retries, then goes to
-- event_failures as before.
--
-- event_transport says which path the relay takes. It starts at 'nats', so this release
-- relays as the previous one did and the new tables stay empty; the switch is wave EV2. The
-- previous release reads none of these tables.

-- One row: the last position given. A sequence is not used, because it is not
-- transactional and positions must have no gaps (D1).
CREATE TABLE event_head (
    one  boolean PRIMARY KEY DEFAULT true CHECK (one),
    pos  bigint NOT NULL
);
INSERT INTO event_head (pos) VALUES (0);

CREATE TABLE event_transport (
    one        boolean PRIMARY KEY DEFAULT true CHECK (one),
    transport  text NOT NULL CHECK (transport IN ('nats', 'postgres')),
    changed_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO event_transport (transport) VALUES ('nats');

CREATE TABLE event_log (
    pos           bigint PRIMARY KEY,
    lane          smallint NOT NULL CHECK (lane >= 0 AND lane < 4),
    id            uuid NOT NULL UNIQUE,
    type          text NOT NULL,
    payload       jsonb NOT NULL,
    created_at    timestamptz NOT NULL,
    sequenced_at  timestamptz NOT NULL DEFAULT now(),
    trace         text
);
CREATE INDEX event_log_lane_pos ON event_log (lane, pos);

-- Updated on every batch without touching an indexed column; the room left on each page
-- keeps those updates HOT (D10).
CREATE TABLE event_cursors (
    consumer     text NOT NULL,
    lane         smallint NOT NULL CHECK (lane >= 0 AND lane < 4),
    pos          bigint NOT NULL,
    lease_owner  uuid,
    lease_until  timestamptz,
    PRIMARY KEY (consumer, lane)
) WITH (fillfactor = 50);
ALTER TABLE event_head SET (fillfactor = 50);

CREATE TABLE event_retries (
    consumer    text NOT NULL,
    pos         bigint NOT NULL,
    event_id    uuid NOT NULL,
    tries       int NOT NULL CHECK (tries > 0),
    next_at     timestamptz NOT NULL,
    last_error  text NOT NULL,
    PRIMARY KEY (consumer, pos)
);
CREATE INDEX event_retries_due ON event_retries (next_at);

-- Once events run through PostgreSQL every outbox row is deleted within a second of its
-- insert, so the waiting room is vacuumed by a count of dead rows, not by its share of a
-- table that stays small (D10).
ALTER TABLE outbox_events SET (autovacuum_vacuum_scale_factor = 0, autovacuum_vacuum_threshold = 5000);
