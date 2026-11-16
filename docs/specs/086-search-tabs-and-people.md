# 086: Search, wave SR2: people by any word, tasks as results, the People and Spaces tabs

Status: built in wave SR2
Phase: before 1.0, under the search exception of ADR-043 (spec 080: the product owner put search first among the new work on 2026-11-15).

## Problem

Spec 080 planned SR2 as three parts: fields for people that find a person by the start of any word of their name (NAV-121), tasks as search results of their own (COL-040), and the People and Spaces tabs of the results, which still show a plain sentence where the content tab has the marks of spec 080, the screen of a search that found nothing and the loading box. This spec is the short study the handoff of SR1 asked for, and what was built.

## Study

### How people are found today

Four places find people by typed text, each its own way:

| Place | Route | Rule of who appears | How text matches |
|---|---|---|---|
| Fields for people and groups (pickers, filters, the task report) | `GET /directory` | Needs *view profiles* (ACC-036); everyone active then | Anywhere in the name or the e-mail address, without an index: every person is read |
| Mentions and Share, without *view profiles* | `sharing.directory` | People with a grant in the page's space (NAV-059) | Anywhere in the name |
| The search box | `GET /search/quick` | Only with *view profiles* | Every word whole, the last as a start, through `users_name_words_idx` |
| The People screen | `GET /people` | `perm_visible_people`: everyone with *view profiles*, otherwise people one shares a space with (NAV-059, spec 013) | Anywhere in the name, without an index |

"Anywhere" finds more than NAV-121 asks ("an" finds Johan and Joanna), and it costs a read of every person on each pause in typing. The box already has the right index but asks for every word but the last in full.

NAV-121 asks for one way: each typed word is the start of a word of the name, in any order, without a wildcard. "jan vr" finds Jan de Vries; "vries j" too; "an" finds Anna and Anne-Marie but not Johan. PostgreSQL's simple parser splits "Anne-Marie" into the whole and both parts, so "marie" finds her as well. Who appears stays as it is: the change is in the words, never in the rule.

### The People and Spaces tabs

The People tab asks `/directory`, which answers 403 to a reader without *view profiles*, so a guest sees an error where the screen should say that nobody matched. It shows the e-mail address beside a name that links nowhere. The Spaces tab filters the reader's list of spaces in the browser and shows a plain sentence when nothing matches. Neither shows a mark or a loading box.

### Tasks

`page_tasks` is the index of the tasks in the version readers see: it is rewritten when a version becomes current (`makeCurrent`), and only then. That settles how a task's permission follows its page:

- A page the reader may not see: the task is not found. The rule is `perm_visible_pages`, the same one that lists the page, at the moment of the search (NAV-006).
- A controlled page that waits for its first approval or was withdrawn: `perm_visible_pages` hides it from readers, so its tasks too.
- A controlled page with an approved version and a working version: a working version never becomes current until approved, so `page_tasks` holds the approved version's tasks. A task written only in the working version is not found by anyone until it is approved; the reader never learns of it.
- A twin page of a draft space keeps no task rows (PUB-817), a template has none, and a page in the trash is not published, so none of them is found.
- An archived page or space: left out, as the content tab leaves them out, unless the reader asks for archived pages.

A task result shows the task's text with the typed words marked, whether it is done, its date, the person it is assigned to, and the page and space it stands on, linking to the page. The name of the assigned person is what the page itself shows to the same reader, so it tells nothing new.

`page_tasks.text` has no index of words, so every search for tasks would read the words of every task. SR2 adds one migration with that index.

## Behaviour

1. **People by any word (NAV-121).** In every field for people or groups, in mentions and Share, in the search box and on the People screen, a person or group is found when each typed word is the start of a word of their name. In fields for people, which show the address, text with an @ finds a person whose address starts with it. Up to eight typed words count; letters and digits make a word, everything else separates words. Who appears is decided as before: NAV-059, ACC-036 and `perm_visible_people`.
2. **The People tab.** It asks the directory of spec 013 (`/people`), so a reader without *view profiles* sees the people they share a space with, never an error. Each row has the person's picture, or their initials on their tint, before the name, the name links to the profile, and the job title and department follow where the person shows them. No e-mail address.
3. **The Spaces tab.** Each row starts with the space's badge, the name links to the space, and the key follows.
4. **Tasks as results (COL-040).** A fourth tab, Tasks, lists the tasks whose text holds the typed words, open ones first, then by date and the newest change. Each row: the mark of a task (open or done, in words for screen readers), the text with the words marked, the date, the assigned person, and the page with its space. More loads 25 at a time; the total stops at 200 like the content tab's.
5. **Loading and nothing found.** On the People, Spaces and Tasks tabs, the loading box of spec 080 holds the place while the first answer loads, and a search that finds nothing shows the screen of NAV-175: a heading that names the words, a paragraph that asks to check the spelling, the content tab as a way forward in the text, and the guide link. The filters of the content tab are not shown on the other tabs, because they do not apply there.

## Permissions and audit

Nothing is granted or recorded; nothing changes state. Every new answer goes through an existing rule: `perm_visible_pages` for tasks, `perm_visible_people` for the People tab, the rules of NAV-059 and ACC-036 for fields, and `perm_spaces` for spaces. A test per path proves that a hidden page's task, a person outside the reader's reach and a space the reader may not see are neither listed nor counted. Mutations under "Spec 086, wave SR2" in `scripts/mutation-check.sh` take each rule away.

## Data

Migration `0075_task_words.sql` (expand, `-- transaction: none`): `CREATE INDEX CONCURRENTLY` of a GIN index on `to_tsvector('simple', text)` of `page_tasks`, so writing tasks goes on while it builds. The previous release reads nothing new and keeps working.

It is the first migration that builds an index alongside the tables, and it showed a fault in the runner: a node that waited for the migration lock did so in one blocking statement, which holds a snapshot, and `CREATE INDEX CONCURRENTLY` on the node holding the lock waits for every older snapshot, so the two waited for each other and PostgreSQL failed one node's start (`TestMigrateIsIdempotentAndConcurrentSafe` failed on it). A node now asks for the lock again every 100 ms with `pg_try_advisory_lock`, and the grants after migrating likewise with `pg_try_advisory_xact_lock`.

No index was added for addresses or groups: measured below, they cost little enough without one, and each would have been a migration of its own.

## Load budget

Measured on 20,000 generated people (four times the 5,000 of the goal) and 5,000 groups in PostgreSQL 16, on the development PC, three runs each; the numbers are in the report.

- **Fields for people:** the old query read every name and address, 38 ms whatever was typed. Now names go through `users_name_words_idx`: 8 to 9 ms for a rare start, 11 to 15 ms for one letter that starts a quarter of all names, 12 to 13 ms for two words, of which 7 to 11 ms is reading 5,000 group names; with 500 groups 1.7 ms. Text with an @ reads every address, 7 to 12 ms, paid only by whoever types one. Budget: 15 ms at four times the goal.
- **The People tab:** the query of spec 013 with the word index instead of a read of every name; unchanged in shape.
- **Tasks:** one query for the page of results and one for the count, like comments, through the new index and `perm_visible_pages` once each; held to the search budget of spec 080 (100 ms on the seeded data, `TestSearchOnRealData`, which now times tasks, a field for people and the People tab too) and a query budget of 10 for the request (`TestTheMainScreensStayWithinTheirQueryBudget`).
- No claim is made for 5,000 people beyond what was measured.

## Decided on the product owner's behalf on 2026-11-16, may be overruled

- **D1. Scope:** NAV-121 everywhere people are found by typed text; COL-040 as a tab of the results; the People and Spaces tabs rebuilt as above. A task filter by status or person stays with the task report (BLK-038).
- **D2.** Every typed word must be the start of a word of the name, in any order. A part inside a word ("an" in Johan) no longer finds it.
- **D3.** Fields for people find the start of an e-mail address when the typed text holds an @, because they show the address and administrators type it. Without an @ the text is read as words of a name, so the index of names is never traded for a read of every address. The People screen and the box do not match addresses, because they do not show them.
- **D4.** A name written without a space between its parts, such as "J.Smith", is one word to PostgreSQL's parser: it is found by "j" or "j.smith", not by "smith". Splitting such names ourselves would make the index and the typed words disagree.
- **D5.** The People tab uses the directory of colleagues instead of the list for pickers, so it follows NAV-059 for every reader and shows no addresses.
- **D6.** Tasks get a tab of their own rather than a kind in the content filters, because a task result looks different from a page result and its filters differ.
- **D7.** A task in a working version of a controlled page is found once that version is approved, by everyone at the same moment; until then it is found by nobody, its authors included, because the index of tasks follows the version readers see.
- **D8.** Tasks on archived pages are left out unless archived pages are asked for, as for pages.

## Acceptance criteria

- [x] A field finds a person by the start of any word of the name and, with an @, by the start of the address, and not by a part inside a word or an address; groups likewise by name (`internal/app`, `TestPeopleFieldsFindTheStartOfAnyWordOfAName`).
- [x] Without *view profiles*, mentions and Share still offer only people of the space; the search box and `/directory` offer nobody (`TestFieldsWithoutViewProfilesOfferOnlyPeopleOfTheSpace`, and the tests of NAV-059 and ACC-036 that stand).
- [x] Nodes that migrate at the same moment while an index is built alongside all start (`internal/db`, `TestMigrateIsIdempotentAndConcurrentSafe`).
- [x] The People screen finds by word starts and keeps to `perm_visible_people`: a guest finds the people of their space and not others (`TestThePeopleTabKeepsToWhomTheReaderMaySee`).
- [x] A task is found by the words of its text; a task on a restricted page, on a controlled page never approved, in the working version of an approved page, on an archived page and in the trash is neither listed nor counted; whoever may see the page finds it (`TestTaskSearchFollowsThePageOfTheTask`).
- [x] A task result says done or open, the date, the assigned person, the page and the space (same test).
- [x] The task search stays within the query budget (`TestTheMainScreensStayWithinTheirQueryBudget`).
- [x] Typed text becomes word starts as described (`internal/identity`, `TestNameWordsAreStartsOfWords`).
- [x] In the browser: the People, Spaces and Tasks tabs show their marks and the screen of nothing found with axe passing (`e2e/tree-search.spec.ts`); design pictures of the three tabs (`e2e/0-design.spec.ts`).

## How the product owner tests it

1. In Share, type the start of someone's last name. They appear. Type a part from the middle of a name: they do not.
2. Search for a word that is in a task. Open the Tasks tab: the task shows with its page, date and person. As someone who may not see that page, the tab finds nothing.
3. As a guest, open the People tab of a search: the people of your space show, with their initials; nobody else, and no error.
4. Search for nonsense and open the Spaces tab: the screen says nothing was found and offers the content tab and the guide.

## Provenance

Own design, from the lines of blueprint 03, spec 080 and spec 013. PostgreSQL documentation on text search parsers, `to_tsquery` prefix matching and GIN indexes (postgresql.org/docs/16, consulted 2026-11-16).
