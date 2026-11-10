-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- ADR-040, spec 049. Two changes to roles.
--
-- 1. Editing a database gets its own permission, database.edit, split off page.edit as
--    specs 036, 040 and 045 split theirs. Whoever held page.edit gets database.edit too, so
--    nobody gains or loses anything. The rule asks database.edit wherever it asked page.edit
--    of an item of kind database. The previous release does not read the new name: it keeps
--    asking page.edit for databases, which the same people still hold.
-- 2. A combination of single permissions that nobody named gets a short name that is never
--    cut off, such as "Custom combination 3 (12 permissions)", instead of the list of its
--    permissions cut at 90 characters.

CREATE OR REPLACE FUNCTION rbac_split_off(broad text) RETURNS text[] LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE broad
        WHEN 'space.administer' THEN ARRAY['site.publish', 'space.automation', 'space.templates', 'space.audit',
            'space.labels', 'space.statuses', 'space.approvals', 'space.bundle', 'space.look', 'page.watchers']
        WHEN 'page.edit' THEN ARRAY['page.move_out', 'database.edit']
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

-- rbac_split_roles_from gives the permissions split off one broader permission to every
-- space role and own set that holds that broader one and none of them. Unlike
-- rbac_split_roles it touches no other split, so a role made after an earlier split without
-- one of its permissions keeps it that way. The trigger on roles brings every holder's
-- grants along.
CREATE FUNCTION rbac_split_roles_from(broad text, finer text[]) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    UPDATE roles SET permissions = ARRAY(SELECT DISTINCT p FROM unnest(permissions || finer) p ORDER BY 1),
        updated_at = now()
    WHERE scope = 'space' AND broad = ANY (permissions) AND NOT permissions && finer;
    UPDATE permission_sets SET permissions = ARRAY(SELECT DISTINCT p FROM unnest(permissions || finer) p ORDER BY 1)
    WHERE broad = ANY (permissions) AND NOT permissions && finer;
END $$;

SELECT rbac_split_roles_from('page.edit', ARRAY['database.edit']);

-- Pages a person may edit: the rule of page.edit for a list, which asks database.edit of a
-- database (permissions.Evaluate, spec 049).
CREATE OR REPLACE FUNCTION perm_editable_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT v.page_id
    FROM perm_visible_pages(u) v
    JOIN pages p ON p.id = v.page_id
    JOIN spaces s ON s.id = p.space_id
    WHERE s.status = 'active' AND p.archived_at IS NULL
      -- Two lists of spaces, each read once, rather than one lookup per page.
      AND CASE WHEN p.kind = 'database'
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
                   (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = u))))
$$;

-- rbac_unnamed_role_name is the name of a combination nobody named: a number that stays
-- with the role, and how many permissions it holds. permissions.UnnamedRoleName is the same
-- in Go, and a test holds the two equal.
CREATE FUNCTION rbac_unnamed_role_name(n int, perms int) RETURNS text LANGUAGE sql IMMUTABLE AS $$
    SELECT 'Custom combination ' || n || ' (' || perms || CASE WHEN perms = 1 THEN ' permission)' ELSE ' permissions)' END
$$;

-- rbac_next_unnamed_number is one more than the highest number an unnamed role of a scope
-- carries, so a number is never given twice while its role exists.
CREATE FUNCTION rbac_next_unnamed_number(sc text) RETURNS int LANGUAGE sql STABLE AS $$
    SELECT COALESCE(max(substring(name FROM '^Custom combination ([0-9]+) \(')::int), 0) + 1
    FROM roles WHERE scope = sc AND name ~ '^Custom combination [0-9]+ \([0-9]+ permissions?\)$'
$$;

-- rbac_free_unnamed_name is the name the next unnamed role of a scope gets. A role an
-- administrator named this way by hand keeps its name; the number after it is used.
CREATE FUNCTION rbac_free_unnamed_name(sc text, perms int) RETURNS text LANGUAGE plpgsql STABLE AS $$
DECLARE
    n int := rbac_next_unnamed_number(sc);
    candidate text := rbac_unnamed_role_name(n, perms);
BEGIN
    WHILE EXISTS (SELECT 1 FROM roles WHERE scope = sc AND lower(name) = lower(candidate)) LOOP
        n := n + 1;
        candidate := rbac_unnamed_role_name(n, perms);
    END LOOP;
    RETURN candidate;
END $$;

CREATE OR REPLACE FUNCTION rbac_role_for(sc text, perms text[]) RETURNS uuid LANGUAGE plpgsql AS $$
DECLARE
    wanted text[] := ARRAY(SELECT DISTINCT p FROM unnest(perms) p ORDER BY 1);
    found uuid;
BEGIN
    SELECT id INTO found FROM roles
    WHERE scope = sc AND ARRAY(SELECT DISTINCT p FROM unnest(permissions) p ORDER BY 1) = wanted
    ORDER BY builtin IS NULL, created_at, id LIMIT 1;
    IF found IS NOT NULL THEN
        RETURN found;
    END IF;
    found := rbac_uuid7();
    INSERT INTO roles (id, scope, name, description, permissions)
        VALUES (found, sc, rbac_free_unnamed_name(sc, cardinality(wanted)),
                'Made from permissions that were given one by one.', wanted);
    RETURN found;
END $$;

-- Combinations made before this migration carry the cut list of their permissions as their
-- name. rbac_rename_unnamed gives those nobody renamed the short name, numbered in the order
-- they were made. Only the name changes, so nobody gains or loses anything.
CREATE FUNCTION rbac_rename_unnamed() RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    r record;
BEGIN
    FOR r IN SELECT id, scope, permissions FROM roles
        WHERE builtin IS NULL AND name LIKE 'Custom: %'
          AND description = 'Made from permissions that were given one by one.'
        ORDER BY created_at, id LOOP
        UPDATE roles SET name = rbac_free_unnamed_name(r.scope, cardinality(r.permissions)), updated_at = now()
        WHERE id = r.id;
    END LOOP;
END $$;

SELECT rbac_rename_unnamed();
