-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 064, wave DC5: time, delegation, periodic review, read and understood, exempt labels,
-- imported signatures and the move from today's approvals. Every table and column is new and
-- the previous release reads none, so it runs beside this one. Like every migration since
-- 0049 it may run twice, so each statement checks first.

-- The due date of a stage of a round, and what the due job does next (DOC-230 to DOC-232).
-- Times are moments of the database's clock (DOC-1240). The job claims rows by next_at and
-- moves it on in the same transaction as what it did, so a clock that goes back never
-- repeats a reminder and two workers never take one row (DOC-1232, DOC-1241).
CREATE TABLE IF NOT EXISTS review_deadlines (
    round_id      uuid NOT NULL REFERENCES workflow_rounds (id) ON DELETE CASCADE,
    stage         integer NOT NULL,
    page_id       uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    started_at    timestamptz NOT NULL,
    remind_at     timestamptz,
    due_at        timestamptz NOT NULL,
    next_at       timestamptz,
    reminders     integer NOT NULL DEFAULT 0,
    escalated_at  timestamptz,
    passed_at     timestamptz,
    PRIMARY KEY (round_id, stage)
);
CREATE INDEX IF NOT EXISTS review_deadlines_next ON review_deadlines (next_at) WHERE next_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS review_deadlines_page ON review_deadlines (page_id);

-- How a reviewer came to be asked besides the entries of the state: a delegation, a stand-in
-- for someone absent, or an escalation, with the person it came from and why (DOC-240 to
-- DOC-242). The entry that asked them stays in source_type and source_id.
ALTER TABLE workflow_reviewers ADD COLUMN IF NOT EXISTS via text NOT NULL DEFAULT ''
    CHECK (via IN ('', 'delegated', 'stand_in', 'escalated'));
ALTER TABLE workflow_reviewers ADD COLUMN IF NOT EXISTS via_from uuid;
ALTER TABLE workflow_reviewers ADD COLUMN IF NOT EXISTS via_reason text NOT NULL DEFAULT '';

-- A person away, with who stands in (DOC-241). Dates are days of the instance's time zone.
CREATE TABLE IF NOT EXISTS absences (
    user_id    uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    starts_on  date NOT NULL,
    ends_on    date NOT NULL,
    stand_in   uuid REFERENCES users (id) ON DELETE SET NULL,
    set_at     timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_on >= starts_on),
    CHECK (stand_in IS DISTINCT FROM user_id)
);

-- Periodic review (DOC-800 to DOC-805). A page may have a shorter period than its state's.
-- One row per page in a timed approved state: its next review date and what the job does next.
ALTER TABLE page_control ADD COLUMN IF NOT EXISTS review_months smallint CHECK (review_months BETWEEN 1 AND 120);
CREATE TABLE IF NOT EXISTS periodic_reviews (
    page_id         uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    version_number  integer NOT NULL,
    due_at          timestamptz NOT NULL,
    next_at         timestamptz,
    reminders       integer NOT NULL DEFAULT 0,
    started_at      timestamptz
);
CREATE INDEX IF NOT EXISTS periodic_reviews_next ON periodic_reviews (next_at) WHERE next_at IS NOT NULL;

-- Read and understood (DOC-820 to DOC-824). An approved version asks the groups its state
-- named, resolved at approval and kept; people who join later are asked when the job sees
-- them (DOC-1418). A confirmation is for one version, and only after the person opened that
-- version in the session that confirms (DOC-824).
CREATE TABLE IF NOT EXISTS read_requests (
    page_id         uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    version_number  integer NOT NULL,
    groups          uuid[] NOT NULL,
    asked_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, version_number)
);
CREATE TABLE IF NOT EXISTS read_confirmations (
    page_id         uuid NOT NULL,
    version_number  integer NOT NULL,
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    asked_at        timestamptz NOT NULL DEFAULT now(),
    late            boolean NOT NULL DEFAULT false,
    opened_at       timestamptz,
    opened_session  uuid,
    confirmed_at    timestamptz,
    reminded_at     timestamptz,
    PRIMARY KEY (page_id, version_number, user_id),
    FOREIGN KEY (page_id, version_number) REFERENCES read_requests (page_id, version_number) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS read_confirmations_open ON read_confirmations (user_id) WHERE confirmed_at IS NULL;

-- Labels that keep a page out of the space workflow (DOC-402).
ALTER TABLE space_workflows ADD COLUMN IF NOT EXISTS exempt_labels text[] NOT NULL DEFAULT '{}';

-- Signatures that came with a bundle (DOC-1212, DOC-1421): a record of another installation,
-- verified against the content hash of the version here, that never counts as an approval.
CREATE TABLE IF NOT EXISTS imported_signatures (
    id                   uuid PRIMARY KEY,
    page_id              uuid NOT NULL,
    version_number       integer NOT NULL,
    source_instance      uuid NOT NULL,
    source_id            uuid NOT NULL,
    printed_name         text NOT NULL,
    meaning              text NOT NULL,
    meaning_name         text NOT NULL,
    method               text NOT NULL,
    content_hash         bytea NOT NULL,
    content_form         smallint NOT NULL,
    signed_at            timestamptz NOT NULL,
    imported_at          timestamptz NOT NULL DEFAULT now(),
    imported_by          uuid NOT NULL,
    FOREIGN KEY (page_id, version_number) REFERENCES page_versions (page_id, number)
);
CREATE INDEX IF NOT EXISTS imported_signatures_page ON imported_signatures (page_id, version_number);
DROP TRIGGER IF EXISTS imported_signatures_immutable ON imported_signatures;
CREATE TRIGGER imported_signatures_immutable BEFORE UPDATE OR DELETE ON imported_signatures
    FOR EACH ROW EXECUTE FUNCTION signatures_refuse_change();

-- An imported signature is a signed record to keep, like one made here (DOC-770).
CREATE OR REPLACE FUNCTION holds_signed_records(object_type text, object_id uuid) RETURNS boolean
    LANGUAGE sql STABLE AS $$
    SELECT CASE object_type
        WHEN 'page' THEN EXISTS (SELECT 1 FROM page_tree t JOIN signatures s ON s.page_id = t.descendant_id
                                 WHERE t.ancestor_id = object_id)
                      OR EXISTS (SELECT 1 FROM page_tree t JOIN imported_signatures s ON s.page_id = t.descendant_id
                                 WHERE t.ancestor_id = object_id)
        WHEN 'space' THEN EXISTS (SELECT 1 FROM pages p JOIN signatures s ON s.page_id = p.id WHERE p.space_id = object_id)
                       OR EXISTS (SELECT 1 FROM pages p JOIN imported_signatures s ON s.page_id = p.id WHERE p.space_id = object_id)
        WHEN 'attachment' THEN EXISTS (SELECT 1 FROM attachments a, signed_file_versions(a.page_id) f
                                       WHERE a.id = object_id AND f.attachment_id = a.id)
        ELSE false
    END
$$;

-- The move from today's approvals (DOC-1101 to DOC-1107). A space that accepted the generated
-- workflow keeps its old rule on record, switched off while the workflow governs it, and can
-- go back until a person first acts under the workflow.
ALTER TABLE space_workflows ADD COLUMN IF NOT EXISTS from_approval_rule boolean NOT NULL DEFAULT false;
-- The approval of today's rounds a page was carried over with: approved before signatures.
ALTER TABLE page_control ADD COLUMN IF NOT EXISTS carried_round uuid;

-- The working days after a moment, in a time zone and a working week of ISO days (1 is
-- Monday). The due moment keeps the time of day of the start. A week without a working day
-- counts every day, so the loop always ends; it runs at most 60 times 7 steps (DOC-1242).
CREATE OR REPLACE FUNCTION control_working_days_after(start timestamptz, days integer, zone text, week integer[])
    RETURNS timestamptz LANGUAGE plpgsql STABLE AS $$
DECLARE
    local timestamp := start AT TIME ZONE zone;
    day date := (start AT TIME ZONE zone)::date;
    counted integer := 0;
BEGIN
    IF days <= 0 THEN
        RETURN start;
    END IF;
    IF NOT (week && ARRAY[1, 2, 3, 4, 5, 6, 7]) THEN
        RETURN start + make_interval(days => days);
    END IF;
    WHILE counted < days LOOP
        day := day + 1;
        IF extract(isodow FROM day)::integer = ANY (week) THEN
            counted := counted + 1;
        END IF;
    END LOOP;
    RETURN (day + local::time) AT TIME ZONE zone;
END
$$;
