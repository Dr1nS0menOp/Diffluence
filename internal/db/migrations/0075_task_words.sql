-- phase: expand
-- transaction: none
-- SPDX-License-Identifier: Apache-2.0
-- Tasks are found by the words of their text (COL-040, spec 086). The search reads
-- to_tsvector('simple', text), the way this index holds it, so it reads only the tasks it
-- finds instead of the words of every task. Built alongside, so writing tasks goes on while it
-- builds. The previous release reads nothing of it, so a rollback keeps working.
CREATE INDEX CONCURRENTLY IF NOT EXISTS page_tasks_words_idx ON page_tasks USING gin (to_tsvector('simple', text));
