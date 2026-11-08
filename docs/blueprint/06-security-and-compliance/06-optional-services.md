# 06.6: Optional services

The product owner chose these in ADR-026: Gotenberg for PDF, draw.io for diagrams, Collabora or LibreOffice for office files, PdfCraft for PDF in the browser, and a service for editing together. They are large programs, and most of them parse hostile files. They are the biggest new attack surface in the plan, so they get a chapter of their own.

## 6.1 Rules for every optional service

- **SEC-500** · built · Off by default. A basic installation needs none of them, and without them the feature says so and offers a download.
- **SEC-501** · built · Each runs in its own container, on an internal network with no route to the internet, and not reachable from outside the host. In Compose that is `internal: true` (ASVS 15.2.5).
- **SEC-502** · built · Each runs as non-root, with a read-only root file system where it can, no extra capabilities, and limits on memory, processes and time.
- **SEC-503** · built · The app authenticates to each service with its own secret, and no service has a default password (ASVS 13.2.1, 13.2.3).
- **SEC-504** · built · Versions are pinned, listed in `docs/dependencies.md`, and watched for advisories, even when the service is no code dependency of ours.
- **SEC-505** · built · A service only gets what the person may see, checked before anything is sent. What comes back is treated like an upload: type from content, and no trust in its name.
- **SEC-506** · built · Nothing a service made is shown on the app's own origin as a document. A failing service leaves the page working (ASVS 16.5.2).
- **SEC-507** · built · The guide lists each service with what it can reach and what it may see, as ASVS 13.1.1 asks. Belongs in `docs/operations/`, with the first service.

## 6.2 Gotenberg

**What can go wrong:** the server-side browser is made to fetch internal addresses, a hostile office file exploits LibreOffice, one person's conversion sees another's.

Gotenberg's Chromium route had a high-rated SSRF bug, CVE-2026-42595, because its default deny list only covered `file:`. It was fixed in 8.32.0, and redirects were part of the problem. The defaults still allow private addresses.

- **SEC-510** · built · Use at least the release that fixes CVE-2026-42595, and pin it. Linked content in office files is blocked from 8.34.0, so that is the floor for office conversion.
- **SEC-511** · built · Start it locked down: `--chromium-deny-private-ips`, `--chromium-deny-public-ips`, `--chromium-disable-javascript`, `--api-disable-download-from`, `--webhook-disable` and `--api-enable-basic-auth`. Leave `--chromium-shared-browser-context`, `--chromium-disable-web-security` and `--chromium-allow-file-access-from-files` off. Turn off the routes that are not used.
- **SEC-512** · built · The app only sends HTML it made itself, with images as parts of the request, so no URL route is ever used. The network rule of SEC-501 is the real barrier.
- **SEC-513** · new · **Medium.** PDF export sends our own HTML. Office conversion sends hostile files. In one container, a successful attack on LibreOffice can see the next person's export. Run two instances, one for each, when office conversion is on. Belongs in wave E.
- **SEC-514** · built · A conversion has a time limit and a size limit, and temporary files go when it ends.

## 6.3 draw.io

**What can go wrong:** the editor runs in our origin, a message from another window is believed, the public editor learns who draws what, a drawing carries a script.

- **SEC-520** · built · Only an address an administrator set is used. `embed.diagrams.net` is never the default: it would tell an outside party who is drawing what.
- **SEC-521** · built · **High.** That address must be another origin than the app. Its scripts would otherwise run with our cookies and our API. It cannot sit under a path of the instance. Say so in spec 012, and refuse a same-origin address in the setting.
- **SEC-522** · built · `frame-src` allows that one address, and nothing else (SEC-259).
- **SEC-523** · new · **Medium.** Messages from the frame must be checked on origin and on source window, and must fit the expected form. Messages to it name the exact origin, never `*` (ASVS 3.5.5). Spec 012 does not say so.
- **SEC-524** · built · A drawing is stored as a PNG with the diagram inside it, as a normal attachment, so it passes the same type check. SVG is never stored. The XML inside the PNG is data that only draw.io reads.
- **SEC-525** · new · Its reference says embed mode is only supported on `embed.diagrams.net`, and its wiki says it works the same on your own host. Test this first. Also check what the editor loads from outside, and turn off plugins and outside libraries. Belongs in wave E, before anything is built.
- **SEC-526** · built · The editor is Apache-2.0, and its Docker wrapper is GPL-3.0, which is fine as a separate program. Stencils have terms of their own, which need reading.

## 6.4 Office files: Collabora, LibreOffice, Euro-Office

**What can go wrong:** a hostile document exploits the converter, a converted copy outlives its source, a token for one file opens another.

- **SEC-530** · built · The choice is made at installation: Collabora by default, LibreOffice through Gotenberg, or none (ADR-026). These are separate programs.
- **SEC-531** · built · For now the service only converts a file for a preview. It does not edit. That leaves out WOPI, and with it a whole set of tokens and callbacks.
- **SEC-532** · new · **Medium.** The security settings of Collabora could not be read for this study, because its documentation is behind a bot check. Read `coolwsd.xml` and the conversion documentation before building: who may call `convert-to`, and how it is limited. Belongs in wave E.
- **SEC-533** · new · Euro-Office is AGPLv3 and a young fork, and no source read for this study says how it connects. It stays out until that is known. As a separate program its licence does not touch ours, but a lawyer should confirm that for what we ship.
- **SEC-534** · built · A converted copy has the same access rule as its source, and goes when the source goes (SEC-339).
- **SEC-535** · later · Editing office files in the browser needs WOPI. Then: a token per person and per file, short-lived. Requests are checked with proof keys, against the keys in discovery, including rotation, with a time stamp no older than 20 minutes. A failing check answers 500.

## 6.5 The service for editing together

**What can go wrong:** this is the biggest change to the security model. A person loses access and keeps editing. A client sends something the gate would refuse. A name in the cursors is made up. Drafts stop being private.

The product owner said yes to this in ADR-026, with Hocuspocus and Yjs. The study in spec 015 lists what comes with it.

- **SEC-540** · built · The browser gets a short-lived token from the API for one page. The service asks the API on each connection, and the session cookie never goes to the service (ASVS 4.4.3, 4.4.4).
- **SEC-541** · built · Connections use `wss`, and the `Origin` header is checked against the public address (ASVS 4.4.1, 4.4.2).
- **SEC-542** · built · **High.** Hocuspocus checks rights when a connection starts. It does not check again by itself. It offers `onTokenSync` and read-only connections, but something must start them. A person whose access ends would otherwise go on reading and writing. On a change of grants or restrictions, the app must tell the service to drop or re-check connections for those pages, within seconds (ASVS 8.3.2).
- **SEC-543** · built · A person without edit rights gets a read-only connection, and the service refuses their changes.
- **SEC-544** · wave H · Names and pictures in cursors come from the server, not from the client, or anyone can pose as a colleague.
- **SEC-545** · built · Limits on the size of an update, of a document and on the number of connections per person. Spec 015 plans for 30 editors.
- **SEC-546** · partly · **High.** Changes go from browser to browser through the service, past `document.Normalize`. A hostile client can send a node or attribute the gate would refuse, and other editors will load it. Publishing normalises, but the colleague's browser already saw it. Check updates in the service against the same list, or have each browser refuse what its schema and link rules do not allow, and test it with a hostile client. Any stored state must pass the gate too. Built: stored state passes the gate, and the editor's schema and link rules refuse the rest, with a test (spec 021). Still open: a test with a hostile client through the real service.
- **SEC-547** · built · The promise of private drafts changes: one shared draft per page (COL-092). The product owner has agreed to the feature, and spec 015 says this change needs its own decision. People must see that a draft is shared, and text in a shared draft is visible to all editors before publishing. **Decision for the product owner.**
- **SEC-548** · new · Text typed and removed can stay in the shared state that every client receives. A secret pasted by mistake must really go. Read how Yjs garbage collection behaves, and how the state is stored. Medium. Belongs in wave H.
- **SEC-549** · built · The service has no business logic and no direct database access to pages. Node.js updates become part of security maintenance.
- **SEC-550** · built · A version made together names everyone who took part, so the audit log stays true.

## 6.6 The PdfCraft frame

**What can go wrong:** a hostile PDF attacks the viewer, the viewer runs in our origin, an alpha program has bugs, a changed PDF replaces the original without a trace.

PdfCraft is MIT or Apache-2.0, is written in Rust, and is alpha. Its web build is a static site with WebAssembly, and the public pages say nothing about embedding it. Its printing is missing in the web build, and changing existing text is thin.

- **SEC-555** · built · **High.** It must not run with our origin's rights. Spec 012 says a sandboxed frame on our own origin. The HTML standard warns that `allow-scripts` together with `allow-same-origin` on the same origin lets a page remove its own sandbox. So: `sandbox="allow-scripts"` without `allow-same-origin`, or another origin. The frame then has no cookies and cannot call the API.
- **SEC-556** · new · **Medium.** Its own path, with a policy of its own: `default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; connect-src 'none'; frame-ancestors 'self'`. `wasm-unsafe-eval` is limited to that path, and the app's policy stays as it is. The frame in an opaque origin may need its files by another route. Test that first.
- **SEC-557** · new · **Medium.** The app reads the file with the person's rights, and hands the bytes to the frame by `postMessage`. Messages are checked on source window, since the origin of a sandboxed frame is `null` (ASVS 3.5.5).
- **SEC-558** · new · A changed PDF comes back as a new version through the normal upload, with the same type check, audit and permission. It never overwrites.
- **SEC-559** · new · Pin the version with a hash, list it in `docs/dependencies.md`, and treat an alpha as untrusted. Measure the size before deciding. If it does not fit, PDF stays a download.
