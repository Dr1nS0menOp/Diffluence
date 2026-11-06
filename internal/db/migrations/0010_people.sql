-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- People, access and administration (spec 013). space.manage_access needs no SQL:
-- lists only ever ask for space.view, and administer implies it in Evaluate.

ALTER TABLE users
    ADD COLUMN expires_at     timestamptz,
    ADD COLUMN last_login_at  timestamptz,
    ADD COLUMN anonymised_at  timestamptz,
    -- Until when a person put off being asked for a stronger sign-in.
    ADD COLUMN sign_in_prompt_after  timestamptz;

ALTER TABLE spaces
    ADD COLUMN kind      text NOT NULL DEFAULT 'team' CHECK (kind IN ('team', 'personal')),
    ADD COLUMN owner_id  uuid;
CREATE UNIQUE INDEX spaces_personal_owner_key ON spaces (owner_id) WHERE kind = 'personal';

ALTER TABLE sessions
    ADD COLUMN reauth_at            timestamptz,
    ADD COLUMN needs_second_factor  boolean NOT NULL DEFAULT false;

-- A SCIM token ends as well (SEC-156). The default also covers tokens made by the
-- previous release while both run: it gives them a year from now.
ALTER TABLE scim_tokens
    ADD COLUMN expires_at  timestamptz NOT NULL DEFAULT (now() + interval '1 year');

CREATE TABLE user_profiles (
    user_id        uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    job_title      text NOT NULL DEFAULT '',
    department     text NOT NULL DEFAULT '',
    location       text NOT NULL DEFAULT '',
    about          text NOT NULL DEFAULT '',
    picture_key    text NOT NULL DEFAULT '',
    visibility     jsonb NOT NULL DEFAULT '{}',
    locked_fields  text[] NOT NULL DEFAULT '{}',
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE group_owners (
    group_id    uuid NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX group_owners_user_idx ON group_owners (user_id);

CREATE TABLE api_tokens (
    id            uuid PRIMARY KEY,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name          text NOT NULL,
    token_hash    bytea NOT NULL UNIQUE,
    can_write     boolean NOT NULL DEFAULT false,
    expires_at    timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_used_at  timestamptz,
    revoked_at    timestamptz
);
CREATE INDEX api_tokens_user_idx ON api_tokens (user_id);

CREATE TABLE login_history (
    id          uuid PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    method      text NOT NULL,
    ip          text NOT NULL DEFAULT '',
    user_agent  text NOT NULL DEFAULT '',
    at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX login_history_user_idx ON login_history (user_id, at DESC);

CREATE TABLE known_devices (
    user_id        uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    device_hash    bytea NOT NULL,
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, device_hash)
);

CREATE TABLE permission_sets (
    id           uuid PRIMARY KEY,
    name         text NOT NULL,
    permissions  text[] NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX permission_sets_name_key ON permission_sets (lower(name));

CREATE TABLE space_categories (
    id    uuid PRIMARY KEY,
    name  text NOT NULL
);
CREATE UNIQUE INDEX space_categories_name_key ON space_categories (lower(name));

CREATE TABLE space_category_links (
    space_id     uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    category_id  uuid NOT NULL REFERENCES space_categories (id) ON DELETE CASCADE,
    PRIMARY KEY (space_id, category_id)
);

-- Left behind by "diffluence audit prune": where the chain starts after pruning.
CREATE TABLE audit_anchors (
    seq          bigint PRIMARY KEY,
    prev_hash    bytea NOT NULL,
    signature    bytea NOT NULL,
    public_key   bytea NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);


-- What identifies a person, apart from the chain (SEC-821). The hash covers it through
-- a digest in the event, so this row can go while the chain still holds.
CREATE TABLE audit_personal (
    seq   bigint PRIMARY KEY,
    salt  bytea NOT NULL,
    data  text NOT NULL
);

-- Mail about an account can go to someone without one, and its link is a secret, so
-- the queue only ever holds it sealed.
ALTER TABLE mail_queue ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE mail_queue ADD COLUMN sealed bytea;
ALTER TABLE mail_queue DROP CONSTRAINT mail_queue_kind_check;
ALTER TABLE mail_queue ADD CONSTRAINT mail_queue_kind_check CHECK (kind IN ('instant', 'digest', 'test', 'account'));

-- New global permission. Members who can use the product may have a personal space.
INSERT INTO global_grants (subject_type, subject_id, permission)
SELECT 'user', u.id, 'create_personal_space'
FROM users u
JOIN global_grants g ON g.subject_type = 'user' AND g.subject_id = u.id AND g.permission = 'use_product'
WHERE u.type = 'member'
ON CONFLICT DO NOTHING;

-- An account past its end date counts as not active, from that moment on.
CREATE OR REPLACE FUNCTION perm_has_global(u uuid, p text) RETURNS boolean
    LANGUAGE sql STABLE AS $$
    SELECT EXISTS (
        SELECT 1
        FROM users x
        JOIN global_grants g ON g.permission = p AND (
               (g.subject_type = 'user' AND g.subject_id = x.id)
            OR (g.subject_type = 'group' AND g.subject_id IN
                   (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = x.id)))
        WHERE x.id = u AND x.status = 'active' AND (x.expires_at IS NULL OR x.expires_at > now()))
$$;

CREATE OR REPLACE FUNCTION perm_spaces(u uuid, p text) RETURNS TABLE (space_id uuid)
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
    WHERE x.id = u AND x.status = 'active' AND (x.expires_at IS NULL OR x.expires_at > now())
      AND perm_has_global(u, 'use_product')
$$;

-- People a user may see: with "view profiles" everyone, otherwise those they share
-- a space with through a direct or group grant. "All members" does not count: one
-- open space would hand a guest the whole directory (spec 013, D5).
CREATE FUNCTION perm_visible_people(u uuid) RETURNS TABLE (user_id uuid)
    LANGUAGE sql STABLE AS $$
    SELECT t.id
    FROM users t
    WHERE t.type <> 'service'
      AND perm_has_global(u, 'use_product')
      AND (t.id = u
        OR perm_has_global(u, 'view_profiles')
        OR EXISTS (
            SELECT 1
            FROM space_grants a
            JOIN spaces s ON s.id = a.space_id AND s.status <> 'trashed'
            JOIN space_grants b ON b.space_id = a.space_id AND b.permission = 'space.view'
            WHERE a.permission = 'space.view'
              AND ((a.subject_type = 'user' AND a.subject_id = u)
                OR (a.subject_type = 'group' AND a.subject_id IN
                       (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = u)))
              AND ((b.subject_type = 'user' AND b.subject_id = t.id)
                OR (b.subject_type = 'group' AND b.subject_id IN
                       (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = t.id)))))
$$;
