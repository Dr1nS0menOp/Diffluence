# Identity providers: OIDC login and SCIM provisioning

Written for: operators.

## Authentik

Verified on 2026-11-04 against real Authentik 2026.8.5, with a real browser going through its login screens. `scripts/idp-test/run.sh` repeats that proof.

**OIDC.** Create an OAuth2/OpenID provider and an application.

- Client type: confidential
- Redirect URI (strict): `https://wiki.example.com/api/v1/auth/oidc/callback`
- Scopes: the default `openid`, `email` and `profile` mappings
- Subject mode: **based on the user's hashed ID** (the default). Linking to SCIM accounts depends on it.
- **Grant types: enable `authorization_code`.** Without it Authentik answers "the request is otherwise malformed", and the audit log shows `provider_error:invalid_request`.

```sh
DIFFLUENCE_OIDC_ISSUER=https://auth.example.com/application/o/<slug>/
DIFFLUENCE_OIDC_CLIENT_ID=...
DIFFLUENCE_OIDC_CLIENT_SECRET_FILE=/run/secrets/oidc_client_secret
DIFFLUENCE_OIDC_DISPLAY_NAME=Authentik
DIFFLUENCE_OIDC_GROUPS_CLAIM=groups        # optional
```

**SCIM.** In Diffluence: Administration, Provisioning, New token. In Authentik: create a SCIM provider with URL `https://wiki.example.com/scim/v2` and that token, using the default user and group mappings, then add it as a backchannel provider of the application. Users and groups arrive automatically.

Authentik sets `externalId` to the same value as the OIDC subject. That is how an account created through SCIM is linked at first login, without trusting an e-mail address. Authentik does not mark addresses as verified by default, so linking on e-mail would not work there anyway.

Choose either SCIM or the `groups` claim for groups, not both. A claim never changes a group that SCIM manages.

## Entra ID

**Not verified against a real Entra ID tenant.** The tests send the request shapes from Microsoft's public SCIM documentation (`Replace` with a capital, booleans as strings, the `id eq ... and members.value eq ...` check). Expect to find something when you first connect it, and report it.

- OIDC: register an app, redirect URI as above, issuer `https://login.microsoftonline.com/<tenant>/v2.0`
- SCIM: Enterprise application, Provisioning, Tenant URL `https://wiki.example.com/scim/v2`, Secret Token from the admin console
- **Map `externalId` to `objectId`** in the attribute mappings. The default is `mailNickname`. Entra's token carries the object id in the `oid` claim, and that is what links the accounts.

## Keycloak and others

Any provider with discovery, the code flow and PKCE should work. For a groups claim, add a group membership mapper. If the provider shares no id between SCIM and OIDC, accounts link on an address it marks as verified.

## Rules that hold for every provider

- Accounts link by provider and subject. An e-mail address alone never logs anyone in.
- A local account with a password is never taken over: that login is refused.
- Disabling a user ends their sessions at once, for every login method.
- Deleting a user through SCIM disables the account. Their content and audit trail stay.
- Groups from SCIM or OIDC are read-only in the admin console.
- If a provider removes the last administrator, `diffluence user create-admin` is the way back.
