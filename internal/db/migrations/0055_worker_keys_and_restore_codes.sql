-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 070. The backup service signs the audit checkpoints with a key of its own, and
-- carries out a restore only with a code it wrote to its own log. The previous release
-- reads none of what this adds: its server keeps signing until a newer key is in force,
-- and its worker ignores the new columns of ops.restores.
--
-- Everything here may run again, as 0049 explains: a restore of a backup from before this
-- migration brings back a schema_migrations without it, on top of the ops that survived.

-- Every checkpoint key after the installation's own, which is derived from master.key. A key
-- signs the checkpoints from from_seq on, until the next one. The endorsement is the
-- installation's key signing from_seq and the public key, so whoever owns the tables but
-- holds neither key cannot add one that verifies. The application role may only read it.
CREATE TABLE IF NOT EXISTS audit_signing_keys (
    from_seq    bigint PRIMARY KEY CHECK (from_seq > 0),
    public_key  bytea NOT NULL CHECK (length(public_key) = 32),
    endorsement bytea NOT NULL CHECK (length(endorsement) = 64),
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- A checkpoint must carry the key in force at its position. An older server that still runs
-- beside a newer backup service during an upgrade is refused here instead of leaving a
-- checkpoint that verification would call tampering.
CREATE OR REPLACE FUNCTION audit_checkpoint_key_in_force() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    in_force bytea;
BEGIN
    SELECT public_key INTO in_force FROM audit_signing_keys
        WHERE from_seq <= NEW.seq ORDER BY from_seq DESC LIMIT 1;
    IF in_force IS NOT NULL AND in_force <> NEW.public_key THEN
        RAISE EXCEPTION 'audit: the checkpoint at seq % is signed with a key that is no longer in force', NEW.seq
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END
$$;
DROP TRIGGER IF EXISTS audit_checkpoints_key_in_force ON audit_checkpoints;
CREATE TRIGGER audit_checkpoints_key_in_force BEFORE INSERT ON audit_checkpoints
    FOR EACH ROW EXECUTE FUNCTION audit_checkpoint_key_in_force();

-- A restore waits for the code the backup service wrote to its log. confirmation is what the
-- console stored; the worker checks it, and seals a restore it confirmed, and the safety
-- backup it made, with a key the server never holds.
ALTER TABLE ops.restores ADD COLUMN IF NOT EXISTS confirmation text;
ALTER TABLE ops.restores ADD COLUMN IF NOT EXISTS code_shown_at timestamptz;
ALTER TABLE ops.restores ADD COLUMN IF NOT EXISTS wrong_codes integer NOT NULL DEFAULT 0;
ALTER TABLE ops.restores ADD COLUMN IF NOT EXISTS worker_seal bytea;
ALTER TABLE ops.restores ADD COLUMN IF NOT EXISTS safety_seal bytea;
