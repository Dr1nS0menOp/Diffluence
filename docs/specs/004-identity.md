# 004: Identity: TOTP, OIDC, passkeys, groups, guests, SCIM

Status: built, waiting for the product owner's acceptance session (written by Claude from the blueprint)
Phase: 1 (TOTP, OIDC) and 2 (groups, guests, SCIM, passkeys)

## Problem
Organisations log in through their identity provider and manage people there. Local accounts must still be strong for small installs.

## Behaviour

**Password and sessions.** Users change their own password with their current one. A change ends all their other sessions. Administrators can disable users, which ends their sessions at once. Users can list and end their own sessions.

**TOTP.** A user with a local password can add an authenticator app: scan a code, confirm with a first code, receive 10 single-use recovery codes. From then on login takes two steps. A code cannot be used twice. Seeds are encrypted with AES-256-GCM. Recovery codes are stored as hashes.

**Passkeys.** Users can register passkeys and log in with one instead of a password and code. They can list and remove them.

**OIDC.** Configured by environment variables, secret through a file. Authorization code flow with PKCE, state and nonce. On first login a user is created when automatic creation is on, and linked by provider and subject, never by e-mail alone. An existing local account with the same e-mail is **not** taken over: the login is refused, because a provider may let users choose their e-mail.

An account the provider created through SCIM is linked on first login by the stable id both protocols share: SCIM's `externalId` equals the token's `sub` (Authentik) or `oid` (Entra ID, when `externalId` is mapped to `objectId`). Providers without a shared id can link on an e-mail address they mark as verified. An account with a password is never linked. A `groups` claim can keep group membership in step.

**Groups.** Administrators manage local groups. Groups from SCIM or OIDC are read-only in the UI. Groups can hold global and space permissions and appear on restrictions.

**Guests.** A guest is a user type. Guests can be granted any space, are never in "all members", and cannot browse the user list unless granted *view profiles*.

**SCIM 2.0.** `/scim/v2` serves Users, Groups, ServiceProviderConfig, ResourceTypes and Schemas for provisioning by Authentik, Entra ID and others, including filter `userName eq` and `displayName eq`, PATCH with add, replace and remove, and pagination. Authentication uses bearer tokens created in the admin console, shown once and stored hashed. Deactivating a user or deleting them through SCIM disables the account and ends its sessions. Content keeps its author, so it is never deleted.

## Permissions and audit
Self-service: authenticated. Everything else: *administrator*. Events: `auth.login|logout|login_mfa`, `auth.password_changed`, `auth.totp_enabled|disabled`, `auth.recovery_code_used`, `auth.passkey_added|removed`, `auth.sessions_revoked`, `user.created|updated|disabled|enabled`, `group.created|updated|deleted`, `group.member_added|removed`, `scim.token_created|revoked`. SCIM-driven changes use the actor type `system` with the token id.

## Acceptance criteria
- [x] A wrong TOTP code is refused and rate limited; a used code cannot be replayed; a recovery code works once
- [x] The password step alone never yields a session when TOTP is on
- [x] An OIDC login with bad state, bad nonce or a bad signature is refused
- [x] OIDC does not take over a local account with the same e-mail
- [x] A user created through SCIM can log in over OIDC, also when the provider does not mark the address as verified
- [x] Someone who sets their address at the provider to a colleague's address cannot log in as that colleague
- [x] A disabled user is logged out at once and cannot log in by any method
- [x] SCIM without a valid token gets 401; a revoked token stops working at once
- [x] Create, update, deactivate, group membership work with the request shapes Authentik and Entra ID send
- [x] A group from SCIM cannot be changed in the UI

## How the product owner tests it
Turn on TOTP and log in again. Connect Authentik for OIDC and for SCIM, create a user and a group there, and see them arrive. Disable the user in Authentik and confirm the session ends.

## Provenance
Own design. Public standards: RFC 6238 (TOTP), OpenID Connect Core, RFC 7636 (PKCE), W3C WebAuthn, RFC 7643 and 7644 (SCIM).
