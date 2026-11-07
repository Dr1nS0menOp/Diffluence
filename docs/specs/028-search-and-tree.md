# 028: Search that carries the load, and a tree that loads by branch

Status: built on branch `wave-s`, waiting for the product owner's acceptance session
Phase: ADR-035. Search comes first, and SPC-031 with it. It also answers a question of the product owner: can one database carry search, writing and reading at once with 5,000 people?

## Problem

People spend about 40 percent of their time in search, and some search more than they write. The self-test of spec 025 found a common word in 50,000 pages to be the first ceiling: about 3.5 searches a second. The load table of blueprint 04.8 has no line for search at all. And a space of 50,000 pages sends 13 to 19 MB to every reader on every load.

## The load model

5,000 people, 10 percent active: 500 people. If 40 percent of their time goes to search, 200 people are searching at any moment. Someone who searches types, reads the results and opens one: say one search each 20 seconds.

| Source | Per person | A second |
|---|---|---|
| Searches | 200 people, one each 20 seconds | 10 |
| The box while typing | About 4 pauses in a search | 40 |
| Results pages | 1 in a search | 10 |
| **Search requests** | | **50** |
| Everything else (blueprint 04.8) | | 130 |
| **Total** | | **180** |

So search is more than a quarter of all requests. It fits the 50 to 80 of blueprint 04.8 section 8.5. A busy morning or an audit can double it, which is why section 3 of the measurements goes past the model.

This model is worked out, and the run below is on one machine with small limits. It is a test of the shape of the load. It does not show that 5,000 people work on real servers (ARC-240).

## Load budget (ARC-238)

| | Class (ARC-233) | Statements | Database time | Writes |
|---|---|---|---|---|
| The box, per pause | Search | 12 to 16 | About 10 ms | None |
| A results page | Search | 8 to 12 | About 14 ms | One line for insights |
| Opening a tree | Reading | 4 to 6 | About 8 ms | None |
| A published page | Saving | Two more rows, through triggers | Under 1 ms extra | Two |

## What the study found

Measured on the code of `main`, with 50,000 pages, 6 spaces, words that follow Zipf's law, nine years of dates, 1 percent restricted, a reader with four of the six spaces, 20,000 comments and 30,000 files.

**Ranking was not the main cost.** A row of the index holds 3.6 kB of words, stored apart. Anything that touches every match has to read that. The old query did so three times: to match, to rank and to count. One search on a common word read 435,000 buffers and took 740 ms when alone.

**Order changes everything.** With a sort on a date and one narrow table, the same search takes 2 ms: PostgreSQL walks from the newest page and stops after a page of results.

**But the planner can be wrong about the very thing that matters here.** A word that is common but only stands in old pages looks like a common word, so the planner walks the dates and reads the text of all pages: 215 ms. The index of words does it in 5.5 ms. It cannot know, because it does not see that word and date go together. Leaving the choice to the planner would make the search for something nine years old the slow one.

**Timing out looked like finding nothing.** Past three seconds the old code answered with no results and no error. During an audit that reads as "it does not exist". Under load this happened all the time.

**The results stopped at 1,000.** `getNextPageParam` ended at the capped total, so the rest could not be reached by anyone.

**Labels and comments were not found by a plain word.** A label needed `label:`, and a word that only stands in a comment ended in "Nothing found".

**Reading depends on the disk.** Opening a page writes to the recent list, and a commit waits for the disk. When another build used the disk, those commits filled the pool and everything waited behind them. That is not search, but it is the same failure: one slow thing takes all connections.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | Stay on PostgreSQL. No engine, no new dependency | It reaches the target. See the comparison |
| D2 | A small table, `search_pages`, kept by triggers on `pages` | A write cannot forget it, there is no Go on the write path, and the previous release keeps it correct without knowing it exists (N and N+1) |
| D3 | Two fixed steps instead of one choice by the planner: read the newest 2,000 pages, and only if that does not fill the page go to the index of words | Both steps have a known worst case. A word that does not fill a page among the newest pages is rare there, so the index has few rows to give |
| D4 | The order: the title as typed, every word in the title, then the rest. Within each, newest first | "Recent pages first" with an exact match on top. Nothing is left out: an old page only stands further on. `ts_rank` goes |
| D5 | The total comes from the picking: it looks up to 200 rows ahead, and stops as soon as a page is full. Past that it says "More than N" | A count cost more than the search |
| D6 | Paging goes on past the total, as far as a person asks | "Find everything" |
| D7 | Spaces and hidden pages are worked out once per search, and only narrow it. `perm_visible_pages` is applied to each row shown | The rule stays the one rule, and counts at this moment. A wrong row in `search_pages` shows nothing |
| D8 | A search that runs out of time says so: `timed_out`, and a line on the screen | "Not known" is not "nothing there". ORG-032 still holds: no error |
| D9 | One connection in four can search at once. Past a second of waiting the answer is 503 with `Retry-After` | Part of ARC-233 and ARC-234, for search. The limit is CPU, so few at a time still answer many |
| D10 | Labels go in the same column of the index as file names, through a trigger | One index, no rewrite of a generated column. Both are rewritten together, so neither wipes the other |
| D11 | Comments get their own section when pages do not fill the first load | A word that only a comment holds must not end in "nothing found", and a common word need not pay for it |
| D12 | Mentions are a tier of their own, at the end | An `OR` with the words kept the index of words from being used |
| D13 | Files in the box go by the words of their name, split at signs, through an index | Reading all file names on every pause in typing brought the whole model down at 30,000 files. `port` no longer finds `report.pdf` in the box. The Files screen still looks inside names |
| D14 | The tree comes in parts: the start, the way to the open page, and branches as they open. 200 rows per level, with Show more | SPC-031 |
| D15 | The filter and the flat views of the tree run on the server | The browser holds only a part of the tree |
| D16 | The full tree route stays for dialogs that list all pages | Move, Content, export and delete open it only when used. See what is left |

## Search on one primary, a standby, or an engine

Isolation is the main question: a heavy search must not make the wiki slow for others.

| | One primary, as built | A PostgreSQL standby takes the searches | Meilisearch 1.12 | OpenSearch 2.19 |
|---|---|---|---|---|
| Licence | PostgreSQL | PostgreSQL | MIT | Apache-2.0 |
| Isolation of CPU | A share of the connections, not of the CPU | Full, apart from the last check | Full | Full |
| After a permission change | At once | Picking may lag, and the rule on the primary still decides per row | Copies go stale: needs the rule as well, and a filter with every hidden page | The same |
| New words found | In the same transaction (ARC-185) | Within replication lag, usually under a second | Only after indexing | After a refresh |
| Typing errors | "Did you mean" | The same | Built in | Built in |
| Word forms | None (ORG-014) | None | Some | Per language, with an analyser |
| Time for one search, in the database | 10 to 14 ms | About the same | 2 ms | 7 to 17 ms |
| At 50 a second | p95 26 ms for the box, through the whole app | Not measured | p95 5.3 ms | p95 10.6 ms |
| Indexing 50,000 pages | 16.5 s migration | Nothing extra | 205 s | 18 s |
| Disk | 29 MB plus the index that already exists | Twice the whole database | 1.6 GB | 126 MB |
| Memory, with 2 GB allowed | Shared with the database | Its own | 646 MB | 1.44 GB |
| When it is down | There is no search, and no wiki | Back to the primary | Needs a way back, so both must exist | The same |
| To run | Nothing new | A second PostgreSQL, which a large installation has anyway (ARC-228) | A new service to back up and rebuild | A heavy service with a JVM |

Engine times are for the engine alone, without the app. Their hidden pages travel in every query: 900 of them here. A reader who may see less costs more. Meilisearch stopped at 200 searches a second in this test with a reset connection, and that was not looked into.

**Advice.** Keep PostgreSQL. An engine is faster per search, and that is not what limits us. It adds a second copy of everything that can go stale, a second thing to back up and rebuild, and a permission filter that grows with what is hidden. For an audit, a word that is found late or a hidden title in a stale copy is worse than 7 ms. The next step, when search slows other work (blueprint 04.8), is a standby. The code is ready for that: picking is one step, and `pickedRows` the other, so picking can move while the rule stays on the primary. That gives real isolation without a new kind of system.

## Measurements

App 2 CPUs and 1.5 GB, PostgreSQL 1.5 CPUs and 2 GB, load tool 0.5 CPU, 20 connections. One Windows machine with WSL, shared with other builds, so numbers vary. Read at 130 a second means page views only, the heaviest form: each one writes.

**1. Before and after, with the same model.** Page views only, 2 people in the instance, no files. Before is the code of `main`.

| Load | Before | After |
|---|---|---|
| Reading alone, 130 a second | p50 7.5 ms | p50 6.5 ms |
| Search alone, 2 a second | p50 339 ms, p95 3.2 s | p50 26 ms, p95 40 ms |
| Reading 130, writing 2, search 10 | Reading p50 15.8 s, 31% failed | Reading p50 6.7 ms, nothing failed |
| **The model:** reading 130, writing 2, typing 40, results 10 | **95% of searches and 95% of reads failed**, at 30 s | Search p95 44 ms, typing p95 28 ms, reading p50 8.5 ms, nothing failed |

**2. The model as it would be lived.** Reading as blueprint 04.8 counts it: page, space, the start of the tree, shortcuts, page facts and the bell. 5,000 people in the instance, 52,000 files, 20,000 comments.

| Load | PostgreSQL 1.5 CPUs | PostgreSQL 2.5 CPUs, the app 1 |
|---|---|---|
| Search alone, 50 a second | Typing p95 26 ms, results p95 51 ms, PostgreSQL at 63% | |
| Reading alone, 130 a second | p50 3.2 ms, p95 18 ms, PostgreSQL at 39% | |
| **The model** | Reading p95 168 ms, nothing failed. Search p50 57 ms, p95 1.0 s, 1 to 3% told it is busy | **Search p95 44 ms, typing p95 32 ms, reading p95 19 ms, nothing failed.** PostgreSQL at 91% |
| 1.5 times the search | Search waits its second and 21 to 29% is told it is busy. Reading p95 181 ms | |
| Twice the search | 45% of searches busy. Reading p95 223 ms, nothing failed | 5 to 10% busy. Reading p95 87 ms, nothing failed |
| Four times the search | Not measured: the load tool gave no result | Not measured |

Both columns have 4 CPUs in all. The app never used more than one, so the second column only moves them to where the work is. Together, search and reading need about 1.0 to 1.3 CPUs of PostgreSQL at the model, so 1.5 is too tight for busy hours.

**3. The worst case.** When the newest 2,000 pages hold none of the typed words, every search goes to the index of words. Each one then takes about 32 ms in the database. With PostgreSQL at 1.5 CPUs the model gave search p50 1.1 s and 56% busy. Reading p95 471 ms, and nothing failed. It came up by itself here: thousands of test pages written during the runs held none of the words.

**4. One search at a time.** With nothing else running.

| Search | Time |
|---|---|
| A word in every page, a common pair, a phrase | 15 to 29 ms |
| A rare phrase, no hit, `OR` | 30 to 53 ms |
| A word only in old pages | 59 ms |
| Labels, a filter by label, a date | 12 to 40 ms |
| Page 200 of a common word | 81 ms |
| `b*t`, a wildcard inside a word | 219 ms: it reads the text of the pages it checks |
| A common word, only comments | 678 ms: comments still go through `perm_visible_comments` for every comment |
| The box: one letter, a common file name | 87 to 90 ms |
| The box: anything else | 16 to 27 ms |

**5. The tree.** The big space, 31,000 pages, as the reader.

| | Before | After |
|---|---|---|
| Opening the space | 275 ms and 13 MB | 26 ms and 0.08 MB |
| Opening a branch | | 13 ms |
| The filter, now on the server | In the browser | 46 ms |
| A flat view | In the browser | 32 ms |
| 20 openings a second | | p95 39 ms, PostgreSQL at 57% |

**6. The engines.** Each with 1.5 CPUs and 2 GB, the same 50,000 pages, filtered by space and by 900 hidden pages. See the comparison above.

## Permissions, tested first

- A restriction counts at once in results, in the total and in the box. Lifting it too.
- Losing a space counts at once.
- A wrong row in `search_pages` shows nothing: the rule decides.
- A hidden child leaves no trace: no arrow, no row, no match, also when asked for by id.
- A branch below a restriction stays closed, also when asked for by id.
- Every tree route needs the space.
- Spelling suggestions keep coming from titles the person may see.

## What is left

- **Dialogs that list all pages** still load the whole tree when opened: Move, Content, export, and the count in Delete. In a space of 50,000 pages that is 13 MB, once, for someone who asked for it.
- **Search does not know word forms.** `plans` does not find `plan`, unless it is the last word typed and the start of it (ORG-014). A real improvement, and it changes what people find, so it is a decision.
- **Typing errors** only give "Did you mean", from titles.
- **The text inside attachments** is still later (ORG-025).
- **A comment with a word** shows only when pages do not fill the first load.
- **Isolation is a share of the connections, not of the CPU.** A standby is the real answer.
- **The top bar** centres search when there is room. On a narrow window the menus on the left push it to the right.
- **Comments as the only filter** take 678 ms for a common word, and a wildcard inside a word 219 ms. Both read more than they should.
- **Opening a page writes** to the recent list, so reading waits for the disk when the disk is busy. It showed while other builds used the same disk.
- **A query count per screen** (ARC-239) is not a test yet. The budget above is measured, not enforced.
