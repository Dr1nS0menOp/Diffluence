# 026: The rest of dynamic blocks

Status: built on branch `wave-o`, waiting for the product owner's acceptance session
Phase: blueprint 03, chapter 5, and the small points left by the UI test of 6 November. Spec 012 planned the blocks in wave E, and wave J closed part of them.

## Problem

Chapter 5 still has 31 open lines. Reports miss the filters a person expects. Whole blocks do not exist yet: popular pages, people, labels, files of a space, a button that starts a page from a template, a timeline. The UI test left nine small points that make the product look unfinished.

## What the study found

**The Report table has no columns.** `reportTable` puts the labels in `detail`, and the screen only draws `cells`. So a reader sees titles and nothing else. BLK-032 is not only about choosing columns.

**The preview setting does nothing.** `child_pages` and `filter_list` accept `preview`, and the server never reads it. `search_documents.body` holds the text, so the first lines cost one query per block.

**Link cards never had an image or a text.** The screen draws `detail`, and the server sends none. `pages.header_attachment_id` has the picture.

**The filters of BLK-029 stop at four.** The status line says two are missing from the form, but creator, contributor, mentioned person and both dates exist nowhere. They are added here, on the server and in the form.

**The people who watch a page are for space administrators.** `GET /pages/{id}/watchers` needs `space.administer`. BLK-052 says contributors can list watchers. The block keeps that rule: anyone else gets the other kinds of contribution and no watcher. A block must not show more than the route does.

**Comments for editors only are not for every reader.** The contributors and contribution blocks leave out comments with `editors_only`, and trashed and purged ones. A count from them would say something a reader cannot see.

**A person in a block goes through the directory.** `people.Service` already applies `perm_visible_people` and the visibility of each field. The blocks package uses that service and does not read `user_profiles` itself, so there is one rule.

**A decision has no person of its own.** The node holds text and mentions. "Filtered by person" means the person is named in the decision, or created the page.

**Tasks have no creation date.** `page_tasks` has `updated_at` only, which moves with every edit. BLK-038 needs `created_at`.

**Migration numbers must be contiguous.** This wave uses 0026, and the pentest holds 0025. 0025 is a reserved file here, like 0009 and 0011 were. Whoever lands second keeps the real one.

**Snippet variables have a setting and no code.** `show_excerpt` accepts `values`, and nothing reads it.

**An excerpt has a `synced` attribute and nothing behind it.** A synced block is an excerpt that says so, shown elsewhere with a mark and a way to its source.

**Page facts in a template already work.** A template holds a document, and the facts index is written on publish. Only a test was missing, so BLK-042 and the facts half of ORG-048 close with one.

## Decisions

| | Decision | Why |
|---|---|---|
| D1 | People in a setting are written as a name or an address, and match exactly, without regard to case | The form stays simple. A filter only narrows pages the reader may already see |
| D2 | `report_table` sends `column_keys` and cells; the screen translates the keys | UI text only lives in translation files |
| D3 | Report columns: creator, changed, labels, status, comments, reactions. Default: creator, changed, labels | BLK-032 |
| D4 | A date setting is `YYYY-MM-DD`, and anything else is dropped at the gate | The same gate as every setting |
| D5 | Popular pages shows nothing while analytics is off. It reads `page_view_days` only, through `perm_visible_pages` | "For spaces where analytics is on", and the days hold no people |
| D6 | Activity has one kind with `scope`: all, favourites, network. Tabs change the scope in the browser, and the server answers each | Each tab is a list like any other |
| D7 | Network means pages last changed by people the reader follows | `follows` already exists |
| D8 | Rotating cards use the answer of link cards. They move every 6 seconds, stop on hover and focus, and never move with reduced motion | BLK-062 |
| D9 | Create from template calls the existing `POST /templates/{id}/use`, which decides. A template with required fields opens no form here: the error says so | No second way to start a page |
| D10 | Section contents takes two anchors, or the title of a collapsible section | BLK-014 says markers, and anchors are our markers |
| D11 | A timeline is a dynamic block with lines of text: lane, label, start, end. No end means a marker. It runs in the browser and the settings hold its data | No new node, and the gate already limits size: 50 lines of 200 bytes |
| D12 | Snippet variables: `{{name}}` in the source, and `name=value` lines in the block that shows it. The server fills them in text only, after every permission check | Text never becomes markup, so a value cannot inject anything |
| D13 | A synced block is `synced_block`, the same path as `show_excerpt`, so every level is checked, loops stop, and "used by" lists it. The source link says Edit only if the reader may edit | One rule for reuse |
| D14 | Settings open in a panel at the side, which leaves the page in view | BLK-001 and BLK-002 |
| D15 | The letter bar filters in the browser. Index and labels list already load at most 500 rows | BLK-021 and BLK-033 |

## The small points of the UI test

Public link dialog: the full address and a copy button. Raw values in the administration show as words. Company home page gets a page picker. History offers "Choose two versions" only with two versions. The date in the restore note no longer says "from now". The import preview counts with a plural. The Care report has a stable row key. The title field grows with a long title, and the placeholder fits a phone.

## Left out

- **BLK-066 and BLK-067:** need a viewer in the page and a real converter.
- **BLK-069:** calendars do not exist.
- **BLK-038, ticking tasks in the report:** already built or not, it is outside what the line says is open.

## Sources

No new outside sources. Behaviour comes from `docs/blueprint/03-functional/05-dynamic-blocks.md` and its sources.
