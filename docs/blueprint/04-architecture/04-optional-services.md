# 04.4: Optional services

The product owner chose these in ADR-026. They are separate programs, so their licences do not bind our code, and an installation works without any of them.

## 4.1 What every helper has in common

- **ARC-069** · wave E · A helper is switched on by an address in the configuration. Nothing is found by itself.
- **ARC-070** · wave E · Without its address the feature is absent, not broken: the menu entry is gone, and the administrator's screen says what to install. This is the rule of NAV-004 applied to helpers.
- **ARC-071** · wave E · A new route `GET /system/capabilities` tells the frontend what is on, so a screen never offers what would fail.
- **ARC-072** · wave E · A helper never blocks `/readyz`. A wiki that cannot make a PDF is still a wiki. The console shows its health, from a check by the worker every minute.
- **ARC-073** · wave E · The server calls helpers only from the worker, with a timeout, and one failure becomes a clear sentence in that job. Requests never wait for a helper.
- **ARC-074** · wave E · Helpers sit on an internal network, and get no route to the internet. They get what a job sends them, and nothing else.
- **ARC-075** · wave E · A helper's address is the operator's choice, so it is configuration, not a setting in the console. A setting in the console would let an administrator point the server at any host on the inside network.
- **ARC-076** · wave H · Addresses that **people** choose are a different case, and go through `internal/outward`: https only, allow-listed hosts, every resolved address checked, no redirects (spec 015 D15). Helpers do not use it, and it never reaches helpers.
- **ARC-077** · wave E · Images of helpers are pinned by version in the compose file, and appear in `docs/dependencies.md` as separate programs, with their licence.
- **ARC-078** · wave E · A file that a helper reads is treated as hostile, also after it comes back. What returns goes through the same checks as an upload.

| Service | Called by | Direction | Without it | Status |
|---|---|---|---|---|
| Mail server | Worker | Out, SMTP | Mail waits in the queue, and the console says so | wave C |
| Mailpit | Worker | Out, SMTP | For testing only | wave C |
| Gotenberg | Worker | Out, HTTP | No PDF export, browser print remains. No office preview through LibreOffice | wave G, wave E |
| Collabora | Worker | Out, HTTP | Office files stay a download | wave E |
| draw.io | Browser, in a frame | Browser to service | The block is absent | wave E |
| PdfCraft | Browser, in a frame | Files served by us | PDF stays a download | wave E |
| Editing together | Browser, API | Both | Private drafts and the conflict dialog remain | wave H |
| Identity provider | Browser, API | Both | Local accounts | built |

## 4.2 Mail

- **ARC-079** · wave C · The worker works through `mail_queue`, with five tries and growing waits. A broken mail server never touches a request.
- **ARC-080** · wave C · The queue holds notification ids. The text is made at sending, with a fresh access check.
- **ARC-081** · wave C · SMTP uses the standard library. `net/smtp` is frozen and takes no new features, which does not hurt here: it still does STARTTLS and auth, and needs no dependency. If mail ever needs more, this is the module to replace.
- **ARC-082** · wave C · DKIM stays with the mail server (spec 010 D14). RFC 8058 wants the unsubscribe headers signed, so one-click unsubscribe only fully works where the server signs. The plain link works everywhere.
- **ARC-083** · wave C · Mailpit is a test tool in its own compose profile. It catches all mail, and nothing leaves the machine.

## 4.3 PDF through Gotenberg

- **ARC-084** · wave G · The worker renders `document.ToHTML`, and posts `index.html` with images and styles as parts of a form to `/forms/chromium/convert/html`. Local content goes this way because that route takes no outside address.
- **ARC-085** · wave G · Gotenberg runs on an internal network with no way out, and its outbound filter stays on. The HTML holds no links to fetch, as all images travel in the form.
- **ARC-086** · wave G · One job, one call, a limit of five minutes. The file lands in storage for 24 hours (spec 014 D4).
- **ARC-087** · wave G · The large image holds Chromium and LibreOffice. Variants with only one of them exist, for those who want less.

## 4.4 Office files

Blueprint 03 asks for a **preview** (BLK-067, FIL-027), not for editing in the browser. That matters a lot: a preview only needs a file turned into a PDF.

- **ARC-088** · wave E · One small interface: `Convert(file) → PDF`. Which program does it is chosen at installation with `DIFFLUENCE_OFFICE_CONVERTER` and its address.
- **ARC-089** · wave E · **Collabora** is the default choice, through `/cool/convert-to/pdf`. It only answers hosts on its `post_allow` list, so the install guide must set that. Confirm the path and the terms of the image at build time: the SDK pages could not be read for this study.
- **ARC-090** · wave E · **LibreOffice** needs no extra container when Gotenberg is there, through `/forms/libreoffice/convert`. This is the choice that costs least.
- **ARC-091** · later · **Euro-Office** joins when its way of connecting is known. It is AGPLv3, which is fine for a separate program, and no source read here confirms how it connects.
- **ARC-092** · wave E · No WOPI. Editing through WOPI needs public routes on our side, tokens per file and a frame per editor. None of that serves a preview, and each is attack surface. If office editing is ever wanted, it is a new study.
- **ARC-093** · wave E · The result is a derived file: `previews/<sha256 of the source>`, made once per version in a job, and removed when nothing uses it. The PDF viewer shows it.
- **ARC-094** · wave E · Office files can hold macros and links. The converter runs without network, per file, with a time limit, and the result is shown only through the PDF viewer.

## 4.5 The PDF viewer

- **ARC-095** · wave E · PdfCraft's web build is a static site with WebAssembly. Measure its size before anything else, because the product owner set that condition. It is alpha, printing is missing from the web build, and embedding is not described by the project, so a proof comes first.
- **ARC-096** · wave E · Our headers forbid it as it stands. Every answer says `X-Frame-Options: DENY`, `frame-ancestors 'none'` and `Cross-Origin-Resource-Policy: same-origin`, the app policy has no `frame-src`, and WebAssembly needs `'wasm-unsafe-eval'`. The viewer gets its own path and its own headers, and the app policy stays as strict as it is.
- **ARC-097** · wave E · The frame is `sandbox="allow-scripts"`, **without** `allow-same-origin`. On our own origin those two together let the frame remove its own sandbox. So the viewer gets an opaque origin: no cookies, no session, and no access to the API.
- **ARC-098** · wave E · The app fetches the file itself, and hands the bytes over with `postMessage`. An opaque origin needs the exact-origin rules of ARC-112, and its `'self'` may not match, so the viewer's policy names our address.
- **ARC-099** · wave E · A changed PDF comes back the same way, and is stored through the normal upload as a new version of the attachment, with all its checks. The viewer itself can write nothing.
- **ARC-100** · wave E · The files ship in the binary only if they are small. If not, they are a folder an operator adds, and the block is absent without it.

## 4.6 Diagrams with draw.io

- **ARC-101** · wave E · The editor runs from an address the administrator set, in a frame with `?embed=1&proto=json`. The public `embed.diagrams.net` is never used: it would tell an outside party who is drawing what.
- **ARC-102** · wave E · One source says embed mode only works on `embed.diagrams.net`, and the project's own wiki says it works the same on a self-hosted instance. The proof at the start of the wave decides this.
- **ARC-103** · wave E · The app talks with `postMessage` under the exact-origin rules of ARC-112. What comes back is untrusted: it never reaches the page as HTML.
- **ARC-104** · wave E · A diagram is stored as a PNG with the diagram inside it (`xmlpng`), through the normal upload. It shows as a plain image, also without the service, and only editing needs the service. No SVG is stored, because SVG can carry scripts.
- **ARC-105** · wave E · The frame needs its own origin, so `allow-same-origin` is safe there, and draw.io needs it for its own storage.
- **ARC-106** · wave E · The self-hosted editor must be checked for outside calls. Its default paths and plugins can point elsewhere, so the guide sets them and a test watches the network.

## 4.7 Frames and the content policy

- **ARC-107** · wave E · The policy in `internal/webui` is a constant today. It becomes a value built at start from the configuration, so `frame-src` names exactly the helpers that are on, and with none it stays as it is. Without `frame-src` a browser falls back to `default-src 'none'`, which is what blocks every frame now.
- **ARC-108** · wave E · The list of frame sources comes only from configuration. BLK-068, frames for any address, stays later and needs its own allow-list.
- **ARC-109** · wave E · No frame ever gets `allow-top-navigation`, and `allow-popups` only if a helper proves it needs it.
- **ARC-110** · wave E · `Trusted Types` stay on for the app. A helper's frame has its own document, so a helper never forces a weaker policy on us.
- **ARC-111** · wave E · One small module handles all frames: it opens one, posts to one origin, and validates messages. Three copies of this code would drift.
- **ARC-112** · wave E · Every message to a frame names its exact origin, never `*`. Every message received checks `origin` and `source`, then checks its shape. A listener only exists while a frame is open.

## 4.8 Editing together

The product owner chose the service from blueprint 04. Part 5 of spec 015 is the plan. This is its place in the architecture.

```
browser ──wss /collab──► diffluence (proxy) ──► collab service (Node.js, Hocuspocus)
                                                   │  authorize, load, store
                                                   ▼
                                          diffluence API, service token
```

- **ARC-113** · wave H · The service merges and relays. It has no business logic, holds no database password, and never writes a version.
- **ARC-114** · wave H · `onAuthenticate` asks the API with the person's session. A person with view rights only cannot connect.
- **ARC-115** · wave H · The service asks again every minute. A change of permissions closes the connection at once: the API tells it through an event, and that listener is the only one the service has.
- **ARC-116** · wave H · The shared state is stored through the API in `collab_documents`, as Yjs bytes, with `onStoreDocument`. If storing fails, the document stays in memory and is tried again, which is how Hocuspocus behaves.
- **ARC-117** · wave H · Publishing stays in Go. The document is taken from the shared state and passes `document.Normalize`, so a hostile client that wrote into the shared state gets stopped there.
- **ARC-118** · wave H · The binary proxies `/collab` to the service when `DIFFLUENCE_COLLAB_URL` is set. That keeps one address, one cookie and no path rule at the reverse proxy. A large installation may route it at the load balancer instead. Test that the WebSocket survives the server's write timeout.
- **ARC-119** · wave H · `'self'` in the content policy covers `wss:` on the same host, as CSP Level 3 says, so same-origin routing needs no change to `connect-src`. Confirm this in the browser test.
- **ARC-120** · wave H · **Two ways of drafting must live side by side**, because the service is optional. With it, a page has one shared draft. Without it, drafts stay private. Both paths keep their tests, and the editor picks one from `/system/capabilities`.
- **ARC-121** · wave H · When the service goes down, `y-indexeddb` keeps typing safe in the browser, and the strip says that changes are not reaching others. Publish waits until the document has synced.
- **ARC-122** · wave H · An upgrade restarts the service, and clients reconnect and merge. Browser and service must speak the same version, so their packages are pinned together.
- **ARC-123** · phase 3 · One service node with a quick restart is the first target. Two nodes use sticky routing by page.
- **ARC-124** · phase 3 · The Redis extension is not the way to scale. Hocuspocus says that all messages are then handled on all instances, so it adds availability and no capacity. It would also add a component whose licence changed: Redis 8 is AGPLv3, RSALv2 or SSPLv1. If it is ever needed, Valkey (BSD) is the candidate.

## 4.9 Later helpers

- **ARC-125** · later · Virus scanning through ICAP (FIL-021).
- **ARC-126** · later · Text inside attachments (ORG-025) needs an extractor in a separate process, and fits the same rules. Collabora can also write text, so a converter may do both. A separate study decides.
