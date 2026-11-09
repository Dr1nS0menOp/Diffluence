-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Roles an administrator makes (spec 036, ADR-034). A role is a named set of permissions,
-- for a space or for the instance, and people and groups get roles instead of single
-- permissions. space_grants and global_grants stay, but only as what roles add up to:
-- triggers keep them equal to the role assignments, so permissions.Evaluate and every
-- perm_* function read the same rows as before and the rule stays one rule. The previous
-- release reads those tables and still works on this schema.

CREATE TABLE roles (
    id           uuid PRIMARY KEY,
    scope        text NOT NULL CHECK (scope IN ('space', 'instance')),
    -- builtin names a role that keeps a fixed meaning, such as viewer. It cannot change.
    builtin      text UNIQUE,
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    description  text NOT NULL DEFAULT '' CHECK (length(description) <= 500),
    permissions  text[] NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX roles_name_key ON roles (scope, lower(name));

-- A role is never deleted while someone holds it: deleting would quietly take access
-- away from people nobody looked at (RESTRICT, spec 036 D5).
CREATE TABLE role_assignments (
    role_id       uuid NOT NULL REFERENCES roles (id) ON DELETE RESTRICT,
    space_id      uuid REFERENCES spaces (id) ON DELETE CASCADE,
    subject_type  text NOT NULL CHECK (subject_type IN ('user', 'group', 'all_members', 'all_guests')),
    subject_id    uuid NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX role_assignments_key ON role_assignments
    (role_id, COALESCE(space_id, '00000000-0000-0000-0000-000000000000'::uuid), subject_type, subject_id);
CREATE INDEX role_assignments_subject_idx ON role_assignments (subject_type, subject_id);
CREATE INDEX role_assignments_space_idx ON role_assignments (space_id, subject_type, subject_id);

-- UUIDv7, as ids.New makes them, for roles this migration creates (PostgreSQL 16 has no
-- uuidv7()): 48 bits of milliseconds, then version 7 over random bits.
CREATE FUNCTION rbac_uuid7() RETURNS uuid LANGUAGE sql VOLATILE AS $$
    SELECT encode(set_bit(set_bit(overlay(uuid_send(gen_random_uuid())
        PLACING substring(int8send(floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint) FROM 3)
        FROM 1 FOR 6), 52, 1), 53, 1), 'hex')::uuid
$$;

-- An assignment must match the scope of its role: a space role in a space, an instance
-- role for a user or group with no space. Anything else is refused, never coerced.
CREATE FUNCTION rbac_check_assignment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    role_scope text;
BEGIN
    SELECT scope INTO role_scope FROM roles WHERE id = NEW.role_id;
    IF role_scope = 'space' AND NEW.space_id IS NULL THEN
        RAISE EXCEPTION 'a space role needs a space' USING ERRCODE = 'check_violation';
    END IF;
    IF role_scope = 'instance' AND (NEW.space_id IS NOT NULL OR NEW.subject_type NOT IN ('user', 'group')) THEN
        RAISE EXCEPTION 'an instance role is for a user or a group, without a space' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER role_assignments_scope BEFORE INSERT OR UPDATE ON role_assignments
    FOR EACH ROW EXECUTE FUNCTION rbac_check_assignment();

-- rbac_sync makes the grants of one subject in one scope equal to what its roles add
-- up to. It only removes and adds the difference, so unchanged grants keep their rows.
-- It runs as the owner of the tables: the application role may not write grants itself.
CREATE FUNCTION rbac_sync(sp uuid, st text, sid uuid) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, public AS $$
BEGIN
    IF sp IS NULL THEN
        DELETE FROM global_grants g WHERE g.subject_type = st AND g.subject_id = sid
            AND NOT EXISTS (SELECT 1 FROM role_assignments a JOIN roles r ON r.id = a.role_id
                WHERE a.space_id IS NULL AND a.subject_type = st AND a.subject_id = sid AND g.permission = ANY (r.permissions));
        INSERT INTO global_grants (subject_type, subject_id, permission)
            SELECT DISTINCT st, sid, p FROM role_assignments a JOIN roles r ON r.id = a.role_id, unnest(r.permissions) p
            WHERE a.space_id IS NULL AND a.subject_type = st AND a.subject_id = sid
            ON CONFLICT DO NOTHING;
        RETURN;
    END IF;
    -- A space that is being deleted takes its grants with it.
    IF NOT EXISTS (SELECT 1 FROM spaces WHERE id = sp) THEN
        RETURN;
    END IF;
    DELETE FROM space_grants g WHERE g.space_id = sp AND g.subject_type = st AND g.subject_id = sid
        AND NOT EXISTS (SELECT 1 FROM role_assignments a JOIN roles r ON r.id = a.role_id
            WHERE a.space_id = sp AND a.subject_type = st AND a.subject_id = sid AND g.permission = ANY (r.permissions));
    INSERT INTO space_grants (space_id, subject_type, subject_id, permission)
        SELECT DISTINCT sp, st, sid, p FROM role_assignments a JOIN roles r ON r.id = a.role_id, unnest(r.permissions) p
        WHERE a.space_id = sp AND a.subject_type = st AND a.subject_id = sid
        ON CONFLICT DO NOTHING;
END $$;

CREATE FUNCTION rbac_assignment_changed() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, public AS $$
BEGIN
    IF TG_OP IN ('DELETE', 'UPDATE') THEN
        PERFORM rbac_sync(OLD.space_id, OLD.subject_type, OLD.subject_id);
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        PERFORM rbac_sync(NEW.space_id, NEW.subject_type, NEW.subject_id);
    END IF;
    RETURN NULL;
END $$;
CREATE TRIGGER role_assignments_sync AFTER INSERT OR UPDATE OR DELETE ON role_assignments
    FOR EACH ROW EXECUTE FUNCTION rbac_assignment_changed();

-- A changed role changes what everyone who holds it may do, at once.
CREATE FUNCTION rbac_role_changed() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, public AS $$
DECLARE
    a record;
BEGIN
    FOR a IN SELECT DISTINCT space_id, subject_type, subject_id FROM role_assignments WHERE role_id = NEW.id LOOP
        PERFORM rbac_sync(a.space_id, a.subject_type, a.subject_id);
    END LOOP;
    RETURN NULL;
END $$;
CREATE TRIGGER roles_sync AFTER UPDATE OF permissions ON roles
    FOR EACH ROW EXECUTE FUNCTION rbac_role_changed();

-- rbac_words names permissions in the words of the interface (permissions.* in
-- web/src/locales/en.json), so a role made from single permissions shows no keys such as
-- create_personal_space. TestRoleNamesUseTheWordsOfTheInterface holds the two equal.
CREATE FUNCTION rbac_words(perms text[]) RETURNS text LANGUAGE sql IMMUTABLE AS $$
    SELECT string_agg(CASE p
        WHEN 'space.view' THEN 'View the space'
        WHEN 'page.create' THEN 'Create pages'
        WHEN 'page.edit' THEN 'Edit pages'
        WHEN 'page.archive' THEN 'Archive pages'
        WHEN 'page.delete_own' THEN 'Delete own pages'
        WHEN 'page.delete_any' THEN 'Delete any page'
        WHEN 'space.export' THEN 'Export'
        WHEN 'comment.add' THEN 'Add comments'
        WHEN 'comment.delete_own' THEN 'Delete own comments'
        WHEN 'comment.delete_any' THEN 'Delete any comment'
        WHEN 'attachment.add' THEN 'Add files'
        WHEN 'attachment.delete_own' THEN 'Delete own files'
        WHEN 'attachment.delete_any' THEN 'Delete any file'
        WHEN 'space.manage_access' THEN 'Manage access'
        WHEN 'restriction.manage' THEN 'Restrict pages'
        WHEN 'space.administer' THEN 'Run the space'
        WHEN 'site.publish' THEN 'Publish to the web'
        WHEN 'space.automation' THEN 'Automation'
        WHEN 'space.templates' THEN 'Templates'
        WHEN 'space.audit' THEN 'See the audit'
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

-- rbac_role_for finds the role of a scope that holds exactly these permissions, a
-- built-in one first, and makes one when there is none. It is how the old way of
-- setting single permissions keeps working: every set becomes a role.
CREATE FUNCTION rbac_role_for(sc text, perms text[]) RETURNS uuid LANGUAGE plpgsql AS $$
DECLARE
    wanted text[] := ARRAY(SELECT DISTINCT p FROM unnest(perms) p ORDER BY 1);
    found uuid;
    base text;
    candidate text;
    n int := 1;
BEGIN
    SELECT id INTO found FROM roles
    WHERE scope = sc AND ARRAY(SELECT DISTINCT p FROM unnest(permissions) p ORDER BY 1) = wanted
    ORDER BY builtin IS NULL, created_at, id LIMIT 1;
    IF found IS NOT NULL THEN
        RETURN found;
    END IF;
    base := left('Custom: ' || rbac_words(wanted), 90);
    candidate := base;
    WHILE EXISTS (SELECT 1 FROM roles WHERE scope = sc AND lower(name) = lower(candidate)) LOOP
        n := n + 1;
        candidate := base || ' (' || n || ')';
    END LOOP;
    found := rbac_uuid7();
    INSERT INTO roles (id, scope, name, description, permissions)
        VALUES (found, sc, candidate, 'Made from permissions that were given one by one.', wanted);
    RETURN found;
END $$;

-- rbac_set_exact gives a subject exactly these permissions in a scope (NULL space is the
-- instance), as one role. An empty list removes every role it holds there.
CREATE FUNCTION rbac_set_exact(sp uuid, st text, sid uuid, perms text[]) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    target uuid;
    sc text := CASE WHEN sp IS NULL THEN 'instance' ELSE 'space' END;
BEGIN
    IF cardinality(perms) = 0 THEN
        DELETE FROM role_assignments WHERE space_id IS NOT DISTINCT FROM sp AND subject_type = st AND subject_id = sid;
        RETURN;
    END IF;
    target := rbac_role_for(sc, perms);
    DELETE FROM role_assignments WHERE space_id IS NOT DISTINCT FROM sp AND subject_type = st AND subject_id = sid
        AND role_id <> target;
    INSERT INTO role_assignments (role_id, space_id, subject_type, subject_id) VALUES (target, sp, st, sid)
        ON CONFLICT DO NOTHING;
END $$;

-- rbac_add_permissions adds permissions to what a subject already holds in a scope.
CREATE FUNCTION rbac_add_permissions(sp uuid, st text, sid uuid, perms text[]) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    current text[];
BEGIN
    IF sp IS NULL THEN
        SELECT COALESCE(array_agg(permission), '{}') INTO current FROM global_grants WHERE subject_type = st AND subject_id = sid;
    ELSE
        SELECT COALESCE(array_agg(permission), '{}') INTO current FROM space_grants WHERE space_id = sp AND subject_type = st AND subject_id = sid;
    END IF;
    PERFORM rbac_set_exact(sp, st, sid, current || perms);
END $$;

-- The built-in space roles keep the meaning of the old templates (permissions.Templates).
-- A test compares them, so the two cannot drift apart.
INSERT INTO roles (id, scope, builtin, name, description, permissions) VALUES
    (rbac_uuid7(), 'space', 'viewer', 'Viewer', 'Reads, exports and comments.',
        ARRAY['space.view', 'space.export', 'comment.add', 'comment.delete_own']),
    (rbac_uuid7(), 'space', 'editor', 'Editor', 'Writes and files pages, and adds files.',
        ARRAY['space.view', 'space.export', 'page.create', 'page.edit', 'page.archive', 'page.delete_own',
              'comment.add', 'comment.delete_own', 'attachment.add', 'attachment.delete_own']),
    (rbac_uuid7(), 'space', 'manager', 'Manager', 'Runs the people and the content of a space, but not the space itself.',
        ARRAY['space.view', 'space.export', 'page.create', 'page.edit', 'page.archive', 'page.delete_own', 'page.delete_any',
              'comment.add', 'comment.delete_own', 'comment.delete_any', 'attachment.add', 'attachment.delete_own',
              'attachment.delete_any', 'restriction.manage', 'space.manage_access']),
    (rbac_uuid7(), 'space', 'space_admin', 'Space admin', 'Everything in the space.',
        ARRAY['space.view', 'space.export', 'space.administer', 'space.manage_access', 'page.create', 'page.edit',
              'page.archive', 'page.delete_own', 'page.delete_any', 'comment.add', 'comment.delete_own',
              'comment.delete_any', 'attachment.add', 'attachment.delete_own', 'attachment.delete_any',
              'restriction.manage', 'analytics.view', 'site.publish', 'space.automation', 'space.templates',
              'space.audit']);

-- rbac_adopt turns grants that have no role behind them into roles, so nobody gains or
-- loses anything. The migration runs it once; the migration test runs it on a rich set
-- of single grants and compares every decision before and after. Sets made by
-- administrators become roles under their own names first.
--
-- Spec 036 also splits four permissions off space.administer (permissions.SplitFrom).
-- Whoever administered a space keeps publishing, automation, templates and the audit
-- of that space, so adoption first adds them wherever administer is.
CREATE FUNCTION rbac_adopt() RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    split_off CONSTANT text[] := ARRAY['site.publish', 'space.automation', 'space.templates', 'space.audit'];
    s record;
    g record;
BEGIN
    FOR s IN SELECT name, permissions FROM permission_sets ORDER BY created_at, id LOOP
        IF NOT EXISTS (SELECT 1 FROM roles WHERE scope = 'space' AND lower(name) = lower(left(s.name, 100))) THEN
            INSERT INTO roles (id, scope, name, description, permissions)
                VALUES (rbac_uuid7(), 'space', left(s.name, 100), 'Made from a permission set.',
                        ARRAY(SELECT DISTINCT p FROM unnest(s.permissions
                            || CASE WHEN 'space.administer' = ANY (s.permissions) THEN split_off ELSE '{}' END) p
                              ORDER BY 1));
        END IF;
    END LOOP;
    INSERT INTO space_grants (space_id, subject_type, subject_id, permission)
        SELECT k.space_id, k.subject_type, k.subject_id, f.p
        FROM space_grants k CROSS JOIN unnest(split_off) AS f(p)
        WHERE k.permission = 'space.administer'
          AND NOT EXISTS (SELECT 1 FROM role_assignments a WHERE a.space_id = k.space_id
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

SELECT rbac_adopt();

-- rbac_drift lists grants that differ from what roles add up to. It must stay empty;
-- the care report and the tests read it.
CREATE FUNCTION rbac_drift() RETURNS TABLE (space_id uuid, subject_type text, subject_id uuid, permission text, kind text)
LANGUAGE sql STABLE AS $$
    WITH derived AS (
        SELECT DISTINCT a.space_id, a.subject_type, a.subject_id, p AS permission
        FROM role_assignments a JOIN roles r ON r.id = a.role_id, unnest(r.permissions) p
    ), actual AS (
        SELECT space_id, subject_type, subject_id, permission FROM space_grants
        UNION ALL
        SELECT NULL::uuid, subject_type, subject_id, permission FROM global_grants
    )
    SELECT x.space_id, x.subject_type, x.subject_id, x.permission, 'grant without role'
    FROM actual x WHERE NOT EXISTS (SELECT 1 FROM derived d WHERE d.space_id IS NOT DISTINCT FROM x.space_id
        AND d.subject_type = x.subject_type AND d.subject_id = x.subject_id AND d.permission = x.permission)
    UNION ALL
    SELECT d.space_id, d.subject_type, d.subject_id, d.permission, 'role without grant'
    FROM derived d WHERE NOT EXISTS (SELECT 1 FROM actual x WHERE x.space_id IS NOT DISTINCT FROM d.space_id
        AND x.subject_type = d.subject_type AND x.subject_id = d.subject_id AND x.permission = d.permission)
        AND (d.space_id IS NULL OR EXISTS (SELECT 1 FROM spaces s WHERE s.id = d.space_id))
$$;
