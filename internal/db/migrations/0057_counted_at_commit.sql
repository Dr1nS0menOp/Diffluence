-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 071. The previous release reads permission_generation as before and does not read
-- audit_erasure_baseline, so both releases run together. It may run twice, as every
-- migration since 0049 may.

-- A change to the facts of a permission check is counted when its transaction commits,
-- once per transaction, instead of at each statement. The row of permission_generation is
-- then locked only for the moment of a commit, so a long transaction that adds people no
-- longer makes every other such change wait for it. The count still becomes visible
-- together with the change, so no node can keep facts older than the count it read.
CREATE OR REPLACE FUNCTION permission_generation_bump_once() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    -- A setting local to the transaction marks it as counted; it ends with the transaction.
    IF current_setting('diffluence.permissions_counted', true) = 'yes' THEN
        RETURN NULL;
    END IF;
    PERFORM set_config('diffluence.permissions_counted', 'yes', true);
    UPDATE permission_generation SET n = n + 1;
    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS users_bump_permissions ON users;
DROP TRIGGER IF EXISTS group_members_bump_permissions ON group_members;
DROP TRIGGER IF EXISTS global_grants_bump_permissions ON global_grants;
DROP TRIGGER IF EXISTS space_grants_bump_permissions ON space_grants;
DROP TRIGGER IF EXISTS spaces_bump_permissions ON spaces;
DROP TRIGGER IF EXISTS group_members_truncate_bumps_permissions ON group_members;
DROP TRIGGER IF EXISTS global_grants_truncate_bumps_permissions ON global_grants;
DROP TRIGGER IF EXISTS space_grants_truncate_bumps_permissions ON space_grants;

CREATE CONSTRAINT TRIGGER users_bump_permissions AFTER INSERT OR DELETE OR UPDATE OF type, status, expires_at ON users
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION permission_generation_bump_once();
CREATE CONSTRAINT TRIGGER group_members_bump_permissions AFTER INSERT OR UPDATE OR DELETE ON group_members
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION permission_generation_bump_once();
CREATE CONSTRAINT TRIGGER global_grants_bump_permissions AFTER INSERT OR UPDATE OR DELETE ON global_grants
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION permission_generation_bump_once();
CREATE CONSTRAINT TRIGGER space_grants_bump_permissions AFTER INSERT OR UPDATE OR DELETE ON space_grants
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION permission_generation_bump_once();
CREATE CONSTRAINT TRIGGER spaces_bump_permissions AFTER INSERT OR DELETE OR UPDATE OF status ON spaces
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION permission_generation_bump_once();

-- A constraint trigger cannot fire on TRUNCATE, which no code path runs; it keeps counting
-- at the statement, as before.
CREATE TRIGGER group_members_truncate_bumps_permissions AFTER TRUNCATE ON group_members
    FOR EACH STATEMENT EXECUTE FUNCTION permission_generation_bump();
CREATE TRIGGER global_grants_truncate_bumps_permissions AFTER TRUNCATE ON global_grants
    FOR EACH STATEMENT EXECUTE FUNCTION permission_generation_bump();
CREATE TRIGGER space_grants_truncate_bumps_permissions AFTER TRUNCATE ON space_grants
    FOR EACH STATEMENT EXECUTE FUNCTION permission_generation_bump();

-- The personal parts of audit events that were erased before each erasure recorded its
-- count in the chain. A check of the chain counts them as recorded, so an installation
-- that erased someone before this release still checks whole. Only the owner writes it.
-- A second run keeps the count of the first: erasures since then recorded their own.
CREATE TABLE IF NOT EXISTS audit_erasure_baseline (
    one   boolean PRIMARY KEY DEFAULT true CHECK (one),
    parts bigint NOT NULL
);
INSERT INTO audit_erasure_baseline (parts)
SELECT count(*) FROM audit_events e
WHERE coalesce(e.payload::jsonb ->> 'personal_digest', '') <> ''
  AND NOT EXISTS (SELECT 1 FROM audit_personal p WHERE p.seq = e.seq)
ON CONFLICT (one) DO NOTHING;
