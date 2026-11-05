-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Reading and getting around (spec 009): page look, personal lists, shortcuts,
-- and the links between pages.

ALTER TABLE pages
    ADD COLUMN emoji                 text NOT NULL DEFAULT '',
    ADD COLUMN full_width            boolean NOT NULL DEFAULT false,
    ADD COLUMN title_align           text NOT NULL DEFAULT 'start' CHECK (title_align IN ('start', 'center')),
    ADD COLUMN hide_children         boolean NOT NULL DEFAULT false,
    ADD COLUMN header_attachment_id  uuid REFERENCES attachments (id) ON DELETE SET NULL,
    ADD COLUMN header_position       smallint NOT NULL DEFAULT 50 CHECK (header_position BETWEEN 0 AND 100);

-- Empty until a person reorders their list. New favourites come first.
ALTER TABLE favourites ADD COLUMN position integer;

CREATE TABLE space_favourites (
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    space_id    uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, space_id)
);

CREATE TABLE user_preferences (
    user_id     uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    preferences jsonb NOT NULL DEFAULT '{}',
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE space_shortcuts (
    id        uuid PRIMARY KEY,
    space_id  uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    label     text NOT NULL,
    target    text NOT NULL,
    position  integer NOT NULL
);
CREATE INDEX space_shortcuts_space_idx ON space_shortcuts (space_id, position);

-- One row per link in a page's current version. The target has no foreign key:
-- a link to a page that is gone stays visible, so it can be cleared up.
CREATE TABLE page_links (
    source_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    target_id  uuid NOT NULL,
    PRIMARY KEY (source_id, target_id)
);
CREATE INDEX page_links_target_idx ON page_links (target_id);

INSERT INTO page_links (source_id, target_id)
SELECT DISTINCT p.id, substring(href #>> '{}' FROM '^/pages/([0-9a-fA-F-]{36})')::uuid
FROM pages p
JOIN page_versions v ON v.page_id = p.id AND v.number = p.current_version
CROSS JOIN LATERAL jsonb_path_query(v.document, '$.**.marks[*] ? (@.type == "link").attrs.href') AS href
WHERE (href #>> '{}') ~ '^/pages/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}';
