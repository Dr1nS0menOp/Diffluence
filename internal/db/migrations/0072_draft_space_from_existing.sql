-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 073, wave PB4: a draft space started from a space that has content (PUB-1302). The start
-- copies every page into a new draft space and pairs each copy with its source in step; it is
-- kept as a run of its own scope, whose pages hold what verification needs to check the copy
-- against the page readers have (PUB-920). Undoing the start before the first release finds the
-- pair by that run (PUB-1311).
--
-- Expand only (PUB-1300): a wider check on release_runs, which the previous release reads but
-- never writes with the new value. It may run twice, as every migration since 0049 may.
ALTER TABLE release_runs DROP CONSTRAINT IF EXISTS release_runs_scope_check;
-- rewrite-ok: the table came with migration 0061 of this part and holds rows only once the experimental part is on
ALTER TABLE release_runs ADD CONSTRAINT release_runs_scope_check
    CHECK (scope IN ('page', 'branch', 'space', 'pages', 'take_off', 'start'));
