# 04.5: The API and its edge

## 5.1 Conventions

- **ARC-127** · built · REST with JSON under `/api/v1`. Times are RFC 3339, ids are UUIDv7.
- **ARC-128** · built · An error is `{"error":{"code","message"}}`. The code is for programs and the message for people. A message never holds internal details.
- **ARC-129** · built · A page or space a person may not see answers 404, not 403, so nobody can test what exists (NAV-005).
- **ARC-130** · built · A JSON body is decoded strictly: known fields only, one object, a size limit. 64 KiB for small bodies and 6 MiB for documents.
- **ARC-131** · built · Publishing uses `base_version`, so two people cannot overwrite each other without being told.
- **ARC-132** · built · A toggle is `PUT` and `DELETE` on a sub-resource, such as `/favourite`, so repeating it is harmless.
- **ARC-133** · built · Lists use `limit` and `offset`. That is fine for a directory. A list that grows and moves, such as the feed, notifications or the audit log, uses a cursor. The audit log pages by `before_seq`. The feed and notifications page by `before`, the time of the last item, and `before_id`, its id, so two items with the same time are never skipped; `before` alone still works as it did (spec 032).
- **ARC-134** · built · A total is capped when counting costs more than it tells: search stops at "1,000 or more" (spec 011 D21).
- **ARC-135** · built · `/api/v1` only grows. A removal needs a new version, and a `Sunset` header for at least 12 months (spec 015 D21).
- **ARC-136** · built · `docs/api/openapi.json` is written from a table next to the routes, and a test fails on any difference (spec 015 D19). "Spec-first with a generated client", the old plan, did not happen, and a generator would be a dependency.

## 5.2 Access rules

- **ARC-137** · built · Every route is registered with an `Access` value, and a route without one panics at start. Public routes carry a comment that says why.
- **ARC-138** · built · The kinds are `Public`, `Authenticated`, `Permission`, `Object` and `ScimToken`. All routes and rules are listed in `docs/api/routes.txt`, and a test fails on a difference.
- **ARC-139** · built · `TestEveryObjectRouteIsProtected` walks the real route table: a stranger gets 404, a viewer gets 403, so a new route is covered from its first line.
- **ARC-140** · built · `Sensitive` asks for a fresh login after 10 minutes, and `SessionOnly` keeps tokens away from passwords, tokens and the console (spec 013).
- **ARC-141** · built · Maintenance mode is a check in the middleware, because it concerns every write, also from SCIM and jobs (spec 013 D17).
- **ARC-142** · built · Tokens have scopes: `read`, `write`, `export`, `admin`. A token never exceeds its owner, because the module's checks still run.
- **ARC-143** · phase 7 · OAuth for the API waits for the platform phase (ADR-026). Tokens with scopes cover what is needed until then.

**Public routes, all of them, now and planned:** the app itself, login with its steps and providers, the OIDC start and callback, the look of the login page (`/appearance` and its font), `/livez`, `/readyz`, `POST /mail/unsubscribe/{token}` (wave C) and `POST /hooks/{token}` (wave H). In each of the last two the token is the permission. Nothing else may be added without a stated reason.

## 5.3 The order of the middleware

```
request id → client address → log and recover → security headers
    → cross-origin protection → session → route access rule → handler
```

- **ARC-144** · built · A request id ties log lines together. A log holds the path, never the query string, which may hold tokens.
- **ARC-145** · built · Go's `CrossOriginProtection` stops cross-site writes through `Sec-Fetch-Site` and `Origin`, with our public origin as the trusted one. GET, HEAD and OPTIONS pass, so a GET must never change anything. That is why unsubscribe uses POST (spec 010 D15).
- **ARC-146** · built · A request without those headers counts as not from a browser, and passes. Tokens and SCIM rely on this. Cookies are the only thing a browser sends by itself, and the session cookie is `SameSite=Strict`, with `__Host-` where TLS is on.
- **ARC-147** · built · A panic becomes a 500 and a log line with a stack. One bad request cannot stop a node.
- **ARC-148** · built · Polling is marked as background, so it does not count as activity for the idle timeout. Otherwise an open tab keeps a session alive for ever (spec 013 D8).

## 5.4 Headers

- **ARC-149** · built · The API answers with a content policy that forbids everything, `nosniff`, `X-Frame-Options: DENY`, `no-referrer`, `Cross-Origin-Opener-Policy` and `Cross-Origin-Resource-Policy: same-origin`, `no-store`, and HSTS with TLS.
- **ARC-150** · built · The app has its own policy: everything from our origin, no inline script, and Trusted Types with one allowed policy for the editor's clipboard.
- **ARC-151** · built · A stored file is served with `default-src 'none'; sandbox`, and only known image types show inline. All else is a download, so a file can never run in our origin.
- **ARC-152** · built · Video and audio join from a short list of detected types, with the same headers (spec 012 D16). PDF stays out of that list, because it is scriptable, and goes through ARC-097.
- **ARC-153** · built · Time limits per request: 10 seconds for headers, 60 for reading, 120 for writing. Uploads and exports extend their own with `ResponseController`. Such an extension must happen before the old deadline passes, because a passed deadline cannot be extended.

## 5.5 Limits

- **ARC-154** · built · Logins are limited per address, per account and address, and per account from all addresses together, at a higher number, so that nobody can lock a person out by guessing wrong on purpose (spec 025).
- **ARC-155** · built · Tokens are limited per minute. A public hook has its own limit.
- **ARC-156** · built · The reverse proxy limits requests per address, so a flood stops before the application. The application's own limits are for abuse by someone logged in. Built as advice in `docs/operations/install.md`.
- **ARC-157** · built · Sizes are limited at each door: document 5 MiB, depth 100, title 255, upload by setting.
