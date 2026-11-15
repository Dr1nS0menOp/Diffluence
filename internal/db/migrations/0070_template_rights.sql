-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 074, wave GT4: rights per template and approval. A template gets two lists that only
-- narrow who may use it and who of its managers may change it, a template of all spaces the
-- spaces it is offered in, and a version waits for one decision by one person before others
-- use it. People who may create pages may write and propose templates, through
-- template.propose, split off page.create as specs 036, 040, 045, 049, 064 and 073 split
-- theirs, so nobody gains or loses anything they could do before (TPL-1102).
--
-- Expand only. The previous release knows none of these tables and reads only rows it could
-- write itself: until every node runs this release, the new release refuses to write a list,
-- a draft or the four-eyes setting (TPL-1103), so an older node never meets one. It may run
-- twice, as every migration since 0049 may.

-- A template can be withdrawn, and a version too (TPL-524). A version that was approved and
-- then replaced keeps working for a form already open, for one day (TPL-603). The wider checks
-- are added NOT VALID, so no table is read under its lock: every row there already passes the
-- narrower check they replace.
ALTER TABLE templates DROP CONSTRAINT IF EXISTS templates_state_check;
ALTER TABLE templates ADD CONSTRAINT templates_state_check
    CHECK (state IN ('draft', 'waiting', 'approved', 'sent_back', 'withdrawn')) NOT VALID;
ALTER TABLE template_versions DROP CONSTRAINT IF EXISTS template_versions_state_check;
ALTER TABLE template_versions ADD CONSTRAINT template_versions_state_check
    CHECK (state IN ('draft', 'waiting', 'approved', 'sent_back', 'superseded', 'withdrawn')) NOT VALID;
ALTER TABLE template_versions ADD COLUMN IF NOT EXISTS superseded_at timestamptz;
-- Before this release every version was approved at once, so all but the one offered were
-- replaced when the next was saved.
UPDATE template_versions v SET state = 'superseded',
    superseded_at = COALESCE((SELECT n.created_at FROM template_versions n
        WHERE n.template_id = v.template_id AND n.number = v.number + 1), now())
FROM templates t
WHERE t.id = v.template_id AND v.state = 'approved' AND v.number < COALESCE(t.approved_version, 0);

-- Who may use a template and who of its managers may change it (TPL-500). An empty list
-- adds no limit. Subjects are people and groups, as page restrictions are.
CREATE TABLE IF NOT EXISTS template_rights (
    template_id   uuid NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
    kind          text NOT NULL CHECK (kind IN ('use', 'edit')),
    subject_type  text NOT NULL CHECK (subject_type IN ('user', 'group')),
    subject_id    uuid NOT NULL,
    PRIMARY KEY (template_id, kind, subject_type, subject_id)
);

-- A template of all spaces is offered in every space, or only in the spaces chosen here
-- (TPL-502). A deleted space drops off the list with its rows.
ALTER TABLE templates ADD COLUMN IF NOT EXISTS offered_in_chosen boolean NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS template_spaces (
    template_id  uuid NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
    space_id     uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    PRIMARY KEY (template_id, space_id)
);
CREATE INDEX IF NOT EXISTS template_spaces_space ON template_spaces (space_id);

-- Every proposal and every decision about a version, with who and why (TPL-522, TPL-602).
-- The reason of a decision is the approver's own words, never an answer of a form.
CREATE TABLE IF NOT EXISTS template_reviews (
    id           uuid PRIMARY KEY,
    template_id  uuid NOT NULL REFERENCES templates (id) ON DELETE CASCADE,
    version      integer NOT NULL,
    decision     text NOT NULL CHECK (decision IN ('proposed', 'approved', 'sent_back', 'withdrawn')),
    person_id    uuid NOT NULL,
    comment      text NOT NULL DEFAULT '' CHECK (length(comment) <= 2000),
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS template_reviews_template ON template_reviews (template_id, created_at);

-- "Every template change needs a second person", for one space, or for the templates of all
-- spaces when space_id is null (TPL-511). No row is off, the default (TPL-1412).
CREATE TABLE IF NOT EXISTS template_four_eyes (
    space_id    uuid REFERENCES spaces (id) ON DELETE CASCADE,
    changed_by  uuid NOT NULL,
    changed_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT template_four_eyes_scope UNIQUE NULLS NOT DISTINCT (space_id)
);

-- The hidden pages of the templates a person writes or looks after, which is where the bell
-- hangs a notification about a template: its owner, and whoever manages the templates of its
-- scope and is on its editors list when it has one (TPL-520, TPL-521, TPL-722).
CREATE OR REPLACE FUNCTION perm_template_notice_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT t.page_id
    FROM templates t
    WHERE t.owner_id = u
       OR ((CASE WHEN t.space_id IS NULL THEN perm_has_global(u, 'administrator')
                 ELSE t.space_id IN (SELECT ps.space_id FROM perm_spaces(u, 'space.templates') ps) END)
           AND (NOT EXISTS (SELECT 1 FROM template_rights r WHERE r.template_id = t.id AND r.kind = 'edit')
                OR EXISTS (SELECT 1 FROM template_rights r WHERE r.template_id = t.id AND r.kind = 'edit'
                    AND ((r.subject_type = 'user' AND r.subject_id = u)
                      OR (r.subject_type = 'group' AND r.subject_id IN
                          (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = u))))))
$$;

CREATE OR REPLACE FUNCTION rbac_split_off(broad text) RETURNS text[] LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE broad
        WHEN 'space.administer' THEN ARRAY['site.publish', 'space.automation', 'space.templates', 'space.audit',
            'space.labels', 'space.statuses', 'space.approvals', 'space.bundle', 'space.look', 'page.watchers',
            'space.workflows', 'space.release']
        WHEN 'page.edit' THEN ARRAY['page.move_out', 'database.edit', 'page.workflow']
        WHEN 'space.manage_access' THEN ARRAY['space.invite_guests']
        WHEN 'page.create' THEN ARRAY['post.create', 'database.create', 'template.propose']
        ELSE '{}'::text[] END
$$;

CREATE OR REPLACE FUNCTION rbac_words(perms text[]) RETURNS text LANGUAGE sql IMMUTABLE AS $$
    SELECT string_agg(CASE p
        WHEN 'space.view' THEN 'View the space'
        WHEN 'page.create' THEN 'Create pages'
        WHEN 'post.create' THEN 'Create blog posts'
        WHEN 'database.create' THEN 'Create databases'
        WHEN 'template.propose' THEN 'Propose templates'
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
        WHEN 'space.release' THEN 'Release to the published space'
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

SELECT rbac_split_roles_from('page.create', ARRAY['template.propose']);
