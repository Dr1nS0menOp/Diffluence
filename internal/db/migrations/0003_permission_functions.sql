-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- The permission rule (spec 002) as SQL, for everything that LISTS content:
-- trees, search, favourites, exports. Single decisions are made by
-- permissions.Evaluate in Go. The matrix test proves the two always agree.
-- All functions are plain STABLE SQL, so the planner can inline them.

-- Does the active user hold this global permission, directly or through a group?
CREATE FUNCTION perm_has_global(u uuid, p text) RETURNS boolean
    LANGUAGE sql STABLE AS $$
    SELECT EXISTS (
        SELECT 1
        FROM users x
        JOIN global_grants g ON g.permission = p AND (
               (g.subject_type = 'user' AND g.subject_id = x.id)
            OR (g.subject_type = 'group' AND g.subject_id IN
                   (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = x.id)))
        WHERE x.id = u AND x.status = 'active')
$$;

-- Spaces (not in the trash) where the user holds space permission p.
CREATE FUNCTION perm_spaces(u uuid, p text) RETURNS TABLE (space_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT DISTINCT g.space_id
    FROM users x
    JOIN space_grants g ON g.permission = p AND (
           (g.subject_type = 'user' AND g.subject_id = x.id)
        OR (g.subject_type = 'group' AND g.subject_id IN
               (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = x.id))
        OR (g.subject_type = 'all_members' AND x.type = 'member')
        OR (g.subject_type = 'all_guests' AND x.type = 'guest'))
    JOIN spaces s ON s.id = g.space_id AND s.status <> 'trashed'
    WHERE x.id = u AND x.status = 'active' AND perm_has_global(u, 'use_product')
$$;

-- Every page below (or at) a view restriction the user does not match.
CREATE FUNCTION perm_hidden_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT DISTINCT t.descendant_id
    FROM page_tree t
    WHERE t.ancestor_id IN (
        SELECT r.page_id
        FROM page_restrictions r
        WHERE r.kind = 'view'
        GROUP BY r.page_id
        HAVING NOT bool_or(
               (r.subject_type = 'user' AND r.subject_id = u)
            OR (r.subject_type = 'group' AND r.subject_id IN
                   (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = u))))
$$;

-- Pages the user may view: the only way lists see content.
CREATE FUNCTION perm_visible_pages(u uuid) RETURNS TABLE (page_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT p.id
    FROM pages p
    WHERE p.space_id IN (SELECT ps.space_id FROM perm_spaces(u, 'space.view') ps)
      AND (p.status = 'published' OR (p.status = 'draft' AND p.created_by = u))
      AND p.id NOT IN (SELECT h.page_id FROM perm_hidden_pages(u) h)
$$;
