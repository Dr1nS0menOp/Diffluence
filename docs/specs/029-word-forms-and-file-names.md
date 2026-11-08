# 029: Word forms, file names, and two checks from the self-test

Status: built on branch `wave-t`, waiting for the product owner's acceptance session
Phase: ADR-037, and the two points the self-test of spec 025 left open.

## Problem

ADR-037 asked that `plans` finds `plan`, that a file name is found the same way in the search box and on the Files screen, and that the install guide says how large the database must be. The self-test of spec 025 also left two points that were only read in the code, not measured.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | A typed word becomes an OR of its forms: the word, its stem, and the stem with the plural endings `s` (or `ies` after a `y`) and `en`. The stem strips one of `ies`, `es`, `en`, `s`, and keeps at least three letters | The index keeps words as written (`'simple'`, migration 0004), and the product is used in more than one language. A dictionary per language would need a second index and a choice of language per page. These endings cover the common plurals of English and Dutch, and cost the index little (`docs/operations/limits.md`) |
| D2 | Quoted words, fields such as `title:plan`, wildcards and numbers get no other forms | Quotes and fields ask for exactly what was typed. A number has no plural |
| D3 | The exact title stays first | The order of spec 028 is unchanged: the title as typed, then every word in the title, then the rest. Forms only widen what is found |
| D4 | The Files screen matches a name with the same words as the search box: whole words, and the start of the last word typed | One way to find a name everywhere. `rep` finds `report.pdf`, `port` does not. This replaces the match inside a name |
| D5 | The install guide names about 2.5 CPUs for the database at 5,000 people | Spec 028 measured 1.5 CPUs as too tight |
| D6 | A test holds the limit on wrong passwords per address, also with good logins of another account in between | The self-test read in the code that a good login gives an attempt back. Measured here, it only gives back its own attempt: wrong passwords on other accounts stop at the same 50 per address and window as without good logins. Nothing needed fixing; the test keeps it so |

## Left

- A full measurement with the setup of spec 028 (the application under the load model). This spec measured PostgreSQL alone.
- Word forms of other languages, and irregular forms such as `children`.
- Another point of the self-test needs a decision of the product owner first, because it touches ADR-013.

## Sources

None outside this repository: the endings are general knowledge of English and Dutch plurals.
