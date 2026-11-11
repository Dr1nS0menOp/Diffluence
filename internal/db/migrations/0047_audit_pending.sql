-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 052, ADR-041, ARC-203. A saving transaction no longer takes the lock of the audit
-- chain. It writes its event here, in the same transaction as the change, and one worker
-- seals waiting events into audit_events a moment later, in the order they became
-- visible. The previous release still writes straight into audit_events under the chain
-- lock, which the sealer takes as well, so both can run side by side during an upgrade.

CREATE TABLE audit_pending (
    pend         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id           uuid NOT NULL UNIQUE,
    occurred_at  timestamptz NOT NULL,
    actor_type   text NOT NULL,
    actor_id     text NOT NULL DEFAULT '',
    action       text NOT NULL,
    object_type  text NOT NULL DEFAULT '',
    object_id    text NOT NULL DEFAULT '',
    space_id     text NOT NULL DEFAULT '',
    result       text NOT NULL,
    ip           text NOT NULL DEFAULT '',
    request_id   text NOT NULL DEFAULT '',
    -- The exact JSON the chain will hash, as audit_events.payload.
    payload      text NOT NULL,
    -- The personal part until it moves to audit_personal (SEC-821). It can be erased here
    -- too, before sealing; the payload keeps only its digest.
    salt         bytea,
    data         text,
    CHECK ((salt IS NULL) = (data IS NULL))
);

-- A waiting event may leave only once its copy is in the chain, and only its personal part
-- may be erased. The application role holds no other way round this: it owns nothing, so
-- it cannot switch the trigger off.
CREATE FUNCTION audit_pending_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'TRUNCATE' THEN
        RAISE EXCEPTION 'audit_pending: waiting events cannot be emptied';
    END IF;
    IF TG_OP = 'DELETE' THEN
        IF EXISTS (SELECT 1 FROM audit_events WHERE id = OLD.id) THEN
            RETURN OLD;
        END IF;
        RAISE EXCEPTION 'audit_pending: an event leaves only once it is sealed';
    END IF;
    IF NEW.salt IS NULL AND NEW.data IS NULL
       AND (NEW.pend, NEW.id, NEW.occurred_at, NEW.actor_type, NEW.actor_id, NEW.action, NEW.object_type,
            NEW.object_id, NEW.space_id, NEW.result, NEW.ip, NEW.request_id, NEW.payload)
           IS NOT DISTINCT FROM
           (OLD.pend, OLD.id, OLD.occurred_at, OLD.actor_type, OLD.actor_id, OLD.action, OLD.object_type,
            OLD.object_id, OLD.space_id, OLD.result, OLD.ip, OLD.request_id, OLD.payload) THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'audit_pending: only the personal part of a waiting event may be erased';
END;
$$;
CREATE TRIGGER audit_pending_guard
    BEFORE UPDATE OR DELETE ON audit_pending
    FOR EACH ROW EXECUTE FUNCTION audit_pending_guard();
CREATE TRIGGER audit_pending_no_truncate
    BEFORE TRUNCATE ON audit_pending
    FOR EACH STATEMENT EXECUTE FUNCTION audit_pending_guard();

-- Every event, sealed or still waiting, for whatever reads what happened rather than
-- checks the chain. A waiting event has no seq yet.
CREATE VIEW audit_entries AS
    SELECT seq, id, occurred_at, actor_type, actor_id, action, object_type, object_id, space_id,
           result, ip, request_id, payload, true AS sealed
    FROM audit_events
    UNION ALL
    SELECT NULL::bigint, id, occurred_at, actor_type, actor_id, action, object_type, object_id, space_id,
           result, ip, request_id, payload, false
    FROM audit_pending;
