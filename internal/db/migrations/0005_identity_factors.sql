-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- TOTP, passkeys, OIDC links, SCIM (spec 004); node registry and audit checkpoints (spec 005).

ALTER TABLE users ADD COLUMN external_id text NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN source text NOT NULL DEFAULT 'local';
ALTER TABLE users ADD COLUMN username text NOT NULL DEFAULT '';

-- New global permission. Members who can use the product may see colleagues;
-- guests get it only when someone grants it.
INSERT INTO global_grants (subject_type, subject_id, permission)
SELECT 'user', u.id, 'view_profiles'
FROM users u
JOIN global_grants g ON g.subject_type = 'user' AND g.subject_id = u.id AND g.permission = 'use_product'
WHERE u.type = 'member'
ON CONFLICT DO NOTHING;

-- Links a user to an identity provider. Never matched by e-mail.
CREATE TABLE user_identities (
    provider       text NOT NULL,
    subject        text NOT NULL,
    user_id        uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_login_at  timestamptz,
    PRIMARY KEY (provider, subject)
);
CREATE INDEX user_identities_user_idx ON user_identities (user_id);

CREATE TABLE totp_credentials (
    user_id        uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    secret_sealed  bytea NOT NULL,
    confirmed_at   timestamptz,
    last_step      bigint NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE recovery_codes (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash  bytea NOT NULL,
    used_at    timestamptz,
    PRIMARY KEY (user_id, code_hash)
);

CREATE TABLE webauthn_credentials (
    id             uuid PRIMARY KEY,
    user_id        uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    credential_id  bytea NOT NULL UNIQUE,
    credential     jsonb NOT NULL,
    name           text NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_used_at   timestamptz
);
CREATE INDEX webauthn_credentials_user_idx ON webauthn_credentials (user_id);

-- Short-lived steps of a multi-step login: the second factor, OIDC state, WebAuthn challenges.
CREATE TABLE auth_challenges (
    token_hash  bytea PRIMARY KEY,
    kind        text NOT NULL,
    user_id     uuid,
    data        jsonb NOT NULL DEFAULT '{}',
    attempts    integer NOT NULL DEFAULT 0,
    expires_at  timestamptz NOT NULL
);
CREATE INDEX auth_challenges_expires_idx ON auth_challenges (expires_at);

CREATE TABLE scim_tokens (
    id            uuid PRIMARY KEY,
    name          text NOT NULL,
    token_hash    bytea NOT NULL UNIQUE,
    created_by    uuid NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_used_at  timestamptz,
    revoked_at    timestamptz
);

CREATE TABLE nodes (
    id             uuid PRIMARY KEY,
    role           text NOT NULL,
    version        text NOT NULL,
    max_migration  integer NOT NULL,
    started_at     timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX nodes_last_seen_idx ON nodes (last_seen_at);

CREATE TABLE audit_checkpoints (
    seq         bigint PRIMARY KEY,
    hash        bytea NOT NULL,
    signature   bytea NOT NULL,
    public_key  bytea NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
