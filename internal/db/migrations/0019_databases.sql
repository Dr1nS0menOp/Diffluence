-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Databases (spec 019): a fourth kind of item. It only widens what is allowed.
ALTER TABLE pages DROP CONSTRAINT pages_kind_check;
ALTER TABLE pages ADD CONSTRAINT pages_kind_check
    CHECK (kind IN ('page', 'post', 'folder', 'link', 'template', 'database'));
