-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Where the file jobs stopped (spec 034). The encryption job of spec 032 and the job that
-- seals files again under a new master key each remember the last file they finished, in
-- name order, so a store of millions of files is done over many runs instead of starting
-- again each time. An empty value means: start at the first file.

ALTER TABLE storage_state
    ADD COLUMN encrypt_cursor text NOT NULL DEFAULT '',
    ADD COLUMN reseal_cursor  text NOT NULL DEFAULT '';
