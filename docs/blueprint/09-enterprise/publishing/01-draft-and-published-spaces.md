# 09.P1: Draft and published spaces

Blueprint 09 holds the enterprise document features that come before 1.0 (ADR-038). Document control came first ([09.1](../01-document-control.md), spec 064). This second part is **draft and published spaces**: two linked spaces in one installation, where people write in the **draft space** and readers only ever have the **published space**. Content crosses from the one to the other only when someone releases it, or when an approval releases it. The plan to build it is [spec 073](../../../specs/073-draft-and-published-spaces.md).

## How to read this study

Every line has a number (PUB-NNN) and a status: **built** (exists today and is reused), **partly**, **new** (planned in spec 073), **later** (wanted, not in spec 073) or **not** (decided against, chapter 15). Each line gives the behaviour, then *Why* (with the clause of a standard, a finding of research, or a lesson from another tool where one applies), *Edge cases*, and *Proof* (how a test will show it). The sources are in [sources.md](sources.md).

This is a clean-room study (`docs/blueprint/02-clean-room-and-licensing.md`). Blueprint 08 names a publishing product only to say which behaviour is meant. Its public documentation was read on 2026-11-14 for behaviour only, as listed in [sources.md](sources.md); no text, names of workflows or states, icons or layouts were taken, and no product's source was read. Other tools that keep draft and live content apart were read the same way (chapter 02), and every requirement here is stated in Diffluence's own terms, from its own code and blueprints.

### One word per concept: release

Diffluence already uses **publish** for making a new version of a page (spec 001): the editor's Publish button, a scheduled publish, a silent publish, the `published` status of a page. Using the same word for the new act would give one word two meanings on one screen ("Publish" in the editor, "Publish" to the other space). This study therefore calls the act of copying a page from the draft space into the published space **release** (decided, PUB-1401). The spaces keep the names of the roadmap: the draft space and the published space. A page is *released*, *not released yet*, *changed since release*, *left out* or *removed in the draft*. One existing screen uses "release" already: space settings let an administrator release an old space key. That is a rare action in another section; when this feature lands, its wording there should become "free the old key", so that "release" has one meaning.

## Chapters

| Chapter | Lines | About |
|---|---|---|
| [02 What people need](02-what-people-need.md) | PUB-100 to PUB-109 | Who uses it and why, what other tools and research teach, the needs as requirement lines |
| [03 The pair](03-the-pair.md) | PUB-200 to PUB-246 | Linking two spaces, the managed published space, what pairs with what, ending a link |
| [04 Out of step](04-out-of-step.md) | PUB-300 to PUB-345 | What "out of step" means for the text, title, place, look, files, labels, comments and permissions, and the statuses |
| [05 Releasing](05-releasing.md) | PUB-400 to PUB-456 | One page, a branch, the whole space; the preview; leaving pages out; taking pages off; runs |
| [06 Links](06-links.md) | PUB-500 to PUB-534 | How links, cards, embeds and blocks are rewritten across the pair |
| [07 Permissions and leaks](07-permissions-and-leaks.md) | PUB-600 to PUB-635 | Who may do what, and every door through which a draft could leak |
| [08 Document control](08-document-control.md) | PUB-700 to PUB-734 | Approval releases, approved versions only, withdrawal, signatures in the published copy |
| [09 Search and notifications](09-search-and-notifications.md) | PUB-800 to PUB-833 | Search, the feed, watchers, mentions, tasks, webhooks |
| [10 Audit](10-audit.md) | PUB-900 to PUB-923 | What is recorded, the release history, verification |
| [11 Edge cases](11-edge-cases.md) | PUB-1000 to PUB-1066 | Moves, deletes, renames, restores, import and export, two drafts into one, concurrency, failures |
| [12 Load budget](12-load-budget.md) | PUB-1100 to PUB-1108 | What it costs, with numbers |
| [13 Screens](13-screens.md) | PUB-1200 to PUB-1242 | The screens, following the design standard |
| [14 Migration and upgrade](14-migration.md) | PUB-1300 to PUB-1312 | The schema, the experimental switch, existing spaces, older nodes |
| [15 What it will not do](15-what-it-will-not-do.md) | PUB-1500 to PUB-1517 | The limits of the feature, each with a reason |

## 1.1 What exists today

Read in the code on 2026-11-14 (`internal/pages/tree.go`, `internal/pages/links.go`, `internal/attachments/attachments.go`, `internal/sites`, `internal/features`, migrations `0002`, `0051` and `0054`).

- **PUB-001** · built · Copying a page or a branch into another space (`pages.Copy`, `copyOne`, `copyChildren`): a new page from the version the copier may see, its files copied as new attachment rows with new stored copies (`attachments.CopyAll`), labels and restrictions kept by choice, and children the copier may see. The copy has no link back to its source after it is made. This is the mechanism a release reuses, not the behaviour: a copy is made once, a release keeps the two pages paired.
- **PUB-002** · built · Links between pages are stored by page id (`pageLink`, `pageCard`, `document.LinkedPages`), with an optional fragment for a heading or anchor. What a link shows (title, space, excerpt) is looked up for each reader (`LinkFacts`), and a page the reader may not see is left out exactly like a page that is gone. This makes rewriting a link a change of one id, and makes an unrewritten link fail closed.
- **PUB-003** · built · Placeholder links (`page_placeholders`) to pages nobody has started.
- **PUB-004** · built · Public sites and public links build static copies on a separate host (ADR-010), only of pages that may be public (`mayBePublic`), and withdraw them when that changes (`enforceLinks`).
- **PUB-005** · built · Document control, wave DC1: `page_control` keeps the approved version as a page's current version, so every reader's route shows approved content, and a never-approved or withdrawn page is hidden from everyone who may not edit it (`perm_control_hidden_pages`). DC2 (reviewers, the editor, the review panel) is being built beside this study.
- **PUB-006** · built · Experimental parts with one switch each for the installation (ADR-043, spec 069, `internal/features/experimental.go`): an off part answers its routes with 404 `experimental_off`, is not offered on any screen, and keeps what exists.
- **PUB-007** · built · Permissions split off broader ones with a migration proof that no decision changes (waves PM, FU, DC1: `rbac_split_off`, `rbac_split_roles_from`).
- **PUB-008** · built · Jobs coordinated through PostgreSQL with `SKIP LOCKED` (ADR-018), the outbox, the audit chain sealed after saving (ADR-041) and its witness (ADR-039).

What is missing is everything in chapters 02 to 14: a lasting link between two spaces, a published space that only a release can change, knowing per page what is out of step, releasing one page, a branch or all, leaving pages out, rewriting links across the pair, approval that releases, and the record of every release.

## 1.2 Permissions

- **PUB-010** · new · `space.release` ("Release to the published space"), held in the **draft** space: release pages, take them off, leave pages out and include them again, and see the release screen. Split off `space.administer` with the migration proof of wave PM, so every space administrator keeps it and nobody else gains it. *Why:* releasing is the act that exposes text to a whole audience; it must be a right that can be given to a release manager without giving them the whole space. *Proof:* the adoption test that no decision changes except the split.
- **PUB-011** · new · Linking two spaces and ending the link needs `space.administer` in **both** spaces (PUB-210). *Why:* the link changes what both spaces are; neither owner may decide for the other. *Proof:* matrix cases for each side missing.
- **PUB-012** · new · The published space is **managed** (PUB-220): no grant, role or administrator right lets anyone write its content; only a release does. The rule is decided in `permissions.Evaluate` and in the SQL lists, and held by a database trigger as well (PUB-224). *Proof:* the permission matrix gains the space kinds draft, published and ordinary against every action.

## 1.3 Decided on the product owner's behalf on 2026-11-14

Each was decided on the product owner's behalf on 2026-11-14, and may be overruled before the wave that uses it.

| Line | Decision | Reason |
|---|---|---|
| PUB-1401 | The act is called **release**; "publish" keeps its meaning of making a version | One word per concept; two meanings of "publish" on one screen would confuse every person who writes |
| PUB-1402 | A published space has exactly one draft space, and a draft space exactly one published space; a space is never both | One answer to "where do I change this?"; no chains, so no loops and no leak through a middle space |
| PUB-1403 | The published space is managed: its content can only change by a release, for everyone, administrators included | A published copy that people also edit by hand is overwritten by the next release or drifts; both lose work |
| PUB-1404 | Linking needs an empty published space, made new or empty already; an existing space with content becomes a published space by starting a draft space from it (PUB-1302) | Matching existing pages by title guesses wrong; titles are not unique in Diffluence |
| PUB-1405 | Page restrictions are not copied; restrictions set in the published space stay across releases; a page with a view restriction in the draft is left out of a whole-space release unless named | Who may draft and who may read are different questions; a draft secret must never become public by a bulk action |
| PUB-1406 | Comments never cross the pair, in either direction | Drafting discussion is not for readers, and readers' questions belong to the page they read |
| PUB-1407 | Labels, the emoji, the header image and the look are copied; page status, owner, watchers and favourites are not | Labels and look are part of what readers see; the others are working marks of the writers |
| PUB-1408 | A version in the published space has the releaser as its author and the release note as its summary; draft authors and draft summaries do not cross | Draft summaries hold working notes, and who drafted can be confidential in regulated teams |
| PUB-1409 | A run releases exactly the versions its preview showed, even when a page changed meanwhile | A person approves what they saw; versions are immutable, so this is exact |
| PUB-1410 | A whole-space release takes off pages removed or archived in the draft by default, listed in the preview, and the person can untick it | "Release the whole space" means readers get the space as it is written |
| PUB-1411 | A run of up to 1,000 changed pages is one transaction; a larger run goes in batches of 250 and says so in the preview | Readers see the space before or after a normal run, never half of it; very large runs cannot hold a transaction for minutes |
| PUB-1412 | A link to a page that is not released yet becomes its own text in the published copy until the target is released; it is never fixed by a silent new version | Every version in the published space comes from a release a person or an approval caused |
| PUB-1413 | A workflow's approved state can be marked "release on approval"; the system then releases, with the approver as cause | "Approval publishes" without a second manual step |
| PUB-1414 | Of a controlled page only the approved version can be released, by hand or by approval | Readers rely only on approved content (DOC-1406) |
| PUB-1415 | The feature is an experimental part, off by default on every installation (ADR-043) | It is a new part after the freeze, used by few installations, and it adds a second write path into spaces; it has to earn its place |
| PUB-1416 | `space.release` is a new permission split off `space.administer` | A release manager needs the right without running the space |
| PUB-1417 | A draft space cannot have a public site or public links | Public exposure comes only from the published space |
| PUB-1418 | The home page of the draft space releases into the home page of the published space | The home page is what readers see first; leaving it out would leave readers a blank start |
| PUB-1419 | Files are copied as new stored files when their version changed since the last release, and count against the published space's storage limit | The two spaces must not share a file that one side can change or purge; ADR-038 counts storage everywhere |
| PUB-1420 | The releaser must be able to view the published space | Nobody changes what they cannot see |
| PUB-1421 | One run per pair at a time; single-page releases wait for a running whole-space run | Two runs would race on the same tree |
| PUB-1422 | Archiving mirrors archiving, and a deleted draft page takes the published page to the published space's trash, never purges it | Every step can be undone from the published side's own trash |
| PUB-1423 | Task lists in a published page are read-only and appear in nobody's Tasks screen | The task belongs to the draft, where it can be ticked |
| PUB-1424 | Search shows the draft and the published copy both, with the draft marked; no merging of results in the first waves | Simple and honest; merging can follow use |
| PUB-1425 | While the part is off, the published space stays read-only and ending the link stays open | Unlocking it would invite edits that the next release overwrites |
| PUB-1426 | A page whose draft parent is not released goes under its nearest released ancestor, with a note in the run's report | Readers get the page; the place is corrected when the parent is released |
| PUB-1427 | Load is measured with one pair made of the largest space of the real-data dump | The pair doubles that space's pages, which is the realistic worst case for one installation |
| PUB-1428 | Watchers of a published page are told once per run, in one message per person | A whole-space release must not send a mail per page |
| PUB-1429 | Reader comments in the published space are shown to authors on the draft page as a count with a link, never copied | Authors learn what readers ask without the comment leaving the space where it was written |
| PUB-1430 | The managed rule is held by a database trigger as well as by the permission rule | An older node during an upgrade, or a bug in a new path, cannot write into the published space |

## 1.4 Only the product owner can decide

- **The word.** Whether the act is called "release" (PUB-1401) or something else in the screens and the documentation. It is reversible until the translation files are opened to translators at 1.0, and it costs only strings.
- **Audiences.** Whether a draft space should ever release into several published spaces, one per audience (for example customers and staff), with pages chosen per audience. It changes the model from a pair to a fan, and is left out on purpose (PUB-1501).
- **Experimental before 1.0.** The product owner wants the feature built before 1.0 (ADR-038) and froze new parts for 1.0 (ADR-043). This study builds it before 1.0 as an experimental part, off by default (PUB-1415). Whether it should instead be on by default at 1.0 is the product owner's to say once it has run on a real installation.

## Sources

See [sources.md](sources.md).
