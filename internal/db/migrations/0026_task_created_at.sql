-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- When a task came to be, for the task report (spec 026). updated_at moves with every edit,
-- so it is the best guess for tasks that already exist, and nothing more.

ALTER TABLE page_tasks ADD COLUMN created_at timestamptz;
UPDATE page_tasks SET created_at = updated_at;
ALTER TABLE page_tasks ALTER COLUMN created_at SET DEFAULT now();
ALTER TABLE page_tasks ALTER COLUMN created_at SET NOT NULL;
