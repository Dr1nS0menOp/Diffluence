-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Groups, spaces, pages, versions, drafts, restrictions (specs 001 and 002).

CREATE TABLE groups (
    id                  uuid PRIMARY KEY,
    origin_instance_id  uuid NOT NULL,
    name                text NOT NULL,
    description         text NOT NULL DEFAULT '',
    source              text NOT NULL CHECK (source IN ('local', 'scim', 'oidc')),
    external_id         text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX groups_name_key ON groups (lower(name));

CREATE TABLE group_members (
    group_id    uuid NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX group_members_user_idx ON group_members (user_id);

CREATE TABLE spaces (
    id                  uuid PRIMARY KEY,
    origin_instance_id  uuid NOT NULL,
    key                 text NOT NULL,
    name                text NOT NULL,
    description         text NOT NULL DEFAULT '',
    icon                text NOT NULL DEFAULT '',
    status              text NOT NULL CHECK (status IN ('active', 'archived', 'trashed')),
    home_page_id        uuid,
    created_by          uuid NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX spaces_key_key ON spaces (lower(key));

-- Layer 2 of the permission model. subject_id is the nil UUID for the
-- "all members" and "all guests" subjects.
CREATE TABLE space_grants (
    space_id      uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    subject_type  text NOT NULL CHECK (subject_type IN ('user', 'group', 'all_members', 'all_guests')),
    subject_id    uuid NOT NULL,
    permission    text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (space_id, subject_type, subject_id, permission)
);
CREATE INDEX space_grants_subject_idx ON space_grants (subject_type, subject_id, permission);

CREATE TABLE pages (
    id                  uuid PRIMARY KEY,
    origin_instance_id  uuid NOT NULL,
    space_id            uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    parent_id           uuid REFERENCES pages (id) ON DELETE SET NULL,
    position            integer NOT NULL DEFAULT 0,
    title               text NOT NULL,
    status              text NOT NULL CHECK (status IN ('draft', 'published', 'trashed')),
    pre_trash_status    text NOT NULL DEFAULT '',
    trash_item_id       uuid,
    current_version     integer NOT NULL DEFAULT 0,
    created_by          uuid NOT NULL,
    updated_by          uuid NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX pages_children_idx ON pages (space_id, parent_id, position);
CREATE INDEX pages_trash_item_idx ON pages (trash_item_id) WHERE trash_item_id IS NOT NULL;

-- Closure table: one row per (ancestor, descendant), including each page with itself.
CREATE TABLE page_tree (
    ancestor_id    uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    descendant_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    depth          integer NOT NULL,
    PRIMARY KEY (ancestor_id, descendant_id)
);
CREATE INDEX page_tree_descendant_idx ON page_tree (descendant_id, depth);

CREATE TABLE page_versions (
    id                  uuid PRIMARY KEY,
    origin_instance_id  uuid NOT NULL,
    page_id             uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    number              integer NOT NULL,
    title               text NOT NULL,
    document            jsonb NOT NULL,
    schema_version      integer NOT NULL,
    author_id           uuid NOT NULL,
    summary             text NOT NULL DEFAULT '',
    restored_from       integer,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (page_id, number)
);

-- Versions are immutable. Deleting is allowed: that is how a purge works.
CREATE FUNCTION page_versions_immutable() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'page_versions rows cannot be changed';
END;
$$;
CREATE TRIGGER page_versions_no_update
    BEFORE UPDATE ON page_versions
    FOR EACH ROW EXECUTE FUNCTION page_versions_immutable();

CREATE TABLE drafts (
    page_id         uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title           text NOT NULL,
    document        jsonb NOT NULL,
    schema_version  integer NOT NULL,
    base_version    integer NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, user_id)
);

-- Layer 3 of the permission model.
CREATE TABLE page_restrictions (
    page_id       uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    kind          text NOT NULL CHECK (kind IN ('view', 'edit')),
    subject_type  text NOT NULL CHECK (subject_type IN ('user', 'group')),
    subject_id    uuid NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, kind, subject_type, subject_id)
);
CREATE INDEX page_restrictions_kind_idx ON page_restrictions (kind, page_id);

CREATE TABLE favourites (
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    page_id     uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, page_id)
);

CREATE TABLE recent_pages (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    page_id    uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    viewed_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, page_id)
);
CREATE INDEX recent_pages_user_idx ON recent_pages (user_id, viewed_at DESC);
