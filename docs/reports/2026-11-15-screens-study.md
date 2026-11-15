# Report: blueprint 03 checked against pictures of another product's screens (15 November 2026)

## What was asked

The product owner asked for blueprint 03 to be checked against 154 pictures of the screens of a running instance of another wiki product, and completed so that nothing those screens do or show is missing: every function, setting, menu item, column and state, and also the formatting and the arrangement of each screen, so that the screens can be rebuilt as the pictures show them. On 2026-11-14 the product owner decided that this may go into the repository (ADR-045, with the exception and its conditions in `docs/blueprint/02-clean-room-and-licensing.md`), and that for arrangement the pictures win over the design standard, while colours, icons and fonts stay ours.

## How it was done

1. **Inventory.** Fourteen analysts each described a run of eleven pictures, element by element: the regions of the screen, every item in order, states, counts, empty states, formatting as roles, and what the screen implies about behaviour.
2. **Second look.** Fourteen more analysts compared the same pictures, with enlarged halves of every wide picture, against the inventory. They added about 1,000 missed elements and corrected about 200 statements, many of them guesses written as facts or cropped screens read as real edges.
3. **Comparison.** Fourteen analysts compared both descriptions with blueprint 03 and the other blueprints, element by element (covered, partly, missing, different on purpose, layout, not a feature), and wrote 332 proposals and 185 layout notes, each tied to a chapter.
4. **Writing.** One writer per chapter (two for chapter 10) turned the proposals into requirement lines, merged duplicates, shaped every line to the decisions in "Where we deliberately differ" and the decision log, and kept a map of every proposal to its lines. Parts that belonged elsewhere were passed on to the right chapter.
5. **Final check.** One pass over all new lines removed duplicates across chapters (about 26), settled contradictions (about 23), made one rule for repeated conventions (Save and Reset, the "new" mark, notes, tables, help links), and removed quoted labels, a brand name, pixel sizes and mentions of the pictures (about 15). Every one of the 517 proposals and notes has an outcome.

The pictures, the inventories and the comparisons stay outside the repository. A few public help pages were read where a picture showed only a name; they are listed in `03-functional/sources.md`.

## What it added

762 requirement lines, 588 with the status **new** and 174 **later**. Blueprint 03 now holds 1,772 requirements.

| Chapter | New lines | Highest number |
|---|---|---|
| 1. The frame and getting around | 90 | NAV-178 |
| 2. Spaces | 73 | SPC-160 |
| 3. Pages | 37 | PAG-165 |
| 4. The editor | 59 | EDT-223 |
| 5. Dynamic blocks | 21 | BLK-090 |
| 6. Files | 18 | FIL-069 |
| 7. Working together | 36 | COL-133 |
| 8. Finding and organising | 12 | ORG-086 |
| 9. People and access | 98 | ACC-171 |
| 10. Administration | 148 | ADM-206 |
| 11. Import, export and publishing | 41 | EXC-089 |
| 12. Analytics, automation and the platform | 110 | PLT-161 (PLT-096 is not used) |
| 13. Other kinds of content | 19 | OTH-043 |

New sections: 1.10 to 1.15 (the sidebar outside a space, help, and the conventions every screen shares), 2.11 and 2.12 (the settings and the look of a space), 3.13 (classification), 5.11 (blocks of document control), 6.7 (watermarks), 7.14 and 7.15 (chat messages, reporting content), 9.9 (where people may connect from), 10.11 (security and data protection), 12.8 to 12.10 (the automation screen, the screen of a rule, connections to other tools), 13.8 and 13.9 (asset registers, questions and answers).

110 lines change the design standard; `docs/design/README.md` lists them under "The arrangement decided on 2026-11-14".

## The largest changes, by area

- **The frame.** Home, Spaces, Recent and Favourites leave the top bar for a global part at the top of the sidebar, which a person can arrange; Recent, Favourites and Spaces open panels beside the sidebar. The strip of spaces and the fixed Home row leave the sidebar of a space, whose header row now leads to the home page. Administration moves from the account menu to a settings button in the top bar. The start screen opens with cards of pages to pick up again.
- **A space.** The sidebar block gets headed groups (shortcuts, content with its filter and a create row, blogs, the space's own optional parts). The settings of a space get one grouping with four pinned sections and folding groups, in which every existing section keeps a place.
- **A page.** The header row carries the publishing state and who else is reading; the star and the lock leave it; the byline loses its date and version and gains reactions; More is grouped, with Watch as a sub-menu.
- **The editor.** The title moves into the strip, the row of tools gets a new order with History and help, the insert dialog gets categories, a grid and a shortcut box, and tables get a button in the active cell and a floating bar under the table.
- **Administration.** One sidebar with tool sections that stay open, a Settings heading with a jump field, and folding groups (Instance, Security, Permissions, Look, Data, later Apps); settings forms sit in a centred column. New: a security overview, data controls, a scan for secrets, an administrator's list of all tokens, a much richer audit log screen, and changing one role in many spaces at once.
- **People and access.** Your account becomes its own area; login policies per group, service accounts, the access check, exports of access, and editable instance permissions shaped as roles.
- **Automation.** A list of rules with columns, filters and labels, a run log, usage and templates, and a builder that shows a rule as a chain of cards with a settings panel, checks per step and readable summaries.

## Decided on the product owner's behalf (may be overruled)

- A settings form keeps **Save** disabled until something changes, with **Reset changes** beside it.
- Where pictures disagreed, the majority was written: no close button on information notes, links inside the instance underlined, a visited colour kept, a lone switch before its label, a tick for a switched-on menu option, tables without lines between rows, numbers aligned to the start.
- The "new" mark lasts until the next feature release (NAV-178).
- The settings column is centred (37 of the 44 pictures of the instance's own administration).
- In the editor: the summary and Discard draft sit in the step that Publish opens; restoring from History goes into the draft; focus starts in the insert dialog's search field; sharing of block templates stays on; an emptied cell keeps its colour.
- Watching stays an entry of the sidebar, not shown by default.
- The default PDF design, and that switching off single shared pages for the instance withdraws them.

## What this study did not prove

- The pictures show what a screen shows, not always what a control does. Where they showed only a name, a line ends with "(needs its own study)" or was left out; every such case is in the writers' notes.
- Several pictures were cropped; the analysts marked where a screen's edge was the picture's edge, but some regions were never visible.
- No line was tested against the running product. Each line is a requirement, to be specified and built in a wave with its own study (`docs/blueprint/09-way-of-working.md`).
