# 03: Functional requirements

This is the full functional description of Diffluence: what the product does, screen by screen and rule by rule, written so that the product owner can test it and a build session can implement it without guessing.

It came from a clean-room study on 2026-11-05 (ADR-023). Public help pages of other wikis were read for behaviour only. Everything here is in our own words, and the look stays ours (`docs/design/README.md`). `03-functional/sources.md` lists every source, as the clean-room process demands.

## How to read it

Every requirement has a number that never changes and is never reused, and one status:

| Status | Meaning |
|---|---|
| **built** | It works today, and tests prove it |
| **partly** | Part of it works; the line says which part |
| **phase N** | Already on the roadmap for that phase (08) |
| **new** | Found in this study, not on the roadmap until now |
| **later** | Worth keeping, not planned yet |

A line describes the end result, not the code. A requirement becomes a spec in `docs/specs/` before it is built, with acceptance criteria and permissions. When something is built, its status changes in the same pull request.

## The chapters

| Chapter | Covers | Requirements |
|---|---|---|
| [1. The frame and getting around](03-functional/01-frame-and-navigation.md) | Top bar, start screen, personal lists, directories, shortcuts, accessibility | 87 |
| [2. Spaces](03-functional/02-spaces.md) | Creating, overview, sidebar, tree, features, permissions, clearing up, archive, personal spaces | 86 |
| [3. Pages: life cycle and reading](03-functional/03-pages.md) | Drafts, publishing, moving, status, owner, approvals, archive, trash, versions, reading | 126 |
| [4. The editor](03-functional/04-editor.md) | Slash menu, blocks, text, links, images, tables, code, columns, charts, paste, find | 164 |
| [5. Dynamic blocks](03-functional/05-dynamic-blocks.md) | Blocks that show something by themselves, with every setting | 69 |
| [6. Files](03-functional/06-files.md) | Dropping files in, tiles, previews, the list, versions | 51 |
| [7. Working together](03-functional/07-collaboration.md) | Comments, reactions, mentions, tasks, sharing, watching, notifications, e-mail, editing together | 97 |
| [8. Finding and organising](03-functional/08-finding-and-organising.md) | Search, labels, templates, blog posts, folders | 74 |
| [9. People and access](03-functional/09-people-and-access.md) | Login, profile, groups, guests, permissions, restrictions | 73 |
| [10. Administration](03-functional/10-administration.md) | Console, look, rules, e-mail, audit, health | 58 |
| [11. Import, export and publishing](03-functional/11-import-export-publishing.md) | Formats, importers, public websites | 48 |
| [12. Analytics, automation and the platform](03-functional/12-analytics-automation-platform.md) | Insights, rules, API, AI, linked instances | 50 |
| [13. Other kinds of content](03-functional/13-other-content-types.md) | Live documents, whiteboards, databases, calendars, the work module, mobile | 24 |

In total 1003 requirements: 776 built, 86 partly, 27 on the roadmap, 87 new and 27 for later.

## Words we use

| Word | Means |
|---|---|
| Space | A collection of pages with its own permissions |
| Page | A document in the tree of a space |
| Blog post | A dated document outside the tree |
| Draft | Unpublished text of one person |
| Version | A published, unchangeable state of a page |
| Block | One unit of a document: a paragraph, table, panel, and so on |
| Dynamic block | A block that shows something by itself |
| Panel | A coloured box that draws attention |
| Collapsible section | A title that opens to show hidden content |
| Status | A short word and colour on a whole page |
| Status mark | The same, inside a line of text |
| Page facts | A table of keys and values that a report collects |
| Tile | How a file shows in a document |
| Favourite | A page or space a person marks for themselves |
| Watch | Ask for notifications about something |
| Restriction | A limit on one page, narrower than the space |
| Member, guest | The two kinds of person. Guests are outsiders with only what is granted |

## Where we deliberately differ

Behaviour can be familiar, but these points were decided differently, for good reasons:

1. **No direct public links to the instance.** Public pages are a static site on another host, with no way back in (ADR-010, chapter 11). The same goes for anonymous access.
2. **Administrators do not read content by default.** There is no silent bypass. Recover access always gives a reason and leaves a trace (chapter 9).
3. **Versions are immutable.** A person cannot delete a version, because the audit trail relies on them. Only an administrator can remove one, with a reason, for example for a leaked secret (chapter 3).
4. **Deleting takes the branch along.** Children do not move up, and a restored page returns to its old place with its restrictions (chapter 3). A restriction can never be lost by passing through the trash.
5. **The server does not fetch outside addresses by default.** Link previews, embeds and header images from image libraries would tell third parties who is reading (chapters 3 and 4).
6. **Files are never trusted.** Only safe images show in the browser. Everything else is a download (chapter 6).
7. **Analytics start without names.** Reading is personal. Page views are not in the audit log, and name reports are an explicit choice (chapter 12).
8. **Mentions and notifications never leak.** Nobody learns a title or a sentence from a page they may not see (chapter 7).
9. **"Not found" comes first.** Someone without access cannot tell that a page exists, unless a space chooses to allow access requests (chapter 7).
10. **Colours in documents are names, not values,** so every page works in dark mode and passes contrast (chapter 4).
11. **Everything can leave.** Every format that goes in can come out, and a bundle moves a space to another instance with its history (chapter 11).

## The order

The product owner decided on 2026-11-05 (ADR-024). The study shows that most of what makes a wiki feel complete sits in the editor and on the page, so that comes first, in waves, before phase 3. Each wave starts with its own study and plan, and ends with its own commit.

| Wave | Theme | Main content | Needs |
|---|---|---|---|
| A | **Writing feels right** | Slash menu, block handle and menu, all text formatting, panels, collapsible sections, columns, status marks, dates, emoji, table and image powers, code options, links to pages, find and replace | Approval of a few extra TipTap packages from the family we already use |
| B | **Reading and getting around** | Byline, page width, contents beside the page, heading links, details panel, tree row menu, tree filter and views, quick search, recent and favourites menus, spaces directory, keyboard shortcuts, print | Nothing new |
| C | **Working together** | Comments, inline comments, reactions, mentions, tasks with people and dates, sharing, watching, notifications, e-mail | A mail server setting |
| D | **Organising** | Labels, templates, blog posts, owner and status, archive, first dynamic blocks | Nothing new |

After that the roadmap continues with waves E to H, then wave I for databases. Moving to and from Confluence and other large tools comes very late (phase 11, ADR-028).

## Non-functional requirements

Details are in 06 and 07.

- Thousands of users per instance. Page loads under 300 ms (p95) at 5,000 simulated users.
- RPO and RTO at most 4 hours.
- Zero-downtime upgrades between consecutive releases.
- Runs on Docker, Kubernetes and LXC (Proxmox).
- WCAG 2.2 AA.
- All text translatable, and right-to-left ready.

## After 1.0

The order stays: AI, then the platform (API, webhooks, apps, marketplace), then linked instances, then the work module, then mobile apps, and last migration between products.
