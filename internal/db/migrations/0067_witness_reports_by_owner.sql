-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 077, wave RV7. A witness report is recorded by the owner of the tables: in Compose the
-- backup service, which holds that role and a key the server lacks (spec 070). From this
-- release the application role reads reports and adds none (db.GrantApp). Each report notes
-- the role that recorded it, and the database stamps it and its time, so the console shows
-- only reports of the owner. Reports from before this migration keep an empty role and stay
-- in the table, unshown. The previous release reads every report and ignores the column. It
-- may run twice, as every migration since 0049 may.

-- Added with an empty default first, so the reports already there keep it; only then does the
-- trigger below fill in the role of each new report.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_attribute
      WHERE attrelid = 'audit_witness_checks'::regclass AND attname = 'recorded_by' AND NOT attisdropped) THEN
    ALTER TABLE audit_witness_checks ADD COLUMN recorded_by text NOT NULL DEFAULT '';
  END IF;
END
$$;

-- The role and the time are the database's own, whatever an insert names: a report cannot
-- claim another role, or a time to come that would keep it the newest.
CREATE OR REPLACE FUNCTION audit_witness_check_stamp() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  NEW.recorded_by := current_user;
  NEW.checked_at := now();
  RETURN NEW;
END
$$;
DROP TRIGGER IF EXISTS audit_witness_checks_stamp ON audit_witness_checks;
CREATE TRIGGER audit_witness_checks_stamp BEFORE INSERT ON audit_witness_checks
    FOR EACH ROW EXECUTE FUNCTION audit_witness_check_stamp();
