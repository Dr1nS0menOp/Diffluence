# 080: Search after the screens study, waves SR1 and onwards

Status: waves SR1 and SR2 built (SR2 in spec 086); SR3 and SR4 planned
Phase: before 1.0. The feature freeze of ADR-043 keeps new parts for after 1.0; on 2026-11-15 the product owner put search first among the new work and allowed new search features before 1.0. Changes to the built search screens are rebuilds, not new parts.

## Problem

The study of 15 November (`docs/reports/2026-11-15-screens-study.md`, ADR-045) completed blueprint 03 from pictures of another product's screens. Search is what people do most (ADR-035), so its lines come first. The study added few lines to chapter 8 itself: most lines that touch search are shared conventions of chapter 1 (the mark of an item's kind, an empty result, a part that loads) and lines in other chapters that name search. This spec collects every one of them, says what exists and what changes, splits the work into waves, and specifies the first.

Before building, the permission and leak paths of the built search were checked again: results, counts, suggestions, filters and the way back to a result. Two of those paths showed more than a reader may see. Both are closed in SR1 (see "What the check of leak paths found").

## The lines in scope

Found with `rg -n -i "search|find|filter|query|DQL" docs/blueprint/03-functional`, then read one by one. Lines about filtering other lists (the administration's lists, the audit log, rules, templates, files, people) are not search and stay with their own screens. The status is the one before SR1.

| Line | Status | What exists | What changes | Wave |
|---|---|---|---|---|
| NAV-090 | new | The search box in the middle of the top bar (spec 028, ADR-035) | Three groups, with Create directly after search | SC1 (the frame); the box stays its own component, `QuickSearch` |
| NAV-093 | new | The box takes the room there is up to `--size-search-main` | Search and Create as one group in the middle | SC1 |
| ADM-133 | new | The box names the organisation in its placeholder (spec 038) | Nothing for search; changing the name is the administration's | — |
| NAV-156 | new | Lists show a page's emoji or the page icon (`PageMark`) | The kind of every item before its title: page, home page, blog post, database, folder, link; a file's kind from its name | SR1 for the results and the box; other lists in their own waves |
| NAV-175 | new | A sentence: nothing found, try removing a filter | A heading, short paragraphs whose ways forward are links in the text, a guide link, no button | SR1 for search |
| NAV-177 | new | `Loading`: a muted sentence | A framed box with a bold sentence and pale lines in the shape of the result | SR1: `LoadingBox`, used by the results; other parts in their own waves |
| NAV-158 | new | Links in text take the link colour | Underlined; a link that leaves the instance has a mark and says so | Used by SR1 in its new texts; the rest with the frame |
| ORG-015 | partly | "Below a page" works through the address | A control that chooses the page | SR1 |
| ORG-017 | partly | Several people work through the address | Each chosen person a chip | SR1 |
| NAV-121 | new | People pickers find by the start of the name | By the start of any word of the name | SR2 |
| COL-040 | new | The text of tasks is part of the page's text | Tasks as results of their own | SR2, after its study |
| EDT-221 | new | No tabs block yet | Text in every part of a tabs block is found | SR3, with the tabs block |
| SPC-117 | new | No switch per kind of item yet | What search does with items of a kind turned off | SR3, a study with SPC-116 |
| SPC-129 | new | Archived spaces leave quick search; the results find them with the archived filter (ORG-020) | Nothing for search; the line is the archive section's text | — |
| ADM-117 | new | Insights count searches (PLT-015) | The count of searches on the administration's summary | The administration's wave |
| PLT-051 | new | Search insights exist | A Search tab among the insight topics | The insights wave |
| NAV-163 | new | No language setting for search | Its own group of fields when it exists (ORG-033) | With ORG-033 |
| ORG-025 | later | — | The text inside attachments; needs a text extractor, a new dependency | SR4, after 1.0 |
| ORG-033 | later | One set of endings for Dutch and English (ORG-014) | Stemming per space language | SR4, after 1.0 |
| PLT-069 | later | — | A rule condition as a search query | With automation, after 1.0 |
| Roadmap, DQL | wanted, later | The box's panel with filters as buttons whose choices are words of the syntax (spec 033) | A query language that means the same as the buttons, and more | SR4, after 1.0 |

No line asks for saved searches, so none are planned. Recent searches (ORG-001, ORG-007) and the chip of a space (ORG-006, replaced by the panel's Spaces filter in spec 033) are built.

## What exists, and what protects the database

Search is PostgreSQL full-text search (ADR-037), in `internal/search`: tiers that pick rows by what is cheap to know, the newest 2,000 pages first, and only the shown rows read in full (spec 028); word forms (spec 029); trigrams for titles and file names (spec 047); a second entry for the working version of a controlled page (spec 064). The protections stay as they are: search takes at most one connection in four, waits at most a second for one and then answers busy (ARC-233, ARC-234); every statement stops after 3 seconds and says that it gave up; a total stops at 200; a results page costs at most 20 queries (`query_budget_test.go`). The screens are `web/src/views/search.tsx`, `components/quick-search.tsx`, `search-filter-bar.tsx` and `search-filters.ts`.

## What the check of leak paths found

1. **The total.** The picking narrows by restrictions only, so a controlled page a reader may not see (never approved, or withdrawn) could be picked. It was not shown, because every shown row passed `perm_visible_pages`, but it was counted, so a reader could learn that a hidden page holds a word. The rule is now asked about every picked row (at most 201) instead of only the shown ones, in the same single query, and the total counts what it lets through. Adding the controlled pages to the picking's own hidden set was tried first and measured: it asks the rule of control a second time per search, which on the real data cost more than the whole search.
2. **The way back.** The breadcrumbs of a result named every ancestor without asking the rule. Because a restriction to view is inherited, the only ancestor of a visible page the rule can hide is a controlled page that waits for its first approval or was withdrawn. The breadcrumb query marks those by a key lookup, and the rule is asked about the marked ones only when there is one; such ancestors are left out. The page screen built its breadcrumbs the same way and gets the same check, so a page and its search result show the same way back.
3. **Below a page.** Narrowing to a page the reader may not see listed the pages under it that they may see, which tells them where the hidden page sits. It now finds nothing, as if the page did not exist (NAV-005).

Suggestions (titles the reader may see only), filters without words (`perm_visible_pages` in the query and its count), comments (`perm_visible_comments`), files, spaces and people in the box (their own rules, and people only with *view profiles*) were checked and are unchanged.

## Wave SR1: behaviour

1. **The mark of a kind (NAV-156).** Each result and each row of the box starts with a mark: the page's emoji where it has one, otherwise an icon for page, home page, blog post, database, folder or link. A file's mark follows the extension of its name: picture, PDF, document, slides, spreadsheet, or file. A space shows its badge and a person their initials. The mark has the kind as its name for screen readers and as a tooltip, so a kind never rests on colour or shape alone, and the title link keeps the title as its name. A result says whether it is the home page of its space (`home`).
2. **Nothing found (NAV-175).** In the upper part of the area: a heading that names the words, one paragraph that asks to check the spelling, with the similar spelling as a link when there is one (ORG-023; the suggestion covers the words to find, never the filter words such as `label:`, which the link keeps as they were), one paragraph with the wider searches as links in the text, and a guide link apart. The wider searches are the same words without any filter (archived pages and the order kept) and the same search with archived pages; only those that would change something are offered. The guide link follows the documentation address the administrator set, or ours, opens in a new tab and says so (NAV-113, NAV-158). There is no button. Comments that match still show under it, as before.
3. **Loading (NAV-177).** While the first results load, the results' place holds `LoadingBox`: a framed box with the bold sentence "Searching…" and pale lines in the shape of three results. The search box and the filters stay usable. Loading more keeps its button, which waits while it loads.
4. **Below a page (ORG-015).** The filters have a field that finds a page by its title and sets "below a page" in the address. Once chosen, the field shows the page's title, or that the page is gone when the reader may not see it, and a button to change it.
5. **Several people (ORG-017).** The people filter adds a person each time; each chosen person is a chip with a button that takes them out. A chip names the person where the reader may see the profile, and says "A person" otherwise, so an id in a shared address never reveals a name.

## Permissions and audit

Nothing new is granted or recorded. Every result, count, way back, suggestion and filter stays within `perm_visible_pages` at the moment of the search (NAV-006, ORG-011); a page the reader may not see behaves as if it did not exist (NAV-005). The tests for the three paths above are the proof.

## Data

No migration. `search.Result` gains `home`, read from `spaces.home_page_id` in the queries that already join the space.

## Load budget

Each search stays within its query budget of 20 (`TestTheMainScreensStayWithinTheirQueryBudget`): the rule is still asked once per search, now about the picked rows (at most 201) rather than the shown 25, which adds key lookups and no second rule; excerpts are still made for the shown rows only. The breadcrumbs stay one query, with one key lookup per ancestor, plus one query only when an ancestor is a controlled page that waits for approval or was withdrawn. "Below a page" adds one query, only when it is set.

Measured on the real-data dump (60,385 pages, a tenth under control), each the median of seven through the whole application, for a reader of the largest space; the numbers are in the report of this wave (`docs/reports/2026-11-15-wave-sr1.md`). `TestSearchOnRealData` holds each search under 100 ms there: run it with `REALDATA_PACKAGE=./internal/app/ REALDATA_RUN=SearchOnRealData bash scripts/stress/real-data.sh <dump>`.

## Decided on the product owner's behalf on 2026-11-15, may be overruled

- **D1. The scope of SR1:** the rebuilt results screen and box as NAV-156, NAV-175 and NAV-177 describe them, the two search lines that were partly built (ORG-015, ORG-017), and the leak paths above. Where the box stands in the top bar (NAV-090, NAV-093) is the frame's wave, SC1; the box stays one component it can place.
- **D2.** A way back to a result leaves out an ancestor the reader may not see, rather than naming it or hiding the result.
- **D3.** Narrowing to a page the reader may not see finds nothing and is no error.
- **D4.** An empty result offers only searches that look wider and would change something: without the filters, and with archived pages. Removing filters keeps archived pages and the order, because they do not narrow.
- **D5.** The kind of a file comes from the extension of its name, never from its content.
- **D6.** `LoadingBox` and `GuideLink` are general components, used by search first; other screens adopt them in their own waves.
- **D7.** A chosen person whose profile the reader may not see is "A person".

## Waves after SR1

- **SR2:** people pickers by the start of any word (NAV-121), checked against NAV-059 and ACC-036 and its load; tasks as results of their own (COL-040), after a study of how a task's permission follows its page; the marks, empty states and loading box on the People and Spaces tabs of the results.
- **SR3:** the tabs block's parts in search (EDT-221), with the editor wave that builds the block; what search does with items of a kind turned off (SPC-117), a study with SPC-116.
- **SR4, after 1.0:** DQL, the query language of the roadmap, from the syntax and the panel of spec 033; the text inside attachments (ORG-025), which needs a text extractor and so a new dependency, the product owner's choice; stemming per space language (ORG-033) with its settings group (NAV-163); a rule condition as a search query (PLT-069) with automation.

## Acceptance criteria

- [x] A page a restriction or document control hides from the reader is neither listed nor counted, for words in its text or title, near words, by date, titles only and with filters; paging neither skips nor repeats (`internal/app`, `TestASearchCountsOnlyPagesTheReaderMaySee`).
- [x] The way back to a result, and the breadcrumbs of the page itself, never name a page the reader may not see, and the author keeps them (`TestSearchBreadcrumbsNameOnlyPagesTheReaderMaySee`).
- [x] Below a page the reader may not see, nothing is found, with words, without words and with filters; below a page they may see, its branch is found (`TestSearchBelowAPageTheReaderMayNotSeeFindsNothing`).
- [x] A result says whether it is the home page of its space, and its kind (`TestSearchResultsSayWhichPageIsAHomePage`).
- [x] A similar spelling covers the plain words only, and its link keeps the filter words (`TestASpellingSuggestionLeavesTheFieldWordsOut`, `internal/search` `TestWordsToSpellLeaveTheFiltersOutAndKeepATitle`, `views/search.test.ts`).
- [x] Each search stays within its query budget (`TestTheMainScreensStayWithinTheirQueryBudget`).
- [x] The kind of a page and of a file is read as described (`web`, `kind-mark.test.ts`); each row of the box carries its mark (`reading.test.ts`).
- [x] Filters in the address: one value set or taken away, several people each once; the wider searches of an empty result are only those that change something (`views/search.test.ts`).
- [x] In the browser: an empty search shows its heading, links in the text and a guide link that opens a new tab, no button, and passes axe; its link without filters finds the page; a result shows the mark of its kind; the page field narrows to a branch; two people show as two chips and one can be taken out (`e2e/tree-search.spec.ts`).
- [x] The design pictures of the results, the results with filters, the box and its filters are retaken, and an empty result has a picture of its own (`e2e/0-design.spec.ts`).

## How the product owner tests it

1. Search for a word that is in a page you can see and in a page that waits for its first approval, as someone who only reads the space. The count shows only the page you can see.
2. Search for a word that is nowhere. The screen says so in a heading, offers to check the spelling, and offers wider searches as links; "How search works" opens the documentation in a new tab.
3. In the filters, type the title of a page under "Below a page" and choose it. Only pages under it remain. Add two people under "Written or edited by"; each shows as a chip, and Remove takes one out.
4. Type in the search box: each row starts with the mark of its kind.

## Provenance

Own design, from the lines of blueprint 03 and the decisions in `docs/blueprint/10-decision-log.md`. PostgreSQL documentation on SQL functions and inlining (postgresql.org/docs/16, consulted 2026-11-15).
