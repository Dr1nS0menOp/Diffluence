-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- The language of a page, and blocks a person saved to use again (spec 027).

ALTER TABLE pages ADD COLUMN language text NOT NULL DEFAULT '';

CREATE TABLE saved_blocks (
    id          uuid PRIMARY KEY,
    owner_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name        text NOT NULL,
    document    jsonb NOT NULL,
    shared      boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX saved_blocks_owner_idx ON saved_blocks (owner_id);
CREATE INDEX saved_blocks_shared_idx ON saved_blocks (shared) WHERE shared;
