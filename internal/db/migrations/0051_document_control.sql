-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 064, wave DC1: document control. A workflow of states and transitions belongs to a
-- space; a space workflow, or a workflow on a page or branch, puts pages under control; one
-- row per controlled page, page_control, says what readers may see (DOC-1300). Readers see
-- the approved version only, and a page that was never approved, or was withdrawn, is
-- hidden from everyone who may not edit it (DOC-300, DOC-301, DOC-322).
--
-- Two permissions are split off as in specs 036, 040, 045 and 049: space.workflows off
-- space.administer, and page.workflow off page.edit. Whoever held the broader permission
-- holds the finer one too, so nobody gains or loses anything. The previous release reads
-- neither name.

CREATE TABLE workflows (
    id               uuid PRIMARY KEY,
    space_id         uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    name             text NOT NULL,
    description      text NOT NULL DEFAULT '',
    current_version  integer NOT NULL DEFAULT 1,
    archived_at      timestamptz,
    created_by       uuid NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);
-- People pick a workflow from a list by its name, so active names are unique per space.
CREATE UNIQUE INDEX workflows_active_name ON workflows (space_id, lower(name)) WHERE archived_at IS NULL;

-- Each saved change is a new version; a running round keeps the version it started under
-- (DOC-110). The definition is checked in Go (internal/workflow).
CREATE TABLE workflow_versions (
    workflow_id  uuid NOT NULL REFERENCES workflows (id) ON DELETE CASCADE,
    version      integer NOT NULL,
    definition   jsonb NOT NULL,
    saved_by     uuid NOT NULL,
    saved_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workflow_id, version)
);

-- At most one space workflow, with the kinds of item it governs, which overrides page
-- workflows (DOC-400, DOC-420).
CREATE TABLE space_workflows (
    space_id     uuid PRIMARY KEY REFERENCES spaces (id) ON DELETE CASCADE,
    workflow_id  uuid NOT NULL REFERENCES workflows (id),
    kinds        text[] NOT NULL,
    set_by       uuid NOT NULL,
    set_at       timestamptz NOT NULL DEFAULT now()
);

-- A workflow set on one page, or on a page and everything below it (DOC-410).
CREATE TABLE page_workflows (
    page_id      uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    workflow_id  uuid NOT NULL REFERENCES workflows (id),
    branch       boolean NOT NULL DEFAULT false,
    set_by       uuid NOT NULL,
    set_at       timestamptz NOT NULL DEFAULT now()
);

-- The one row the read path adds (DOC-1300). Once a page is approved, the pages row keeps
-- the approved version as its current version, so every reader's list, search, link and
-- export shows approved content; head_version is the newest version, which only authors
-- see. Before the first approval the page is hidden from readers, and its current version
-- is its newest.
CREATE TABLE page_control (
    page_id            uuid PRIMARY KEY REFERENCES pages (id) ON DELETE CASCADE,
    workflow_id        uuid NOT NULL REFERENCES workflows (id),
    workflow_version   integer NOT NULL,
    state_id           text NOT NULL,
    state_kind         text NOT NULL CHECK (state_kind IN ('draft', 'in_review', 'approved', 'rejected', 'obsolete')),
    approved_version   integer,
    -- The approved state the page was approved into: an unchanged version returns there.
    approved_state_id  text,
    approved_at        timestamptz,
    approved_by        uuid,
    head_version       integer NOT NULL DEFAULT 0,
    governed_by        text NOT NULL CHECK (governed_by IN ('space', 'page', 'branch')),
    -- The page whose page workflow governs, for a page or branch workflow (DOC-422).
    governed_from      uuid,
    updated_at         timestamptz NOT NULL DEFAULT now()
);
-- What readers may not see: never approved, or withdrawn. The visibility rule reads only
-- these rows, so an instance without such pages pays almost nothing.
CREATE INDEX page_control_hidden ON page_control (page_id) WHERE approved_version IS NULL OR state_kind = 'obsolete';
-- Saving a workflow refuses to remove a state a page is in (DOC-110).
CREATE INDEX page_control_workflow ON page_control (workflow_id, state_id);
-- Space over page (DOC-420) and branch inheritance find their pages by where control came from.
CREATE INDEX page_control_governed_from ON page_control (governed_from) WHERE governed_from IS NOT NULL;
CREATE INDEX page_workflows_workflow ON page_workflows (workflow_id);
CREATE INDEX space_workflows_workflow ON space_workflows (workflow_id);

-- One round per working version: it ends approved, superseded by a later change, or ended
-- when control is removed or a space workflow takes over (DOC-330, DOC-420).
CREATE TABLE workflow_rounds (
    id                uuid PRIMARY KEY,
    page_id           uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    workflow_id       uuid NOT NULL REFERENCES workflows (id),
    workflow_version  integer NOT NULL,
    version_number    integer NOT NULL,
    status            text NOT NULL CHECK (status IN ('open', 'approved', 'withdrawn', 'superseded', 'ended')),
    ended_reason      text NOT NULL DEFAULT '',
    started_at        timestamptz NOT NULL DEFAULT now(),
    ended_at          timestamptz
);
CREATE UNIQUE INDEX workflow_rounds_open ON workflow_rounds (page_id) WHERE status = 'open';
CREATE INDEX workflow_rounds_page ON workflow_rounds (page_id, started_at);
-- Every transition taken, by a person or by the system with its cause (DOC-104, DOC-105).
CREATE TABLE workflow_answers (
    id             uuid PRIMARY KEY,
    round_id       uuid REFERENCES workflow_rounds (id) ON DELETE CASCADE,
    page_id        uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    user_id        uuid,
    transition_id  text NOT NULL,
    from_state     text NOT NULL,
    to_state       text NOT NULL,
    version_number integer NOT NULL,
    comment        text NOT NULL DEFAULT '',
    cause          text NOT NULL DEFAULT '',
    answered_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX workflow_answers_page ON workflow_answers (page_id, answered_at);

-- Controlled pages a person may not see: never approved or withdrawn (DOC-301, DOC-322),
-- and the person is not an author, that is, lacks the edit permission of the item's kind or
-- is kept out by an edit restriction (DOC-300). Archiving does not count here: an author of
-- an archived page still sees it. It reads only the hidden rows of page_control, through
-- their partial index, so an instance without such pages pays one empty index scan. The
-- pages are then looked up by key, as an array: as a join, PostgreSQL read every page to
-- hash them (21 ms on the real data of spec 061).
-- permissions.Evaluate is the same rule, and the matrix test holds the two equal.
CREATE FUNCTION perm_control_hidden_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT p.id
    FROM pages p
    WHERE p.id = ANY (ARRAY(SELECT pc.page_id FROM page_control pc
                            WHERE pc.approved_version IS NULL OR pc.state_kind = 'obsolete'))
      AND NOT (
          CASE WHEN p.kind = 'database'
            THEN p.space_id IN (SELECT e.space_id FROM perm_spaces(u, 'database.edit') e)
            ELSE p.space_id IN (SELECT e.space_id FROM perm_spaces(u, 'page.edit') e) END
          AND p.id NOT IN (
            SELECT r.page_id
            FROM page_restrictions r
            WHERE r.kind = 'edit'
            GROUP BY r.page_id
            HAVING NOT bool_or(
                   (r.subject_type = 'user' AND r.subject_id = u)
                OR (r.subject_type = 'group' AND r.subject_id IN
                       (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = u)))))
$$;

-- Editing is decided over perm_visible_pages, and an author is never in the hidden set, so
-- perm_editable_pages needs no change.
CREATE OR REPLACE FUNCTION perm_visible_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT p.id
    FROM pages p
    WHERE p.space_id IN (SELECT ps.space_id FROM perm_spaces(u, 'space.view') ps)
      AND (p.status = 'published' OR (p.status = 'draft' AND p.created_by = u))
      AND p.kind <> 'template'
      AND p.id NOT IN (SELECT h.page_id FROM perm_hidden_pages(u) h)
      AND p.id NOT IN (SELECT c.page_id FROM perm_control_hidden_pages(u) c)
$$;

CREATE OR REPLACE FUNCTION perm_listed_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT p.id
    FROM pages p
    WHERE p.space_id IN (SELECT ps.space_id FROM perm_spaces(u, 'space.view') ps)
      AND (p.status = 'published' OR (p.status = 'draft' AND p.created_by = u))
      AND p.kind <> 'template'
      AND p.archived_at IS NULL
      AND p.id NOT IN (SELECT h.page_id FROM perm_hidden_pages(u) h)
      AND p.id NOT IN (SELECT c.page_id FROM perm_control_hidden_pages(u) c)
$$;

CREATE OR REPLACE FUNCTION rbac_split_off(broad text) RETURNS text[] LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE broad
        WHEN 'space.administer' THEN ARRAY['site.publish', 'space.automation', 'space.templates', 'space.audit',
            'space.labels', 'space.statuses', 'space.approvals', 'space.bundle', 'space.look', 'page.watchers',
            'space.workflows']
        WHEN 'page.edit' THEN ARRAY['page.move_out', 'database.edit', 'page.workflow']
        WHEN 'space.manage_access' THEN ARRAY['space.invite_guests']
        WHEN 'page.create' THEN ARRAY['post.create', 'database.create']
        ELSE '{}'::text[] END
$$;

CREATE OR REPLACE FUNCTION rbac_words(perms text[]) RETURNS text LANGUAGE sql IMMUTABLE AS $$
    SELECT string_agg(CASE p
        WHEN 'space.view' THEN 'View the space'
        WHEN 'page.create' THEN 'Create pages'
        WHEN 'post.create' THEN 'Create blog posts'
        WHEN 'database.create' THEN 'Create databases'
        WHEN 'page.edit' THEN 'Edit pages'
        WHEN 'database.edit' THEN 'Edit databases'
        WHEN 'page.workflow' THEN 'Set a workflow on a page'
        WHEN 'page.archive' THEN 'Archive pages'
        WHEN 'page.delete_own' THEN 'Delete own pages'
        WHEN 'page.delete_any' THEN 'Delete any page'
        WHEN 'page.move_out' THEN 'Move pages to another space'
        WHEN 'page.watchers' THEN 'Manage who watches'
        WHEN 'space.export' THEN 'Export'
        WHEN 'space.bundle' THEN 'Bundle the space'
        WHEN 'comment.add' THEN 'Add comments'
        WHEN 'comment.delete_own' THEN 'Delete own comments'
        WHEN 'comment.delete_any' THEN 'Delete any comment'
        WHEN 'attachment.add' THEN 'Add files'
        WHEN 'attachment.delete_own' THEN 'Delete own files'
        WHEN 'attachment.delete_any' THEN 'Delete any file'
        WHEN 'space.manage_access' THEN 'Manage access'
        WHEN 'space.invite_guests' THEN 'Ask for guests'
        WHEN 'restriction.manage' THEN 'Restrict pages'
        WHEN 'space.administer' THEN 'Run the space'
        WHEN 'site.publish' THEN 'Publish to the web'
        WHEN 'space.automation' THEN 'Automation'
        WHEN 'space.templates' THEN 'Templates'
        WHEN 'space.audit' THEN 'See the audit'
        WHEN 'space.labels' THEN 'Labels'
        WHEN 'space.statuses' THEN 'Statuses and reviews'
        WHEN 'space.approvals' THEN 'Approval rule'
        WHEN 'space.workflows' THEN 'Workflows'
        WHEN 'space.look' THEN 'Look and shortcuts'
        WHEN 'analytics.view' THEN 'View analytics'
        WHEN 'use_product' THEN 'Use the product'
        WHEN 'view_profiles' THEN 'See people'
        WHEN 'invite_people' THEN 'Invite people'
        WHEN 'create_space' THEN 'Create spaces'
        WHEN 'create_personal_space' THEN 'Have a personal space'
        WHEN 'administrator' THEN 'Administrator'
        WHEN 'system_administrator' THEN 'System administrator'
        ELSE p END, ', ' ORDER BY p)
    FROM unnest(perms) AS p
$$;

SELECT rbac_split_roles_from('space.administer', ARRAY['space.workflows']);
SELECT rbac_split_roles_from('page.edit', ARRAY['page.workflow']);
