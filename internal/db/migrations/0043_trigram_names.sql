-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Part of a word and a small typo also find a title or a file name (ADR-038, spec 047,
-- D11). pg_trgm ships with PostgreSQL and has been a trusted extension since version 13,
-- so the role that owns the tables may create it. The checks before migrating refuse an
-- installation where it is missing, before anything changes (D12). The previous release
-- reads neither index, so a rollback keeps working.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX search_pages_title_trgm_idx ON search_pages USING gin (lower(title) gin_trgm_ops);
CREATE INDEX attachments_filename_trgm_idx ON attachments USING gin (lower(filename) gin_trgm_ops);
