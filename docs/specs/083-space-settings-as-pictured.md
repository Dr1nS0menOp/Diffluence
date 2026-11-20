# 083: The settings of a space, as blueprint 03 arranges them

Status: built in wave SC5 on top of SC1 (spec 078) and SC4 (spec 081), steps 0 to 6 and 9; step 7 (the row of tools of People) and step 8 (column order and sort marks of the tables) moved to the wave of the tool rows (D33, D34). "As built" at the end says what differs from the plan
Phase: the screens of blueprint 03, fourth item of the order in `docs/handoff/2026-11-15-screens-study.md`, second part (wave SC5), after the study of 2026-11-14 (ADR-045)

## Problem

A space administrator keeps a space in shape from one place: its name and key, who may do what, how pages are reviewed and approved, what is in the trash, how the space looks, what it sends to other tools. Today that place has six groups chosen in spec 033 (General; Who can do what; Content; Automation; Import and export; Archive or delete), a section that only lists links (Keep it tidy), and nine screens that are links out of the settings into the frame of the space. The study of 2026-11-14 rewrote how the settings of a space are arranged (blueprint 03, chapter 2, section 2.11): a head that stays in place, four sections on their own, a divider, five groups that fold, every screen of the settings inside the same frame, one way to start a section, and forms that save as one unit. The product owner asked on 2026-11-15 for the screens to be rebuilt as those lines say (ADR-045). This spec plans the rebuild of the settings of a space. It writes no code: SC5 builds on the frame of SC1 and on the shared settings pieces of SC4, and neither has landed yet.

The look stays ours: colours, icons, fonts and every value come from `tokens.css` (ADR-021, 02 "One exception"). Only the arrangement follows the lines.

## Study

### What SC1 and SC4 give this wave

Spec 078 (branch `wave-sc1`, built, not merged) brings:

- the current-row look of NAV-144 and the hover and focus of NAV-145 for every sidebar, the sections of the settings included (spec 078, D16);
- no sidebar button on the Settings frame (spec 078, D4), and the thin line beside the sidebar (NAV-089);
- the space menu of SPC-107 (`components/space-block.tsx`), whose entries lead into the settings: People (to `/access` for those who manage access), Automation (to `/settings/automation`), Space settings, Archive space and Delete space (with `DeleteSpaceDialog`, moved into `views/space-organise.tsx`);
- `SETTINGS_PERMISSIONS` in `components/space-block.tsx`: the permissions that let a person open the settings at all. It repeats what `SECTION_NEEDS` in `views/space.tsx` knows (see "Seen, not changed").

Spec 081 (branch `wave-sc4`, plan only) builds, for the administration, the pieces that this wave reuses rather than makes a second time:

| SC4 piece | SC4 step | What SC5 uses it for |
|---|---|---|
| A typed registry of entries (`views/admin-sections.ts`) and its unit test | 1 | The same shape for `views/space-sections.ts` |
| `SettingsFrame` with a header row that stays, its own scroll, the kept scroll position, the scroll into view of SC4 D7, ellipsis with the full name on hover and focus, the Experimental mark | 2 | The sidebar of space settings, with the differences the lines demand (below) |
| `SettingsSection`: title row with primary action and More, introduction with help link, notices, content; width per entry | 5 | Every section of a space |
| `Notice` in three kinds, `.note` restyled everywhere (SC4 D4) | 6 | The archived note, explanations of Archive and Delete, errors |
| The table rule of NAV-157 for every app table, `TableBox` with an empty state (SC4 D5) | 7 | Every table of space settings |
| Save and Reset changes as one unit, inactive until something changed, with the description (NAV-162, NAV-163) | 8a | Every form of space settings that does not apply at once |
| The empty block of NAV-173 | 9 | Sections a person may fill that hold nothing |
| `ToolRow` (ADM-103) | 10 | The row above People, if SC4 builds it (D16) |

SC4 builds Save and Reset changes inside `SectionForm`, which loads and saves one section of `/admin/settings/{section}`. Space settings save through other routes, so SC5 needs the unit apart from that loader. **Request to SC4:** let step 8a put the unit (the two buttons, their inactive state and description, Reset putting back the stored values) in `components/` as `SaveReset`, and let `SectionForm` use it. If SC4 lands without that, step 1 of this plan extracts it first, with SC4's tests unchanged.

### Where SC5 and SC4 touch the same thing

- **Fold state (SC4 D3).** Both sidebars keep their shut groups in the person's `settings_shut`. Today the settings of a space use the ids `general`, `access`, `content`, `automation`, `exchange` and `danger`, and the administration `overview`, `people`, `content`, `automation`, `look` and `system`: folding Content or Automation in one folds it in the other. SC4 moves the administration to ids that start with `admin-`. SC5 moves the settings of a space to ids that start with `space-` (`space-general`, `space-access`, `space-pages`, `space-look`, `space-connections`), so neither depends on the other landing first, and a generic word such as `look` or `access` can never collide again (D3). The settings of a template keep their one id `template`.
- **`SettingsFrame`.** SC4 step 2 gives the frame a model for the administration and says that the settings of a space pass none of it until SC5. SC5 adds the model for a space. The frame then takes one discriminated description of its sidebar (`kind: 'admin' | 'space' | 'single'`), not a set of flags (D4). The settings of a template use `single`, which keeps today's look (D19).
- **Where the lines differ between the two sidebars**, SC5 follows the lines of 2.11 and does not copy the administration: the head of space settings holds the jump field and has no line under it at any time (SPC-136; the administration's gets a line once scrolled, ADM-060); the four sections at the top are plain entries with icons, not headed tool sections (SPC-138); a group heading has a thin chevron in a column of its own, not a boxed arrow (SPC-146; ADM-062); the band of the current section reaches to the end of the jump field (SPC-147).

### How the settings of a space are built today

- **Route.** `/spaces/:key/settings` renders `SpaceSettingsView` (`views/space.tsx`); the section is `?section=` (spec 033 D6). The list `SPACE_SECTIONS` holds 18 ids. `SECTION_NEEDS` names the permissions that open each part, any one of them enough (specs 036 and 040); every other part needs `space.administer`. A person sees only the parts they may open; with none they see a muted sentence. Each part calls existing routes under `/api/v1/spaces/{id}/...`, each with its own `Access` rule on the server; the browser decides nothing about access.
- **Frame.** `SettingsFrame` with a head of its own (`SettingsBack`: "Back to the space", then the badge and name), the jump field, and six groups that fold. Entries that are screens of their own are links that leave the frame for the Space frame.
- **Sections.** Each starts with an `h1` and then, usually, an `h2` with the same words (spec 033 D7). Most changes apply at once; Details and Space key have their own Save; the review period saves when the field loses focus.

### Every section today and its place in the target

The target is that of SPC-138 to SPC-144. "Address" is the address that opens it after this wave.

| Today (group, entry, address) | Holds | Target (place, entry) | Address after |
|---|---|---|---|
| General, Details, `?section=details` | name, description, animal | Pinned, **Details**; the animal moves to Picture of the space | same |
| General, Space key, `?section=key` | the key, old keys and freeing them | Pinned, Details | `?section=key` opens Details (D7) |
| General, Features, `?section=features` | the switches of the space | General, Features | same |
| General, Keep it tidy, `?section=links` | links to Content, Templates, Archive, Labels, Insights, Automation, Webhooks | goes: each target is an entry (SPC-145) | `?section=links` opens the first section the person may open, as an unknown id does today |
| General, Shortcuts, `?section=shortcuts` | the shortcuts of the sidebar | Pages and files, Shortcuts | same |
| General, Space categories, `?section=categories` | the categories of the space | Pinned, Details, in its fold (SPC-096) | `?section=categories` opens Details |
| Who can do what, Space owners, `?section=people` | the owners, the home page | Pinned, Details (owners as a field, home page in the fold) | `?section=people` now opens **People** (D8) |
| Who can do what, People and roles, `?section=roles` | `SpaceRolesPanel` | Pinned, **People** (SPC-118) | `?section=people`; `?section=roles` opens it |
| Who can do what, Who has access, link to `/access` | the Access screen: tabs Roles and Who has access, presets, inviting a guest | Access, **Permissions** (D9) | `/spaces/:key/access`, in the Settings frame |
| Who can do what, Audit of this space, link to `/audit` | the audit of the space | Access, **Audit of the space** | `/spaces/:key/audit`, in the Settings frame |
| Content, Page statuses, `?section=statuses` | the statuses, typing one's own | General, **Statuses and approvals** (SPC-140) | same |
| Content, Labels, `?section=labels` | a fixed list, renaming | Pages and files, Labels | same |
| Content, Workflows, `?section=workflows` | workflows of document control | General, Workflows | same |
| Content, Published space, `?section=published` (while publishing is on) | the pair of spaces | Pages and files, Published space | same |
| Content, Pages that do not exist yet, `?section=placeholders` | titles that pages link to | Pages and files, **Pages still to write** (SPC-126) | same |
| Content, Content, link to `/content` | the table of pages | Pinned, **Content** | `/spaces/:key/content`, in the Settings frame |
| Content, Templates, link to `/templates` | the templates of the space | Look, Templates | `/spaces/:key/templates`, in the Settings frame |
| Content, Trash, link to `/trash` | the trash of the space | Pages and files, Trash | `/spaces/:key/trash`, in the Settings frame |
| Automation, Rules for pages, `?section=rules` | review period, approval rule, title alignment, picture upload | review and approval to Statuses and approvals; alignment to **Layout**; the picture to **Picture of the space** | `?section=rules` opens Statuses and approvals |
| Automation, Automation, link to `/settings/automation` | the rules of the space | Pinned, **Automation** | same path, in the Settings frame |
| Automation, Webhooks, link to `/settings/webhooks` | the webhooks | Connections, Webhooks | same path, in the Settings frame |
| Import and export, Import a file, `?section=import` | import into the space | General, Import | same |
| Import and export, Export, `?section=export` | export of the space | General, Export | same |
| Import and export, PDF design, `?section=pdf` | the PDF design | Look, PDF design | same |
| Import and export, Public site, link to `/sites` | the public site | Access, Public site | `/spaces/:key/sites`, in the Settings frame |
| Archive or delete, `?section=danger` | Archive or Restore, Delete | General, **Archive the space** and **Delete the space** (SPC-129 to SPC-131) | `?section=archive`, `?section=delete`; `?section=danger` opens Archive the space |

New entries made only of parts that exist today:

| Entry | Place | Address | Made of |
|---|---|---|---|
| Picture of the space | General | `?section=picture` | `AnimalChoice` out of Details and the picture upload out of Rules for pages |
| Layout | Look | `?section=layout` | the title alignment out of Rules for pages (PAG-126) |
| Archive the space | General | `?section=archive` | the archive and restore buttons of Archive or delete |
| Delete the space | General | `?section=delete` | `DeleteSpaceDialog` of SC1 |
| Insights | General | `/spaces/:key/insights`, in the Settings frame | `SpaceInsightsView`, linked today only from Keep it tidy |
| Files | Pages and files | `?section=files` | the body of `FilesView` (D10) |
| Archived pages | Pages and files | `?section=archived` | the body of `ArchiveView` (D10) |

Entries the lines name that are **not** built in this wave, because nothing behind them exists yet and the feature freeze of ADR-043 holds (D1): Import notes (SPC-128, later), Related content (PAG-159, later), Order of pages (SPC-113, SPC-114), Pages nothing leads to (SPC-125), Restricted pages as a report of its own (SPC-065, ACC-163; the Content screen keeps its filter), Single shared pages (EXC-085), Look of the space (SPC-156, later), Watermark (FIL-059, later), a feed of the space (COL-081, COL-126, later), linked instances (PLT-049, phase 8), sections of apps (PLT-039, PLT-097, phase 7), and the header and footer of Layout (SPC-158, SPC-159). Each takes its place in the registry of step 1 when its own wave builds it; SPC-139 to SPC-144 say where.

### What the lines change that is not a move

- **The head** (SPC-136): a back action with an arrow toward the start side and **Space settings** in bold; a row with the badge and name, with no star and no menu; the jump field across the sidebar. It stays in place once its top margin has scrolled away, is opaque, and has no line or shadow under it.
- **The sidebar** (SPC-134, SPC-138, SPC-146, SPC-147): about a quarter of the width; four entries with icons; a divider as wide as the jump field; five groups whose headings carry a thin chevron in a column of their own; entries as text, indented one step past their heading, in compact rows of one height; no lines between groups; its own scroll that keeps its position; the band of the current entry reaching to the end of the jump field.
- **Every screen inside** (SPC-135): Content, Trash, Automation and the other screens open in the large area with the settings sidebar beside them.
- **A section** (SPC-148 to SPC-150): the name as the largest heading with More or the primary action at its end, the description with its help link, notes, a row of tools, the content; parts with visible sub-headings; one of two widths.
- **Forms and actions** (SPC-151 to SPC-153): one field per row, tiles for a choice among a few, Save and Reset changes, an action section ending in one primary button with no Cancel.
- **Tables** (SPC-154, SPC-155): the title first and widest, actions last, a sort mark, filters above that apply at once.

## Lines in scope

Every line of chapter 2, section 2.11, the lines of other sections of chapter 2 that name a section of space settings this wave builds or moves, and the lines of chapter 1 that space settings need, with the status after this wave. "Not this wave" names a line that needs a feature that does not exist yet or its own study. Lines in other chapters that describe the inside of one section (PAG-132, PAG-139, PAG-155, PAG-157, ORG-080, EXC-049, EXC-064, EXC-085, ACC-163, PLT-105 to PLT-152, FIL-059) keep their status: each belongs to the wave of its section, which builds it inside the frame and the section head of this wave.

### Chapter 2, section 2.11

| Line | Was | After SC5 | What changes |
|---|---|---|---|
| SPC-134 | new | built | Settings frame, a sidebar about a quarter wide, a thin line beside it (SC1); only permitted entries; nobody without any sees Space settings (D6) |
| SPC-135 | new | built | One section at a time at its own address; the screens of their own open in the same large area (D5) |
| SPC-136 | new | built | The head of the sidebar, as above |
| SPC-137 | new | built | Narrowing by name, groups open while it holds text, Enter opens the first match, a muted sentence for none. The study it asks for is decided in D11 |
| SPC-138 | new | built | Automation (while on), People, Content, Details; the divider; the five groups |
| SPC-139 | new | partly | General: Features, Archive the space, Delete the space, Export, Import, Picture of the space, Statuses and approvals, Workflows, Insights. Import notes and Related content wait |
| SPC-140 | new | built | Statuses, typing one's own, the review period and the approval rule in one section; Workflows follows it |
| SPC-141 | new | partly | Access: Permissions, Public site, Audit of the space. Single shared pages waits for EXC-085 |
| SPC-142 | new | partly | Pages and files: Pages still to write, Files, Trash, Archived pages, Labels, Shortcuts, Published space (while on). Order of pages, Pages nothing leads to and Restricted pages wait |
| SPC-143 | new | partly | Look: Templates, Layout (title alignment), PDF design. Look of the space, Watermark and the header and footer of Layout wait |
| SPC-144 | new | partly | Connections: Webhooks. The feed, linked instances and apps wait |
| SPC-145 | new | built | The old groups and Keep it tidy go; every earlier section keeps a place, and its old address leads there (D7) |
| SPC-146 | new | built | Indent, chevron column, all groups open at first, several open, no lines between groups, scroll kept. Shut groups kept per person (D12) |
| SPC-147 | new | built | The reach of the band and of the hover tint |
| SPC-148 | new | built | Through `SettingsSection` of SC4 |
| SPC-149 | new | built | Visible sub-headings; spec 033 D7 ends for space settings |
| SPC-150 | new | built | Full or column per entry in the registry (D13) |
| SPC-151 | new | partly | One field per row, tiles for the title alignment. No number on a scale and no preview in the sections of this wave |
| SPC-152 | new | built | Save and Reset changes on every form that does not apply at once |
| SPC-153 | new | built | Archive the space, Delete the space, Export and Import as action sections |
| SPC-154 | new | built | Through the table rule of SC4, with column order and actions in the tables of space settings |
| SPC-155 | new | not this wave | No table of this wave has several rows per item or two levels; it comes with the report of ACC-163 and with template sets |

### Chapter 2, other sections

| Line | Was | After SC5 | What changes |
|---|---|---|---|
| SPC-009, SPC-012, SPC-013, SPC-014, SPC-015 | built | built | Now fields of Details |
| SPC-091 | new | partly | One form in the order of the line, with Save and Reset changes. The line on how required fields are marked waits for NAV-165 (SC4 left ADM-088 to its own wave) |
| SPC-092 | new | built | The badge at the top of Details, without a control |
| SPC-093 | new | partly | The old keys and freeing them in the same form. The information button waits for NAV-152, as SC4 left ADM-089; the explanation stands as a hint under the field meanwhile |
| SPC-094 | new | built | The owners as a people field, at least one |
| SPC-095 | new | partly | The administrators as overlapping pictures with a link to People filtered to the role. Who created the space and when needs the space to carry its creator, an API change: it waits |
| SPC-096 | new | built | The fold of NAV-169 with description, categories and home page |
| SPC-115 | new | partly | The switches of today in one bordered card under the heading band of the first group. Kinds of item and parts of the sidebar have no switch of a space yet: they wait (D14) |
| SPC-116 | new | built | Switches that apply at once, with no Save, hint, badge or lock; an experimental one only while its part is on |
| SPC-118 | new | partly | Picture or group mark, name with second line, role in the row with its explanation, More at the end. The checkbox column waits for SPC-122 (D15) |
| SPC-119 | new | partly | The service account line and the Guest mark. The group line of ACC-126 and the e-mail address of ACC-023 need fields the list does not carry: they wait |
| SPC-120 | new | built | Search, the kind and role filters, the add button at the end (D16) |
| SPC-121 | new | built | Several roles one at a time; More with Remove access; Export as CSV in the title row's More |
| SPC-122 | new | not this wave | Changing many holders in one step needs its own study and a route that summarises all of them at once |
| SPC-123 | new | not this wave | A note of news needs the dismissal of NAV-150, which SC4 did not build |
| SPC-126 | new | built | A table of two columns, starting the missing page from its row (`/spaces/:key/new`, EDT-064) |
| SPC-127 | new | built | The entry stays when empty, with one sentence, replacing spec 023 D8 |
| SPC-129, SPC-130 | new | built | An information note, one primary button, a confirmation; Restore for an archived space |
| SPC-131 | new | built | The explanation, archive and export offered first, the danger button that asks for the key |

### Chapter 1, lines space settings need

| Line | Was | After SC5 | What changes |
|---|---|---|---|
| NAV-113 | partly (SC4) | built | Every entry of a space has its introduction (D17) |
| NAV-160, NAV-162, NAV-163 | partly (SC4) | partly | Built for space settings too; Your account follows in its wave |
| NAV-161 | new | partly | Switches in Features and for typing one's own status apply at once with a short message; the role in a row of People as today |
| NAV-166 | new | built | A `Switch` component (D14) |
| NAV-167 | new | partly | The card of Features. Lists of switches elsewhere follow in their waves |
| NAV-169 | new | partly | The fold of Details |
| NAV-173 | partly (SC4) | partly | The empty block for Shortcuts, Labels' fixed list, Webhooks, Automation and Templates of a space |
| ADM-157 | new | partly | The Experimental mark beside the title of Automation and Published space |

## Behaviour

1. **Opening.** Space settings open from the space menu (SC1) at `/spaces/:key/settings`. Without a section in the address, Details opens when the person may open it, else the first entry they may open in the order of the sidebar (D18). Someone who may open no entry gets the sentence that the page does not exist or they cannot see it, in the Plain frame, and the space menu shows them no Space settings (as SC1 does).
2. **The head.** At the top of the sidebar: a back action, an arrow toward the start side named **Back to the space**, then **Space settings** in bold. Under it a row with the badge and the space's name, which is not a link and has no star and no menu. Under that the jump field, across the sidebar, with a search mark. The head stays in place once its own top margin has scrolled away; it is opaque and has no line or shadow under it.
3. **The four entries.** Under the head, each with its icon and name: **Automation** (only while the part is on for the installation and the space, with the Experimental mark), **People**, **Content**, **Details**. Then a divider as wide as the jump field, with space before and after it.
4. **The groups.** **General**, **Access**, **Pages and files**, **Look**, **Connections**, in that order, with their entries in the order SPC-139 to SPC-144 give, leaving out those that wait (D1). A group heading is a button in the same weight and colour as the entries, with a thin chevron in a column of its own further to the start, pointing down while open and toward the end side while shut. Entries are text, indented one step past the heading, in compact rows of one height. All groups are open on the first visit; several can be open at once whatever is selected; a person's shut groups are kept in `settings_shut` under the `space-` ids. No lines between groups, nothing after the last entry. A group with no entry the person may open is absent.
5. **The current entry and hover.** The open entry has the look of NAV-144 and `aria-current="page"`. Its band ends where the jump field ends; it starts at the sidebar's start for the four entries and at the chevrons' column for an entry of a group. The hover tint of NAV-145 has the same shape. A long name ends in an ellipsis and shows in full on hover and focus.
6. **Scrolling.** The sidebar has its own scrollbar over its full height and scrolls apart from the large area. Going to another entry keeps its scroll position. An entry opened from an address or the jump field that lies out of view is scrolled into view once, without motion (SC4 D7).
7. **The jump field.** Typing narrows the list to the entries whose name, or a name an entry took over, holds the words (D11), and shows every group that has one open while the field holds text. Enter opens the first match. With no match the list says so in one muted sentence. Escape empties the field.
8. **Screens of their own.** Content, Trash, Permissions, Audit of the space, Public site, Templates, Automation, Webhooks and Insights keep their addresses and open in the large area with the settings sidebar beside them, their entry marked as current (D5). Their own filters stay in the address as today. Files and Archived pages open in the settings at their `?section=` address; from the space menu they keep opening in the frame of the space (D10).
9. **A section from top to bottom.** Through `SettingsSection` of SC4: the name as the one `h1`, with the section's primary action or its More at the end of that line; one or two sentences on what the section is for, with a help link where a document exists (D17); notes about the whole section, such as that the space is archived, as a `Notice`; a row of tools where the section has a list; the content. Parts have a visible `h2` with a line of explanation under it and more space above than below, set apart by space alone. No breadcrumb, no save bar.
10. **Widths.** Full width beside the sidebar for Automation, Content, Permissions, Audit of the space, Trash, Files, Archived pages, Templates, Webhooks, Insights and Pages still to write; one column of reading width centred in the large area for every other entry, People and Public site included (D13).
11. **Details.** One form, top to bottom: the space's badge, large, with no control; the name, the key with its hint (SPC-093) and the old keys under it, each with a quiet **Free this key** that applies at once; the owners as a people field with a small picture before each name, at least one; then, read only, the administrators as up to five overlapping pictures and a quiet link **See all administrators** to People filtered to that role; then the fold **More details**, shut at first, with the description, the categories and the home page. **Save** and **Reset changes** close the form. Save sends only what changed, part by part, in this order: name and description, owners, categories, home page, key; it stops at the first refusal, says in an error notice which part was refused and which were saved, and keeps what was typed for the parts not saved (D2). A changed key is saved last, because the address follows it: the form then opens at the new address. A person with only `space.look` sees the badge and the fold, open, with the home page as its only field (D6).
12. **Picture of the space.** The animal or character of `AnimalChoice`, with Save and Reset changes, for `space.administer`; under a sub-heading, the uploaded picture with Upload and Remove, which apply at once, for `space.look`.
13. **Statuses and approvals.** Three parts. **Statuses**: the list of statuses with their colours, adding and removing applying at once as today, and the switch **Editors may type their own status**, which applies at once (NAV-161). **Review**: the review period in months, a number field as wide as its content with its unit after it, with Save and Reset changes. **Approval before publishing**: the checkbox and the fixed approvers, with Save and Reset changes. Each part shows only to those who may change it (`space.statuses` for the first two, `space.approvals` for the third). The mode chooser of PAG-132 is not built here.
14. **Layout.** The default title alignment of new pages as two tiles, Start and Centre, each with a bold title and a muted line, the chosen one with a filled radio, with Save and Reset changes.
15. **Archive the space.** An information note saying that the space becomes read-only for everyone, leaves the Spaces menu, the directory and quick search, can still be found through a search filter, and can be restored at any time; then one primary button, **Archive the space**, which asks first. For an archived space the note says that it is archived, and the one primary button is **Restore the space**, which does not ask.
16. **Delete the space.** The explanation that the space goes to the trash for the retention period and an administrator can restore it from there; links to Archive the space and to Export as something milder; then one button in the danger style, **Delete the space**, which opens `DeleteSpaceDialog` and asks for the key.
17. **Export and Import.** Heading, explanation, the choices under medium-bold sub-headings, one primary button at the foot named for the action, no Cancel (SPC-153). What each offers does not change.
18. **Features.** One bordered card with rounded corners, a muted heading band **Features of the space**, then one row per switch of today with its icon, its name and the switch at the row's end, thin lines between rows. A switch applies at once and a short message says it was saved, also to screen readers. A switch whose experimental part is off for the installation is absent; one the instance turned off is absent (NAV-004).
19. **People.** The title row has **More** with **Export as CSV**. Above the list one row: a search by name at the start, then the filters **Kind** (people, groups, service accounts, all members, all guests) and **Role**, nothing chosen at first, applying at once and kept in the address; at the end **Add people**, which opens today's way of adding holders. One row per holder: the picture or group mark; the name, with the Guest mark for a guest and the line "Service account" for one; the role chosen in the row as today, several roles one at a time (ADR-040), a combination without a name muted under its short name, with the button beside it that explains what the role allows and how the holder got it; **More** at the end with **Remove access**, which asks first when it takes away view, as today. A change that would leave the space without an administrator is refused (SPC-053). Only the holder and role columns have a heading.
20. **Permissions.** The Access screen without its tabs: a part **Who has access**, the list of everyone who can open the space and how (directly, through a group, as all members or all guests) with its search and Export as CSV; for `space.administer`, a part **Start from a set** with the two presets; and the part to invite a guest. Who holds which role is changed in People, which the introduction links to.
21. **Pages still to write.** A table of two columns: the missing title, and the pages that link to it. For a person who may create pages, each row ends with **Create page**, which starts the page with that title under the linking page (`/spaces/:key/new`). With no rows the entry stays and the section says in one sentence what would appear here.
22. **Experimental entries.** Automation and Published space are absent while their part is off; their addresses still answer, with today's sentence that the part is off and where to turn it on (SC4 D8). While on, the entry and the title carry the Experimental mark.
23. **Words.** Every entry's name and introduction, the group names, the head and the empty blocks go through `en.json`. "Pages that do not exist yet" becomes **Pages still to write**; "Audit of this space" becomes **Audit of the space**; "Back to the space" stays as the name of the back action.

## Permissions and audit

Nothing about access changes, and this is checked rather than assumed:

- Each entry of the registry names the permissions that open it, any one enough. For every part that moves, the entry that now holds it opens for **at least** everyone the old part opened for, and each part inside it keeps its own test: Details opens for `space.administer` or `space.look`, and shows to a `space.look` holder only the home page, which the old Space owners section showed them; Statuses and approvals opens for `space.statuses` or `space.approvals`; Layout for `space.look`; Picture of the space for `space.administer` or `space.look`, each part by its own permission; People and Permissions for `space.manage_access`; Audit of the space for `space.audit`; Public site for `site.publish`; Templates for `space.templates`, or `template.propose` while guided templates are on; Automation and Webhooks for `space.automation`; Labels for `space.labels`; Workflows for `space.workflows`; Published space for `space.administer` or `space.release`; Shortcuts for `space.look`; Export for `space.administer` or `space.bundle`; every other entry for `space.administer`. A unit test holds the table of today's `SECTION_NEEDS` and proves it (step 1).
- The set of permissions that lets a person open space settings at all is derived from the registry, and the space menu of SC1 uses it instead of its own `SETTINGS_PERMISSIONS`. The set does not change: the test compares it with SC1's list.
- A screen of its own opened at its address by someone who may open no entry of space settings renders in the Space frame, exactly as today, so a person who could reach it before, such as a reader with `analytics.view` on `/insights`, still can; the server decides what data they get (D6).
- No route of the server is added, removed or given another rule. Every moved part calls the same routes, which keep their `Access` rule in `httpapi.Router`, and the route walker stays as it is. The server keeps refusing what it refuses today; the browser's checks only spare a person a broken screen.
- No audit or outbox event is added or changed: every save is the same request as before. A Save of Details that changes three parts makes the same three requests that three sections made before, each with its own transaction and audit event.
- The guards that ask first stay: removing view in People, deleting a space by its key (SPC-073), the summary before a role change (SPC-057), and the last administrator (SPC-053).

## Data

No migration and no change to the API. The preferences document keeps `settings_shut`; the new group ids are `space-general`, `space-access`, `space-pages`, `space-look` and `space-connections`, within the form `^[a-z][a-z0-9-]{0,39}$` the server already checks. The old ids are no longer read by space settings and stay in a person's list until it is written anew. The list holds at most 30 ids; the old ids of both sidebars, the template's id and the ten new ids of SC4 and SC5 together are 23, so no person can reach the limit through them.

## Load budget

- **Server:** no route changes, so `internal/app/query_budget_test.go` keeps every budget. Nothing on saving pages, reading or search.
- **Opening Details** reads the space (already held), the owners (`GET /spaces/{id}/overview`, which the old section read), the old keys (which Space key read) and the holders for the administrators' pictures (the list People reads, an indexed read of the reading class, only for those who may manage access; others see no pictures). The categories and the tree for the home page load only when the fold opens, so a visit that does not open it costs three small reads where the three old sections cost the same three across three visits.
- **Saving Details** makes at most five writes in sequence, the same requests and the same rules as the separate sections made.
- **Screens of their own** make exactly the requests they make today; the frame around them adds none, because the space is the one the sidebar and the screen both read through `spaceByKeyQuery`, cached.
- **People** filters in the browser over the list it loads today; no request is added. A space whose direct holders run into the hundreds still loads them in one read, as today; paging is a later need, not this wave.
- **Browser:** the sidebar renders about 35 entries; the head that stays and the kept scroll position use the mechanism of SC4, one `IntersectionObserver`, no scroll listener.

## Upgrade notes

- **Bookmarked addresses.** Every address of today still opens something sensible, without a server redirect:
  - `?section=details`, `features`, `shortcuts`, `statuses`, `labels`, `workflows`, `published`, `placeholders`, `import`, `export` and `pdf` keep their section, some under a new name or in a new group.
  - `?section=key` and `?section=categories` open Details; `?section=roles` opens People; `?section=rules` opens Statuses and approvals; `?section=danger` opens Archive the space. Each replaces the address in the history, so Back does not loop (D7).
  - `?section=people` opened Space owners and now opens People, the list of who holds which role; the owners are in Details (D8).
  - `?section=links` opens the first section the person may open, as any unknown id does today.
  - `/spaces/:key/content`, `/trash`, `/access`, `/audit`, `/sites`, `/templates`, `/insights`, `/settings/automation` and `/settings/webhooks` keep their addresses and now open inside the Settings frame for those who may open space settings.
  - `/spaces/:key/files`, `/archive`, `/labels`, `/people` and `/overview` do not change.
  - A key change now lands on `?section=details` of the new key, where it landed on the first section.
- **Mixed releases.** Only the frontend changes. During a rolling upgrade a browser gets the frontend of whichever node served it; both talk to the same API. A person whose browser holds the old frontend sees the old sidebar until a reload.
- **Folded groups.** On the first visit every group of the new sidebar is open, because the `space-` ids are not in anyone's list yet.
- **Where things went.** The changelog lists each moved part and its new place: the key, the owners, the categories and the home page to Details; the animal and the picture to Picture of the space; the review period and the approval rule to Statuses and approvals; the title alignment to Layout; Archive or delete as two sections; People and roles renamed People; Who has access renamed Permissions, without tabs; Keep it tidy gone; the screens of their own inside the settings.
- **Documents.** `docs/operations/public-sites.md` and `published-spaces.md` name Space settings and the section Published space, which stay; they need no change. `docs/design/README.md` changes in step 9.

## Design pictures and end-to-end tests

### Pictures to take again (all in `web/e2e/design-baselines/`)

`space-settings`, `space-roles`, `space-roles-open`, `space-access`, `space-access-people` (the tabs go, so this picture is removed and its state is part of `space-permissions`), `space-audit`, `trash`, `content`, `sites`, `templates`, `templates-states`, `templates-none`, `templates-fifty`, `rules`, `webhooks`, `space-insights`, `publishing-settings`, `publishing-start`, `publishing-verified`, `workflow-editor`, `workflow-editor-phone`. The pictures of the frame of a space (`files`, `archive`, `labels`, `space-people`) do not change. The pictures of the settings of a template do not change (D19). The builder lists every picture that changed in the report, and the product owner sees each before and after.

The test `SpaceSettingsView: space settings` waits for the heading Details and keeps doing so.

### New pictures

Named `SpaceSettingsView: …` in `web/e2e/0-design.spec.ts`, each looked at in light, dark, narrow and right-to-left:

- the sidebar with the four entries, the divider, the five groups, one shut, and Automation with the Experimental mark (`space-settings-sidebar`);
- the sidebar scrolled, with the head in place and the current entry deep in Connections (`space-settings-scrolled`);
- the jump field holding a word that matches an old name, and one that matches nothing (`space-settings-jump`, `space-settings-jump-none`);
- Details at rest, and with the fold open and a change made, Save and Reset changes active (`space-details`, `space-details-changed`);
- Details for a person with only `space.look` (`space-details-look`);
- Statuses and approvals with its three parts (`space-statuses`);
- Features with its card and switches (`space-features`);
- Archive the space, for an active and for an archived space (`space-archive`, `space-archive-restore`), and Delete the space (`space-delete`);
- People with a filter set (`space-settings-people`), and Permissions (`space-permissions`);
- Pages still to write, with rows and empty (`space-placeholders`, `space-placeholders-empty`);
- Layout with its tiles (`space-layout`);
- a section in the column with very little content and Content at full width, because a picture only guards the state it shows (`space-settings-column-short`, `space-settings-full`).

### End-to-end tests

- **New `web/e2e/space-settings.spec.ts`:** the head's back action returns to the space; the four entries and the five group buttons in order; a group folds, stays shut across a reload and another section, opens on a jump, and folding it does not fold a group of the administration; the jump field finds an entry by its name and by an old name, Enter opens it, no match says so; the scroll position stays between entries; an entry deep in Connections opened by address shows in the sidebar; the current entry has `aria-current`; Content, Trash and Automation open with the settings sidebar beside them; with automation off, Automation is absent and its address says the part is off; axe on each state; right-to-left and 360 px.
- **Bookmarks:** each old address of the upgrade notes opens the entry named there, and Back after an alias does not return to the alias.
- **Details:** Save and Reset changes inactive on a fresh form, active after a change; Reset puts back the stored values; changing name, owners and key in one Save changes all three and lands on the new key's address; a refused owner change (no owner left) says which part was refused, keeps what was typed and saves the parts before it.
- **Access, written first:** a member with only `space.view` gets "does not exist" on `/spaces/:key/settings` and on every `?section=` id, old and new, and sees the screens of their own as today in the frame of the space; a holder of only `space.look` sees Details with only the home page, Layout, Picture of the space and Shortcuts, and nothing else; a holder of only `space.audit` sees only Audit of the space; a proposer of templates sees only Templates while guided templates are on. The server's routes behind each entry still answer 403 as before.
- **Archive and Delete:** archiving asks first and the section then offers Restore; deleting asks for the key.
- **Files that visit space settings** and need their addresses or headings checked: `0-design.spec.ts`, `access.spec.ts`, `control.spec.ts`, `editor.spec.ts`, `exchange.spec.ts`, `feedback.spec.ts`, `installation.spec.ts`, `links-tables.spec.ts`, `new-version.spec.ts`, `organising.spec.ts`, `people.spec.ts`, `platform.spec.ts`, `publishing.spec.ts`, `reading.spec.ts`, `roles.spec.ts`, `space-home.spec.ts`, `templates-form.spec.ts`, `walk.spec.ts`, `writing.spec.ts`, and `frame.spec.ts` of SC1 (the space menu's People now opens the People section).

### Unit tests (written first)

- `web/src/views/space-sections.test.ts`: every entry sits in exactly one place; ids are unique; every id of today opens an entry, directly or by alias; the four entries and the groups come in the order of SPC-138 to SPC-144; an experimental entry is left out while its part is off; for every old part, the permissions of the entry that holds it include today's `SECTION_NEEDS` (frozen in the test); the set that opens space settings equals SC1's `SETTINGS_PERMISSIONS`.
- `web/src/components/settings-frame.test.ts`: the jump finds entries by name and by old name, finds pinned entries, and opens shut groups only while it holds text; folding writes `space-` ids only.
- `web/src/views/space-details.test.ts`: which requests a Save of Details makes for a given change, in which order, and where it stops after a refusal.

### Mutations

No rule of the server changes, so no mutation line is added. The unit test of the registry and the access e2e above are the proof that nothing was lost in a move.

## Build plan

Each step is one builder's work, ends with `make check` green, and commits its pictures. Sizes: S up to about 200 changed lines, M up to about 500, L more. Steps 5 to 8 may run in any order after step 4; everything else in the order given.

| Step | What | Lines | Size | Pictures |
|---|---|---|---|---|
| 0 | Rebase `wave-sc5` on `main` once SC1 and SC4 have landed. Check that the settings sidebar has SC1's current-row look, that the administration's fold ids start with `admin-`, and that `SaveReset`, `SettingsSection`, `Notice` and the table rule exist; if `SaveReset` is still inside `SectionForm`, extract it with SC4's tests unchanged. Run the space-settings e2e once as the baseline | | XS to S | none |
| 1 | **The registry.** `web/src/views/space-sections.ts`: one typed list of entries (id, name key, place, icon, width, needs, experimental part, address as a section id or a path, aliases, names taken over, documentation path) and the order of groups, reusing the entry type of SC4's registry; its unit test first, with today's `SECTION_NEEDS` frozen in it. `SpaceSettingsView` reads it instead of `SPACE_SECTIONS`, `SECTION_NEEDS` and the inline groups; the space menu of SC1 derives its set from it; the aliases replace the address. The old frame draws the new groups for now | SPC-138 to SPC-145, SPC-134 | M | `space-settings` changes in the sidebar |
| 2 | **The sidebar.** `SettingsFrame` gains the `space` model (D4): the head of SPC-136, the four entries with icons, the divider, the chevron column and indentation, `space-` fold ids, the reach of the band, Enter in the jump field, names taken over, and SC4's own scroll, kept position and scroll into view. Icons that are missing are added to `components/icons.tsx` as our own line drawings | SPC-134, SPC-136 to SPC-138, SPC-146, SPC-147 | L | new `space-settings-sidebar`, `space-settings-scrolled`, `space-settings-jump`, `space-settings-jump-none` |
| 3 | **Screens inside the frame.** A wrapper that renders a screen's body in the Settings frame for someone who may open space settings and in the Space frame otherwise; the bodies of Content, Trash, Access, Audit, Sites, Templates, Insights, Automation and Webhooks split from their `Frame`; `?section=files` and `?section=archived`; the space menu's People to `?section=people` | SPC-135, SPC-108 | M | every picture of those screens |
| 4a | **Details and Picture of the space.** The one form of Details with its ordered save (D2), its unit test first; the old keys under the key; the owners field; the administrators' pictures; the fold; Picture of the space from `AnimalChoice` and the upload | SPC-091 to SPC-096, SPC-009 to SPC-015 | M | new `space-details`, `space-details-changed`, `space-details-look` |
| 4b | **Statuses and approvals, Layout.** The three parts with their units; the review period with Save instead of on leaving the field; the title alignment as tiles (a `ChoiceTiles` component in `components/` if SC4 has not made one) | SPC-140, SPC-143, SPC-151, SPC-152 | M | new `space-statuses`, `space-layout` |
| 4c | **Archive, Delete, Pages still to write, Permissions.** The two action sections; the placeholders as a table that stays when empty, with Create page; the Access screen without tabs | SPC-126, SPC-127, SPC-129 to SPC-131, SPC-141, SPC-153 | M | new `space-archive`, `space-archive-restore`, `space-delete`, `space-placeholders`, `space-placeholders-empty`, `space-permissions` |
| 5 | **The head of a section and the width.** `SettingsSection` of SC4 on every entry, width from the registry, the introduction of every entry in `en.json`, help links where a document exists, the archived note as a `Notice`, the Experimental mark beside the title, visible sub-headings, primary actions of today moved into the title row or their part's heading row | SPC-148 to SPC-150, NAV-113, ADM-157 | L (mostly words) | new `space-settings-column-short`, `space-settings-full` |
| 6 | **Features and the switch.** `Switch` in `components/` (NAV-166), its unit and axe tests; the card of NAV-167 for Features; the switch for typing one's own status; the saved message | SPC-115, SPC-116, NAV-161, NAV-166, NAV-167 | M | new `space-features` |
| 7 | **People.** The row of tools (search, Kind, Role, Add people; `ToolRow` if SC4 built it, D16); the row of SPC-118 without the checkbox; the second lines that exist; More with Remove access; the title row's More with Export as CSV; the filter to administrators that Details links to | SPC-118 to SPC-121 | M | new `space-settings-people`; `space-roles`, `space-roles-open` |
| 8 | **Tables and action sections.** Title first and widest, actions last, the sort mark, in the tables of Content, Trash, Audit of the space, Webhooks, Templates and Pages still to write; Export and Import as action sections | SPC-153, SPC-154 | S to M | the screens touched |
| 9 | **Documents and the last look.** `docs/design/README.md` (the Settings frame row, "Settings and lists", the components table: `Switch`, `ChoiceTiles` if new), blueprint 03 status lines as in "Lines in scope", the changelog with the moves, the full e2e once and `0-design.spec.ts` twice in a row, the report with every before and after picture, the handoff | | M | none new |

If the wave runs long, step 7 is the one to move to a wave of its own: the arrangement is whole without it, and the People list of today keeps working in the new place.

## Acceptance criteria

- [ ] Every line marked built or partly above behaves as written, proved by the tests above.
- [ ] Every entry sits in exactly one place, and every address of today opens the entry the upgrade notes name (unit test and e2e).
- [ ] No route of the server, no access rule and no guard changed: the route walker and the query budgets pass unchanged, and the registry test proves every old part opens for at least the permissions it opened for.
- [ ] Someone who may open no part of space settings sees "does not exist" at every address of space settings, and the screens of their own as today.
- [ ] Folding a group of space settings never folds a group of the administration, and the other way round.
- [ ] `make check`, the whole e2e suite once, and `0-design.spec.ts` twice in a row are green.
- [ ] The product owner has seen the before and after pictures of every changed screen in the report.

## How the product owner tests it

1. Open a space, press More beside its name, then Space settings. At the top of the sidebar: an arrow back, **Space settings** in bold, the space's animal and name, and a search field. Under it People, Content and Details with icons (and Automation, if automation is on), a short line, then General, Access, Pages and files, Look and Connections, all open.
2. Fold Pages and files, open another section and reload: it stays folded. Open the administration: its groups are not folded by this.
3. Type "key" in the search field and press Enter: Details opens. Type "nothing like this": the list says nothing matches.
4. In Details, change the name and add an owner. Save and Reset changes become active. Press Reset changes: both values come back. Change them again and press Save. Open More details: the description, the categories and the home page are there.
5. Open Content, then Trash: each opens beside the settings sidebar, with its entry marked.
6. Open Statuses and approvals: the statuses, the review period and the approval rule are on one screen. Open Layout: the title alignment is two tiles.
7. Open Archive the space and archive it: it asks first, and then offers Restore the space. Restore it.
8. Open a link you saved earlier to `…/settings?section=rules`: Statuses and approvals opens.
9. Log in as a member who only reads the space and type its settings address: the page says it does not exist, and the space menu shows no Space settings.

## Decided on the product owner's behalf on 2026-11-16, may be overruled

- **D1. Only what exists gets an entry.** Entries the lines name for features that are not built (the list under "Every section today") wait for their own waves, within the feature freeze of ADR-043, as SC4 D1 decided for the administration. An entry with nothing behind it would be a promise the screen cannot keep. The Content screen keeps its filters for restricted pages and for pages nothing leads to meanwhile.
- **D2. Details saves part by part.** SPC-091 makes Details one form with one Save, but its parts are five routes on the server, each with its own rule and audit event. A route that saves all of them in one transaction would be a change of the API and of the access rules, which this wave does not make. Save sends the changed parts in a fixed order, stops at the first refusal and says what was and was not saved; the key goes last because the address follows it. If the product owner wants one transaction, it is a server change of its own.
- **D3. Group ids of space settings start with `space-`.** Together with SC4 D3 this ends the shared ids without a migration, and works whichever of the two waves lands first.
- **D4. One frame, one description per kind.** `SettingsFrame` takes a discriminated description of its sidebar (`admin`, `space`, `single`) rather than flags for each difference, so a reader sees in one place how the two sidebars differ, and the differences are only those the lines demand.
- **D5. Screens of their own keep their addresses.** SPC-135 asks that they open in the large area at their own address. They keep the paths they have (`/content`, `/trash`, `/access` and the others) instead of moving under `?section=`, because their own filters live in the address and links to them exist across the app; the frame around them changes, the address does not.
- **D6. Nobody gains or loses a screen.** The frame of a screen of its own depends on whether the person may open space settings; someone who may not sees it in the frame of the space as today, so no reader loses a screen they could reach. Details opens for `space.look` too, showing only the home page, because the old Space owners section showed it to them.
- **D7. Old section ids lead to the new place.** `key`, `categories`, `roles`, `rules` and `danger` are aliases that replace the address; nothing needs a server redirect, and Back does not loop.
- **D8. `?section=people` means People.** SPC-138 names the role list People. The old Space owners section had that id; its content is now in Details, and a bookmark to it opens the list of who holds which role, which a person looking for "people" expects.
- **D9. Permissions is the Access screen without its tabs.** The role list is People (SPC-118); keeping it a second time on the Access screen would be two places to change one thing. Permissions keeps what only it has: who can open the space and how, the presets, inviting a guest.
- **D10. Files and Archived pages in two frames.** Readers reach them from the space menu, in the frame of the space (SPC-106, SPC-107). SPC-142 also makes them sections of the settings. The same body opens in both: at `/files` and `/archive` in the frame of the space, at `?section=files` and `?section=archived` in the settings. Which frame a screen uses never depends on the person's permissions there.
- **D11. The jump field finds names, and the names an entry took over.** SPC-137 leaves open whether it also finds settings inside sections. Space settings have no index of settings like the administration's; instead each entry lists the names of the sections it absorbed (Space key, Space owners, Space categories, Rules for pages, Page statuses, People and roles, Who has access, Archive or delete), so a person who looks for the old name finds the new place. An index of settings inside sections is a later need.
- **D12. Shut groups are kept per person.** SPC-146 says every group starts open; it does not say whether a person's folding is forgotten. The administration keeps it (ADM-062), and one rule for both sidebars is simpler to learn: all groups are open until the person folds one.
- **D13. Widths.** SPC-150 names Automation as full width and Details, People, PDF design and Watermark as the column. Every list that is a table with more than three columns, and every screen of its own that holds one, gets the full width; every form, action section and short list gets the column, Public site and Shortcuts included.
- **D14. The switch comes now, the new switches wait.** Features is the first screen whose choices apply at once one by one (SPC-116, NAV-161), so `Switch` of NAV-166 is built here; SC4 D13 found no screen in the administration that needed one. Switches for kinds of item and for parts of the sidebar have no setting of a space behind them yet; adding them is a change of the server and waits. The switch for access requests, which the line does not list, stays in the card: removing a working setting is not this wave's to do.
- **D15. No checkbox column without an action.** SPC-118 puts a checkbox in every row of People for the changes of SPC-122, which waits for its own study. A checkbox that does nothing would break NAV-004, so the column comes with SPC-122.
- **D16. The row above People.** SPC-120 names dropdowns; SPC-154 and ADM-103 make the row of tools one rule for every list. If SC4 has built `ToolRow`, People uses it; if SC4 moved its step 10 to a wave of its own, People has a search and two labelled native dropdowns, which SPC-120 describes, and moves to `ToolRow` in that wave.
- **D17. Help links.** As SC4 D10: the link goes to the documentation address the administrator set, or the default, followed by the path the registry names. Today only Public site (`operations/public-sites.md`), Published space (`operations/published-spaces.md`) and Templates (`operations/templates.md`) have a document; every other entry shows its sentences without a link until it has one.
- **D18. Details is the landing section.** Without a section in the address, Details opens, as today, rather than the first entry of the sidebar (Automation, which is often off and is a builder rather than a place to start); someone who may not open Details lands on the first entry they may open.
- **D19. The settings of a template keep their look.** `TemplateSettingsView` uses the same frame for the settings of one template (spec 074). The lines of 2.11 do not cover it, guided templates are experimental, and its many pictures belong to its own wave; it keeps today's sidebar through the `single` kind.

## Through the API

The product owner decided on 2026-11-17 that every feature must be usable through the API, so that an MCP client can do what a person does on screen. Every entry of space settings reads and changes through the routes below, with a signed-in session; each route keeps the `Access` rule it had, and the module behind it decides. No route was missing, and this wave added none. As SC4 D22 says for the administration, a personal access token only reads (RV10); letting a token change settings is for the spec that brings MCP. `{id}` is the space's id; `GET /api/v1/spaces/{id}` returns what a person may do there (`can`), its status, features, review period, approval rule, title alignment, picture and home page, which several entries read.

| Entry | Reads | Changes | Rule |
|---|---|---|---|
| Automation | `GET /spaces/{id}/rules`, `GET /rules/{rid}` | `POST /spaces/{id}/rules`, `PUT` and `DELETE /rules/{rid}` | `space.automation`, while automation is on |
| People | `GET /spaces/{id}/role-assignments` | `PUT /spaces/{id}/role-assignments`, `POST /spaces/{id}/role-assignments/preview` | `space.manage_access` |
| Content | `GET /spaces/{id}/content` | `POST /spaces/{id}/content/preview`, `POST /spaces/{id}/content/run` | `space.administer` |
| Details | `GET /spaces/{id}`, `GET /spaces/{id}/overview` (owners, administrators), `GET /spaces/{id}/old-keys`, `GET /spaces/{id}/categories`, `GET /categories`, `GET /spaces/{id}/tree` | `PATCH /spaces/{id}` (name, description), `PUT /spaces/{id}/key`, `DELETE /spaces/{id}/old-keys/{alias}`, `PUT /spaces/{id}/owners`, `PUT /spaces/{id}/categories`, `PUT /spaces/{id}/home` | `space.administer`; the home page `space.look` |
| Features | `GET /spaces/{id}` (`features`) | `PUT /spaces/{id}/features` | `space.administer` |
| Archive the space | `GET /spaces/{id}` (`status`) | `POST /spaces/{id}/archive`, `POST /spaces/{id}/unarchive` | `space.administer` |
| Delete the space | | `DELETE /spaces/{id}` | `space.administer` |
| Export | | `GET /spaces/{id}/export.zip`, `POST /spaces/{id}/bundles` | `space.export`; a bundle `space.bundle` while bundles are on |
| Import | | `POST /spaces/{id}/imports` | `page.create` |
| Picture of the space | `GET /spaces/{id}` (`icon`, `has_icon`), `GET /spaces/{id}/icon` | `PATCH /spaces/{id}` (`icon`), `POST` and `DELETE /spaces/{id}/icon` | the animal `space.administer`; the upload `space.look` |
| Statuses and approvals | `GET /spaces/{id}/statuses`, `GET /spaces/{id}` (`review_months`), `GET /spaces/{id}/approvals` | `PUT /spaces/{id}/statuses`, `PUT /spaces/{id}/review`, `PUT /spaces/{id}/approvals` | `space.statuses`; the approval rule `space.approvals` |
| Workflows | `GET /spaces/{id}/workflows`, `GET /spaces/{id}/signature-meanings` | `POST /spaces/{id}/workflows`, `PUT /workflows/{wid}`, `PUT /spaces/{id}/workflow`, `POST /spaces/{id}/workflows/from-template`, `POST /spaces/{id}/signature-meanings` | `space.workflows` |
| Insights | `GET /spaces/{id}/insights`, `/insights/content`, `/insights/people` | | `analytics.view` |
| Permissions | `GET /spaces/{id}/access`, `GET /spaces/{id}/access.csv`, `GET /spaces/{id}/grants` | `POST /spaces/{id}/preset`, `POST /spaces/{id}/guests`, `PUT /spaces/{id}/grants` | `space.manage_access`; presets `space.administer`; guests `space.invite_guests` |
| Public site | `GET /spaces/{id}/sites`, `GET /sites/{sid}` | `POST /spaces/{id}/sites`, `PATCH` and `DELETE /sites/{sid}` | `site.publish` |
| Audit of the space | `GET /spaces/{id}/audit`, `GET /spaces/{id}/control-reports/{kind}`, `GET /spaces/{id}/auditor-export` | | `space.audit` |
| Pages still to write | `GET /spaces/{id}/placeholders` | starting one is `POST /pages` | `space.administer`; `page.create` |
| Files | `GET /spaces/{id}/files`, `/files/filters`, `/files/usage` | `POST /spaces/{id}/files/delete` | `space.view` to list; `space.administer` for usage and deleting |
| Trash | `GET /spaces/{id}/trash` | `POST /trash/{tid}/restore`, `DELETE /trash/{tid}`, `POST /spaces/{id}/trash/bulk` | `space.administer` |
| Archived pages | `GET /spaces/{id}/archive` | `POST /pages/{pid}/unarchive` | `space.view`; `page.archive` |
| Labels | `GET /spaces/{id}/labels` | `PUT /spaces/{id}/labels`, `POST /spaces/{id}/labels/rename` | `space.labels` |
| Shortcuts | `GET /spaces/{id}/shortcuts` | `PUT /spaces/{id}/shortcuts` | `space.look` |
| Published space | `GET /spaces/{id}/publishing/linking-in`, `/publishing/end-preview` and the releases routes of spec 073 | the publishing routes of spec 073 (`link`, `settings`, `start`, `end`) | `space.administer` or `space.release`, while publishing is on |
| Templates | `GET /spaces/{id}/manage-templates`, `GET /spaces/{id}/my-templates`, `GET /spaces/{id}/template-review` | `POST /spaces/{id}/templates`, `POST /spaces/{id}/templates/import`, `PATCH` and `DELETE /templates/{tid}`, `POST /spaces/{id}/my-templates`, `PUT /spaces/{id}/template-review` | `space.templates`; proposing `template.propose` while guided templates are on |
| Layout | `GET /spaces/{id}` (`title_align`) | `PUT /spaces/{id}/title-align` | `space.look` |
| PDF design | `GET /spaces/{id}/pdf-design` | `PUT /spaces/{id}/pdf-design` | `space.administer` |
| Webhooks | `GET /spaces/{id}/webhooks`, `GET /webhooks/{wid}` | `POST /spaces/{id}/webhooks`, `PATCH` and `DELETE /webhooks/{wid}` | `space.automation` |

The folded groups are part of the person's preferences: `GET` and `PUT /api/v1/me/preferences`, `settings_shut`.

## As built (wave SC5, 2026-11-20)

- The registry is `web/src/views/space-sections.ts`, with its unit test holding the needs of before; the sidebar is the `space` kind of `SettingsFrame` (D4); the screens of their own use `SpaceToolFrame` of `views/space-settings-frame.tsx`; the view is `views/space-settings.tsx`; Details is `views/space-details.tsx` with its ordered save in `space-details-save.ts`; Statuses and approvals is `views/space-statuses.tsx`. New components: `Switch` and `ChoiceTiles`; `useSaved` says that a setting which applies at once was saved.
- Steps 7 and 8 moved (D33, D34). Lines SPC-118 to SPC-121 and SPC-154 keep the status "new" for space settings: People is today's role list in its new place, and the tables keep their columns.
- The access e2e and the bookmarks e2e are in `web/e2e/space-settings.spec.ts`; the pictures are listed in the report of the wave.

## Decided on the product owner's behalf on 2026-11-20, may be overruled

- **D20. Space settings in the space menu: the same permissions as before.** The menu derives its set from the registry, leaving out People and Permissions, which its own People item reaches: someone who only manages access sees no Space settings item, as before (the e2e `a manager runs access` holds it), and their People item now opens the People section beside the settings sidebar, with Permissions in the same sidebar.
- **D21. A screen of its own keeps its own heading.** Content, Trash, Automation, Templates, Insights, Public site and Webhooks keep the heading and actions they had; they now stand beside the settings sidebar. Their move to `SettingsSection` belongs to each screen's own wave, as SC4 D23 left `.note`. Permissions and Audit of the space take their new names.
- **D22. An old id of a screen of its own leads to its address.** `?section=content`, `?section=trash` and the like replace the address with the screen's own, as an alias does.
- **D23. Someone who may open only screens of their own lands on the first of them.** An auditor who opens `/settings` lands on Audit of the space instead of an empty area (D18 extended).
- **D24. A space keeps at least one owner, in the form.** The last owner has no Remove. The server still takes an empty list on `PUT /spaces/{id}/owners`; refusing it there would change the API's behaviour and is for a later wave. The e2e for a refused part therefore uses a key another space holds.
- **D25. Pages still to write starts the page in the space, not under the linking page.** `/spaces/:key/new` takes only the title; starting it under the linking page needs a parent there, a change of its own.
- **D26. The administrators in Details are initials.** `GET /spaces/{id}/overview` names them without ids, so Details draws their initials, and **See all administrators** opens People unfiltered until the filter of step 7 exists.
- **D27. The jump field of a space has the search mark and border of the administration's**, so the two Settings frames look alike where the lines do not differ.
- **D28. The fold of Details keeps its fields in the form while shut.** Typing kept over a reload for a new version (spec 043) finds them by their place in the form; the lists behind the fold still load only when it opens.
- **D29. The review period and the approval rule save with Save and Reset changes**, each a unit of its own; the statuses and the switch for typing one's own status apply at once with a short message.
- **D30. Features shows the switch of automation only while automation is on for the instance** (NAV-004); the switch of requests for access stays (D14).
- **D31. Room for wave UR1.** The entry of spec 094 for what someone without access sees goes into the Access group after Permissions; the registry marks the place with a comment, and whichever wave lands second adds the one line, its words and its case.
- **D32. Every new part of a section has a visible `h2` and a line under it**; the first heading of a moved section that only repeated the title is gone, since the section's head carries the title (SPC-149).
- **D33. People's row of tools waits (step 7).** The plan allowed it to move; it goes to the wave of the tool rows (ADM-103) together with the administration's, so both lists get one row of tools at once.
- **D34. The tables and the empty blocks wait (step 8, SC4 D24).** Column order, actions last and the sort mark in the tables of Content, Trash, Audit, Webhooks and Templates, and the empty block of NAV-173 for Automation, Templates and Document control, change the same lists as the tool rows; they move to that wave. Pages still to write has its two-column table now.

## Supersedes

ADR-050 records that this spec supersedes, once built: the six groups of space settings of spec 033 (ADR-034); "sections that are screens of their own are links" of spec 033; the section Keep it tidy; spec 033 D7 for space settings; "settings and the administration stay at the left beside their sidebar" of ADR-038's Look item for space settings; spec 023 D8 (Pages still to write shown only with rows); and the meaning of the address `?section=people`.

## Seen, not changed

- `SECTION_NEEDS` in `views/space.tsx` has two doc comments above it, one of them out of date. Step 1 replaces both with the registry.
- SC1's `SETTINGS_PERMISSIONS` (`components/space-block.tsx`) repeats the permissions of `SECTION_NEEDS`; two lists that must change together. Step 1 derives it from the registry.
- The Features list holds `access_requests`, which SPC-115 does not name (D14).
- The placeholders section today shows nothing at all while empty, not even its heading, which SPC-127 replaces.

## Provenance

Own words, from blueprint 03 chapter 2 sections 2.2, 2.6, 2.7, 2.8, 2.9 and 2.11, and chapter 1 sections 1.13 and 1.14, as written in the study of 2026-11-14 (ADR-045, 02 "One exception"), the built code on `main`, the code of `wave-sc1`, and specs 033, 078 and 081. No pictures, inventories or other product were consulted in writing this spec.
