# Spec 030: the rest of blueprint 03 (wave U)

Status: built on branch `wave-u`. Phase: the last wave of `docs/blueprint/03-functional-requirements.md` before the look of ADR-034.

## The study

On 7 November 2026, blueprint 03 had 14 lines `new` and 51 `partly`. Each was weighed against four reasons to leave it:

1. **Waits for the product owner:** floating bars (EDT-006, EDT-080), a first-page picture on tiles (FIL-026), a storage limit on copies, moves and imports.
2. **Comes later by decision:** the look, settings and roles of ADR-034, calendars (chapter 13), and helpers that are not installed (BLK-066, BLK-067, BLK-069).
3. **Needs a new dependency**, which the product owner must approve.
4. **Too large for one wave next to search:** each such line is named below, so a next wave can start from it.

This wave builds what is clear, small enough to test well, and useful to the people who keep an instance tidy.

## What is built

| Line | What | Load budget (ARC-238) |
|---|---|---|
| ACC-066 | The lock beside the page title is open on a page without restrictions of its own and closed on a restricted one, and its name says which | Reading, no extra query: `restricted` was already in the page |
| ADM-033 | The spaces in the administration filter by name or key, at least N pages, at least N MiB, and no change for 1, 3, 6 or 12 months | Reading, no extra query: filtered in the browser over the list it already loads |
| SPC-061 | The Content screen filters on the day of the last change and the day of creation, from and until, as whole days. A wrong date answers 400 | Reports, the same one query, four more conditions on indexed columns |
| ACC-040 | The user list shows, for each guest, the keys of the spaces they can open now. Members show no list, because they reach most spaces | Reading, one subquery per guest row, at most 100 rows a page |
| ADM-010 | A person's screen in the administration shows their groups, the spaces they can open and how many pages they own. Titles stay out, because administrators read no content by default (ADM-012) | Reading, one query, at most 200 groups and 200 spaces |
| ADM-049 | The attention list warns when the disk for files has less than a tenth free, with the free percentage. Object storage and systems that cannot tell stay silent | Reading, one `statfs` call, no query |
| EDT-150 | Measured, see below | — |

### EDT-150, measured

A page of 1,000 paragraphs and 100,000 words, in headless Chromium on the development machine (16 threads), through the e2e harness:

| Step | Time |
|---|---|
| Open the page to read, until the last paragraph is in the page | 389 ms |
| Open the editor until it has the focus | 532 ms |
| Type 37 characters at the end | 385 ms, about 10 ms a key |
| Type at the top | 76 ms for 6 characters |

The line stays `partly`: the tools were not measured on their own, and a slow device was not tried. The throwaway test was not kept.

## Permissions and failure paths

- The guest's spaces come from `perm_spaces`, the same rule as everywhere. A mutation that lists every space instead is caught.
- The overview of a person is behind `administrator`. Another person gets 403, an unknown id 404, and no title is in the answer. A mutation that counts the pages of others is caught.
- A date filter takes `2026-11-07` only. The last day counts whole; a mutation that drops that is caught.

## Left, line by line

| Line | Why it is left |
|---|---|
| NAV-016, NAV-058 | The Recent menu with tabs and hover cards in the byline, history and comments belong to the new look (ADR-034) |
| SPC-025 | Dragging shortcuts: Move up and down do the job; dragging comes with the sidebar of ADR-034 |
| SPC-026 | Calendar: chapter 13 |
| SPC-060 | Last viewed per page needs analytics with names, which is off by default; left until analytics is next touched |
| SPC-075 | Progress of a permanent removal: needs the removal to become a job with steps. Next wave |
| SPC-079 | Moving a page out of a personal space: move between spaces exists, the line needs a check of the flow only. Next wave |
| PAG-004, ORG-045 | A template panel beside the editor instead of a dialog: the editor layout of ADR-034 |
| PAG-005 | Import from Create: the importer exists, the entry in Create is next |
| PAG-024 | Who can see it, in the publish options: restrictions on a page never published need care. Next wave |
| PAG-073 | The archive as a tree: the tree by branch of spec 028 can show it. Next wave |
| PAG-092 | Tables compared per cell: a larger piece of the comparison. Next wave |
| EDT-006, EDT-080 | Wait for the product owner |
| EDT-020, EDT-058, EDT-074, EDT-138, EDT-143, EDT-149 | Editor details of spec 027 that need the editor itself; one wave of their own |
| BLK-066, BLK-067, BLK-069 | Real helpers and calendars |
| FIL-026 | Waits for the product owner |
| COL-040 | Tasks in search: search belongs to wave T |
| COL-043 | Give access in the share dialog: touches permissions; its own study |
| COL-064 | Review reminders: a job and a notification type. Next wave |
| COL-078, ADM-022, COL-080 | Defaults for new people and mail in their language: there is one language today, so mail in the person's language has nothing to choose. The defaults are next |
| COL-096 | 30 editors on one page: needs the load test on a real server |
| ORG-015, ORG-017 | Controls for "below a page" and several people in the filters: the filters of ADR-034 |
| ORG-051, ORG-059, ORG-060, ORG-061 | Template screens: one wave with ADM-035 |
| ORG-068 | Following all posts: watches of a new kind. Next wave |
| ACC-024, ACC-025 | Account screens and preferences: the settings of ADR-034 |
| ACC-072 | Inspect in Share: with COL-043 |
| ACC-073 | A notification about a page the reader cannot see needs a form without the title. Next wave |
| ADM-028 | A screen for mail settings: the settings of ADR-034 |
| ADM-035, ADM-036 | Instance templates and statuses in the console: the settings of ADR-034 |
| EXC-012 | PDF design: a cover and a logo in the PDF writer. Its own wave |
| PLT-016, PLT-019, PLT-026, PLT-027, PLT-037, PLT-041 | Analytics and automation details, and places for apps: phase 7 work, kept with automation for the instance (roadmap) |
| OTH-010 to OTH-013, OTH-023 | Databases: person, tag and page columns, views and entries as pages are one wave of their own |
