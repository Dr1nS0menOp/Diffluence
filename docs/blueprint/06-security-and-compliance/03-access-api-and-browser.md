# 06.3: Access rules, the API and the browser

## 3.1 Access rules

**What can go wrong:** this is the most common serious bug in a wiki. A list shows a page that a direct read refuses, a new route forgets its check, an id from another space is accepted, a page loses a restriction on its way through the trash.

- **SEC-200** · built · Every route is registered through `httpapi.Router` with an explicit rule, and a route without one stops the server from starting. `docs/api/routes.txt` shows them all, and a test fails when it differs.
- **SEC-201** · built · Modules decide, not handlers, so no caller can skip a check (ASVS 8.3.1).
- **SEC-202** · built · A route walker tries every object route as a stranger and as a viewer. A stranger gets 404, a viewer 403 (ASVS 8.2.1, 8.2.2).
- **SEC-203** · built · Everything that lists content goes through `perm_visible_pages`: tree, search, favourites, recent lists, links, directory counts, exports. The matrix test covers 102,960 cases, and compares Go with SQL.
- **SEC-204** · built · A page someone may not see behaves as if it does not exist. "Not found" comes first.
- **SEC-205** · built · A change to a permission counts at once, because rules are read on each request (ASVS 8.3.2).
- **SEC-206** · built · Moves, the trash and purges cannot lose a restriction: a move that would drop one needs the permission, a restore waits for its parent, and a purge takes everything below.
- **SEC-207** · built · JSON bodies refuse unknown fields, and handlers read typed structs, so nobody can set a field they should not (ASVS 15.3.3).
- **SEC-208** · built · Answers use typed structs with chosen fields, not whole rows (ASVS 15.3.1).
- **SEC-209** · built · Administrators have no content access by default. Recover access needs a reason and is audited.
- **SEC-210** · partly · The rules are in `05-data-and-permissions.md` (ASVS 8.1.1), and rules per field are not written down (ASVS 8.1.2). The access table in wave F should cover them.
- **SEC-211** · built · People directories limit what a guest can see, and a person can hide their address. `GET /directory` hands out addresses to everyone with *view profiles* today.
- **SEC-212** · built · Bulk actions check every page, and report what was skipped. A bulk action must never act on a page the person cannot see.

## 3.2 The API

**What can go wrong:** injection, a flood of requests, a token with more power than it needs, an error that tells too much.

- **SEC-220** · built · SQL is parameterised everywhere, with fixed statements. Sort orders use `CASE` on a checked value, not string building (ASVS 1.2.4). The blueprint named sqlc, and hand-written pgx took its place.
- **SEC-221** · built · The search language has no way in: every term is reduced to letters and digits before it reaches `to_tsquery`.
- **SEC-222** · wave D · The richer search language of wave D must keep to this rule: a query that cannot be understood falls back to plain words, and never reaches PostgreSQL as syntax (ORG-032).
- **SEC-223** · built · Bodies are limited per route, a content type is required, and exactly one JSON value is accepted.
- **SEC-224** · built · Errors say what a person can do about them. Internal details stay in the log, with a request id to find them (ASVS 16.5.1).
- **SEC-225** · built · There is no CORS: no cross-origin caller is allowed, so nothing can be misconfigured (ASVS 3.4.2).
- **SEC-226** · built · Only the methods of a route work, because the router matches on them. `TRACE` has no route (ASVS 4.1.4, 13.4.4).
- **SEC-227** · built · Personal API tokens have a name, an end date and a limited scope, are stored hashed, and are shown once (ACC-019).
- **SEC-228** · built · Routes that manage tokens, sessions or factors work with a session only, never with a token (`SessionOnly`). A stolen token cannot make a new one.
- **SEC-229** · built · Scopes `read`, `write`, `export` and `admin` are checked in the middleware, the module's checks still apply, and a token never exceeds its owner.
- **SEC-230** · built · The audit log says which token made a call. `audit.Event` knows user, system and anonymous, so it needs a field for this. Spec 015 does not name it.
- **SEC-231** · new · Medium. Only login has a limit. Any other route can be called as fast as a person likes, and search and exports are the expensive ones. Limits per person and per token belong in phase 3 (ASVS 2.4.1, 15.2.2). Spec 015 limits tokens, and sessions need the same.
- **SEC-232** · partly · Every refused request writes an audit event, so a flood of refusals can fill the log and slow writing. It was accepted for now in the threat model. It needs the limit of SEC-231.
- **SEC-233** · built · An OpenAPI description matches the routes, and every entry has its access rule.
- **SEC-234** · later · Full OAuth for apps stays in phase 7, by the product owner's choice. Tokens with scopes serve scripts until then.

## 3.3 Requests from other sites

**What can go wrong:** a page elsewhere makes a logged-in browser change something here.

- **SEC-240** · built · Go's cross-origin protection refuses unsafe methods from other origins, using `Sec-Fetch-Site`, with the `Origin` header as a fallback (ASVS 3.5.1, 3.5.3). The blueprint named a CSRF token, and this replaced it.
- **SEC-241** · built · The session cookie is `SameSite=Strict`, so it does not travel on requests from another site.
- **SEC-242** · built · The protection only covers unsafe methods, so nothing may change on `GET`. Two `GET` routes have small effects: reading a page updates the person's own recent list, and `?download=1` writes an audit event. Neither gives an attacker anything, and strict cookies do not travel cross-site.
- **SEC-243** · new · A test that no new `GET` route writes to the database. Low. Belongs next to the route walker.
- **SEC-244** · built · Public routes for incoming hooks and one-click unsubscribe have no session, so the protection does not matter to them. Their token is the proof, and they can only do one small thing.

## 3.4 What the browser is told

**What can go wrong:** a script from a stored document, the app in a frame on another site, a page left in a cache.

- **SEC-250** · built · The app's policy allows scripts and styles from our own origin only, no inline script, no `eval`, no frames, no plugins, and `frame-ancestors 'none'` (ASVS 3.4.3, 3.4.6).
- **SEC-251** · built · Trusted Types are required, so a string cannot become HTML by accident. Only the editor's own clipboard policy exists.
- **SEC-252** · built · Every browser test fails on a policy violation, and a test proves that an inline script cannot run.
- **SEC-253** · built · API answers forbid everything with `default-src 'none'`, and carry `nosniff`, `no-referrer`, `Cross-Origin-Opener-Policy`, `Cross-Origin-Resource-Policy` and `no-store` (ASVS 3.4.4, 3.4.5, 3.4.8, 14.3.2).
- **SEC-254** · built · `style-src-attr 'unsafe-inline'` is the one loosening. The editor sets column widths that way, and a style attribute cannot run code. `img-src` allows `data:` for the QR code of TOTP. Documents cannot use it, because images must be attachments.
- **SEC-255** · partly · The policy has no reporting address (ASVS 3.4.7, Level 3). A violation in the field goes unseen. Add `report-to` with a small limited route. Low.
- **SEC-256** · partly · HSTS is sent when the public address is https (ASVS 3.4.1). See SEC-257.
- **SEC-257** · new · Medium. Cookies only get `Secure`, the `__Host-` prefix and HSTS when `DIFFLUENCE_PUBLIC_URL` starts with https, and the reference `compose.yaml` defaults to `http://localhost:8080`. An operator who puts a TLS proxy in front and forgets the address gets weak cookies, without any warning. Log a loud warning at start, and show it in the admin console. Belongs in wave F.
- **SEC-258** · built · The app never uses `dangerouslySetInnerHTML`, and text reaches the screen through React or the editor, so a search snippet is text with markers (ASVS 3.2.2).
- **SEC-259** · built · `frame-src` stays `'none'` unless an administrator sets up a service. Then only that address is allowed. External embeds stay out (BLK-068).
- **SEC-260** · wave H · `connect-src` gets the `wss:` address of the collab service only when that service is on.

## 3.5 What stays in the browser

**What can go wrong:** the next person on a shared computer sees what the last one searched for.

- **SEC-265** · built · **High.** ASVS 14.3.1 (Level 1) asks to clear authenticated data from the browser when a session ends, and 14.3.3 asks to keep sensitive data out of browser storage. Wave B keeps recent searches in `localStorage` under `diffluence.searches`, and the tree keeps page ids per space. Logout only clears the query cache, so the next person sees the last person's searches. Store these per person, and remove them at logout. Belongs before wave C lands.
- **SEC-266** · new · Wave C keeps comment drafts in the browser (COL-011), and PAG-016 keeps page text there when saving fails. That is Content in storage on a device. Key it per person, remove it at logout, and keep it for a short time. Spec 010 should say so.
- **SEC-267** · built · The theme, the sidebar state and the scroll position hold no content.
- **SEC-268** · built · The query cache is cleared at logout.

## 3.6 Too much of everything

**What can go wrong:** one person or one file makes the server slow for everyone.

- **SEC-270** · built · Documents are limited to 5 MiB and a depth of 100, bodies are limited per route, and uploads have a configurable limit.
- **SEC-271** · built · Server timeouts are set for reading headers, reading and writing. Only uploads and exports get longer.
- **SEC-272** · built · Imports run one at a time in the worker, with a limit on size, files, unpacked size and time.
- **SEC-273** · new · There is no quota per person or space, so one person can fill the disk (ASVS 5.2.4, Level 3). Files can be counted already, and FIL requirements should say what happens at the limit. Low. Belongs in wave E.
- **SEC-274** · partly · ASVS 15.1.3 asks to write down which functions are heavy and how they are protected. Today: hashing, search, exports, imports, the audit chain's lock. The load test of phase 3 should measure them.
- **SEC-275** · wave E · Dynamic blocks run queries when a page is read. Each needs a limit, and a block that fails or runs too long must leave the page working. A page full of reports must not be a denial of service.
