# 015: Wave H, insight, automation, the platform, and what stays later

Status: built on branch `wave-h`, see `docs/handoff/2026-11-06-wave-h.md`
Phase: wave H of `docs/blueprint/03-functional-requirements.md`, covering chapters 12 and 13 and editing together from chapter 7.13. The product owner asked on 2026-11-05 to build the whole of blueprint 03, one commit per wave. This is the last wave, and it depends on the events, notifications and mail of wave C.

## Problem

Space administrators cannot tell what is read, and what is out of date. Repeating work stays manual: nobody can say "remind the owner after a year" once and be done. Outside systems have no stable way in. Two people on one page still get a conflict dialog.

Chapters 12 and 13 also hold things that should not be built now. This spec says which, and why, so that "the whole of 03" means something honest.

## What this wave does not build

Status **later** and the phases after 1.0 are in the blueprint for a reason. The request was "the whole of 03", so this list needs a second look.

| Requirements | Roadmap | Why not now | What it would take |
|---|---|---|---|
| PLT-042 to 048, AI | Phase 6, after 1.0 | The roadmap puts it after the first stable release, and it is always optional. It needs a provider choice, `pgvector` next to PostgreSQL, and an audit of AI actions. None of the core depends on it. | Its own study, and the product owner's choice of providers |
| PLT-049 and 050, linked instances | Phase 8, after 1.0 | It needs mutual trust between instances, and merging people and history. The bundle of wave G and our stable ids lay the ground. | A trust protocol, and a design for conflicts |
| PLT-037, the OAuth part | Phase 7, after 1.0 | Running an authorization server is security critical, so it must be a widely used module, which needs the product owner's approval. Scoped tokens serve scripts today. | See question 3 |
| PLT-039 and 041, apps and marketplace | Phase 7, after 1.0 | Apps need a separate origin, a sandbox, consent and signing. Places for apps to hook in are code for a need that does not exist yet. | Its own study |
| OTH-019, work module | Phase 9 | The chapter says its blueprint comes later. It is a second product. | A blueprint |
| OTH-021, mobile apps | Phase 10 | Other codebases and stores. The web app works on phones (OTH-020). | Its own project |
| OTH-001 to 005, live documents | Later | They need editing together first. | Part 5, then a decision |
| OTH-006 to 009, whiteboards | Later | A canvas with its own data model: a product by itself. | A study, and a drawing library |
| OTH-010 to 014, databases | Later | A new content type with typed fields, views, permissions and search. | A study |
| OTH-015 to 017, calendars | Later | Possible, with iCalendar. The chapter says none of these is decided. | The product owner's word that it matters |

OTH-018 needs nothing: presenting a page came with PAG-114, so its status can move to built. COL-081 and COL-048 stay later too.

If the product owner agrees, the 31 requirement numbers in this table stay open after this wave, plus COL-048 and COL-081. That does not count Part 5, or items marked later in other chapters.

## What the study found

**Analytics.** A page view connects a named colleague to what they read, so it is personal data. Our own rule since 2026-11-03 is that views are not in the audit log and are a feature with a switch. French guidance on audience measurement points the same way: measure for the publisher alone, reuse nothing, keep data for a limited time (13 and 25 months are the figures cited), and still inform people. No outside tool is involved here, and no cookie is added. Where pages are read by a team of ten, a small count can point at a person, which is why names are never a side effect.

**Automation.** Rule builders studied share one shape: trigger, conditions, actions, a run log, and limits so that a rule cannot run away. People want ready-made rules more than an empty form.

**Webhooks.** There is no W3C standard. The Standard Webhooks specification is the nearest common ground: headers `webhook-id`, `webhook-timestamp` and `webhook-signature`, an HMAC-SHA256 over `id.timestamp.body`, several signatures during a key change, and a tolerance of five minutes. The id stays the same across retries, so receivers can drop duplicates. The server calling addresses that people chose is a classic attack on the inside network. OWASP says: prefer an allow-list, resolve the name and connect to the checked address, follow no redirects, and check every address a name gives. Deny-lists alone get bypassed.

**The API.** RFC 9700 makes PKCE mandatory, demands exact redirect matching, and drops the implicit and password grants. The Sunset header (RFC 8594) announces when something goes away. Our route table in `docs/api/routes.txt` already holds every route with its rule, which is most of an API description.

**Spreadsheets.** A CSV cell that starts with `=`, `+`, `-`, `@`, a tab, a carriage return, a line feed or a full-width variant of them can run as a formula. OWASP advises quoting the field, doubling quotes and prefixing a single quote. Titles and search terms are written by people, so every export needs this.

**Editing together.** Yjs is a shared data type that merges by itself, MIT, with a ProseMirror binding that also brings shared cursors and an undo that stays per person. It replaces the editor's history. TipTap's own extension sits on top of it. A provider moves the updates and is separate from Yjs. `y-indexeddb` keeps work in the browser for offline use. Hocuspocus is an MIT WebSocket server for Yjs, and runs on Node.js. Go ports of Yjs exist, and are young. The stable binding uses Yjs 13, and a successor targets Yjs 14, so versions must be pinned together.

**What accessibility demands.**

| Source | Demand | What we do |
|---|---|---|
| 1.1.1 Non-text content | A chart needs a short label and a longer text with its trend. Where possible, the data goes in a table. | Every chart has a caption, a sentence with the totals and the trend, and a table in a disclosure. |
| 1.4.1 Use of colour | Two series may not differ by colour alone. | One series per chart. A second is a second chart, or a table. |
| 1.3.1, 4.1.2 Sortable tables | The sort state must be known. | A button in each header, and `aria-sort`. |
| 3.3.1, 3.3.3 Errors | A rule form says what is wrong, and how to fix it. | Each step shows its message beside the field. A rule cannot be switched on while one is open. |
| 3.3.4 Error prevention | An action on many pages can be checked first. | Test run shows what a rule would do. |
| 2.5.7 Dragging | Order of steps cannot depend on dragging. | Move up and Move down. |
| 4.1.3 Status messages | Presence and live changes must not interrupt. | Who else is here is a quiet `role="status"`, updated at most every 30 seconds. Editing together has its own table in Part 5. |
| 2.2.2 Pause, stop, hide | Moving content can be stopped. | Charts do not animate, and live cursors stop moving with `prefers-reduced-motion`. |

**Licences checked.** Yjs and its related projects MIT, `y-prosemirror` MIT, Hocuspocus MIT, as their public repositories say. TipTap's collaboration packages are in the family we use, and their licence must be read from the package itself before anything is installed: the public pages read for this study did not state it.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | Views are counted by a small request from the reading screen, not by `pages.Get` | Menus, exports and prefetches read pages too. Only a person reading counts. |
| D2 | In counts mode, a reader is an HMAC of their id with a salt for that day. The salt and the hashes are deleted when the day closes | Daily unique readers can be counted, and nothing can be traced back or restored later (PLT-003). |
| D3 | Counts mode gives unique readers per day, never over a longer period | An exact figure over a year would need an identity that lasts. The screen says what the number means. |
| D4 | Name mode stores one row per person and page, and leaving it deletes those rows in the same transaction | PLT-003. There is no way back by design. |
| D5 | *View analytics* is a new space permission, given to space administrators | PLT-007. It changes `Evaluate`, the SQL and the matrix test together. |
| D6 | Search terms are kept without people, shown from three uses up, and kept for 90 days | A term can hold a name. |
| D7 | Charts are HTML and CSS bars with design tokens | The design standard allows `svg` only for icons. Bars are enough here. |
| D8 | Every CSV goes through one writer with OWASP's guard | Once, in one place. The audit export moves to it too. |
| D9 | A durable JetStream consumer matches events to rules, and each match becomes a job | Events exist already, and jobs give retries and limits. `UNIQUE (rule, event)` stops double runs after a redelivery. |
| D10 | A rule acts through the same module methods as a person, as its account | PLT-023 then holds by construction. No rule code touches tables or skips a check. |
| D11 | Facts are loaded when the rule runs, as seen by its account | A page the account cannot see makes the run stop with "not visible". A rule cannot leak by condition. |
| D12 | The chain of rules travels with the event. A rule never runs on its own chain, and a chain stops at five | PLT-032. |
| D13 | Values are plain placeholders such as `{page.title}`, with no expressions | No template language means no injection. Values enter documents as text nodes. |
| D14 | Schedules are presets: hourly, daily, weekly, monthly, at a time | No cron text for people to get wrong. |
| D15 | One outward client, `internal/outward`, for all calls to other servers: https only, allow-listed hosts, every resolved address checked, connect to the checked address, no redirects, 10 seconds, 64 KiB read | One place for the hard part. Deliberate difference 5 in blueprint 03 stays true, because the list starts empty. |
| D16 | Webhooks are built now, although the roadmap says phase 7 | Rules need the same delivery, signing and log, and PLT-029 asks for it. |
| D17 | Webhooks follow the Standard Webhooks format | Receivers have code for it, and it needs only the standard library. |
| D18 | The incoming rule trigger is the one new public route, with a 256-bit token stored hashed | PLT-025. The token is the permission, and the route says so in a comment. |
| D19 | The API description is OpenAPI 3.1, written from a table in Go next to the routes, and a test fails on any difference | Spec-first in blueprint 04 never happened, and a generator is a dependency. |
| D20 | Scopes on tokens are few: `read`, `write`, `export`, `admin`. A token never exceeds its owner | ACC-019 asks for a limited scope. More words can follow from real use. |
| D21 | `/api/v1` only grows. A removal needs a new version and a `Sunset` header for at least 12 months | PLT-037. |
| D22 | The command line tool is a set of subcommands in the same binary, and reads its token from the environment or a file | No second binary, and no token in the process list. |
| D23 | Presence is built now by polling, with no new dependency: who is reading, and who is editing | It gives PAG-034's warning and PAG-120, whatever the product owner decides about Part 5. |
| D24 | Editing together is not started without the product owner's choice | It adds a runtime and changes what a draft is. |

## The product owner's decisions (ADR-026)

The list of what this wave does not build stands, except databases: they moved up to wave I (ADR-028). Editing together: yes, with the service from blueprint 04, so Part 5 is built. OAuth for the API stays in phase 7. Name mode stays off by default.

## For the product owner to decide

| # | Question | Options | Advice |
|---|---|---|---|
| 1 | Is the "does not build" list above right? | Yes, or name what must move up. Calendars are the smallest of them. | Keep the list. |
| 2 | Do we start editing together? | See Part 5 for what comes with it. **a.** Yes, with the service from blueprint 04. **b.** Yes, with a relay in Go. **c.** Not now: presence and the warning remain. | **a.** |
| 3 | OAuth now? | **a.** Later, in phase 7, as the roadmap says. **b.** Now, with `ory/fosite` (Apache-2.0 by our knowledge, to be confirmed), a large module. The alternative is scoped tokens only. | **a.** |
| 4 | Name mode for analytics | It watches colleagues. It stays off by default, and the setting names the risk. In the Netherlands the works council usually has a say in this. This is not legal advice. | Keep it off, and have someone check before a customer turns it on. |

## Scope

**In:** PLT-001 to PLT-036 as far as not built, PLT-037 without OAuth, PLT-038, PLT-040, PAG-034 (the warning), PAG-105, PAG-120, COL-097 by polling, SPC-019 (most viewed), ORG-027.

**Out:** the table above. **Part 5** only with the product owner's choice.

## Part 1: Analytics

### H1. Recording

- **Open:** the reading screen sends one small request, with the page it came from if any. At most one count per person and page per 30 minutes.
- **Engaged:** after 20 seconds with the tab visible, a second request marks the visit as engaged (PLT-011). The Page Visibility API decides what counts.
- **Not counted:** your own page (PLT-005), a page in an archived space, a space or instance with analytics off.
- **Failure:** a failed count changes nothing for the reader, and is not retried.
- **Days:** a periodic task closes each day: counts move to `page_view_days`, and the hashes and the salt go.
- **Retention:** one year by default, as a setting (PLT-004).

### H2. Settings

Administration, Analytics: on or off, mode, retention, with a sentence on what each mode stores. Moving to counts only asks first, says it deletes names for good, and is audited. Spaces can turn it off through their features (SPC-047). Turning it off stops recording and keeps what exists, and a separate button deletes it.

### H3. Page insights

The view count joins the byline for those who may see it (PAG-105), and opens a screen at `/pages/:id/insights`.

- **Shows:** views and readers per day, engaged against brief visits, a period of 7 days, 30 days, 3 months, this year or a range.
- **Links:** which pages bring readers here, and where readers go next, from the `from` value. Hidden pages are left out (PLT-006).
- **Name mode:** who read it, when, how often, and the last version they saw, and whether people mentioned in the page have read it (PLT-010, PLT-012).
- **Empty:** a page with no data says why. A page with analytics off says that instead.
- **Keyboard:** the period is a `select`, and the data table sits in a `details`.

### H4. Space and instance insights

`/spaces/:key/insights` and Administration, Insights.

- **Space:** popular pages, stale content with a link to the Content screen (PLT-020), and the content table with every column sortable. In name mode: active readers and contributors, and the people table.
- **Instance:** active users, views, new pages, comments and searches per day (PLT-014).
- **Search:** top searches and searches that found nothing (PLT-015, ORG-027), for administrators.
- **CSV:** every table (PLT-019). Only active items, unless asked.
- **Most viewed** joins the space overview (SPC-019).
- **Rule:** every list goes through `perm_visible_pages` for the person looking.

## Part 2: Automation

### H5. Rules

`/spaces/:key/settings/automation`, and Administration, Automation for instance rules.

- **The list:** name, trigger, state, last run, owner. Turn on or off, copy, export as JSON, import.
- **A new rule:** start from a template or from empty. Templates: remind the owner of pages not updated for a year; label new pages below a page; notify a group on a new blog post; ask for review when a status changes; tell the owner when a page gets many comments.
- **Building one:** a form in three parts: When, If, Then. Each step is a row with a picker, then the fields of that choice. Conditions group with all, any and not. Actions run in order, with Move up, Move down, and Remove.
- **Lookups and values:** a Find pages step gives later steps `{found.title}`, and a value picker inserts placeholders, so nobody has to remember them.
- **Acts as:** the owner, or a service account the person may use. It says what that account may do in the space.
- **Test run:** pick a page, and see per step what would happen, with nothing changed.
- **Keyboard and accessibility:** a form of native fields in order, with messages beside the field and a summary at the top.
- **Edge cases:** an owner who is disabled stops the rule and tells the space administrators. A label or group that is gone shows on the rule, and stops it from running. A rule needs *administer* on the space, and an instance rule needs *administrator*.

### H6. Running

- **Triggers:** PLT-025, each mapped to an event in the outbox. Waves C to F provide most. Schedules run from the periodic tasks. By hand: an entry in More, with the rule's name. Incoming webhook: a secret address per rule.
- **Conditions and actions:** PLT-026 to PLT-029, each calling the module's own method.
- **Limits:** 100 runs an hour per rule by default, as a setting. At the limit, the rule pauses and the owner is told. 50 actions per run, and 100 pages per lookup.
- **Run log:** every run with what it did, skipped and why, kept 30 days (PLT-031). A failing rule notifies its owner, once per hour at most.
- **Audit:** every action as the account with the rule's name in the details (PLT-035). `Caller` gains a field for this, and `record` passes the chain on.
- **E-mail:** only to people on the instance, through the queue of wave C.
- **Edge cases:** a page deleted before the run gives "gone" in the log. A retry after a crash does not repeat finished actions, because each step is stored.

### H7. Calling out

- **Allowed hosts:** Administration, Outgoing calls. Empty by default, so nothing leaves.
- **Webhook action:** a host from that list, a path, and a JSON body built from values. Page text only if the rule asks for it, and the form says that it leaves the instance.
- **Chat systems:** the same, with ready-made bodies (PLT-029).

## Part 3: The platform

### H8. API description

- **`docs/api/openapi.json`,** also at `/api/v1/openapi.json` for those logged in. Each route has its method, path, access rule as `x-access`, parameters and answers.
- **The test:** fails when a route has no entry, or an entry has no route.
- **A policy page:** `docs/api/README.md`: what stays stable, how a removal is announced, limits.

### H9. Tokens with scopes

Tokens from wave F gain scopes. The middleware maps a route to a scope: safe methods need `read`, others `write`, exports `export`, admin routes `admin`. The checks of the module still apply, so a token never does more than its owner. Tokens are limited per minute, and failures are audited.

### H10. Webhooks

Space settings, Webhooks, for space administrators, and for the instance in Administration.

- **Endpoint:** an address on an allowed host, the events, a secret shown once.
- **Delivery:** from the same consumer, signed by D17. Five attempts over a day, with growing waits. 2xx counts as delivered, and anything else is retried.
- **Log:** 30 days of deliveries, with status and time, and Send again. Bodies are not kept.
- **What is sent:** ids, type and time. Titles only if the endpoint opts in. Text never.
- **Permission:** an endpoint on a space gets events about pages its owner may see, checked at delivery.
- **Broken endpoints:** after 50 failures in a row it switches off, and tells its owner.
- **Secrets:** sealed by `internal/keys`. Rotation keeps two secrets for a day and sends both signatures.

### H11. Command line tool

`diffluence client export`, `import`, `backup-content`, `pages list`, and `spaces list`. `DIFFLUENCE_URL`, and the token through `DIFFLUENCE_TOKEN` or `DIFFLUENCE_TOKEN_FILE`. JSON on standard output, messages on standard error, and clear exit codes. It only uses the public API (PLT-040).

## Part 4: Presence

Every 20 seconds an open page says "reading" or "editing", and asks who else is there.

- **On a page:** small pictures by the actions, up to five, then a number (PAG-120).
- **In the editor:** a line in the strip that says who else is editing, with no block on working (PAG-034).
- **Live updates:** the same answer says when comments, labels or the status changed, and the screen refreshes those parts (COL-097).
- **Rules:** only people who can see the page, and nobody who has turned it off. An administrator can turn it off for the instance. Entries end after 60 seconds, and are not stored longer.

## Part 5: Editing together, only with the product owner's choice

COL-088 to COL-096. This part is the plan, and is not started until question 2 has an answer.

### What would be added

With option **a**, the service from blueprint 04:

| What | Where | Licence | Why |
|---|---|---|---|
| `yjs` | Browser, service | MIT | The shared document that merges |
| `@tiptap/extension-collaboration` | Browser | Read from the package first | Connects the editor to Yjs |
| `@tiptap/extension-collaboration-caret` | Browser | Read from the package first | Cursors with names |
| `@tiptap/y-tiptap` | Browser | Read from the package first | The binding TipTap uses |
| `@hocuspocus/provider` | Browser | MIT, confirm per package | Talks to the service |
| `y-indexeddb` | Browser | MIT | Keeps typing safe offline (COL-091, PAG-016) |
| `@hocuspocus/server` | Service | MIT | The WebSocket server |
| Node.js | Service | Its own permissive licence | A second runtime, next to Go |
| `y-protocols`, `lib0` | Come along | MIT | Used by the above |

Also: a new `collab` container and systemd unit, a WebSocket path in every reverse proxy guide, `connect-src` for `wss:` in our Content Security Policy, Node.js security updates from then on, and a row for each in `docs/dependencies.md`. For more than one service node, a Redis extension or sticky routing by page, which phase 3 must settle.

Option **b** replaces the last three rows with one Go module (`coder/websocket`, ISC, or `gorilla/websocket`, BSD-2) and a relay of our own in the Go binary. Clients do all the merging, and the server stores updates it cannot read. There is no second runtime, but the safety of the text then rests on our own storage and compaction, and COL-090 promises no lost text. The Go ports of Yjs are too young to trust with that.

Option **c** adds nothing. Part 4 is what remains, and the conflict dialog stays.

### What it changes

| Now | Then |
|---|---|
| A draft is private, one per person (PAG-013) | One shared draft per page (COL-092). Everyone with edit rights sees it while it is typed. **This changes a promise, and needs a decision of its own** |
| Autosave to `drafts` | The service saves. An existing private draft is offered once, when the shared one starts |
| ProseMirror history, and `closeHistory` in block commands (wave A) | Yjs undo, per person. Block commands call `stopCapturing` instead |
| Title in a field | Title in the shared document |
| The version names one author | The version names everyone who took part: `page_version_authors` |
| The conflict dialog | Goes away for pages in shared mode |

### What stays

- The service has no business logic and never writes a version. Publishing stays in Go, and every document passes `document.Normalize`.
- The service asks the API for every connection, with the person's session. It asks again each minute, and on a change of permissions it closes the connection.
- A person with view rights only cannot connect.

### Keyboard and accessibility

| Source | Demand | What we do |
|---|---|---|
| 4.1.3 Status messages | Others joining must not interrupt. | One quiet line, at most every 30 seconds |
| 1.4.1 Use of colour | A cursor is more than a colour. | A name label on every remote cursor, in text |
| 2.2.2 Pause, stop, hide | Moving cursors can be calmed. | With reduced motion, name labels stay still and fade |
| 2.4.3 Focus order | A remote change must never move your focus. | The binding keeps the local selection, and a test proves it |

### Plan

1. Read the licences, pin versions, and add the packages after the product owner's approval.
2. The service, authorize route, and permission checks.
3. Editor on the shared document. Undo, block commands, title.
4. Cursors, pictures, jump to a person, invite (COL-089, COL-093, COL-094).
5. Offline and return (COL-091, COL-095).
6. Publish with several authors.
7. Load test with 30 editors (COL-096).

### Tests

Two browsers in one line. A dropped connection with typing on both sides. A person whose rights end while typing. A reader refused. Undo only takes back your own work. A block move with a colleague typing. Publish passes the gate after a hostile client wrote into the shared document. An upgrade of the service while people type.

### Acceptance criteria, if approved

- [ ] AC-E1 Two people type in the same line, and nothing is lost
- [ ] AC-E2 Each sees the other's cursor with a name
- [ ] AC-E3 Typing offline merges on return, also after a reload
- [ ] AC-E4 A person who loses edit rights is removed within a minute, and at once after a permission change
- [ ] AC-E5 Undo only takes back your own work
- [ ] AC-E6 A published version names everyone who took part, and passed the document gate
- [ ] AC-E7 30 people can edit one page

## API

| Route | Rule | What |
|---|---|---|
| `POST /pages/{id}/views`, `PATCH /pages/{id}/views` | `page.view` | Count a visit, mark it engaged |
| `GET /pages/{id}/insights` | `analytics.view` | Page insights |
| `GET /spaces/{id}/insights`, `/insights/content`, `/insights/people` | `analytics.view` | Space insights |
| `GET /admin/insights`, `/admin/insights/search` | permission `administrator` | Instance and search insights |
| `GET, PUT /admin/analytics` | permission `administrator` | Mode, retention, switch |
| `DELETE /admin/analytics/data` | permission `administrator` | Remove view data |
| `GET, POST /spaces/{id}/rules` | `space.administer` | Rules of a space |
| `GET, PUT, DELETE /rules/{id}` | `space.administer`, or `administrator` for an instance rule | One rule |
| `POST /rules/{id}/test` | same | Test run |
| `POST /rules/{id}/run` | same, or `page.view` for a rule with a manual trigger | Run by hand |
| `GET /rules/{id}/runs` | same | Run log |
| `GET, POST /admin/rules` | permission `administrator` | Instance rules |
| `POST /hooks/{token}` | **public**, justified in the code: the token is the permission | Incoming trigger |
| `GET, PUT /admin/outgoing-hosts` | permission `administrator` | Allowed hosts |
| `GET, POST /spaces/{id}/webhooks` | `space.administer` | Webhooks of a space |
| `GET, PATCH, DELETE /webhooks/{id}` | `space.administer` | One webhook |
| `GET /webhooks/{id}/deliveries`, `POST .../{delivery}/resend` | `space.administer` | Log, Send again |
| `POST /webhooks/{id}/rotate` | `space.administer` | New secret |
| `GET, POST /admin/webhooks` | permission `administrator` | Instance webhooks |
| `GET /openapi.json` | authenticated | The API description |
| `PUT /pages/{id}/presence` | `page.view` | Say you are here, hear who else is |
| `POST /collab/authorize`, only with Part 5 | service token | The service asks about a connection |

## Data

Migration 0012, expand only:

- `page_view_days`: page, day, views, engaged, readers.
- `page_view_seen`: day, hash, page, last time. Emptied when the day closes.
- `analytics_salts`: day, salt. Deleted with it.
- `page_view_people`: page, person, first, last, views, last version. Name mode only.
- `page_view_links`: day, from page, to page, count.
- `analytics_days`: day, active users, views, new pages, comments, searches.
- `search_terms`: day, term, searches, without results. No people.
- `automation_rules`: space (empty for an instance rule), name, enabled, definition, owner, acting account, times.
- `automation_runs`: rule, event, state, steps, report, times, with `UNIQUE (rule_id, event_id)`.
- `automation_hooks`: rule, token hash.
- `outgoing_hosts`: host, added by, added at.
- `webhook_endpoints`: space, address, events, sealed secrets, enabled, failures, owner.
- `webhook_deliveries`: endpoint, event, attempts, state, next attempt, last status.
- `page_presence`: page, person, mode, seen at.

*View analytics* joins `AllSpacePermissions`. The migration grants it to everyone who holds *administer* on a space. Expand only: the previous release ignores a permission it does not know, which the matrix test must prove.

## Build order

1. `internal/outward`, and the CSV writer.
2. Recording, day closing, settings, permission.
3. Insights screens.
4. The rule engine, consumer, jobs, limits, audit.
5. The rule screens and templates.
6. Webhooks and deliveries.
7. OpenAPI, scopes, policy page.
8. Command line tool.
9. Presence.
10. Docs, status lines, handoff.
11. Part 5, only with the product owner's choice.

## Test plan

| Level | What |
|---|---|
| Go unit | Condition trees. Placeholders. Sort of events to rules. The CSV guard on every dangerous start. Signatures against the examples of the specification |
| Go integration | Own view not counted. The 30-minute limit. Closing a day removes salt and hashes. Moving to counts mode deletes names. Reports leave out hidden pages. A rule cannot do more than its account. A rule cannot trigger itself. A chain stops at five. Limits pause a rule. A redelivered event runs once. A retry does not repeat steps |
| Outward | A name that resolves to a private address, one of several addresses private, a redirect, a port not allowed, a slow answer, a huge answer, DNS changing between check and connect |
| Webhooks | Signature, retries, Send again, switching off, rotation with two signatures, a page the owner lost access to |
| API | Each route has an entry and each entry a route. A scope too small is refused. A token never exceeds its owner |
| Route walk | New object routes join by themselves |
| Mutation | Names survive a stricter mode. Hidden page in a report. Own views counted. A rule skipping the account's check. A rule triggers itself. Redirect followed. A private address allowed. An unsigned delivery. A hook token not checked. A scope not checked. A CSV cell unguarded |
| Browser | Rule built by keyboard, test run, run log, insights with the table, sorting, presence between two browsers, the editing warning |
| Pictures | InsightsView for page, space and instance, RulesView, RuleView, WebhooksView, settings screens |
| Accessibility | axe on every new screen, and `aria-sort` in tables |

## Acceptance criteria

- [ ] AC-1 With analytics off, nothing is recorded, for the instance and for a space
- [ ] AC-2 Counts mode stores nothing that points to a person after the day closes
- [ ] AC-3 Moving to a stricter mode removes names for good
- [ ] AC-4 Own views do not count, and one person counts once per 30 minutes
- [ ] AC-5 Reports hold nothing about pages the viewer may not see, and only people with the permission see them
- [ ] AC-6 Every chart has a text and a table, and every table sorts and exports safely as CSV
- [ ] AC-7 Search insights hold no people, and rare terms stay hidden
- [ ] AC-8 A rule can be built, tested and turned on with the keyboard alone
- [ ] AC-9 A rule never does more than its account may do
- [ ] AC-10 A rule cannot trigger itself, loops stop, and limits pause a rule and tell its owner
- [ ] AC-11 Every run is in the log, and every action in the audit log with the rule's name
- [ ] AC-12 Templates work out of the box, and rules can be exported and imported
- [ ] AC-13 Nothing leaves the instance until an administrator allows a host
- [ ] AC-14 A private address cannot be reached, by a name, by several addresses or by a redirect
- [ ] AC-15 Webhooks are signed, retried, logged, and switch off when broken
- [ ] AC-16 The API description matches the routes, and a test proves it
- [ ] AC-17 A token's scope limits it, and it never exceeds its owner
- [ ] AC-18 The command line tool exports and imports a space with a token
- [ ] AC-19 People on a page see each other, and the editor warns when someone else is editing
- [ ] AC-20 axe finds nothing, and the new pictures have been looked at

## Risks

| Risk | Answer |
|---|---|
| Name mode is used to watch people | Off by default, the setting names it, and it is audited |
| A small count points to one person | Only people with the permission see counts, and the docs say so |
| Rules flood the system | Limits per rule, per run, and in the chain |
| A rule with a powerful account does damage | Test run, the audit trail, and it can only do what the account may do |
| Our outward client misses a trick | The allow-list starts empty, and each OWASP case is a test |
| Polling for presence costs requests | Every 20 seconds, only on open pages, and it can be turned off. Phase 3 measures it |
| The OpenAPI table drifts from the code in detail | The test covers routes and rules, not every field. Fields are checked by hand, and the docs say so |
| "The whole of 03" and this spec differ | Said at the top, and asked as question 1 |
| Part 5 changes the promise of private drafts | It waits for the product owner |

## How the product owner tests it

1. In Administration, Analytics, turn analytics on, in counts mode.
2. Read a page as someone else, for over 20 seconds. The next day, or after running the day closing, open its insights.
3. Turn on name mode, read again, then go back to counts and see the names go.
4. In a space, make a rule from a template. Use Test run on a page. Turn it on, and trigger it.
5. Make a rule that triggers itself, and see that it stops.
6. Add a webhook to a test address that is not allowed yet, and see the refusal. Allow the host, and watch the delivery log.
7. Create a token with `read`, try to change a page with it, and see the refusal.
8. Run `diffluence client export` with that token.
9. Open a page in two browsers as two people, and see each other. Edit in both, and see the warning.

## Provenance

`docs/blueprint/03-functional/12-analytics-automation-platform.md`, `13-other-content-types.md`, `07-collaboration.md` and their sources, `docs/blueprint/04-architecture.md`, and `08-roadmap.md`. Read on 2026-11-05, for behaviour only, in own words:

- Summaries of the CNIL's guidance on audience measurement, from law firms and vendors. These are secondary sources, so check the CNIL's own pages before relying on the figures.
- The Standard Webhooks specification through its public descriptions, and Svix's glossary pages on signatures and replay.
- OWASP's cheat sheet on Server-Side Request Forgery, and its page on CSV injection.
- RFC 9700 on OAuth security, RFC 8594 on the Sunset header.
- W3C WCAG 2.2 Understanding 1.1.1.
- Yjs's documentation and public repository, `y-prosemirror`'s repository page, TipTap's documentation on collaboration, Hocuspocus's documentation and repository page.

Library documentation only. No source code of any other wiki was read. Own design.
