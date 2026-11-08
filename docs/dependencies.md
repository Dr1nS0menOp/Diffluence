# Dependencies

Rule from the product owner (2026-11-04): keep this list as short as possible, to avoid constant security advisories and abandoned code. But never hand-write an important or security-critical part when a widely used, maintained module does the job, unless it lacks a feature we need. Small non-critical code gets written here and adds no dependency.

Every direct dependency is listed with its licence and the reason it exists. Licences must be MIT, BSD, Apache-2.0 or ISC.

## Backend (Go)

| Module | Licence | Why | Approved |
|---|---|---|---|
| `jackc/pgx` | MIT | PostgreSQL driver | Blueprint 04 |
| `nats-io/nats.go`, `nats-server` | Apache-2.0 | Message queue, embedded for small installs | Blueprint 04 |
| `google/uuid` | BSD-3 | UUIDv7 ids | Blueprint 05 |
| `golang.org/x/crypto` | BSD-3 | Argon2id password hashing | Blueprint 06 |
| `coreos/go-oidc`, `golang.org/x/oauth2` | Apache-2.0, BSD-3 | OIDC login. Token verification must not be hand-written. | Blueprint 04 |
| `go-webauthn/webauthn` | BSD-3 | Passkeys. Far too intricate to write ourselves. | Blueprint 04 |
| `pquerna/otp` | Apache-2.0 | TOTP codes and the QR code for enrolment. Replaced the planned hand-written TOTP and the frontend QR library. | The product owner, 2026-11-04 |
| `yuin/goldmark` | MIT | Markdown parser, no dependencies of its own | The product owner, 2026-11-04 |
| `golang.org/x/net/html` | BSD-3 | HTML import. Parsing hostile HTML must not be hand-written. | The product owner, ADR-026 |
| `golang.org/x/text/unicode/norm` | BSD-3 | Unicode NFC for passwords (SEC-107). It was already in the build through other modules; only its listing is new. Unicode tables must not be hand-written. | Spec 035 |

## Data

| Data | Licence | Why | Approved |
|---|---|---|---|
| SecLists `xato-net-10-million-passwords.txt` (`internal/identity/leaked/passwords.bloom`) | MIT | The check of new passwords against leaked ones, which must work offline. Kept as a Bloom filter of 10 MB, never as text. `go run ./cmd/password-filter list.txt internal/identity/leaked/passwords.bloom` rebuilds it. Source: `github.com/danielmiessler/SecLists`, `Passwords/Common-Credentials/`, sha256 `c95989fe...72fe`, 5,054,227 unique entries in lower case | The product owner, 2026-11-05 |

## Web builds, not compiled here

Downloaded by `scripts/fetch-craft.py`, which refuses any file but the pinned release. The files are stored in `web/public/vendor/`, each with its LICENSE-MIT, LICENSE-APACHE, NOTICE and ATTRIBUTION.md, and `scripts/craft-checksums.sha256` with a test keeps them unchanged. They are loaded only when someone opens a file in them, in a sandboxed frame without `allow-same-origin`. The ArtCraft name and logos stay unmodified, as part of the product; a fork of these builds must remove them. Spec 019 holds the measurements.

| Build | Licence | Why | Pin | Approved |
|---|---|---|---|---|
| PdfCraft 0.18.2 (`storytold/pdfcraft`, `pdfcraft-web-0.18.2.zip`, 18.7 MiB) | MIT or Apache-2.0 | Viewing and changing a PDF in the browser | sha256 `3f2c11ae...6867` | The product owner, ADR-026 |
| GridCraft 0.16.1 (`storytold/gridcraft`, `gridcraft-web-0.16.1.zip`, 8.9 MiB) | MIT or Apache-2.0 | Opening a spreadsheet file, or a database for heavy work | sha256 `496785cf...8368` | The product owner, 2026-11-06 |

## Pictures

| What | Licence | Why |
|---|---|---|
| Kenney, Animal Pack Remastered: the 30 round PNGs in `web/public/animals/`, 115 KB, with its `LICENSE.txt` | CC0 1.0 | The animals of spaces (ADR-031). No code. Credit is not required, and it is given here with thanks |

## The service for editing together (Node.js, optional)

Only in the `collab` container, and only where an operator installs it. Pinned in `collab/package-lock.json`. Node.js security updates apply from then on.

| Package | Licence | Why | Decided |
|---|---|---|---|
| `@hocuspocus/server` 4.8.0, `@hocuspocus/common` 4.8.0 | MIT, read from each package | The WebSocket server for Yjs | ADR-026 |
| `yjs` 13.6.33, `y-protocols` 1.0.7, `lib0` 0.2.119 | MIT | The shared document | ADR-026 |
| `crossws` 0.4.17, `async-mutex` 0.5.0, `isomorphic.js` 0.2.5, `kleur` 4.1.5 | MIT | Come with Hocuspocus | ADR-026 |
| `tslib` 2.9.1 | 0BSD | Comes with `async-mutex` | ADR-026 |

## Deliberately not a dependency

| Part | Decision | Why |
|---|---|---|
| Sanitiser (`bluemonday`, in the blueprint) | Not used | We store no HTML. `document.Normalize` rebuilds documents from an allow-list, which is stricter. |
| `sqlc` (in the blueprint) | Not used | Hand-written parameterised SQL has been enough, and it saves a code generation step. |
| SCIM library | Not used | The only Go server library is not widely used. Our part is JSON mapping plus `eq` filters, and the real checks stay in the identity module. Revisit if clients need more of the protocol. |
| Version comparison | Own code | A small longest-common-subsequence routine, not security critical. |
| Encryption | Standard library | AES-GCM, Ed25519 and HKDF all come from Go itself. `internal/keys` only arranges them. |
| Rate limiter, migration runner | Own code | Small, and the blueprint allows our own migration runner. |
| TipTap colour, highlight and alignment packages | Own marks | They store free values, and our documents only allow names from a list (spec 008). Each mark is a few lines. |
| TipTap drag handle | Own code | Ours moves top-level blocks and is small. Yjs is here now, so it could be looked at again. |
| Emoji data | Own list | About 150 common emoji with our own keywords in `emoji.json`. The characters come from the system font, so nothing else ships. |
| Message bus for jobs | PostgreSQL | Imports and periodic tasks use a table with `SKIP LOCKED`. One moving part fewer than a second NATS consumer. Events still go through the outbox and NATS. |

## Frontend (npm)

See `web/package.json`. The list and its reasons are below, and `scripts/check-licenses.sh` fails the build on a licence outside the allow-list.

| Package | Licence | Why | Approved |
|---|---|---|---|
| `react`, `react-dom` | MIT | UI | Blueprint 04 |
| `@tanstack/react-query` | MIT | Server state and caching | Blueprint 04 |
| `@tanstack/react-router` | MIT | Routing | Blueprint 04 |
| `@tiptap/*` | MIT | Editor, on ProseMirror (MIT) | Blueprint 04 |
| `yjs` 13.6.33 | MIT | The shared document that merges by itself | ADR-026 |
| `@tiptap/extension-collaboration` 3.32.2, `@tiptap/extension-collaboration-caret` 3.32.2, `@tiptap/y-tiptap` 3.2.0 | MIT, read from each package | Connect the editor to Yjs, with cursors and an undo per person. Pinned together with TipTap | ADR-026 |
| `@hocuspocus/provider` 4.8.0 | MIT, read from the package | Talks to the service | ADR-026 |
| `y-indexeddb` 9.0.12 | MIT | Keeps typing safe offline | ADR-026 |
| `y-protocols` 1.0.7, `lib0` 0.2.119 | MIT | Come with Yjs | ADR-026 |
| `@tiptap/suggestion` | MIT | The menus at the cursor for `/` and `:`. Its helper for placing a popup, `@floating-ui/dom`, was already there through `@tiptap/react`. | The product owner, 2026-11-05 |
| `i18next`, `react-i18next` | MIT | Translation framework | Blueprint 04 |
| `lowlight`, `highlight.js` | MIT, BSD-3 | Colours in code blocks | The product owner, 2026-11-04 |

Development only: `vite`, `typescript`, `vitest`, `eslint`, `prettier`, `@playwright/test`, `@axe-core/playwright`, all named in blueprint 09.

## Wanted, waiting for the product owner

| What | For | Why it is not here |
|---|---|---|
| An S3 client, such as `minio-go` (Apache-2.0) | ARC-192, ARC-193 | A new module, and the choice for phase 3 is the product owner's |
| A text extractor as a separate program | ORG-025, ARC-126 | A new program to run and to keep up to date |

No Radix or other component library: native HTML elements plus our own CSS were enough, and accessibility is checked with axe.
