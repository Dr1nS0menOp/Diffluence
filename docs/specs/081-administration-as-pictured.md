# 081: The administration, as blueprint 03 arranges it

Status: built in wave SC4 (2026-11-20), steps 0 to 9 and 11; step 10 (tool rows, ADM-103) moved to a wave of its own (D25)
Phase: the screens of blueprint 03, fourth item of the order in `docs/handoff/2026-11-15-screens-study.md` (wave SC4), after the study of 2026-11-14 (ADR-045)

## Where `main` moved since this plan (2026-11-20)

The plan was written on 2026-11-16 against `main` before SC1. When the wave was built, `main` held SC1 (spec 078), SC2 (spec 079), GT5, SR1, SR2, RV10 to RV13 and FX4. What that changes here:

- **SC1 landed** as the study expected: the settings button, the current-row look, no sidebar button on the Settings frame, the line beside the sidebar. `settings-frame.tsx` and `views/admin.tsx` were as described below.
- **RV10 (spec 085, ADR-052)** made every route below `/api/v1/admin/` that changes something session-only, decided in the router. "No route of the server is added, removed or given another rule" below still holds for this wave: every moved part calls the same routes, which now carry the rule RV10 gave them. The section "Through the API" lists each route with that rule.
- **RV11 (spec 090)** records each changed field of a settings section, before and after, in `settings.updated`. Two screens now save the same section in part (Look and Announcements share `look`, Rules and Outgoing calls share `general`, Organising and Defaults for new spaces share the organising settings). Each still sends the whole section, so the audit row names only the fields that really changed: a save of Announcements records the banner fields alone. Nothing in the server changes for this.
- **SC2** brought `GuideLink` (NAV-113's documentation link). The introduction of a section uses it, with a path per entry (D10).
- **The product owner's rules of 2026-11-17**: until 1.0 the API and every part of the app may change radically, and every feature must be usable through the API (vital for use through MCP). The new section "Through the API" proves the second for the administration; no route was missing (D21, D22).

## Problem

An administrator works in one place for everything the instance does: people, spaces, rules, the look, the health of the installation. Today that place has six groups (Overview; People and access; Content; Automation; Look; System) chosen in ADR-034, and its sections sit at the start of the large area beside the sidebar (the Look item of ADR-038). The study of 2026-11-14 rewrote how the administration is arranged: a sidebar in two tiers, three tool sections and five settings groups with each section in exactly one, a centred column for forms and short lists, one way to start a section (title, introduction, notices, content), quieter tables, and notes with an icon and a kind. The product owner asked on 2026-11-15 for the administration to be rebuilt as those lines say (ADR-045). This spec plans that rebuild. It writes no code: SC4 builds on the frame of SC1, which has not landed yet.

The look stays ours: colours, icons, fonts and every value come from `tokens.css` (ADR-021, 02 "One exception"). Only the arrangement follows the lines.

## Study

### What SC1 gives this wave

Spec 078 (branch `wave-sc1`) builds the frame. For the administration it brings:

- the settings button at the end of the top bar, for administrators only, and Administration out of the account menu (NAV-095, ADM-059);
- the current-row look of NAV-144 for every sidebar, including the sections of the settings sidebars (spec 078, D16): accent text in normal weight, the tint with rounded corners, a short bar on its start edge, `aria-current`;
- the neutral hover tint and the rounded focus outline of NAV-145 in sidebars;
- no sidebar button on the Settings frame (spec 078, D4), so the administration's sidebar is always shown on a wide screen;
- the thin line beside the sidebar (NAV-089).

SC1 does not change `components/settings-frame.tsx` or `views/admin.tsx`. Everything below starts from those files as they are on `main` today.

### How the administration is built today

- **Route.** `/admin` and `/admin/:tab` render `AdminView` (`web/src/app.tsx`). `AdminView` shows "not found" unless the person holds the global permission `administrator` and the tab is one of the 28 known ids. Every screen calls existing routes under `/api/v1/admin/...`, each with its own access rule on the server; the browser decides nothing about access.
- **Frame.** `SettingsFrame` (`components/settings-frame.tsx`) draws a sidebar with a head, a search, and groups that fold; the large area is `.column.wide.settings-area`. The administration passes its own search, `SettingsSearch`, which finds settings inside sections from a hand-kept index (`views/admin-search.ts`). Which groups are shut is stored in the person's preferences as `settings_shut`, shared with the settings of a space.
- **Sections.** One list `TABS` and one list `ADMIN_GROUPS` in `views/admin.tsx`. Every screen starts with `<h1>` and then `HealthBand`, a band about the health of the installation, before its content.
- **Forms.** `SectionForm` (`views/admin-rules.tsx`) loads one settings section, keeps the changes, and saves the whole section with one primary Save. For the sections `access`, `retention`, `backups` and `health` a change that lets more through is refused with `403 confirmation_required`, and the form then shows the fields of a confirmed change to a system administrator only and sends them to `PUT /admin/settings/{section}/confirmed` (spec 076; spec 075, D27).
- **Look of lists.** Tables have a line under each row; notes are `.note` and `.note-error`, a tinted line without an icon; long lists filter with dropdowns above them.

### Every section today and its place in the target

The target groups are those of ADM-068 to ADM-075. "Moves" means part of a screen goes to another entry; the part keeps its component, its route on the server and its guard.

| Today (group, entry, address) | Holds | Target (tier, group, entry) | Address after |
|---|---|---|---|
| Overview, Summary, `summary` | tiles, attention list | Tool, Overview, Summary | same |
| People, Users, `users` | the list of users, bulk bar, new user, **service accounts** | Tool, People and access, Users; service accounts move to their own entry | same |
| People, Invitations, `invitations` | waiting requests, new invitation, CSV import | Tool, People and access, Invitations | same |
| People, Groups, `groups` | the list of groups | Tool, People and access, Groups | same |
| People, Roles, `permissions` | roles, **who holds an instance permission**, **space categories** | Settings, Permissions, Roles; the holders move to Instance permissions, categories to Organising | same |
| People, Provisioning, `scim` | SCIM tokens | Settings, Security, Provisioning | same |
| People, Sessions, `sessions` | logging everyone out, incident steps | Settings, Security, Sessions | same |
| Content, Spaces, `spaces` | all spaces, recover access, storage limit, **spaces in the trash** | Tool, Tools, Spaces; the trash moves to its own entry | same |
| Content, Templates, `templates` | templates of the instance | Settings, Instance, Templates; gains space templates from Organising | same |
| Content, Organising, `organising` | home of the organisation, draft days, **default preset of new spaces**, features of spaces, experimental parts, **saving a space as template** | Settings, Instance, Organising; the preset moves to Defaults for new spaces, space templates to Templates; gains space categories | same |
| Content, Document control, `control` | templates of document control | Settings, Instance, Document control | same |
| Content, Rules, `rules` | general (language, time zone, working days, help address, **embed addresses**), features, **access**, retention | Settings, Instance, Rules; access moves to Login and second factor, embed addresses to Outgoing calls | same |
| Content, Export and publishing, `export` | formats, limits, public switches, public links, **bundle key and bundle upload** | Settings, Security, Export and publishing; bundles move to Import a space | same |
| Automation, Automation, `automation` | rules of the instance | Tool, Tools, Automation | same |
| Automation, Webhooks, `webhooks` | webhooks | Settings, Data, Webhooks | same |
| Automation, Outgoing calls, `outgoing` | hosts that rules and webhooks may call | Settings, Security, Outgoing calls; gains the embed addresses | same |
| Look, Look, `look` | logo, name, organisation, accent, login text, terms, **banner**, welcome page, reset | Settings, Look, Look; the banner moves to Announcements; gains the font | same |
| System, System health, `system` | checks, nodes and queues, monitoring files, log level | Tool, Overview, System health | same |
| System, Backups, `backups` | schedule, list, restore | Settings, Data, Backups | same |
| System, Upgrades, `upgrades` | nodes, migrations, marker, how to upgrade | Tool, Overview, Upgrades | same |
| System, Configuration, `configuration` | values and where each is set | Tool, Overview, Configuration | same |
| System, Care, `care` | maintenance mode, jobs, failed events, largest, unused files | Settings, Data, Care | same |
| System, Settings, `settings` | trash retention, upload size, blocked types, disk thresholds, **font**, example space, **login methods** | Settings, Instance, **General** (renamed); the font moves to Look, login methods to Login and second factor | same |
| System, Mail, `mail` | switches, queue | Settings, Instance, Mail | same |
| System, Audit log, `audit` | the log, signatures and witness | Tool, Overview, Audit log | same |
| System, Events, `events` | events, older events | Tool, Overview, Events | same |
| System, Insights, `insights` | figures over time | Tool, Overview, Insights | same |
| System, Analytics, `analytics` | the analytics switch and its terms | Tool, Overview, Analytics | same |

New entries made only of parts that exist today:

| Entry | Tier and group | Address | Made of |
|---|---|---|---|
| Announcements | Tool, Tools | `announcements` | the banner fields of the `look` section (ADM-069) |
| Service accounts | Tool, People and access | `service-accounts` | `ServiceAccounts`, out of Users (ADM-070) |
| Login and second factor | Settings, Security | `login` | the `access` section form of Rules, with its guard, and the line on login methods of Settings (ADM-072) |
| Instance permissions | Settings, Permissions | `instance-permissions` | the list of who holds which instance permission, out of Roles, read-only as today (ADM-073, ADR-034) |
| Defaults for new spaces | Settings, Permissions | `space-defaults` | the default preset of the Organising form (ADM-073) |
| Import a space | Settings, Data | `import-space` | the bundle key and bundle upload of Export and publishing (ADM-075) |
| Spaces in the trash | Settings, Data | `spaces-trash` | the trashed spaces of Spaces (ADM-075) |

Entries the lines name that are **not** built in this wave, because nothing behind them exists yet and the feature freeze of ADR-043 holds: Access check and Permission export (ACC-156, ACC-166), Import from other tools (phase 11), Link shortcuts (EDT-207), PDF font (EXC-069), Code blocks (EDT-218), Security overview (ADM-125), Access in many spaces at once (ADM-167), Access to all spaces (ADM-165), Access requests, Guests, App permissions, PDF design (EXC-056), Header and footer (ADM-147), Watermark (ADM-150), Use of dynamic blocks (ADM-188), Space reports (ADM-171), Linked instances (PLT-049), Other tools menu (NAV-100), Tokens (ADM-194), Encryption (ADM-203), Compliance (ADM-205), and the apps groups of ADM-076. Each takes its place in the registry of step 1 when its own wave builds it; ADM-077 says where.

### What the lines change that is not a move

- **Sidebar** (ADM-060 to ADM-066): a header row with a way back and the bold name that stays in place; tool sections with icons, always open; a Settings heading one step larger, the jump field under it, then the groups that fold with a boxed arrow, their entries indented to the column of the icon entries' names; its own scroll that keeps its position.
- **Column** (ADM-078): forms and short lists in one column of reading width centred in the large area; wide lists (Users, Spaces, Audit log, Automation, Events, Insights) at full width beside the sidebar.
- **Head of a section** (ADM-080 to ADM-082, NAV-113): the title as the first and largest text, its primary action and More at the end of the title row, then one or two sentences with a link to the documentation, notices, the content; parts with a bold heading one step smaller and space above.
- **Notices** (NAV-149, NAV-150, ADM-084, ADM-085): a tinted block with an icon in its own column, an optional bold title, a kind (information, warning, error) that never rests on colour.
- **Tables** (NAV-157, ADM-107 to ADM-110, NAV-172): a quiet heading row with a line under it, rows set apart by space, a line after the last row, a hover tint, row actions in a last column whose heading only screen readers hear; an empty table keeps its heading row with a centred sentence.
- **Forms** (NAV-160, NAV-162, NAV-163, ADM-086, ADM-087, ADM-090, ADM-091): labels above narrow fields of one width, hints under the control, Save and Reset changes as one unit under its fields, inactive until something changed, one Save per part that saves apart.
- **Empty sections** (NAV-173, ADM-114): a centred block with a heading, a few lines, the primary button and a guide link, and no create button in the title row while empty.
- **Tool rows** (ADM-103): search first, a button per filter that shows its value in words, More filters, the screen's own action at the end.

## Lines in scope

Every line of chapter 10, section 10.1, and the lines of chapter 1 that the administration needs, with its status after this wave. "Not this wave" names a line that needs a feature that does not exist yet, its own study, or the settings of a space (wave SC5).

### Chapter 10, section 10.1

| Line | Was | After SC4 | What changes |
|---|---|---|---|
| ADM-001 | built | replaced by ADM-059 | Done in SC1 |
| ADM-002 | built | replaced by ADM-068 to ADM-075 | The sections are those of the registry |
| ADM-003 | built | built | The jump field moves under the Settings heading and also finds every entry by name (D6) |
| ADM-059 | new | built (SC1) | |
| ADM-060 | new | built | Header row: back to the start screen, the bold name, stays in place, a line under it once the list scrolled beneath it |
| ADM-061 | new | built | Two tiers, a Settings heading one step larger, the line beside the sidebar from SC1 |
| ADM-062 | new | built | Boxed fold arrow, indented text entries, several open, state kept per person (D3) |
| ADM-063 | new | built | Jump field under the Settings heading with a search mark and a visible border |
| ADM-064 | new | built | Own scroll, header stays, position kept; the study it asks for is decided in D7 |
| ADM-065 | new | built | The look of SC1's NAV-144 and NAV-145 |
| ADM-066 | new | partly | Ellipsis and full name on hover and focus. No entry leads out of the instance yet, so no outward mark |
| ADM-067 | new | partly | The Experimental mark on Automation and Import a space while their part is on. The New mark waits for NAV-178 (D11) |
| ADM-068 | new | partly | Overview: Summary, Insights, Analytics, Audit log, System health, Events, Upgrades, Configuration (all of it) |
| ADM-069 | new | partly | Tools: Automation, Spaces, Announcements. Access check, Permission export and Import from other tools wait |
| ADM-070 | new | built | People and access: Users, Groups, Invitations, Service accounts |
| ADM-071 | new | partly | Instance: General, Rules, Organising, Mail, Templates, Document control. Link shortcuts, PDF font and Code blocks wait |
| ADM-072 | new | partly | Security: Login and second factor, Sessions, Provisioning, Export and publishing, Outgoing calls. Security overview waits |
| ADM-073 | new | partly | Permissions: Instance permissions, Roles, Defaults for new spaces. The others wait |
| ADM-074 | new | partly | Look: Look. PDF design, Header and footer and Watermark wait |
| ADM-075 | new | partly | Data: Care, Backups, Import a space, Spaces in the trash, Webhooks. The others wait |
| ADM-076 | later | later | |
| ADM-077 | new | built | Every entry in exactly one group, proved by a test of the registry |
| ADM-078 | new | built | Centred column or full width, per entry in the registry |
| ADM-079 | new | partly | Centred column, labels above controls, hints under them, `FilePicker`, one primary action per unit. Repeated blocks follow the same pattern where a screen has them |
| ADM-080 | new | built | Title first, primary action and More in the title row (D9), then the order of the line |
| ADM-081 | new | built | Every entry has its introduction (NAV-113) |
| ADM-082 | new | built | Part headings one step smaller, with their own action at the end of their row |
| ADM-083 | new | not this wave | Tabs inside a section: no section holds two lists that need them once the moves are done (D12) |
| ADM-084, ADM-085 | new | built | Notices as wide as the column, the whole-section ones under the introduction; no news notice exists yet |
| ADM-086, ADM-087 | new | built | |
| ADM-088 | new | not this wave | The required mark of NAV-165 is one rule for every form of the app; it gets its own small wave |
| ADM-089 | new | not this wave | Needs the explanation button of NAV-152 |
| ADM-090, ADM-091 | new | built | |
| ADM-092 | new | partly | Forms edit in place. The table with one Edit button belongs to Instance permissions as ACC-148 rebuilds it |
| ADM-093 | new | not this wave | The switch of NAV-166 is a new control; no screen of this wave needs one (D13) |
| ADM-094 | new | partly | Save and Reset changes inactive with their description (NAV-162); starting buttons elsewhere wait for their screens |
| ADM-095 | new | built | A warning sentence above Reset to defaults, Log everyone out and Remove for good |
| ADM-096 to ADM-100 | new | not this wave | They arrive with the screens that use them (Look, Watermark, Import a space) |
| ADM-101, ADM-102 | new | not this wave | Needs a list of which settings the environment fixes; the Configuration page shows them meanwhile |
| ADM-103 | new | not this wave | The tool row of Users and of the Audit log moved to a wave of its own (D25) |
| ADM-104 to ADM-106 | new | not this wave | Pills, count line and tiles come with the screens of 10.2 and 10.6 |
| ADM-107 | new | built | |
| ADM-108 to ADM-110 | new | partly | Column order and widths, actions last, hover tint, the existing sorting and checkboxes. Bold parents wait for a list that has them |
| ADM-111 to ADM-113 | new | not this wave | |
| ADM-114 | new | partly | Groups, Service accounts and Webhooks; Automation, Templates and Document control in SC5 (D24) |
| ADM-115 | new | not this wave | No section depends on an optional service in a way that empties it; the notes of today stay (D14) |
| ADM-116 to ADM-128 | new | not this wave | New content of the summary and the security overview: after 1.0 under ADR-043, unless the product owner says otherwise |

### Chapter 1, lines the administration needs

| Line | Was | After SC4 | What changes |
|---|---|---|---|
| NAV-113 | new | partly | Built for the administration; the settings of a space follow in SC5 |
| NAV-149, NAV-150 | new | partly | One `Notice` component, used for every note of the administration and by `ErrorNote` everywhere; `.note` elsewhere keeps its look (D23). The dismissed note of news waits for its first use |
| NAV-157 | new | built | One table rule for every table of the app (D5) |
| NAV-160, NAV-162, NAV-163 | new | partly | Built for the administration; space settings and Your account follow in their waves |
| NAV-171 | new | partly | Save and Reset changes |
| NAV-172 | new | built | Through `TableBox` |
| NAV-173 | new | partly | Built for Groups, Service accounts and Webhooks (D24) |

## Behaviour

1. **The sidebar from top to bottom.** A header row: a back action with an arrow toward the start side that leads to the start screen, then **Administration** in bold. It stays in place while the rest scrolls, and a thin line appears under it only once something scrolled beneath it. Then the three tool sections, **Overview**, **Tools** and **People and access**, each under a bold heading with no fold control, each entry an icon and its name. Then, after extra space, a bold **Settings** heading one step larger than the group headings, the jump field, and the five groups **Instance**, **Security**, **Permissions**, **Look** and **Data**. A group heading is a button with a boxed arrow before the words, pointing down while open and toward the end side while shut; its entries are text only, compact, starting where the names of the icon entries start. Groups are set apart by space. Every group starts open.
2. **Folding.** Folding a group changes nothing in the large area. Which groups are shut is kept in the person's `settings_shut`, under ids that start with `admin-` (D3), so it follows them across sections, reloads and devices. A jump that finds something in a shut group shows that group open while the jump field holds text, as today.
3. **Scrolling.** The sidebar scrolls on its own over its full height. Going to another section keeps its scroll position. When a section is opened from an address or from the jump field and its entry lies outside the visible part, the sidebar scrolls once, without motion, so that the entry shows (D7).
4. **Entries.** The open section's entry has the look of NAV-144 and `aria-current="page"`. A name too long for the row ends in an ellipsis and shows in full on hover and on focus, through its `title` and because the row's accessible name is the whole name. An entry whose experimental part is off is absent; one whose part is on carries the word **Experimental** in a lozenge after its name. Its address still answers while the part is off, with the existing sentence that the part is off and where to turn it on (D8).
5. **The jump field.** Under the Settings heading, with a search mark at its start and a visible border at rest. Typing lists, under the field, the entries whose name holds the words, of both tiers, and the settings inside sections from the index of `admin-search.ts`; choosing one opens its section. The names of entries come from the registry, so a renamed entry is found under its new name without a second list.
6. **The large area.** The registry gives each entry one of two widths: `column`, one column of reading width centred between the sidebar and the far edge with equal margins, for forms and short lists; or `full`, starting beside the sidebar, for wide lists and builders. A wide table inside a column scrolls in its `TableBox`; the page never scrolls sideways.
7. **A section from top to bottom.** Its name as the one `h1`, the same words as its entry; the primary action and then More (only when it holds something) at the end of the title row; one or two sentences on what the section is for, ending in a link to the documentation (NAV-113, D10); the health band of today, now a notice (D15); notices about the whole section; then the content. Parts have an `h2` one step smaller, with their own action at the end of the heading row, a line of explanation under it, and more space above than below. The largest gap of the screen lies under the head. There is no breadcrumb, and no Save at the top.
8. **Notices.** One `Notice` component: information, warning or error, each with its own icon in a column of its own level with the first line, an optional bold title (always for a warning), the text, an optional row of actions (a primary button, then a link) and a guide link last. A notice about a setting or a lasting cause has no close button. `ErrorNote` renders the error kind, so every failure in the administration looks the same.
9. **Tables.** A quiet heading row of small bold words with a line under it; rows set apart by space without lines; a line after the last row; a tint on the row under the pointer; the first column a little in from the row's start and the widest, holding what the row is about; row actions in the last column, aligned to the end, under a heading that only screen readers hear, a single action as a quiet text button, more behind More; a sorted column with one arrow after its heading's words, in the accent colour. An empty table keeps its heading row, with one muted sentence centred about a row below it.
10. **Forms.** Labels small and bold above their fields; fields narrow and of one width, a number field as wide as its content, with its unit after it; hints in smaller muted text directly under the control; a checkbox with its label beside it. Each unit that saves apart ends with **Save** and **Reset changes**, small and close together at the start side under its fields; both are inactive until something in the unit changed, and Save then says in its description that nothing has changed yet. Reset changes puts back the stored value of every field of the unit. A failed save keeps what was typed. Units are set apart by a thin rule and space. The section ends after its last unit.
11. **A guarded change.** On Login and second factor, Rules (retention), Backups and General (disk thresholds) the forms keep the behaviour of spec 076 exactly: a change that lets more through is refused with `confirmation_required`; only then, and only to a system administrator, the fields for the second factor (and the reason, for retention) appear inside the unit, above Save, whose words become those of the confirmed change; Reset changes also takes the confirmation away. An administrator without system administration sees the refusal as an error notice that says a system administrator must confirm it.
12. **Empty sections.** A section that the person may fill and that holds nothing (Groups, Service accounts, Webhooks, Automation, Templates, Document control) shows no create button in its title row. One centred block in its upper part holds a short bold heading, one to three lines, the primary button that makes the first item, and the guide link. A table of such a section keeps its heading row above the block.
13. **Tool rows** on Users and the Audit log. One row above the list: a search field first, with its search mark and a stronger border, filtering as one types; then one button per common filter (Users: type, role, group; Audit log: person, action, time), each an icon and a name that opens a small chooser and, once set, shows its name and value in words in the active look; then More filters for the rest (Users: status, space, last login, how they log in; Audit log: object, space, result); the screen's own action, if any, alone at the end. A choice applies at once and stays in the address, as the filters do today. The Audit log ends the row with **Reset**, a quiet button that returns to the default view.
14. **Moves.** Each part listed in the study shows only in its new entry. Its component, the routes it calls and the access it needs do not change. The jump field index and the links of the attention list point at the new entries.
15. **Words.** Every entry's name and introduction, the group names and the empty blocks go through `en.json`. "Settings" as the name of the section `settings` becomes **General**, because Settings now heads the whole settings part. The part of Rules that was headed General becomes **Language, time and help** (D16).

## Permissions and audit

Nothing about access changes, and this is checked rather than assumed:

- `AdminView` still shows "not found" to anyone without the global permission `administrator`, for every id old and new. The server keeps refusing every `/api/v1/admin/...` route to them; the browser's check only spares a person a broken screen.
- No route of the server is added, removed or given another rule. Every moved part calls the same routes, which keep their `Access` rule in `httpapi.Router`: the route walker of `internal/httpapi` stays as it is.
- The guards of spec 076 and spec 075 (D26, D27) move with their forms: `SectionForm` keeps `canConfirm` (system administrator), `confirmNote` and `withReason` for `access`, `retention` and `health`, and the Backups screen keeps its own. The server decides what lets more through, inside the transaction that stores it, so no move in the browser can skip a guard.
- What a system administrator alone sees inside a screen (disk thresholds, backups and restore, the log level, confirmed changes) keeps the same test of `system_administrator` in the browser and the same rule on the server.
- Two entries edit one settings section each in part: Look and Announcements share the section `look`; Rules and Outgoing calls share `general`; Organising and Defaults for new spaces share the organising settings. Each screen loads the section fresh when it opens and saves the whole section, as the single screen did. Two administrators saving the same section at once could overwrite each other's change, which is also true today; this wave does not change it (D17).
- No audit or outbox event is added or changed: every save is the same request as before, and writes the same `settings.updated` or module event.

## Data

No migration and no change to the API. The preferences document keeps `settings_shut`; the administration's new group ids start with `admin-` (`admin-instance`, `admin-security`, `admin-permissions`, `admin-look`, `admin-data`), within the form `^[a-z][a-z0-9-]{0,39}$` that the server already checks. The old ids (`overview`, `people`, `content`, `automation`, `look`, `system`) are no longer read by the administration; they may stay in a person's list, which holds at most 30 ids, until the person folds a group again and the list is written anew. The tool sections do not fold, so they store nothing.

## Through the API

Every section of the administration is a screen over routes of `/api/v1`; the browser decides nothing a script could not do. Below, per entry, the routes that read it and the routes that change it, all relative to `/api/v1`. Every route under `/admin/` needs the global permission `administrator` (or `system_administrator` where marked **sys**). The rule a route enforces is printed in `docs/api/routes.txt`; in short:

- **(s)** session-only: a signed-in session, never a personal access token. Every change under `/admin/` is session-only since RV10 (spec 085, ADR-052), and so are many reads.
- **(f)** sensitive: a session with a fresh login (spec 076).
- **(t)** also readable with a personal access token that has the Administration scope.

A script or an MCP server uses the administration through a session: `POST /auth/login` (and the second factor, where the person has one), then the routes below with the session cookie; a request that a browser marks as coming from another site is refused. Whether a token should change the administration is decided in D22.

| Entry | Reads | Changes |
|---|---|---|
| Summary | `GET /admin/summary` (s), `GET /admin/attention` (s), `GET /admin/health` (s) | `POST /admin/spaces/{id}/handover` (s), `POST /admin/pages/{id}/recover` (f) |
| Insights | `GET /admin/insights` (t), `GET /admin/insights/search` (t), both with `?format=csv` | none |
| Analytics | `GET /admin/analytics` (t) | `PUT /admin/analytics` (s), `DELETE /admin/analytics/data` (s) |
| Audit log | `GET /admin/audit-events` (s) with the filters of the screen, `GET /admin/audit-events/export` (s), `GET /admin/audit-events/verify` (s), `GET /admin/audit-overview` (s), `GET /admin/pages/{id}` (s) | none: the log is append-only |
| System health | `GET /admin/system` (s), `GET /admin/health` (s), `GET /admin/monitoring` (s), `GET /admin/monitoring/{kind}` (s), `GET /admin/log-level` (s) | `PUT /admin/log-level` (s), `PUT /admin/log-level/confirmed` (f, sys), `DELETE /admin/log-level` (s), `POST /admin/publishing/check` (s) |
| Events | `GET /admin/events` (s) | none |
| Upgrades | `GET /admin/upgrades` (s) | none: upgrades happen on the hosts |
| Configuration | `GET /admin/configuration` (s) | none: the environment sets it (ADM-101 waits) |
| Automation | `GET /admin/rules` (t), `GET /admin/automation` (t), `GET /rules/{id}`, `GET /rules/{id}/runs` | `POST /admin/rules` (s), `PUT /admin/automation` (s), `PUT /rules/{id}`, `DELETE /rules/{id}`, `POST /rules/{id}/test`, `POST /rules/{id}/hook` |
| Spaces | `GET /admin/spaces` (s), with every space and its totals; the filters of the screen are applied to that list | `PUT /admin/spaces/{id}/storage-limit` (s), `POST /admin/spaces/{id}/recover` (f) |
| Announcements | `GET /admin/settings/look` (s); everyone reads the banner in `GET /appearance` | `PUT /admin/settings/look` (s): `banner_text`, `banner_level`, `banner_ends` |
| Users | `GET /admin/users` (s) with `q`, `type`, `status`, `source`, `role`, `space_id`, `group_id`, `idle_days`; `GET /admin/users/{id}`, `/reach`, `/signing`, `/type-impact` (s) | `POST /admin/users` (s), `POST /admin/users/bulk` (s), `PATCH /admin/users/{id}` (s), `PUT /admin/users/{id}/permissions` (f), `PUT /admin/users/{id}/expiry` (s), `PUT`/`DELETE /admin/users/{id}/signing` (f/s), `POST /admin/users/{id}/disable`, `/enable`, `/unlock-password` (s), `/reset-mfa`, `/reset-password`, `/anonymise` (f) |
| Groups | `GET /admin/groups` (s), `GET /admin/groups/{id}/members` (s), `GET /admin/groups/{id}/impact` (s) | `POST /admin/groups` (s), `PATCH`/`DELETE /admin/groups/{id}` (s), `PUT`/`DELETE /admin/groups/{id}/members/{user}` (s), `PUT /admin/groups/{id}/owners` (s), `PUT /admin/groups/{id}/permissions` (s) |
| Invitations | `GET /admin/invitations` (s), `GET /invitation-requests` | `POST /admin/invitations` (s), `DELETE /admin/invitations/{id}` (s), `POST /invitation-requests/{id}/{verdict}`, `POST /admin/users/import` (s, `?preview=1` to look first) |
| Service accounts | `GET /admin/users?type=service` (s) | `POST /admin/service-accounts` (s), `POST /admin/service-accounts/{id}/tokens` (f) |
| General | `GET /admin/settings` (s), `GET /admin/settings/health` (s) | `PUT /admin/settings` (s), `PUT /admin/settings/health` (s), `PUT /admin/settings/health/confirmed` (f, sys), `POST /admin/example-space` (s) |
| Rules | `GET /admin/settings/general`, `/features`, `/retention` (s) | `PUT` of the same (s), `PUT /admin/settings/retention/confirmed` (f, sys), `DELETE /admin/settings/retention/lowering` (s) |
| Organising | `GET /admin/organising` (t), `GET /admin/features` (t), `GET /admin/experimental` (t), `GET /categories` | `PUT /admin/organising` (s), `PUT /admin/features` (s), `PUT /admin/experimental` (s), `POST /admin/categories` (s), `DELETE /admin/categories/{id}` (s) |
| Mail | `GET /admin/mail` (t), `GET /admin/settings` (s) | `PUT /admin/settings` (s): the four mail switches; `POST /admin/mail/test` (s) |
| Templates | `GET /admin/templates` (t), `GET /admin/template-review` (t) | `POST /admin/templates` (s), `POST /admin/templates/import` (s), `PATCH`/`DELETE /templates/{id}`, `PUT /admin/template-review` (s), `POST /admin/space-templates` (s) |
| Document control | `GET /admin/workflow-templates` (s) | `POST /admin/workflow-templates` (s), `PUT /admin/workflow-templates/{id}` (s), `POST /admin/workflow-templates/{id}/archive` (s) |
| Login and second factor | `GET /admin/settings/access` (s), `GET /admin/system` (s) for the login methods | `PUT /admin/settings/access` (s), `PUT /admin/settings/access/confirmed` (f, sys); identity providers are set in the environment |
| Sessions | `GET /admin/sessions/count` (s) | `POST /admin/sessions/end` (f) |
| Provisioning | `GET /admin/scim-tokens` (s) | `POST /admin/scim-tokens` (f), `POST /admin/scim-tokens/{id}/rotate` (f), `DELETE /admin/scim-tokens/{id}` (s) |
| Export and publishing | `GET /admin/export-policy` (t), `GET /admin/public-links` (t) | `PUT /admin/export-policy` (s) |
| Outgoing calls | `GET /admin/outgoing-hosts` (t), `GET /admin/settings/general` (s) for the embed addresses | `PUT /admin/outgoing-hosts` (s), `PUT /admin/settings/general` (s): `embed_hosts` |
| Instance permissions | `GET /admin/global-permissions` (s) | changed on the person or group: `PUT /admin/users/{id}/permissions` (f), `PUT /admin/groups/{id}/permissions` (s) |
| Roles | `GET /roles?scope=`, `GET /admin/roles/{id}/holders` (t), `GET /admin/permission-sets` (s), `GET /admin/role-assignments` (s) | `POST /admin/roles` (s), `PUT`/`DELETE /admin/roles/{id}` (s), `POST /admin/roles/{id}/preview` (s), `PUT /admin/permission-sets` (s), `DELETE /admin/permission-sets/{id}` (s), `PUT /admin/role-assignments` (s) |
| Defaults for new spaces | `GET /admin/organising` (t) | `PUT /admin/organising` (s): `default_preset` |
| Look | `GET /admin/settings/look` (s), `GET /appearance` | `PUT /admin/settings/look` (s), `PUT`/`DELETE /admin/appearance/logo` (s), `PUT`/`DELETE /admin/appearance/font` (s), `POST /admin/appearance/reset` (s) |
| Care | `GET /admin/jobs` (s), `GET /admin/event-failures` (s), `GET /admin/reports/largest` (s), `GET /admin/storage/unused` (s) | `PUT /admin/maintenance` (f, sys), `POST /admin/jobs/{id}/retry`, `/cancel` (s), `POST /admin/event-failures/{id}/replay` (s), `POST /admin/storage/unused` (s) |
| Backups | `GET /admin/backups` (s) | `PUT /admin/settings/backups` (s), `PUT /admin/settings/backups/confirmed` (f, sys), `POST /admin/backups/schedule/code` (s, sys), `POST /admin/backups/request` (s), `POST /admin/backups/restore` (f, sys), `POST /admin/backups/restore/code` (s, sys), `DELETE /admin/backups/restore` (s, sys) |
| Import a space | `GET /admin/bundles/key` (t), `GET /imports/{id}` | `POST /admin/bundles` (s), `PUT /imports/{id}/mapping`, `POST /imports/{id}/run`, `POST /imports/{id}/undo` |
| Spaces in the trash | `GET /admin/trash/spaces` (s) | `POST /trash/{id}/restore`, `DELETE /trash/{id}` |
| Webhooks | `GET /admin/webhooks` (t), `GET /webhooks/{id}/deliveries` | `POST /admin/webhooks` (s), `PATCH`/`DELETE /webhooks/{id}`, `POST /webhooks/{id}/rotate`, `POST /webhooks/{id}/deliveries/{id}/resend` |

The arrangement itself is no feature of the instance: which groups a person folded is their preference, `settings_shut` in `GET`/`PUT /me/preferences`, and the registry of sections lives in the frontend. Routes outside `/admin/` keep the rule of their module (`Object(action)` or the module's own check).

## Load budget

- **Server:** no route changes, so `internal/app/query_budget_test.go` keeps every budget. Each screen calls exactly the routes its parts called before. Opening Login and second factor reads the `access` section and `/admin/system` (for the login methods line), which Rules and General read before. Nothing on saving pages, reading or search.
- **Browser:** the sidebar hides experimental entries from `/system/capabilities`, which the app already holds with a stale time, so no request is added. The registry and the sidebar render about 40 entries; the scroll line under the header uses one `IntersectionObserver` on a sentinel, not a scroll listener.
- **Moves that split a screen** make one screen two, so an administrator who wants both sees two loads of the same small section (a few hundred bytes). Negligible against the ARC-233 budget.

## Upgrade notes

- **Addresses.** Every address of today keeps working and opens the entry with the same id. Seven ids are added. No redirect is needed, and the server's link to `/admin/backups` in backup mails stays right.
- **Mixed releases.** During a rolling upgrade a browser gets the frontend of whichever node served it; both talk to the same API, which this wave does not change. A person whose browser holds the old frontend sees the old sidebar until a reload.
- **Folded groups.** On the first visit every group of the new sidebar is open, because the new group ids are not in anyone's list yet. Nothing else of a person's preferences changes.
- **Where things went.** The changelog lists each moved part and its new place: the font to Look; the banner to Announcements; the login methods and the access rules to Login and second factor; service accounts to their own entry; spaces in the trash to their own entry; bundles to Import a space; the default preset to Defaults for new spaces; space categories to Organising; saving a space as template to Templates; embed addresses to Outgoing calls; who holds an instance permission to Instance permissions; and Settings renamed General.
- **Documents that name places in the administration** follow in step 11: `backup-restore.md` (System, Backups becomes Data, Backups), `identity-providers.md` and `incidents.md` (Provisioning now under Security), `regulated-use.md` (Settings, General becomes Rules, Language, time and help), `upgrade.md` (System, Upgrades becomes Overview, Upgrades), `install.md`, `monitoring.md`, `public-sites.md`, `published-spaces.md`, `templates.md`, `development.md`, `README.md` and `docs/design/README.md` (Fonts: Administration, Look).
- **Spec 075.** CSL-800 put Upgrades, Worker, Events and Encryption key in the System group. That group goes; ADR-048 places Worker (CS2) in Overview after System health, and Encryption key (CS5) in Security as the section Encryption of ADM-203. Spec 075's D28 is superseded for the order only.

## Design pictures and end-to-end tests

### Pictures to take again (all in `web/e2e/design-baselines/`)

Every picture of the administration changes: `admin-settings`, `admin-summary`, `admin-look`, `admin-rules`, `admin-roles`, `admin-analytics`, `admin-outgoing`, `admin-insights`, `admin-users-narrow`, `admin-users-1150-light`, `admin-users-1150-dark`, `admin-spaces-1150`, `admin-document-control`, `installation-upgrades-phone-rtl`, `installation-events`, `installation-configuration`. The table rule of NAV-157 and the notice of NAV-149 are one rule each for the whole app (D4, D5), so every picture that shows an app table or a note outside the administration changes too; the builder lists them in the report, and the product owner sees each before and after.

The test `AdminView: the admin console` goes to `/admin/settings` and waits for the heading Appearance, which moves to Look: it waits for the heading General instead.

### New pictures

Named `AdminView: …` in `web/e2e/0-design.spec.ts`, each looked at in light, dark, narrow and right-to-left:

- the sidebar with both tiers, one group shut, an entry with the Experimental mark (`admin-sidebar`);
- the sidebar scrolled, with the line under the header row and the current entry deep in Data (`admin-sidebar-scrolled`);
- Login and second factor at rest, and after a refused change with the fields of a confirmed change (`admin-login`, `admin-login-confirm`);
- a form with a change, Save and Reset changes active (`admin-general-changed`);
- Announcements (`admin-announcements`);
- an empty section with its block, on a fixture without webhooks (`admin-webhooks-empty`);
- the three kinds of notice, on the screens that show them with stable data (`admin-notices`);
- the tool row of Users with a filter set and More filters open (`admin-users-filters`);
- a section in the centred column with very little content and one with a lot, side by side in two pictures (`admin-column-short`, `admin-column-long`), because a picture only guards the state it shows.

### End-to-end tests

- **New `web/e2e/admin-frame.spec.ts`:** the header row's back action leads to the start screen; the three tool headings and the five group buttons in order; a group folds, stays shut across a reload and another section, and opens on a jump; the jump field finds an entry by name and a setting inside a section; the scroll position stays when moving between sections; an entry deep in Data opened by address shows in the sidebar; the current entry has `aria-current`; with automation off, Automation is absent from the sidebar and its address says the part is off; axe on each state; right-to-left and 360 px.
- **Moves:** each moved part shows at its new address and no longer at its old one; the attention list's links reach the right entries.
- **Guards survive (spec 076):** on `/admin/login`, an administrator without system administration who weakens the second-factor rule is refused; a system administrator is asked for a code and the change holds after it; the retention form on Rules still asks for a reason; the disk thresholds on General still go through the confirmed path. The existing tests in `access.spec.ts`, `backups.spec.ts`, `installation.spec.ts` and `people.spec.ts` keep what they assert and change only addresses and headings.
- **Forms:** Save and Reset changes are inactive on a fresh form, active after a change, Reset changes puts back the stored values, a failed save keeps what was typed.
- **Access:** `access.spec.ts`'s list of admin addresses gains the seven new ids; a member gets "not found" on each, and the server's routes behind them still answer 403 or 404 as before.
- **Files that visit the administration** and need their addresses or headings checked: `0-design.spec.ts`, `access.spec.ts`, `backups.spec.ts`, `create-kinds.spec.ts`, `file-details.spec.ts`, `health.spec.ts`, `installation.spec.ts`, `more-blocks.spec.ts`, `people.spec.ts`, `platform.spec.ts`, `roles.spec.ts`, `space-home.spec.ts`, `together.spec.ts`, `walk.spec.ts`, `writing.spec.ts`.

### Unit tests (written first)

- `web/src/views/admin-sections.test.ts`: every entry sits in exactly one group (ADM-077); ids are unique; every id of today is still there; the tool sections and groups come in the order of ADM-068 to ADM-075; an entry with an experimental part is left out while it is off; an unknown id is not a section.
- `web/src/components/settings-frame.test.ts`: the jump finds entries of both tiers and settings, and opens shut groups only while it holds text; folding writes `admin-` ids only.
- `web/src/views/admin-search.test.ts` (or the existing test file): every index entry names an id of the registry, so a moved setting can never point at a section that no longer holds it.

### Mutations

No rule of the server changes, so no mutation line is added. The unit test of the registry and the e2e of the guards are the proof that nothing was lost in a move.

## Build plan

Each step is one builder's work, ends with `make check` green, and commits its pictures. Sizes: S up to about 200 changed lines, M up to about 500, L more. Steps 3 and 4 may run in either order; everything else in the order given.

| Step | What | Lines | Size | Pictures |
|---|---|---|---|---|
| 0 | Rebase `wave-sc4` on `main` once SC1 has landed. Check that the settings sidebar has SC1's current-row look and that no settings sidebar has a sidebar button. Run the administration's e2e once as the baseline | | XS | none |
| 1 | **The registry.** `web/src/views/admin-sections.ts`: one typed list of entries (id, name key, tier, group, icon, width, experimental part, documentation path) and the order of groups; its unit test first. `AdminView` reads it instead of `TABS` and `ADMIN_GROUPS`; the old frame draws the eight groups for now; Settings is renamed General; experimental entries hide while off | ADM-068 to ADM-075 (regrouping), ADM-077, ADM-002 | M | admin pictures change in the sidebar only |
| 2 | **The sidebar.** `SettingsFrame` gains, for the administration only, the header row, the tool tier, the Settings heading with the jump field under it, the boxed fold arrow and indentation, its own scroll, the scroll into view of D7, ellipsis and title, and the Experimental mark; group ids with `admin-`. The settings of a space pass none of this and look as today until SC5. Icons for the tool entries are added to `components/icons.tsx` as our own line drawings | ADM-060 to ADM-067, ADM-003, ADM-063 | L | new `admin-sidebar`, `admin-sidebar-scrolled`; every admin picture again |
| 3 | **Moves: people, security and permissions.** Service accounts; Login and second factor (the `access` form with its guard, and the login methods line); Instance permissions; Defaults for new spaces; embed addresses into Outgoing calls; the Rules part renamed. The settings index and attention links follow; the e2e of spec 076 move to the new addresses before the move, and fail until it is done | ADM-070, ADM-072, ADM-073 | M | new `admin-login`, `admin-login-confirm` |
| 4 | **Moves: tools, look and data.** Announcements (the banner fields of `look`); the font into Look; Spaces in the trash; Import a space (bundle key and upload, experimental `bundles`); space categories into Organising; saving a space as template into Templates. Index and e2e follow | ADM-069, ADM-071, ADM-074, ADM-075 | M | new `admin-announcements` |
| 5 | **The head of a section and the column.** A `SettingsSection` component (title row with primary action and More, introduction with its documentation link, notices, content) used by every entry; width from the registry; the health band as a notice under the introduction; the introduction of every entry in `en.json`; part headings and spacing; primary actions of today's screens (such as adding a user, a group, a webhook or a rule) moved into the title row or their part's heading row | ADM-078, ADM-079, ADM-080, ADM-081, ADM-082, NAV-113 | L (mostly words) | new `admin-column-short`, `admin-column-long` |
| 6 | **Notices.** `Notice` in `components/`, with the three kinds and their icons; `.note` and `.note-error` restyled to the same look everywhere; `ErrorNote` renders the error kind; the administration's notes become `Notice`s in the places ADM-085 names | NAV-149, NAV-150, ADM-084, ADM-085 | M | new `admin-notices`; pictures with notes elsewhere |
| 7 | **Tables.** The table rule of NAV-157 for every app table; in the administration the first column widest, row actions last under a heading only screen readers hear, the hover tint; `TableBox` gains an empty state that keeps the heading row with its centred sentence | NAV-157, NAV-172, ADM-107 to ADM-110 | M | every picture with an app table |
| 8a | **Forms: `SectionForm`.** Save and Reset changes as one unit, inactive until changed, with the description; the guarded path of spec 076 unchanged in behaviour, its e2e green before and after; narrow fields of one width, hints under controls, checkbox labels beside | NAV-160, NAV-162, NAV-171, ADM-086, ADM-087, ADM-090, ADM-094 | M | new `admin-general-changed` |
| 8b | **Forms: the rest.** General's own form, Organising, Defaults for new spaces, Export and publishing, Mail, Look's logo, Outgoing calls onto the same unit, one Save per unit with a rule between units; the warning sentence above Reset to defaults, Log everyone out and Remove for good | NAV-163, ADM-091, ADM-095 | M | the screens touched |
| 9 | **Empty sections.** The empty block for Groups, Service accounts, Webhooks, Automation, Templates and Document control; no create button in their title row while empty | NAV-173, ADM-114 | S | new `admin-webhooks-empty` |
| 10 | **Tool rows.** A `ToolRow` with search first, filter buttons with choosers that show their value in words, More filters and the screen's own action; Users and the Audit log use it, their filters stay in the address; Reset on the Audit log | ADM-103 | L | new `admin-users-filters`; users and audit pictures |
| 11 | **Documents and the last look.** `docs/design/README.md` (the Settings frame, the components table: `SettingsSection`, `Notice`, `ToolRow`; Settings and lists; Fonts), blueprint 03 status lines as in "Lines in scope", CSL-800 in blueprint 11, the operations documents of the upgrade notes, the changelog, the full e2e once and `0-design.spec.ts` twice in a row, the report with every before and after picture, the handoff | | M | none new |

If the wave runs long, step 10 is the one to move to a wave of its own: the arrangement is whole without it, and the dropdown filters keep working.

## Acceptance criteria

- [x] Every line marked built or partly above behaves as written, proved by the tests above.
- [x] Every entry sits in exactly one group, and every address of today still opens the entry with its id (unit test).
- [x] No route of the server, no access rule and no guard changed: the route walker, the query budgets and the e2e of spec 076 pass unchanged in what they assert.
- [x] A member sees "not found" on every address of the administration, old and new.
- [x] `make check`, the whole e2e suite once, and `0-design.spec.ts` twice in a row are green.
- [ ] The product owner has seen the before and after pictures of every changed screen in the report.

## How the product owner tests it

1. Log in as an administrator and press the settings wheel at the end of the top bar. The sidebar starts with a back arrow and **Administration**, then Overview, Tools and People and access with icons, then **Settings** with a search field and five groups: Instance, Security, Permissions, Look, Data.
2. Fold Security, open another section, and reload: Security stays shut. Type "font" in the search field under Settings: Look appears, and choosing it opens Look with the font part.
3. Scroll the sidebar down and open Backups, then Care: the sidebar does not jump. A thin line shows under the header row while the list is scrolled.
4. Open General: the form sits in a centred column. Save and Reset changes are grey. Change the upload size: both become active. Press Reset changes: the old value is back and both are grey again.
5. Open Login and second factor and choose that nobody needs a second factor, then Save. As an administrator without system administration you are told a system administrator must confirm it. As a system administrator you are asked for a code from your authenticator app, and the change holds after it.
6. Open Users: one row above the list with search first and buttons for type, role and group. Choose a role: the button shows it in words, and the address holds it.
7. Open Webhooks on an instance without webhooks: a centred block explains what webhooks are, with one button to add the first.
8. Log in as someone who is not an administrator and type `/admin/login` in the address bar: the page says it does not exist.

## Decided on the product owner's behalf on 2026-11-16, may be overruled

- **D1. Only what exists gets an entry.** Entries the lines name for features that are not built (the list under "Every section today") wait for their own waves, within the feature freeze of ADR-043. An entry with nothing behind it would be a promise the screen cannot keep.
- **D2. Addresses keep their ids.** `/admin/settings` stays the address of General, `/admin/permissions` of Roles, `/admin/scim` of Provisioning. Bookmarks, the attention list, the server's backup mail and the tests keep working, and nothing needs a redirect. New entries get new ids. The words people see follow the lines; the address is not a word they read.
- **D3. Group ids of the administration start with `admin-`.** The settings of a space and the administration share `settings_shut`, and today both use `content` and `automation`, so folding one folds the other. The prefix ends that without a migration.
- **D4. One look for notes.** NAV-149 is the rule for a note in the flow of any screen, so `.note` takes the new look everywhere at once instead of two looks living side by side; the icon and title come through `Notice`, which other screens adopt in their waves. The cost is pictures outside the administration in this wave.
- **D5. One rule for tables.** NAV-157 is one rule for every table of the app, so it changes in one place, for the same reason as D4. Tables inside a page keep their own rules.
- **D6. The jump field finds entries too.** ADM-063 puts the jump field under the Settings heading; it keeps finding settings inside sections (spec 013 F9) and also finds entries of both tiers by their name from the registry, so there is one search and no second list of names.
- **D7. The current entry is brought into view once.** ADM-064 leaves open whether it should be. When a section opens from an address or the jump field and its entry is out of view, the sidebar scrolls once, without motion, so that it shows; moving by the sidebar itself never scrolls it. Without this, a link from the attention list into Data would leave the person not knowing where they are.
- **D8. An experimental entry that is off is absent, its address is not.** CSL-801 and ADM-115 hide the entry; the address answers with today's sentence that the part is off and where to turn it on, so an old bookmark explains itself instead of saying "not found".
- **D9. More in the title row only when it holds something.** A section without rare actions shows no empty More.
- **D10. The documentation link of an introduction.** It goes to the documentation address the administrator set (`help_url`), or the default address of the help menu, followed by the path the registry names for the entry, such as `operations/backup-restore.md` for Backups. An entry without a document of its own shows its sentences without a link. Nothing is fetched to show it (SEC-900).
- **D11. The New mark waits.** NAV-178 needs every section to know the release that brought it; no entry of this wave is new to a release. It comes with the first section that is.
- **D12. No tabs inside sections yet.** After the moves no section holds two lists that need tabs (ADM-083); parts with headings do the work, as ADM-082 says.
- **D13. No switch control yet.** NAV-161 asks for switches only where a choice applies at once; the forms of this wave save with Save, where checkboxes are right. The experimental parts list keeps its checkboxes that apply at once until ADM-154 rebuilds that list.
- **D14. Missing services keep their notes.** The PDF helper and the identity provider are said in notes today; the empty state of ADM-115 comes with the sections that would be empty without their service.
- **D15. The health band becomes a notice under the introduction.** ADM-080 makes the title the first text on the screen, so the band that sat above every screen's content moves below the head, as a warning or error notice that links to System health.
- **D16. Rules' first part is called Language, time and help.** General now names the section `settings`, and two things called General in one sidebar would be one word for two concepts.
- **D17. Shared sections save as today.** Splitting a screen in two does not add a lock or a version check: the risk of two administrators overwriting each other exists already within one screen. If it should be closed, it is a server change of its own for every settings section.
- **D18. Worker and Encryption key** of spec 075 go to Overview (after System health) and to Security (as Encryption, ADM-203), which supersedes the order of CSL-800 (ADR-048).
- **D19. Space categories go to Organising, saving a space as template to Templates, the CSV import of users stays in Invitations.** The lines do not name them; each goes where its kind belongs (ADM-077).
- **D20. Embed addresses go to Outgoing calls.** ADM-072 names ADM-024, the allowed outside addresses for link cards and embeds, for Outgoing calls; the hosts that rules and webhooks may call are there already. Both lists say what may leave or be fetched, so they belong together, each as its own part.

## Decided on the product owner's behalf on 2026-11-20, may be overruled

Taken while building, where the plan left a choice open or `main` had moved.

- **D21. No route was added.** The table "Through the API" shows a reading and a changing route for every part of every section. The filters of Spaces stay in the browser over the full list `GET /admin/spaces` returns, because that list is already the whole answer; a script filters it the same way. Until 1.0 these routes may still change (the product owner, 2026-11-17).
- **D22. A token still only reads the administration.** The rule of 2026-11-17 asks that every feature be usable through the API; RV10 (ADR-052) made every change under `/admin/` session-only, and many reads too. A script or an MCP server can use the whole administration through a session, so the rule holds. Letting a token change the administration is a security decision of its own (scopes, the second factor of spec 076, what an agent may do unattended), so it waits for the spec that brings MCP rather than being changed here as a side effect.
- **D23. `.note` keeps its look outside the administration (narrows D4).** Inside the administration every note is now a `Notice`, and `ErrorNote` renders the error kind everywhere. The plain `.note` outside the administration keeps its look until each screen adopts `Notice` in its own wave: restyling it at once would change pictures of screens whose waves have not studied them, and two looks already lived side by side for a note and a page panel. The table rule of D5 does apply to every app table at once, as planned.
- **D24. Empty blocks where the empty state is ours alone.** Groups, Service accounts and Webhooks show the block of NAV-173. Automation, Templates and Document control keep their sentence: their lists are shared with the settings of a space (rules, templates) or are a builder (document control), and wave SC5 rebuilds those screens with the same block.
- **D25. Tool rows move to a wave of their own.** Step 10 is the one the plan allowed to move. The arrangement is whole without it, the dropdown filters of Users and the Audit log keep working and stay in the address, and ADM-103 stays open.
- **D26. A shut group shows the section opened from elsewhere.** The jump field of the administration lists results instead of narrowing the sidebar, so "open while the field holds text" (item 2) never applied to it. Instead, when a section is opened by its address or the jump field and its group is shut, the group shows open until its heading is pressed; the person's folded list does not change.
- **D27. Save and Reset changes, also where a choice applied at once.** The mail switches and the export formats now save with their unit's Save, as NAV-163 asks of a unit that saves apart. The experimental parts and the features for all spaces keep applying at once, as D13 says.
- **D28. The health band is a warning or an error notice with a title**, "The disk is filling up", because NAV-150 gives every warning a bold title. Its text and its link to System health are as before.
- **D29. Instance permissions says where to change them.** The list stays read-only (ADR-034); its introduction says that a permission is changed on the person or the group, which is also where the API changes it.
- **D30. The section `settings` keeps its first heading.** General starts with a part "Trash and files", so the parts of a long form each have a heading (ADM-082); the rule that hid the first heading of a section from sight (spec 033 D7) is gone with it.

## Supersedes

ADR-048 records that this spec supersedes, once built: the six groups of the administration (ADR-034); "settings and the administration stay at the left beside their sidebar" of ADR-038's Look item, for the administration; the first heading of a section of spec 033 D7, for the administration; the place of the jump field above the whole sidebar (spec 013 F9); and the order of the System group of CSL-800 (spec 075 D28). The settings of a space keep their sidebar until wave SC5.

## Provenance

Own words, from blueprint 03 chapter 10 section 10.1 and chapter 1 sections 1.13 to 1.15 as written in the study of 2026-11-14 (ADR-045, 02 "One exception"), the built code on `main`, and specs 075, 076 and 078. No pictures, inventories or other product were consulted in writing this spec.
