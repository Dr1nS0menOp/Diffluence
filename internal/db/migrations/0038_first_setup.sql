-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- The one-time code that guards the first setup screen (spec 038). Only a hash of the code
-- is kept, so reading the database does not give the code. A new code replaces the old one,
-- a used code closes the screen for good, and wrong tries are counted so a code cannot be
-- guessed. Nothing reads this table in the previous release.

CREATE TABLE setup_codes (
    id          uuid        PRIMARY KEY,
    code_hash   bytea       NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    wrong_tries integer     NOT NULL DEFAULT 0 CHECK (wrong_tries >= 0)
);
