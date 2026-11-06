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

## Pictures

| What | Licence | Why |
|---|---|---|
| Kenney, Animal Pack Remastered: the 30 round PNGs in `web/public/animals/`, 115 KB, with its `LICENSE.txt` | CC0 1.0 | The animals of spaces (ADR-031). No code. Credit is not required, and it is given here with thanks |

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
| TipTap drag handle | Own code | It needs the whole collaboration stack, including Yjs. Ours moves top-level blocks and is small. Revisit when real-time editing arrives. |
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
| `@tiptap/suggestion` | MIT | The menus at the cursor for `/` and `:`. Its helper for placing a popup, `@floating-ui/dom`, was already there through `@tiptap/react`. | The product owner, 2026-11-05 |
| `i18next`, `react-i18next` | MIT | Translation framework | Blueprint 04 |
| `lowlight`, `highlight.js` | MIT, BSD-3 | Colours in code blocks | The product owner, 2026-11-04 |

Development only: `vite`, `typescript`, `vitest`, `eslint`, `prettier`, `@playwright/test`, `@axe-core/playwright`, all named in blueprint 09.

No Radix or other component library: native HTML elements plus our own CSS were enough, and accessibility is checked with axe.
