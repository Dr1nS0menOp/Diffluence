# Spec 033: the Stil look, settings in the sidebar, and People

Status: built on branch `wave-x`, waiting for the product owner's acceptance session.
Phase: after blueprint 03, first in the order of ADR-034, before roles you make yourself.

## Why

After testing, the product owner found the function right and the look short of it: settings menus and many buttons were not attractive, the People table of a space was poor, and the sidebar disappeared on the home page. They chose direction **A, Stil** from three proposals (`docs/design/proposals/2026-11-07-three-directions.html`), recorded in ADR-034. In their words, nobody takes the product if they do not like the design.

## What the product owner asked, and the answer

| Ask | Answer in this wave |
|---|---|
| The look of Stil: white and blue, hardly any lines or boxes | Sidebar without its border line, no lines in the strip and above the settings links, tiles without a frame, tables of the app as rows with a line under each and a quiet heading instead of a grid |
| Sidebar nearly always in view, also on the home page | A **Personal** frame for Home, Recent, Favourites, Tasks, Watching and all spaces: your own places and your spaces |
| Sidebar parts fold, and can be made larger; kept per person | `SidebarPart`: **This space** folds from its heading and has a handle to size it by mouse or arrow keys. Both are stored in the person's preferences (`sidebar_shut`, `sidebar_links_height`), so they follow them to other devices. The personal sidebar has two parts that fold too |
| No guide lines in the tree, tidier filters | No guide lines. The tree follows the space part without a **Pages** heading, folded from the first level, so the top pages show with their branches shut. Its tools are a quiet row of icons above it; the filter and the order of the pages unfold from the filter icon and stay open while they do something |
| The search box in the Stil look, saying where it searches | A softer, rounded field that turns white with an accent edge in use. Its placeholder is "Search in" the organisation, a company or personal name given at the first setup (`diffluence user create-admin --organisation`) and changed under Administration, Look; without one it says "Search". The name is one line of at most 200 characters without < or >, and only ever shown as text |
| Labels without an outline | Plain links with a quiet `#`. Adding one reads as words until hovered or used |
| Space settings in the sidebar, the large area to change them | A **Settings** frame: groups General, Who can do what, Content, Automation, Import and export, Archive or delete, which fold, with **Jump to a setting**. One section at a time at `?section=`; sections that are screens of their own (Access, Audit, Content, Trash, Automation, Webhooks, Public sites) are links |
| The administration in categories | The same frame. Overview; People and access; Content; Automation; Look; System, with Care and System health under System. The existing search keeps finding settings inside a section |
| People of a space rebuilt | Rows with a picture, the name, a Guest mark, how they got in, and the role. Someone who manages access picks the role in the row; a change that takes away view asks first, as on the Access screen. Tabs Everyone, Administrators, Groups, Guests, and a search. A button leads to adding people |
| People lists in the administration filter by dropdowns | Two new dropdowns beside type, status, source, group and last login: **role** (administrator, system administrator, may create spaces, may invite people) and **space** (people who can view it) |
| The search box always searches all spaces, with filters that unfold | No chip that limits the box to the current space. Clicking the box unfolds one panel: filters as buttons (Spaces, Type, Labels, People, Date), each with a chooser, then the results. The space chooser shows the chosen spaces and the first ones in the person's order, and a field finds any other space by name (Enter picks the first match). Spaces, labels and people become words of the existing syntax (`space:`, `label:`, `author:`); type and date go as the `kind` and `within` parameters the results page already reads. The results page shows the words as chips that take one out again. Keyboard: Tab moves through the buttons, which say whether they are open or pressed; leaving the box and its panel closes it |
| Automation for the instance, choosing which spaces it touches | A rule of the instance has **Where this rule acts**: every space, or only chosen spaces as chips. Events in other spaces do not start it, the By hand menu leaves it out there, and its lookups search only its spaces |

Roles you make yourself (RBAC) are not in this wave: they get their own study and wave, as ADR-034 says. The Access screen keeps working as it is.

## Decisions

- **D1.** The chosen spaces of an instance rule live in its definition (`definition.spaces`), not in a new column. Migration numbers must be contiguous and two other waves hold the next ones; the definition is already stored as JSON and travels with export and import. A rule of a space with spaces is refused; a space that does not exist is refused; at most 500.
- **D2.** An event without a space, such as a new group member, is not narrowed by spaces: those triggers belong to the instance.
- **D3.** The guest mark comes from the server in the list of who has access (`guest` on each holder), one query for all, rather than one request per person.
- **D4.** The user list filters on role with an allow-list of four global permissions, and on space through `perm_spaces`, the same rule as everywhere else. Both are for administrators only, like the list.
- **D5.** The parts of the sidebar are preferences, validated on the server: only known part names, a height of 0 or 4 to 40 rem.
- **D6.** Settings sections have addresses, so a link, a reload or Back lands on the same section.
- **D8.** The filters of the search box are words of the existing syntax, not a second set of parameters, so the coming query language (DQL) can grow from them and a query and the buttons mean the same thing. Type and date have no words yet; giving them words belongs to DQL, which is not built here.
- **D9.** A part of the sidebar never shrinks below what it holds, so parts cannot run over each other; the sidebar scrolls instead, and the space part scrolls inside its own height.
- **D7.** Under the page title, the first heading of a section only speaks to screen readers, so the name does not show twice.

## Load budget (ARC-238)

| Feature | Queries | Class |
|---|---|---|
| Sidebar parts | none extra: they ride on the preferences already loaded | reading |
| Personal frame | the two lists the space strip already loads (cached) | reading |
| People of a space | the existing list plus one query for guests; the grants list only for those who manage access | reading |
| User list filters | the same one query, with two extra conditions | reading |
| Scope of an instance rule | no extra query to match; one count when saving | saving |

## Tests

Failing first, then passing: an instance rule acts only in its chosen spaces; its lookup looks only there; one without spaces acts everywhere; only an instance rule chooses spaces and only real ones; the people of a space say who is a guest; the user list filters by role and by space and refuses unknown values; the sidebar parts are kept and refuse unknown values. Five mutations cover the new rules. Unit tests cover the filter words of the search box (written and read back), jumping to a setting, and the tabs of the People screen. A browser test checks that the box finds pages in other spaces, that choosing a space narrows the list, and that the results page keeps and shows the words. Browser tests moved with the screens: settings tests open their section; tree filter tests open the filter first.

## Sources

Our own proposals and ADR-034. Screenshots of other products, shared by the product owner, were used for behaviour and structure only, never for look, words or icons (blueprint 02).
