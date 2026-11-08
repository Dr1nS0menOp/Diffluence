-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Nothing may fall open through a delete (DAT-015, GAP-019, spec 035). A page's parent was
-- ON DELETE SET NULL, so deleting a parent row alone would lift every view restriction its
-- children inherit. NO ACTION refuses that, and is checked at the end of a statement, so
-- deleting a whole branch at once, as the purge does, keeps working. The previous release
-- only ever deletes whole branches, so it runs on this schema too.

DO $$
DECLARE
    name text;
BEGIN
    SELECT c.conname INTO name
      FROM pg_constraint c
     WHERE c.conrelid = 'pages'::regclass AND c.contype = 'f'
       AND c.conkey = ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid = 'pages'::regclass AND attname = 'parent_id')]::smallint[];
    EXECUTE format('ALTER TABLE pages DROP CONSTRAINT %I', name);
END $$;

ALTER TABLE pages ADD CONSTRAINT pages_parent_id_fkey
    FOREIGN KEY (parent_id) REFERENCES pages (id) ON DELETE NO ACTION;
