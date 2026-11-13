-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 069, ADR-043: automation rules, databases, bundles between instances and restore
-- from the console are experimental for 1.0, and off on a new installation. An
-- installation that already used one keeps it: this turns on each part that left a trace.
--   automation       a rule exists, at the instance or in a space
--   databases        a page of kind database exists, also in the trash
--   bundles          a bundle export job is still listed, or the audit log holds an import
--   console_restore  a restore was ever asked for from the console
-- On a new installation nothing has left a trace, so the list is empty. The previous
-- release never reads this key. ON CONFLICT keeps a choice an administrator already made,
-- also when a restore of an older backup runs this migration again.
INSERT INTO settings (key, value)
SELECT 'experimental', jsonb_build_object('on', COALESCE(jsonb_agg(part ORDER BY part), '[]'::jsonb))
FROM (VALUES
    ('automation', EXISTS (SELECT 1 FROM automation_rules)),
    ('databases', EXISTS (SELECT 1 FROM pages WHERE kind = 'database')),
    ('bundles', EXISTS (SELECT 1 FROM jobs WHERE kind = 'bundle_export')
        OR EXISTS (SELECT 1 FROM audit_events WHERE action = 'bundle.imported')
        OR EXISTS (SELECT 1 FROM audit_pending WHERE action = 'bundle.imported')),
    ('console_restore', EXISTS (SELECT 1 FROM ops.restores))
) AS used (part, in_use)
WHERE in_use
ON CONFLICT (key) DO NOTHING;
