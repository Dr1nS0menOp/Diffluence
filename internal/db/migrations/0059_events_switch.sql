-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 068, wave EV2: the switch from NATS to PostgreSQL and the drain of NATS.
--
-- outbox_events.trace carries the W3C traceparent of the request that saved the event, so a
-- consumer of the event log works inside that trace (spec 068, 4.1). It is empty while
-- tracing is off. Releases before this one leave it empty.
--
-- event_transport.nats_drained_at is when a worker saw that the NATS consumers had nothing
-- left after the switch. The release that removes NATS refuses to upgrade until it is set
-- (spec 068, 2.11). Switching back to nats clears it.
--
-- It may run twice, as every migration since 0049 may.
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS trace text;
ALTER TABLE event_transport ADD COLUMN IF NOT EXISTS nats_drained_at timestamptz;
