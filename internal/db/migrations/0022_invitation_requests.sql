-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Asking to invite someone without an account (spec 022, ADR-033).

-- A request waits for an administrator. The address and the note are only there for
-- that decision, so an answered row no longer holds them.
CREATE TABLE invitation_requests (
    id            uuid PRIMARY KEY,
    email         text NOT NULL,
    page_id       uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    space_id      uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    requester_id  uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    note          text NOT NULL DEFAULT '',
    status        text NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting', 'approved', 'refused')),
    decided_by    uuid,
    reason        text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    decided_at    timestamptz,
    CONSTRAINT invitation_requests_answer CHECK (
        status = 'waiting' OR (reason <> '' AND decided_at IS NOT NULL AND email = '' AND note = ''))
);
CREATE UNIQUE INDEX invitation_requests_waiting_key ON invitation_requests (email, space_id) WHERE status = 'waiting';
