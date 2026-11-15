-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 064, wave DC4: electronic signatures (blueprint 09.8). A transition can require a
-- signature; signing needs a fresh confirmation of identity for that one signature, and
-- writes a record bound to the content hash of the version signed. Every table and column
-- is new and the previous release reads none of them. Like every migration since 0049 it may
-- run twice, so each statement checks first.

-- The hash of a version's signed content, and the version of the canonical form it was
-- computed over (DOC-711, DOC-1303). Written when a version is first signed: most versions
-- are never signed, and a migration runs no Go code to hash the older ones. A version row is
-- immutable already, so the hash has a row of its own, written once and never changed.
CREATE TABLE IF NOT EXISTS version_hashes (
    page_id       uuid NOT NULL,
    number        integer NOT NULL,
    content_hash  bytea NOT NULL,
    content_form  smallint NOT NULL,
    hashed_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, number),
    FOREIGN KEY (page_id, number) REFERENCES page_versions (page_id, number)
);

-- Who may sign: one row per person, written by an administrator who confirmed who the person
-- is, and how (DOC-751). It is never part of a role (DOC-014).
CREATE TABLE IF NOT EXISTS signing_rights (
    user_id        uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    enabled_by     uuid NOT NULL,
    identified_by  text NOT NULL,
    enabled_at     timestamptz NOT NULL DEFAULT now()
);

-- A signing step in progress: begun for one signature, confirmed by the person, and taken by
-- that signature alone within two minutes of the confirmation (DOC-730, DOC-1416). Only the
-- hash of its token is kept, as for sessions.
CREATE TABLE IF NOT EXISTS signing_confirmations (
    token_hash    bytea PRIMARY KEY,
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    session_id    uuid NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    purpose       text NOT NULL,
    return_path   text NOT NULL DEFAULT '',
    passkey       jsonb,
    began_at      timestamptz NOT NULL DEFAULT now(),
    confirmed_at  timestamptz,
    method        text,
    expires_at    timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS signing_confirmations_expires ON signing_confirmations (expires_at);
CREATE INDEX IF NOT EXISTS signing_confirmations_user ON signing_confirmations (user_id);

-- The meanings a space adds to the four every space has (DOC-721). A meaning is never
-- removed: the workflows that name it keep it, and each record holds its words.
CREATE TABLE IF NOT EXISTS signature_meanings (
    space_id     uuid NOT NULL REFERENCES spaces (id) ON DELETE CASCADE,
    key          text NOT NULL,
    name         text NOT NULL,
    description  text NOT NULL DEFAULT '',
    created_by   uuid,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (space_id, key)
);

-- One electronic signature (DOC-710 to DOC-712). The printed name is the name at the moment
-- of signing (DOC-1225); the meaning's words are those shown when signing. The version it
-- signs cannot go while the record exists, nor the page: retention wins (DOC-770).
CREATE TABLE IF NOT EXISTS signatures (
    id                   uuid PRIMARY KEY,
    page_id              uuid NOT NULL,
    space_id             uuid NOT NULL,
    version_number       integer NOT NULL,
    workflow_id          uuid NOT NULL,
    workflow_version     integer NOT NULL,
    transition_id        text NOT NULL,
    round_id             uuid,
    stage                integer,
    signer_id            uuid NOT NULL REFERENCES users (id),
    printed_name         text NOT NULL,
    meaning              text NOT NULL,
    meaning_name         text NOT NULL,
    meaning_description  text NOT NULL,
    method               text NOT NULL CHECK (method IN ('password_and_code', 'passkey', 'identity_provider')),
    content_hash         bytea NOT NULL,
    content_form         smallint NOT NULL,
    audit_event_id       uuid NOT NULL,
    signed_at            timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (page_id, version_number) REFERENCES page_versions (page_id, number)
);
CREATE INDEX IF NOT EXISTS signatures_page ON signatures (page_id, version_number);
CREATE INDEX IF NOT EXISTS signatures_signer ON signatures (signer_id);

-- Nobody changes or deletes a signature, administrators included (DOC-770). Someone with the
-- database itself can still do so; verification then shows it (DOC-741).
CREATE OR REPLACE FUNCTION signatures_refuse_change() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'an electronic signature cannot be changed or deleted' USING ERRCODE = 'restrict_violation';
END
$$;
DROP TRIGGER IF EXISTS signatures_immutable ON signatures;
CREATE TRIGGER signatures_immutable BEFORE UPDATE OR DELETE ON signatures
    FOR EACH ROW EXECUTE FUNCTION signatures_refuse_change();
DROP TRIGGER IF EXISTS signatures_no_truncate ON signatures;
CREATE TRIGGER signatures_no_truncate BEFORE TRUNCATE ON signatures
    FOR EACH STATEMENT EXECUTE FUNCTION signatures_refuse_change();

-- The hash of a version, once kept, stays as it was.
DROP TRIGGER IF EXISTS version_hashes_immutable ON version_hashes;
CREATE TRIGGER version_hashes_immutable BEFORE UPDATE OR DELETE ON version_hashes
    FOR EACH ROW EXECUTE FUNCTION signatures_refuse_change();
-- The control history of a page and the auditor's export read the audit entries of the
-- pages they show by object (DOC-1010, DOC-1020). Only the sealer writes this table, so the
-- build holds back sealing for its duration and no save.
-- rewrite-ok: only the sealer writes audit_events, and saves wait in audit_pending, so the build delays sealing and never a save
CREATE INDEX IF NOT EXISTS audit_events_page_idx ON audit_events (object_id, seq) WHERE object_type = 'page';

-- A reviewer's approval that was signed is final for its stage.
ALTER TABLE workflow_reviewers ADD COLUMN IF NOT EXISTS signature_id uuid;

-- The file versions a signed version of a page shows: they stay as long as the signature
-- (DOC-770). Attachments asks this before a file or one of its versions may go.
CREATE OR REPLACE FUNCTION signed_file_versions(page uuid) RETURNS TABLE (attachment_id uuid, version integer)
    LANGUAGE sql STABLE AS $$
    SELECT DISTINCT f.attachment_id, f.version
    FROM signatures s
    JOIN attachment_pins pin ON pin.page_id = s.page_id AND pin.page_version = s.version_number
    CROSS JOIN LATERAL unnest(pin.attachment_ids, pin.versions) AS f (attachment_id, version)
    WHERE s.page_id = page
$$;

-- Whether an item of the trash holds signed records, which stay until retention ends: a page
-- or a page below it that was signed, a space with a signed page, a file a signed version
-- shows (DOC-770). The trash asks it before a purge.
CREATE OR REPLACE FUNCTION holds_signed_records(object_type text, object_id uuid) RETURNS boolean
    LANGUAGE sql STABLE AS $$
    SELECT CASE object_type
        WHEN 'page' THEN EXISTS (SELECT 1 FROM page_tree t JOIN signatures s ON s.page_id = t.descendant_id
                                 WHERE t.ancestor_id = object_id)
        WHEN 'space' THEN EXISTS (SELECT 1 FROM pages p JOIN signatures s ON s.page_id = p.id WHERE p.space_id = object_id)
        WHEN 'attachment' THEN EXISTS (SELECT 1 FROM attachments a, signed_file_versions(a.page_id) f
                                       WHERE a.id = object_id AND f.attachment_id = a.id)
        ELSE false
    END
$$;
