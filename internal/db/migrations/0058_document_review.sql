-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 064, wave DC2: reviewers, rules to pass, review comments and instance templates.
-- A state of kind in review names people, groups and roles; when a page enters it, they
-- are resolved and stored on the round, so a later change of a group rewrites nothing
-- (DOC-200, DOC-201). Every column and table is new; the previous release reads none.
-- Like every migration since 0049 it may run twice, so each statement checks first. The few
-- workflows of an instance are found by their template without an index.

-- Each entry into a review state is a stage of the round: it asks its reviewers afresh.
ALTER TABLE workflow_rounds ADD COLUMN IF NOT EXISTS review_stage integer NOT NULL DEFAULT 0;

-- The people a stage asked, how they came to be asked, and their answer. Rows are never
-- deleted while the round exists: a removal keeps the row with its reason (DOC-202), so a
-- "changes" answer stays on record after it.
CREATE TABLE IF NOT EXISTS workflow_reviewers (
    round_id        uuid NOT NULL REFERENCES workflow_rounds (id) ON DELETE CASCADE,
    stage           integer NOT NULL,
    state_id        text NOT NULL,
    user_id         uuid NOT NULL,
    source_type     text NOT NULL CHECK (source_type IN ('user', 'group', 'role', 'added')),
    source_id       uuid,
    added_by        uuid,
    asked_at        timestamptz NOT NULL DEFAULT now(),
    answer          text CHECK (answer IN ('approve', 'changes')),
    comment         text NOT NULL DEFAULT '',
    answered_at     timestamptz,
    removed_at      timestamptz,
    removed_by      uuid,
    removed_reason  text NOT NULL DEFAULT '',
    PRIMARY KEY (round_id, stage, user_id)
);
-- What a person is asked to review, and the reading it gives them (DOC-203).
CREATE INDEX IF NOT EXISTS workflow_reviewers_user ON workflow_reviewers (user_id) WHERE removed_at IS NULL;

-- Review comments belong to the round, for its reviewers and authors only (DOC-251, DOC-501).
CREATE TABLE IF NOT EXISTS workflow_review_comments (
    id          uuid PRIMARY KEY,
    round_id    uuid NOT NULL REFERENCES workflow_rounds (id) ON DELETE CASCADE,
    user_id     uuid NOT NULL,
    body        text NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS workflow_review_comments_round ON workflow_review_comments (round_id, created_at);

-- Instance templates (DOC-130). A space copies one; the copy records where it came from and
-- never changes with the template (DOC-1405).
CREATE TABLE IF NOT EXISTS workflow_templates (
    id           uuid PRIMARY KEY,
    name         text NOT NULL,
    description  text NOT NULL DEFAULT '',
    version      integer NOT NULL DEFAULT 1,
    definition   jsonb NOT NULL,
    saved_by     uuid NOT NULL,
    saved_at     timestamptz NOT NULL DEFAULT now(),
    archived_at  timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS workflow_templates_active_name ON workflow_templates (lower(name)) WHERE archived_at IS NULL;
ALTER TABLE workflows ADD COLUMN IF NOT EXISTS template_id uuid REFERENCES workflow_templates (id);
ALTER TABLE workflows ADD COLUMN IF NOT EXISTS template_version integer;

-- Pages a person reviews now: an answer still to give or to change in the open round, in
-- the stage the page is in, as a member who views the space. Being asked opens that page
-- and nothing around it, past its restrictions and past "never approved", for the round
-- only (DOC-203, DOC-1224). permissions.Evaluate decides approval.read by the same rule.
CREATE OR REPLACE FUNCTION perm_review_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT r.page_id
    FROM workflow_reviewers wr
    JOIN workflow_rounds r ON r.id = wr.round_id AND r.status = 'open' AND r.review_stage = wr.stage
    JOIN page_control pc ON pc.page_id = r.page_id AND pc.state_id = wr.state_id
    JOIN pages p ON p.id = r.page_id AND p.status <> 'trashed' AND p.kind <> 'template'
    JOIN users x ON x.id = wr.user_id AND x.type = 'member'
    WHERE wr.user_id = u AND wr.removed_at IS NULL
      AND p.space_id IN (SELECT ps.space_id FROM perm_spaces(u, 'space.view') ps)
$$;
