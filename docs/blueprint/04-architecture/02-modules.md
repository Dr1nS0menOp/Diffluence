# 04.2: Modules

The binary stays simple only if its inside stays strict. This chapter says who owns what, and who may call whom.

## 2.1 The rules

- **ARC-023** · built · A module is a Go package below `internal/`. It offers a `Service` with exported methods, and takes what it needs in `NewService`.
- **ARC-024** · built · Every method takes a `Caller` and checks permissions itself (`permissions.Require...`), so no path through the code can skip them. A handler never decides.
- **ARC-025** · built · A state change writes its audit event and outbox event in the same transaction as the change.
- **ARC-026** · built · Imports only go one way. The order is in 2.3, and a cycle does not compile.
- **ARC-027** · partly · Modules do not write each other's tables. Reading is looser than `CLAUDE.md` says: lists join `spaces` and `users` for a name, and `perm_*` functions are shared on purpose. One exception writes: `spaces.Purge` queues file deletions itself. Still open: that exception moves behind `attachments`.
- **ARC-028** · built · Where two modules must react to each other in one transaction, the lower one offers a hook. `trash` holds a `Handler` per object type, so it needs no import of `pages`.
- **ARC-029** · wave C · A module that only reacts to something listens to an event, and is not called by the sender. Publishing a page does not know that notifications exist.
- **ARC-030** · built · Wiring lives in one place, `internal/app`. Nothing else builds a service.

**The looser reading rule, in words.** A list needs a space name or a person's name next to each row. One join is one query, and a call per row would be hundreds. So a list may read display columns of another module's table. It may never decide from them, and it may never write them. Decisions go through `permissions`.

## 2.2 Who owns what

| Module | Owns | Status |
|---|---|---|
| `identity` | Users, sessions, groups, TOTP, passkeys, global grants, preferences | built |
| `permissions` | The access rule, space grants, restrictions, `perm_*` SQL functions | built |
| `document` | The document format: gate, text, Markdown, comparison, links. No tables | built |
| `attachments` | Files on pages and their versions | built |
| `storage` | Where bytes live. No knowledge of pages | built |
| `pages` | Pages, tree, versions, drafts, look, personal lists, links between pages | built |
| `spaces` | Spaces, shortcuts, the directory | built |
| `search` | The search index | built |
| `trash` | The list of deleted items and when they go | built |
| `exchange` | Markdown import and export | built |
| `jobs` | Queued jobs and periodic tasks | built |
| `audit` | The hash-chained log | built |
| `outbox` | The event table and the relay | built |
| `queue` | The connection to NATS | built |
| `settings` | Instance settings | built |
| `keys` | Envelope encryption and the signing key | built |
| `scim`, `sso` | Identity provider protocols | built |
| `httpapi` | Router, access rules, middleware, handlers | built |
| `webui` | Serves the built frontend | built |
| `example` | The example space (spec 016) | Being built |
| `comments` | Comments, inline anchors, reactions | wave C |
| `watches` | Who watches what, follows | wave C |
| `tasks` | The task index | wave C |
| `notify` | Fan-out, the bell, preferences, digests | wave C |
| `mail` | The queue, rendering, SMTP | wave C |
| `features` | What is on per space | wave D |
| `templates` | Page and space templates | wave D |
| `blocks` | Answers for dynamic blocks, the indexes behind them | wave E |
| `outward` | The only client for addresses people chose | wave H |
| `analytics` | Counts of reading | wave H |
| `automation` | Rules and runs | wave H |
| `webhooks` | Endpoints and deliveries | wave H |

- **ARC-031** · wave D · Labels, blog posts, folders and links are kinds of page (`pages.kind`, spec 011 D1), so they inherit versions, drafts, restrictions, comments and search. No second table copies those rules.
- **ARC-032** · wave G · `exchange` grows importers and exporters around one `Plan` (spec 014 D8), and keeps its rule of only reading through the module APIs.
- **ARC-033** · wave G · `document.ToHTML` is the one renderer for export, PDF and public sites. It lives in `document`, built on the allow-list.
- **ARC-034** · wave H · One CSV writer holds the guard against spreadsheet formulas, and the audit export moves to it (spec 015 D8). It starts in wave F, whose console has CSV too, so wave H reuses it.

## 2.3 The direction of imports

```
ids, apperr, config, version, keys, storage, db
        ▲
audit, outbox, queue, instance
        ▲
identity
        ▲
permissions, settings
        ▲
search, trash
        ▲
attachments
        ▲
pages
        ▲
spaces, exchange, scim, sso, jobs
        ▲
comments, watches, tasks, notify, mail, templates, blocks, analytics, automation, webhooks
        ▲
httpapi
        ▲
app
```

- **ARC-035** · wave D · `features` sits low, below `pages`, because pages, comments and spaces all ask it (spec 011 D25).
- **ARC-036** · wave C · A new feature module sits above `pages` and uses its exported API. If `pages` must call it, an event is the answer, or a hook as in ARC-028.

## 2.4 The frontend

| Place | Holds |
|---|---|
| `web/src/app.tsx` | Frames and routes |
| `web/src/router.tsx` | A small router of our own (ADR-016) |
| `web/src/api.ts` | The only code that calls the server, with its types |
| `web/src/views/` | One file per screen |
| `web/src/components/` | Shared parts, with the editor in `components/editor/` |
| `web/src/design/` | Tokens and all styles |
| `web/src/locales/` | All words |

- **ARC-037** · built · Server state lives in TanStack Query. There is no other global store.
- **ARC-038** · built · Personal state that follows a person lives on the server (preferences). State of one device lives in `localStorage` (collapsed branches, recent searches).
- **ARC-039** · built · A document is only ever drawn by the editor, in read-only mode for readers, so no HTML from a document reaches the browser.
- **ARC-040** · wave H · `api.ts` stays hand-written. The OpenAPI file is written from the routes and tested against them (spec 015 D19). The old plan of a generated client is dropped.
- **ARC-041** · later · When the bundle gets too large, screens load on demand. The editor is the first candidate, because readers need a smaller part of it. Measure first.
