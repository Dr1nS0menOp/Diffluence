# Handoff 2026-11-07: search and the tree

Branch `wave-s`. Spec 028 holds the study, the decisions, the comparison and every number.

## What is built

- **Search:** a small table `search_pages`, kept by triggers on `pages`. A search walks the newest pages first and goes to the index of words only when that does not fill the page. Order: the title as typed, every word in the title, the rest, mentions last, newest first in each. `ts_rank` is gone.
- **Rights:** spaces and hidden pages narrow the search once. Every row shown passes `perm_visible_pages` at that moment.
- **Honesty:** `timed_out` when a search gives up, `capped` with "More than N", and paging past the total.
- **Load:** one connection in four for search, then 503 with `Retry-After`. Table scans are off while picking by words.
- **Found by a plain word:** labels, through the column of file names, and comments in a section of their own when pages do not fill the first load. Files in the box by the words of their name, people by the words of theirs.
- **The box** stands in the middle of the top bar, in a bar of three parts.
- **The tree** (SPC-031): the start, the way to the open page, opened branches, 200 rows a level with Show more. The filter and flat views run on the server.

## For the product owner

- **The size of the database server.** At the model of 5,000 people, PostgreSQL needs more than 1.5 CPUs at busy hours. With 2.5 everything stayed fast. The install guide does not say this yet.
- **Word forms.** `plans` does not find `plan`. Adding it changes what people find, so it is a choice.
- **In the box, a file is found by the words of its name.** `port` no longer finds `report.pdf` there. The Files screen still looks inside names.
- **No engine.** Meilisearch and OpenSearch were measured. Advice: stay on PostgreSQL, and add a standby for search when it slows other work.

## Not proven

- Real servers, and 5,000 real people. This is one machine, shared with other builds, so the numbers move between runs.
- Four times the search: the load tool gave no result.
- The standby: reasoned, not built or measured.
- The first lines of a busy morning: the cache of PostgreSQL was warm in every run.

## Left

See "What is left" in spec 028. The worst: comments as the only filter (678 ms), a wildcard inside a word (219 ms), and dialogs that still load the whole tree.

## Seen on the way

- **Right to left, with a space chip,** the search box is narrow: the menus take the room. Left to right at 1280 pixels it is about 400 pixels wide.
- **One picture test got a taller window.** "The table button and its menu" scrolled, and then a field of a section ended under the sticky top bar. Axe counts only buttons as cover, so whether it passed depended on the gaps between the buttons of the bar: the wider search box closed one. The check is the same, on a page that needs no scroll.
- **Going to a page reveals it once its row has come.** The tree comes in parts, so on a fresh load the parents of the open page are not known at first. The reveal waits for the row, then opens the branches above it once, as before.
- **The sort is still called Relevance** on the results page. It now means the order of spec 028, D4.
