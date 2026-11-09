-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Eight more permissions, split off broader ones (spec 040, PRM-207 to PRM-214), as spec
-- 036 did for four. permissions.SplitFrom names the pairs. Whoever held the broader
-- permission gets the finer one too, so nobody gains or loses anything; a role can then
-- hold one without the other. The previous release does not read the new names: it keeps
-- asking for the broader permission, which the same people still hold.

-- rbac_split_off says which finer permissions came out of a broader one. Administer
-- implies space.invite_guests as it implies manage access, without a grant of its own.
CREATE FUNCTION rbac_split_off(broad text) RETURNS text[] LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE broad
        WHEN 'space.administer' THEN ARRAY['site.publish', 'space.automation', 'space.templates', 'space.audit',
            'space.labels', 'space.statuses', 'space.approvals', 'space.bundle', 'space.look', 'page.watchers']
        WHEN 'page.edit' THEN ARRAY['page.move_out']
        WHEN 'space.manage_access' THEN ARRAY['space.invite_guests']
        ELSE '{}'::text[] END
$$;

-- rbac_with_split_off adds to a list of permissions what was split off any of them.
CREATE FUNCTION rbac_with_split_off(perms text[]) RETURNS text[] LANGUAGE sql IMMUTABLE AS $$
    SELECT ARRAY(SELECT DISTINCT p FROM (
        SELECT unnest(perms) AS p
        UNION ALL
        SELECT unnest(rbac_split_off(b)) FROM unnest(perms) AS b) x ORDER BY 1)
$$;

CREATE OR REPLACE FUNCTION rbac_words(perms text[]) RETURNS text LANGUAGE sql IMMUTABLE AS $$
    SELECT string_agg(CASE p
        WHEN 'space.view' THEN 'View the space'
        WHEN 'page.create' THEN 'Create pages'
        WHEN 'page.edit' THEN 'Edit pages'
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

-- rbac_adopt now adds every split-off permission, of spec 036 and of this one, wherever
-- the broader one is. Its test runs it on grants that know none of them.
CREATE OR REPLACE FUNCTION rbac_adopt() RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    s record;
    g record;
BEGIN
    FOR s IN SELECT name, permissions FROM permission_sets ORDER BY created_at, id LOOP
        IF NOT EXISTS (SELECT 1 FROM roles WHERE scope = 'space' AND lower(name) = lower(left(s.name, 100))) THEN
            INSERT INTO roles (id, scope, name, description, permissions)
                VALUES (rbac_uuid7(), 'space', left(s.name, 100), 'Made from a permission set.',
                        rbac_with_split_off(s.permissions));
        END IF;
    END LOOP;
    INSERT INTO space_grants (space_id, subject_type, subject_id, permission)
        SELECT k.space_id, k.subject_type, k.subject_id, f.p
        FROM space_grants k CROSS JOIN LATERAL unnest(rbac_split_off(k.permission)) AS f(p)
        WHERE NOT EXISTS (SELECT 1 FROM role_assignments a WHERE a.space_id = k.space_id
              AND a.subject_type = k.subject_type AND a.subject_id = k.subject_id)
        ON CONFLICT DO NOTHING;
    FOR g IN SELECT space_id, subject_type, subject_id, array_agg(permission ORDER BY permission) AS perms
        FROM space_grants k
        WHERE NOT EXISTS (SELECT 1 FROM role_assignments a WHERE a.space_id = k.space_id
            AND a.subject_type = k.subject_type AND a.subject_id = k.subject_id)
        GROUP BY space_id, subject_type, subject_id ORDER BY space_id, subject_type, subject_id LOOP
        PERFORM rbac_set_exact(g.space_id, g.subject_type, g.subject_id, g.perms);
    END LOOP;
    FOR g IN SELECT subject_type, subject_id, array_agg(permission ORDER BY permission) AS perms
        FROM global_grants k
        WHERE NOT EXISTS (SELECT 1 FROM role_assignments a WHERE a.space_id IS NULL
            AND a.subject_type = k.subject_type AND a.subject_id = k.subject_id)
        GROUP BY subject_type, subject_id ORDER BY subject_type, subject_id LOOP
        PERFORM rbac_set_exact(NULL, g.subject_type, g.subject_id, g.perms);
    END LOOP;
END $$;

-- rbac_split_roles gives every space role the permissions split off what it already holds.
-- The trigger on roles then brings every grant along, so the people who hold the role keep
-- exactly what they could do. Built-in roles change once, here, to keep their meaning.
CREATE FUNCTION rbac_split_roles() RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    UPDATE roles SET permissions = rbac_with_split_off(permissions), updated_at = now()
    WHERE scope = 'space' AND permissions IS DISTINCT FROM rbac_with_split_off(permissions);
    UPDATE permission_sets SET permissions = rbac_with_split_off(permissions)
    WHERE permissions IS DISTINCT FROM rbac_with_split_off(permissions);
END $$;

SELECT rbac_split_roles();

-- permission_generation counts every change to the facts a permission check reads:
-- accounts, group members, instance and space grants, and the status of a space. A node
-- keeps those facts in memory between requests only while the number stays the same
-- (ARC-231), so a change counts at once on every node, and no lost message can leave a
-- revoked grant working. The previous release does not read it.
CREATE TABLE permission_generation (
    one boolean PRIMARY KEY DEFAULT true CHECK (one),
    n   bigint NOT NULL DEFAULT 0
);
INSERT INTO permission_generation DEFAULT VALUES;

CREATE FUNCTION permission_generation_bump() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE permission_generation SET n = n + 1;
    RETURN NULL;
END;
$$;

-- Statement level: one count per statement, however many rows it changed.
CREATE TRIGGER users_bump_permissions AFTER INSERT OR DELETE OR UPDATE OF type, status, expires_at ON users
    FOR EACH STATEMENT EXECUTE FUNCTION permission_generation_bump();
CREATE TRIGGER group_members_bump_permissions AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON group_members
    FOR EACH STATEMENT EXECUTE FUNCTION permission_generation_bump();
CREATE TRIGGER global_grants_bump_permissions AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON global_grants
    FOR EACH STATEMENT EXECUTE FUNCTION permission_generation_bump();
CREATE TRIGGER space_grants_bump_permissions AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON space_grants
    FOR EACH STATEMENT EXECUTE FUNCTION permission_generation_bump();
CREATE TRIGGER spaces_bump_permissions AFTER INSERT OR DELETE OR UPDATE OF status ON spaces
    FOR EACH STATEMENT EXECUTE FUNCTION permission_generation_bump();

-- A reaction names its page or comment by id without a foreign key, because it can be on
-- either. When the page or comment goes for good, through a purge or with its space, its
-- reactions go too (PRM-176). Reactions left behind before this migration go now; nobody
-- could reach them.
CREATE FUNCTION reactions_follow_target() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM reactions WHERE target_type = TG_ARGV[0] AND target_id = OLD.id;
    RETURN NULL;
END;
$$;
CREATE TRIGGER pages_take_reactions AFTER DELETE ON pages
    FOR EACH ROW EXECUTE FUNCTION reactions_follow_target('page');
CREATE TRIGGER comments_take_reactions AFTER DELETE ON comments
    FOR EACH ROW EXECUTE FUNCTION reactions_follow_target('comment');
DELETE FROM reactions r WHERE r.target_type = 'page' AND NOT EXISTS (SELECT 1 FROM pages p WHERE p.id = r.target_id);
DELETE FROM reactions r WHERE r.target_type = 'comment' AND NOT EXISTS (SELECT 1 FROM comments c WHERE c.id = r.target_id);

-- Faster forms of two list functions (ARC-205), with the same answers: the matrix test
-- compares them with the rule for every case. perm_hidden_pages hands the restricted pages
-- to page_tree as an array, so their branches are found through the index instead of a scan
-- of the whole tree. perm_listed_pages reads pages once instead of joining them to
-- perm_visible_pages; its conditions are those of perm_visible_pages, and the two change
-- together.
CREATE OR REPLACE FUNCTION perm_hidden_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT DISTINCT t.descendant_id
    FROM page_tree t
    WHERE t.ancestor_id = ANY (ARRAY(
        SELECT r.page_id
        FROM page_restrictions r
        WHERE r.kind = 'view'
        GROUP BY r.page_id
        HAVING NOT bool_or(
               (r.subject_type = 'user' AND r.subject_id = u)
            OR (r.subject_type = 'group' AND r.subject_id IN
                   (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = u)))))
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
$$;
