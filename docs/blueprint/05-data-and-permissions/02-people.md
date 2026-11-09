# 05.2: People and identity

Owner of these tables: `internal/identity`, unless a line says otherwise.

## 2.1 The instance

- **DAT-020** · built · `instance`: `id`, `name`, `created_at`, and `singleton`, which allows exactly one row. Made at the first start. Never deleted. A bundle carries its id as the origin.

## 2.2 Users

- **DAT-021** · built · `users`: `id`, `origin_instance_id`, `type` (`member`, `guest`, `service`), `status` (`active`, `disabled`), `email` (unique ignoring case), `display_name`, `username`, `source` (`local`, `oidc`, `scim`), `external_id`, `created_at`, `updated_at`.
  - **Life:** created by the command line, an administrator, SCIM or single sign-on. Disabled by an administrator or SCIM. A disabled person keeps their content, and every session ends.
  - **Never deleted.** That keeps every author id resolvable (DAT-004).
  - **Export:** a bundle holds the name and address of people the content refers to, and nothing else (wave G).
- **DAT-022** · built · `users` gains `expires_at`, `last_login_at`, `anonymised_at`. From `expires_at` the account counts as not active, by rule, in Go and in SQL.
- **DAT-023** · built · **Anonymising:** name becomes `Former user` and a short code, address becomes one that cannot receive mail, `username` and `external_id` are emptied, status becomes disabled. See 05.1, section 1.4, for what it reaches.
- **DAT-024** · built · `user_profiles`: job title, department, location, about, picture key, field visibility, locked fields. Cascades with the user. The picture is re-encoded to a 256 pixel PNG, so it holds no metadata.
- **DAT-025** · built · `user_preferences`: `user_id`, `preferences` (JSON), `updated_at`. Personal, not audited. Cascades. Unknown keys are refused at the door.
- **DAT-026** · built · `user_preferences.notifications`: per type, digest, quiet hours, pause (`TestPreferencesDecideWhenMailGoes`).

## 2.3 Ways to log in

| Number | Status | Table | Holds | Deletion |
|---|---|---|---|---|
| **DAT-027** | built | `local_credentials` | `password_hash` (Argon2id) | Cascade, and on anonymise |
| **DAT-028** | built | `user_identities` | `provider`, `subject`, `user_id`, `last_login_at`. Never matched on e-mail | Cascade, and on anonymise |
| **DAT-029** | built | `totp_credentials` | `secret_sealed`, `confirmed_at`, `last_step` | Reset by an administrator, cascade |
| **DAT-030** | built | `recovery_codes` | `code_hash`, `used_at`. Ten, each usable once | With the second factor |
| **DAT-031** | built | `webauthn_credentials` | `credential_id`, `credential`, `name`, `last_used_at` | By the person, cascade |
| **DAT-032** | built | `sessions` | `token_hash`, `user_id`, times, `ip`, `user_agent` | Logout, expiry, password change, disabling |
| **DAT-033** | built | `auth_challenges` | `token_hash`, `kind`, `user_id`, `data`, `attempts`, `expires_at`. Reset, invitation and sign-up links reuse it in wave F | Expire |
| **DAT-034** | built | `scim_tokens` | `name`, `token_hash`, `created_by`, `last_used_at`, `revoked_at` | Revoked, kept as a record |
| **DAT-035** | built | `api_tokens` | Name, hash, scope, expiry at most a year, last use. Prefix `dfl_` | Revoked, expired, on anonymise |
| **DAT-036** | built | `login_history`, `known_devices` | Last 20 logins with time, method, address and device. No place (ADR-026) | Rolling, on anonymise |
| **DAT-037** | built | `sessions.reauth_at`, `sessions.needs_second_factor` | Step-up and a forced second factor | With the session |

## 2.4 Groups

- **DAT-038** · built · `groups`: `id`, `origin_instance_id`, `name` (unique ignoring case), `description`, `source` (`local`, `scim`, `oidc`), `external_id`, times. A group from SCIM or single sign-on is read-only in the screens.
  - **Deleting:** global and space grants of the group go. Entries on page restrictions stay on purpose, so a page restricted to only this group stays closed (PRM-052).
- **DAT-039** · built · `group_members`: `group_id`, `user_id`, `created_at`. Cascades from both sides.
- **DAT-040** · built · `group_owners`: people who manage members from outside the console. A group from SCIM stays read-only for them too.
- **DAT-041** · built · `permission_sets`: named sets that only fill in boxes. Deleting one changes no grant.

## 2.5 Grants at the global layer

- **DAT-042** · built · `global_grants`: `subject_type` (`user`, `group`), `subject_id`, `permission`, `created_at`. No foreign key: `DeleteGroup` removes rows by hand. See PRM-001 to PRM-006.

## 2.6 Following

- **DAT-043** · built · `follows`: `user_id`, `followed_id`, `created_at`. Personal, not audited. It only feeds For you, which only shows what the reader may see. Who may be followed follows PRM-083 (GAP-030).
