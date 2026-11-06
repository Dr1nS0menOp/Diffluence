# The Diffluence API

Our own web app uses the same API as everyone else. The routes and what each needs are in [`routes.txt`](routes.txt), and the same list is an OpenAPI 3.1 description in [`openapi.json`](openapi.json), also at `/api/v1/openapi.json` for anyone logged in. A test fails when either differs from the routes.

The description covers each route, its method, its path parameters and its rule, as `x-access`. It does not describe every field: those are checked by hand.

## What stays stable

- `/api/v1` only grows. New routes and new fields can appear, so ignore fields you do not know.
- Nothing in `/api/v1` changes meaning or goes away without a new version.
- A removal is announced at least 12 months ahead with a `Sunset` header (RFC 8594) on the route, and in the changelog.

## Tokens

A personal access token goes in `Authorization: Bearer dfl_...`. It has scopes:

| Scope | Allows |
|---|---|
| `read` | Reading: `GET`, `HEAD` and `OPTIONS` |
| `write` | Everything that changes something |
| `export` | Routes that take content out, such as `export.zip` |
| `admin` | Routes under `/api/v1/admin/` |

A scope only limits a token. A token is its owner, so every check on the owner still applies, and a route marked `session-only` never takes a token.

## Limits

One token makes at most 600 calls a minute, and then gets `429`. Refusals because of a scope are in the audit log, and so is a token's use, at most once a minute.

## Errors

An error is `{"error": {"code": "...", "message": "..."}}`. The code is stable, and the message is for people.

## Webhooks

A webhook follows the Standard Webhooks format: the headers `webhook-id`, `webhook-timestamp` and `webhook-signature`, and an HMAC-SHA256 over `id.timestamp.body`. During a change of secret, the signature header holds two values. The id stays the same across attempts, so you can drop a double. The body holds ids, a type and a time, and a title only if the endpoint asked for it. Page text is never sent.

## The command line tool

`diffluence client` uses only this API. Set `DIFFLUENCE_URL`, and `DIFFLUENCE_TOKEN` or `DIFFLUENCE_TOKEN_FILE`.

| Command | What it does |
|---|---|
| `diffluence client spaces list` | The spaces you can see, as JSON |
| `diffluence client pages list <space-id>` | The pages of a space, as JSON |
| `diffluence client export <space-id> <file>` | Saves a space as a zip |
| `diffluence client import <space-id> <file>` | Imports a file into a space |
| `diffluence client backup-content <folder>` | Saves every space you can see |

Exit codes: 0 done, 1 failed, 2 wrong use, 3 refused by the server.
