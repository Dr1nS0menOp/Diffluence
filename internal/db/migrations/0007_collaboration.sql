-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Working together (spec 010): comments, reactions, watching, tasks, mentions,
-- notifications, mail and requests for access.

CREATE TABLE comments (
    id                  uuid PRIMARY KEY,
    origin_instance_id  uuid NOT NULL,
    space_id            uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    page_id             uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    parent_id           uuid REFERENCES comments (id) ON DELETE CASCADE,
    author_id           uuid NOT NULL,
    body                jsonb NOT NULL,
    anchor              jsonb,
    status              text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    editors_only        boolean NOT NULL DEFAULT false,
    resolved_by         uuid,
    resolved_at         timestamptz,
    trash_item_id       uuid,
    purged              boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now(),
    edited_at           timestamptz,
    edited_by           uuid
);
CREATE INDEX comments_page_idx ON comments (page_id, created_at);
CREATE INDEX comments_parent_idx ON comments (parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX comments_recent_idx ON comments (created_at DESC);

CREATE TABLE comment_seen (
    user_id  uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    page_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    seen_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, page_id)
);

CREATE TABLE reactions (
    target_type  text NOT NULL CHECK (target_type IN ('page', 'comment')),
    target_id    uuid NOT NULL,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    emoji        text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (target_type, target_id, user_id, emoji)
);
CREATE INDEX reactions_target_idx ON reactions (target_type, target_id);
CREATE INDEX reactions_recent_idx ON reactions (created_at DESC);

CREATE TABLE watches (
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    target_type  text NOT NULL CHECK (target_type IN ('page', 'branch', 'space')),
    target_id    uuid NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, target_type, target_id)
);
CREATE INDEX watches_target_idx ON watches (target_type, target_id);

CREATE TABLE follows (
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    followed_id  uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, followed_id)
);

-- Who the current version mentions. Comparing this with a new version gives the
-- mentions that are new, so nobody is told twice about the same one.
CREATE TABLE page_mentions (
    page_id  uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    user_id  uuid NOT NULL,
    PRIMARY KEY (page_id, user_id)
);

-- An index of the tasks in current versions. The document stays the one truth.
CREATE TABLE page_tasks (
    id            uuid PRIMARY KEY,
    page_id       uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    space_id      uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    text          text NOT NULL,
    assignee_id   uuid,
    creator_id    uuid NOT NULL,
    due_on        date,
    done          boolean NOT NULL DEFAULT false,
    done_at       timestamptz,
    position      integer NOT NULL,
    due_notified  boolean NOT NULL DEFAULT false,
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX page_tasks_page_idx ON page_tasks (page_id);
CREATE INDEX page_tasks_assignee_idx ON page_tasks (assignee_id) WHERE assignee_id IS NOT NULL;
CREATE INDEX page_tasks_creator_idx ON page_tasks (creator_id);
CREATE INDEX page_tasks_due_idx ON page_tasks (due_on) WHERE NOT done AND NOT due_notified;

-- A row holds ids, never a title or a sentence: what it shows is read when it is
-- shown, so a person without access can learn nothing from an old row.
CREATE TABLE notifications (
    id            uuid PRIMARY KEY,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type          text NOT NULL,
    reason        text NOT NULL,
    actor_id      uuid,
    page_id       uuid REFERENCES pages (id) ON DELETE CASCADE,
    space_id      uuid REFERENCES spaces (id) ON DELETE CASCADE,
    comment_id    uuid,
    group_key     text NOT NULL,
    source_event  uuid NOT NULL,
    data          jsonb NOT NULL DEFAULT '{}',
    read_at       timestamptz,
    hidden_at     timestamptz,
    digested_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, source_event)
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL AND hidden_at IS NULL;
CREATE INDEX notifications_age_idx ON notifications (created_at);

CREATE TABLE mail_queue (
    id                uuid PRIMARY KEY,
    user_id           uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind              text NOT NULL CHECK (kind IN ('instant', 'digest', 'test')),
    notification_ids  uuid[] NOT NULL DEFAULT '{}',
    status            text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sending', 'sent', 'failed', 'dropped')),
    attempts          integer NOT NULL DEFAULT 0,
    next_attempt_at   timestamptz NOT NULL DEFAULT now(),
    sent_at           timestamptz,
    last_error        text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mail_queue_due_idx ON mail_queue (next_attempt_at) WHERE status = 'queued';
-- Mentions in the same minute become one mail.
CREATE UNIQUE INDEX mail_queue_open_instant_key ON mail_queue (user_id) WHERE status = 'queued' AND kind = 'instant';

CREATE TABLE access_requests (
    id            uuid PRIMARY KEY,
    page_id       uuid NOT NULL REFERENCES pages (id) ON DELETE CASCADE,
    space_id      uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    requester_id  uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind          text NOT NULL CHECK (kind IN ('view', 'edit')),
    asked_id      uuid NOT NULL,
    status        text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'allowed', 'refused')),
    answered_by   uuid,
    note          text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    answered_at   timestamptz
);
CREATE UNIQUE INDEX access_requests_open_key ON access_requests (page_id, requester_id, kind) WHERE status = 'open';

ALTER TABLE user_preferences ADD COLUMN notifications jsonb NOT NULL DEFAULT '{}';
ALTER TABLE page_versions ADD COLUMN silent boolean NOT NULL DEFAULT false;
ALTER TABLE pages ADD COLUMN comments_closed boolean NOT NULL DEFAULT false;
ALTER TABLE spaces ADD COLUMN features jsonb NOT NULL DEFAULT '{}';

-- Widens what the trash can hold: an older node never writes the new value.
ALTER TABLE trash_items DROP CONSTRAINT trash_items_object_type_check;
ALTER TABLE trash_items ADD CONSTRAINT trash_items_object_type_check
    CHECK (object_type IN ('page', 'attachment', 'space', 'comment'));

CREATE INDEX page_versions_recent_idx ON page_versions (created_at DESC);
