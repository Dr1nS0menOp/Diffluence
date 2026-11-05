# 013: Wave F, people, access and administration

Status: planned in detail, not built yet
Phase: wave F of `docs/blueprint/03-functional-requirements.md`. The product owner asked on 2026-11-05 to build the whole of blueprint 03, one commit per wave, and tests it when all waves are done.
Comes after: wave C (mail queue, notifications, Share), wave D (owners, statuses, templates, per-space features) and wave E. Migration 0010.

## Problem

A person is still only a name and an e-mail address. Colleagues cannot be found or recognised, nobody can reset a forgotten password, an invitation does not exist, and a guest looks like a member. A space has three roles and no way to see who really has access. An administrator has a console with lists, but no summary, no search, no way to brand the instance, to freeze it for a backup, or to answer a GDPR request. This wave gives people a face, gives accounts a full life cycle, and turns the console into something an operator can run an instance from.

## What the study found

**How people expect it to work.** Directories list people with a picture, name, job title and department, and a small card shows on names. A profile is partly filled and locked by the identity provider, and a person decides what others see. Self-service recovery is a link by e-mail that works once. Invitations let the person choose their own password. Consoles open on a summary with what needs attention. Sensitive actions ask for the password again in a running session. Erasure is mostly done by anonymising an account, because the content must stay.

**What accessibility demands.** These come from WCAG 2.2 and the WAI-ARIA Authoring Practices, and change the design:

| Source | Demand | What we do |
|---|---|---|
| 3.3.8 Accessible authentication | A password may not be the only way past a memory test. Fields must work with password managers, and paste may never be blocked, also not in code fields. | Correct `autocomplete` on every field, no paste blocking, passkeys stay offered, and reset, invitation and step-up forms follow the same rule. |
| 2.2.1 Timing adjustable | Before a time limit ends, a person gets a warning and at least 20 seconds to extend it. | The idle warning shows two minutes before the end, with Stay logged in. |
| 1.4.13 Content on hover or focus | Cards can be dismissed, hovered, and stay. | The name card works like the tree card of wave B. |
| 2.5.7 Dragging | Cropping a picture and ordering need another way. | The crop takes arrow keys and plus and minus, with buttons. |
| 4.1.3 Status messages, alert pattern | An alert does not take focus and should not vanish by itself. | The site banner is a static region. Only a new maintenance state uses `role="alert"`. |
| 1.4.1 Use of colour | A guest mark cannot be colour alone. | The mark is a word, in `.lozenge`. |
| 1.4.3 and 1.4.11 Contrast | A chosen accent colour must keep contrast. | The server checks the same pairs as `check-design.mjs`. |
| 3.3.4 Error prevention | Actions with legal or lasting effect can be checked and confirmed. | Anonymising, type changes, bulk actions and deleting a group show what they do first. |

**What the numbers say about one accent colour.** The blueprint says the app checks the colour in light and dark. On white (`#ffffff`) text needs a luminance of at most 0.183 for 4.5:1. On our dark surface (`#11161f`) it needs at least 0.21. No single colour passes both, so an accent must be a pair (D6).

**Passwords.** NIST SP 800-63B drops rules about odd characters and forced changes. It asks for a check against known bad passwords, allows long passphrases, and allows paste. The sources read say 15 characters when a password stands alone, and 8 as part of several factors. We have a minimum of 12, in `MinPasswordLength`. The primary text must be read before this number changes, so it stays until the product owner decides.

**What our own code says.** These findings shape the plan:

- `auth_challenges` already stores hashed, expiring, single-purpose tokens, and `identity.PurgeExpired` clears them. Reset, invitation and sign-up links use it. No new table.
- The audit event `user.created` holds the e-mail address, and audit rows cannot be changed. Anonymising cannot reach them (D10).
- `audit_events` refuses every delete through a trigger, and the application role has no delete right. A retention setting cannot simply delete rows, or the protection of ADM-039 is gone (D11).
- `storage.Store` cannot list objects, so finding unused files needs a new method on the interface.
- `settings.Values` only reads whole numbers, so it needs typed sections.
- `Directory` has no rule for guests beyond the route, and returns the e-mail address. The people directory needs its own rule.
- Every poll from wave C would keep a session alive, which makes an idle timeout meaningless (D8).
- `formatDate` and `formatDay` are used 28 times in 9 files, and read no preference.

**Safety.** A reset link never says whether an account exists, and takes the same time either way. A reset does not skip the second factor. A Manager may not make themselves an administrator. A token must never reach account security or the console. A space administrator's audit view must not show the title of a page they may not see (NAV-005). A CSV opened in a spreadsheet can run a formula, so exports of names must neutralise a leading `=`, `+`, `-` or `@`. Images from people are decoded on the server, so size is checked before decoding.

**Erasure.** Anonymising works by sanitising the fields of an account, and the main risk named in the sources is missing a field. Data stays in backups for their retention period, and must be erased again after a restore. Delay is itself a breach, so the screen must make this a short job.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | A personal space is an ordinary space with `kind` and `owner_id`. Its owner has normal grants, and `Evaluate` gets no branch for it | No second access rule. "No grant means no access" stays true |
| D2 | The key of a personal space is `~` and ten characters of the person's id | `~` is not allowed in team keys, so no clash. It holds no name, so it survives a rename and an anonymisation |
| D3 | New space permission `space.manage_access`. `space.administer` implies it, in `Evaluate` | Manager needs it. Implying it avoids a data migration, and nobody can lock a space by unticking it |
| D4 | Someone with only `space.manage_access` cannot add, remove or change `space.administer`, or touch a subject that holds it | Otherwise Manager is administrator with one extra step |
| D5 | Guests see people they share a space with through a direct or group grant. "All members" does not count | One open space would otherwise hand a guest the whole directory, against ACC-036 |
| D6 | An accent is a pair: one colour for light, one for dark. Hover and tint are derived, and every pair is checked | One colour cannot pass in both themes |
| D7 | A person's picture is re-encoded on the server to a 256 pixel PNG, from the crop the browser sends | It strips location data, needs no CSP change, and uses only the standard library |
| D8 | Requests marked as background do not count as activity for the idle timeout | Polling would keep every open tab logged in for ever |
| D9 | Step-up is an access rule, `Sensitive(...)`, valid for 10 minutes | It is visible in `docs/api/routes.txt`, and no route can forget it |
| D10 | Anonymising cannot change audit rows. The screen says so, and the audit retention period is the limit | The chain must stay intact, and an audit trail has its own legal ground. Backups get the same note |
| D11 | Audit retention is a setting, and pruning is a command run with the owner's database role, which leaves a signed anchor | The application still cannot delete audit rows |
| D12 | Tokens work on normal routes within their scope, and never on routes marked `SessionOnly` | A leaked script token must not reach passwords, tokens or the console |
| D13 | Names in answers become a `PersonRef` next to the old `*_name` fields | A lookup route would be a way to test ids. The old fields stay for one release |
| D14 | Group owners are checked in the module, and a group from SCIM stays read-only for them too | The identity provider stays the source of truth |
| D15 | Space categories are one list kept by administrators, and space administrators pick from it | Free typing gives ten spellings of one word |
| D16 | The starter text of a new space comes from the browser, through the normal document gate | UI words stay in the translation files, and the person's language applies |
| D17 | Maintenance mode is a check in the middleware, not in `Evaluate` | It concerns every write, also from SCIM and jobs, and not a person's rights |
| D18 | The release check is off by default, and goes to one fixed address | A check reveals an installation (ADM-050) |
| D19 | Login history shows the address and device, not a place | A place needs a location database, see "For the product owner to decide" |
| D20 | First day of the week is stored, and has no visible effect until a calendar exists | The browser's date field cannot be told. Saying so is better than a setting that seems broken |

## Scope

**In:**

- Chapter 1: NAV-024, 025, 036, 038, 049, 052, 055 to 060, 068, 070, 071, 077.
- Chapter 2: SPC-007, 008, 014, 054 to 059, 076 to 081.
- Chapter 9: ACC-011 to 017, 019 to 028, 032 to 034, 038 to 043, 059, 060, 064 to 068, 071 to 073.
- Chapter 10: ADM-003 to 005, 008 to 011, 013, 014, 016 to 018, 021 to 029, 033 to 037, 043 to 045, 048 to 050, 053, 055, 056.

**Out, and where it goes:**

| Item | Goes to |
|---|---|
| Space templates (SPC-006) | Wave D. NAV-071 only adds the starter text |
| List of restricted pages (ACC-067 as a filter, SPC-065) | Wave D's Content screen, which this wave links to |
| LDAP and SAML (ACC-047, 048), audit streaming (ADM-046) | Later, as the blueprint says |
| Metrics and the automatic restore test (ADM-052, 058) | Phase 3 |
| OAuth scopes and apps | Wave H. Personal tokens are a part of it, not all of it |
| Translations beyond English | After 1.0. The language setting exists, with one choice |
| Allowed outside addresses in use (ADM-024) | The setting is here, the block that uses it is wave E's |

## Feature plan

### F1. The permission model

Everything that changes a rule is listed here, because a rule lives in three places. See "The rule in three places" under Test plan.

- **Manager (SPC-054):** a fourth named set: everything of an editor, plus delete any page, comment and file, plus *manage restrictions* and `space.manage_access`. It has no *administer*, so name, key, shortcuts, archive and delete stay out of reach.
- **Routes:** `GET` and `PUT /spaces/{id}/grants` move from `space.administer` to `space.manage_access`. `/preset` stays with *administer*.
- **Limit (D4):** `SetGrants` refuses with `needs_administer` when a person without *administer* adds or removes *administer*, or changes a subject that holds it.
- **Named sets (SPC-055):** administrators define sets for the whole instance. A set only fills in boxes, like the built-in ones, and adds no rule. Deleting one changes no grant.
- **Create personal space (ACC-059):** a global permission, given to members who can use the product by the migration, as 0005 did for *view profiles*.
- **Expired accounts:** see F5.
- **Edge cases:** an archived space still lets people be managed, as *administer* does today. The last direct administrator stays protected by `keepsAnAdmin`.

### F2. Who has access (SPC-007, 008, 056 to 059, ACC-060)

- **Creating a space:** the form has a choice: Only me, All members can view, All members can edit. The default comes from the setting (ADM-023). One line says who will have access.
- **The permissions screen:** gets a search on name, and a tab **People** that lists each person with their role and how they got it: directly, through which group, or as all members.
- **Before saving:** a change shows who gains and who loses what, with a count for groups. Save asks only when someone loses *view*.
- **For everyone who can view:** `/spaces/:key/people` lists who has access, without the boxes.
- **Export:** CSV with name, kind, role and source, audited, with formulas neutralised.
- **Global permissions:** the console shows per permission who holds it, directly or through which group.
- **Keyboard:** the tabs follow the tabs pattern from wave B. The summary is a `Dialog`.
- **Edge cases:** a group of 10,000 people pages through the list. A guest without *view profiles* sees "All members" as a line, not as names.

### F3. Personal spaces (SPC-076 to 081, NAV-024, NAV-049)

- **Creating:** the account menu offers Create personal space, or My space when there is one. One per person, enforced by a unique index.
- **Start:** private: only the owner has a grant, as space administrator.
- **Key:** D2. The name is the person's display name, and a rename follows it.
- **Directory:** the kinds Team and Personal join All, Favourites and Archived. Others only see a personal space when the owner gave them access.
- **Settings:** the same screens. The owner's own grant has no Remove, and `SetGrants` refuses with `owner_grant`, also through a group change, because the grant is direct.
- **When the owner is disabled:** the space stays, and shows in the console under Spaces with no active administrator. An administrator hands it over to a person, which writes a grant for that person and not for the administrator, or archives it.
- **Moving a page out:** works through move between spaces (wave D).
- **Edge cases:** a person without the permission gets no menu entry, and the route refuses. An anonymised owner's space stays, handed over or archived first.

### F4. Profiles and the directory (NAV-055 to 060, ACC-020 to 023, 033, 038)

- **Directory:** `/people`, with picture, name, job title and department. Filter by text and group, 50 at a time. Service accounts never show.
- **Profile:** `/people/:id`, with picture, name, job title, department, location, local time, e-mail and a short text, as far as the viewer may see, then recent pages and spaces through `perm_visible_pages`.
- **Who may see a person:** F1's rule, D5. A person who may not see someone gets "not found".
- **Field visibility:** per field: everyone who may see the profile, members only, only me. E-mail starts as members only.
- **Locked fields:** an identity provider fills title and department through SCIM, and they show a lock with the reason.
- **Picture:** Upload, then a square crop. Drag to move, wheel or buttons to zoom. Arrow keys move, plus and minus zoom. PNG, JPEG or GIF up to 5 MiB. Width and height are read before decoding, and more than 40 megapixels is refused. Remove goes back to initials.
- **Name card:** `PersonName` is a link with the wave B card: picture, name, title, links to the profile and pages. It follows the *cards on hover* preference. Without access, a name is plain text with no link.
- **Marks:** a guest shows the word Guest. A disabled person shows No longer active, and an anonymised one a neutral name. Both stay on their work, and neither shows in pickers.
- **Accessibility:** a `dl` of facts, and the picture has empty `alt` because the name is beside it.
- **Edge cases:** an empty profile shows the name and one sentence. An unknown time zone leaves out the local time. A person's own profile has Edit.

### F5. Accounts through their life (ACC-011 to 017, 039 to 043)

- **Forgot password:** a link on the login page. It always answers "If this address has an account, a mail is on its way", with equal timing, and is rate limited. The link works once, for an hour. It ends all sessions, and the second factor still applies. It does nothing for accounts of an identity provider, and is hidden when mail is not set up.
- **Invitation:** an administrator enters addresses, a type and groups. The link lasts 7 days, and the person picks a name and password. Resend and Cancel are in the list.
- **Self sign-up:** off by default. Only domains on the list. The address is proven by a link before the account exists, and the answer never reveals that an address is known.
- **Password rules:** minimum length, the blocklist, not the person's name or address. No odd character rules. A person can show the password while typing.
- **Second factor required:** none, administrators, or all members. After login a person without one reaches only `/me` to set it up, by a flag on the session. A passkey counts. Accounts of an identity provider are exempt, and the screen says so.
- **Idle timeout:** a setting. A warning shows two minutes before the end, with Stay logged in and a count. The draft is safe because autosave continues. After the end, login returns to the same address.
- **Login history:** the last 20 with time, method, address and device. A login from a new device sends a mail when mail is set up.
- **Expiry of a guest:** a date on the account. From that moment the account counts as not active, by rule. A job then ends sessions and sets the status, and tells administrators seven days before.
- **Guests list:** the Users screen filters on guests, with spaces and last login.
- **Inviting a guest to a space:** with `space.manage_access`, when the instance allows it. The invitation carries a viewer grant on that space only.
- **Turning a guest into a member and back:** a dialog names the spaces they gain or lose through "all members", and the global permissions that change.
- **Service accounts:** can be created, cannot log in by any browser method, and use tokens from an administrator.
- **Edge cases:** a link used twice says it is no longer valid. The last administrator cannot be turned into a guest. Disabling during a reset ends it.

### F6. Tokens and step-up (ACC-019, ADM-005)

- **Tokens:** Your account, Tokens. A name, an expiry of at most a year, and a scope: read, or read and write. Shown once, with a `dfl_` prefix so secret scanners find them. Stored hashed, with the last use. An administrator can turn the feature off, which stops all tokens.
- **What a token does:** it acts as the person, so every permission check stays the same. A write needs the write scope. `SessionOnly` routes refuse tokens.
- **Step-up:** a route wrapped in `Sensitive` answers `reauth_required` after 10 minutes. The client opens a dialog for the password or a passkey, then repeats the request. Accounts of an identity provider go through a fresh login with `prompt=login`.
- **Which actions:** give or take *administrator*, reset someone's password or second factor, anonymise, recover access, create a SCIM or service token, maintenance mode, audit retention, change the second-factor rule.
- **Edge cases:** a failed step-up counts towards the login limit. A session that is older than the idle time never passes.

### F7. Preferences and dates (NAV-077, ACC-024 to 026)

- **Your account** gets sections: Profile, Preferences, Security, Tokens, Notifications, Watches.
- **New preferences:** language, time zone, date format, first day of the week (D20), typing replacements, toolbar pinned, editor width.
- **Dates:** `useDates()` from one provider replaces `formatDate` and `formatDay` in all 9 files, and the words of `relativeTime` follow the language.
- **Time zones:** checked against the IANA list, with `time/tzdata` built in. A new person gets the defaults of ADM-022.
- **First use (NAV-036, 070):** a card on the start screen explains spaces and pages, and can be dismissed. A short tour of three steps in a `Dialog` shows once, and can be skipped. Both are remembered in the preferences.
- **Starter text (NAV-071):** D16.

### F8. Groups (ACC-032 to 034)

- **Owner:** a group can have owners, who manage members from a screen outside the console, `/groups/:id`. They cannot rename, delete or change permissions.
- **Own groups:** on your own profile.
- **Deleting:** a dialog lists the spaces and the number of restrictions that use it. It is refused when it would leave a space with no administrator. A restriction that held only this group stays, and then nobody is on it, which closes rather than opens.
- **Restrictions (ACC-064 to 068):** the dialog gets three states: Open, View only for most, Private. A person setting a restriction is added to it by the server, unless a group already covers them. It says how many pages below are affected, counting what the person can see. The lock shows open or closed.
- **When something is off:** the author is told when access is recovered (ACC-071). When the last person on a restriction is disabled, space administrators are told (ACC-073). Share's inspect entry opens the explanation (ACC-072).

### F9. The console (ADM-003 to 005, 008 to 011, 033, 034, 043 to 045)

- **Summary:** the first page: users, spaces, pages, files, storage, failed jobs, and what needs attention.
- **Search:** a field above the menu finds settings by name, and goes to them. The index is built from the same words the screens use.
- **Users:** filters on type, status, group, last login and login method, in the address. Selecting rows offers Disable and Add to group, with a count first. CSV import has a preview, runs as a job, and reports each row.
- **A user's screen:** groups, spaces with their role, last login, and the number of pages they own, with Hand over. It shows no page titles (ADM-012).
- **Clean-up:** people who have not logged in for 30, 90 or 365 days.
- **Spaces:** pages, storage and last activity, with filters. A list of spaces with no active administrator.
- **Audit:** filters on person, action, object, space, result and time, in the address. A row opens with all its details and a link to the object.
- **Audit of a space:** a space administrator sees their own space. A row about a page they may not see shows the action and the time, and no title or details.

### F10. The look of the instance (NAV-025, ADM-013, 014, 016 to 018)

- **Name and logo:** PNG, JPEG or WebP, checked like an attachment. SVG is refused, because it can carry scripts. Served from a public route, as the login page needs it.
- **Accent:** two colours (D6). The form shows a preview, and each failing pair by name. `appearance.ts` applies it as it does the font.
- **Login page:** a welcome text, plain and without markup, and a link to terms, through `SafeHref`.
- **Banner:** text, level, end date. A person can dismiss it, remembered per banner. The banner is a static region, because it is there on load.
- **Welcome page (NAV-038):** an administrator pastes the address of a page, since they may not be able to read it. It shows above the feed for those who may see it, and the check is the normal one.
- **Reset:** one action, which asks first, returns all of this to the defaults.
- **Help address (NAV-068):** a field.

### F11. Rules, mail and care (ADM-021 to 029, 035 to 037, 048 to 050, 053, 055, 056)

- **Settings** become typed sections: general, features, access, retention, mail, look. Each has its own limits and one audit event.
- **Features:** comments, reactions, blogs, invitations, personal spaces, self sign-up, analytics, automation. Off hides and deletes nothing.
- **Defaults:** language, time zone, permission set for new spaces, draft retention, text in mail.
- **Audit retention:** D11. The console shows the policy, and the command `diffluence audit prune`, run with the owner's role, verifies the chain, writes a signed anchor and removes older rows. Verification then starts at the anchor.
- **Mail:** server, sender, reply-to and Send test. The password stays in a file or the environment. The queue view shows address, kind, status and time, with no content.
- **Content defaults:** screens for templates, statuses and categories, on what waves D and F built.
- **Jobs:** Retry and Cancel on failed and queued jobs, audited. A running job cannot be cancelled in this wave.
- **Attention list:** mail not set, no backup seen, a node with an old version, disk nearly full, spaces with no active administrator, accounts about to expire.
- **New release:** D18. A switch, then one request a day.
- **Maintenance mode:** a system administrator turns it on with a message. All writes get 503 with `maintenance`, except login, logout and turning it off. Jobs and the mail queue wait. Open editors keep their text in the browser. Nodes read the state at most five seconds late.
- **Reports:** largest spaces, pages and files, by size only.
- **Unused files:** `Store` gets `List`. A job compares it with `attachment_versions`, skips files younger than 24 hours, and lists what it found. Removal is a separate action, which asks first.

### F12. Personal data (ACC-027, 028)

- **Download:** Your account, Download my data, runs as a job: profile, preferences, groups, sessions, login history, own comments and reactions, and the titles of own pages they can still see, as JSON in a zip.
- **Removal:** a person asks through a button, and administrators get a notification.
- **Anonymise:** an administrator, with step-up and a reason. Name becomes `Former user` and a short code, address becomes one that cannot receive mail, and profile, picture, sessions, tokens, passkeys, login history, identity links, follows and preferences go. Content stays. Mentions show the neutral name, because they resolve by id.
- **What it cannot reach:** the dialog says: audit rows until their retention ends, backups until they expire, and names people typed in text. After a restore it must be done again, which the operations guide will say.

## API

All new routes, with their access rule.

| Route | Rule | What |
|---|---|---|
| `POST /auth/forgot`, `POST /auth/reset` | public: nobody is logged in yet | Reset by mail, rate limited |
| `GET, POST /auth/invitation` | public: the token is the proof | Accept an invitation |
| `POST /auth/signup`, `POST /auth/signup/confirm` | public: off by default | Self sign-up |
| `GET /appearance` | public: the login page needs it | Name, logo, accent, welcome text |
| `GET /appearance/logo` | public: the login page needs it | The logo |
| `POST /auth/reauth` | authenticated, session only | Step-up |
| `GET /people`, `GET /people/{id}` | authenticated, module decides | Directory, profile |
| `GET /people/{id}/card`, `GET /people/{id}/picture` | authenticated, module decides | Card, picture |
| `GET, PUT /me/profile`, `PUT, DELETE /me/picture` | authenticated | Own profile |
| `GET /me/logins` | authenticated, session only | Login history |
| `GET, POST, DELETE /me/tokens` | authenticated, session only | Personal tokens |
| `POST /me/export`, `POST /me/removal-request` | authenticated, session only | Personal data |
| `POST /me/personal-space` | permission `create_personal_space` | Create it |
| `GET /groups/{id}`, `PUT, DELETE /groups/{id}/members/{user}` | object `group.manage_members` | Group owners |
| `GET /spaces/{id}/people` | object `space.view` | Who has access |
| `GET /spaces/{id}/access`, `/access.csv`, `POST /grants/preview` | object `space.manage_access` | Access in detail |
| `POST /spaces/{id}/guests` | object `space.manage_access` | Invite a guest |
| `GET /spaces/{id}/audit` | object `space.administer` | Audit of a space |
| `GET /admin/summary`, `GET /admin/attention` | permission `administrator` | Console |
| `GET, POST, DELETE /admin/invitations` | permission `administrator` | Invitations |
| `POST /admin/users/bulk`, `POST /admin/users/import` | permission `administrator` | Bulk, CSV |
| `POST /admin/users/{id}/anonymise` | sensitive, `administrator` | Anonymise |
| `POST /admin/users/{id}/handover` | permission `administrator` | Hand over pages |
| `GET /admin/users/{id}/type-impact` | permission `administrator` | What a type change does |
| `POST /admin/service-accounts/{id}/tokens` | sensitive, `administrator` | Service tokens |
| `GET /admin/global-permissions` | permission `administrator` | Who holds what |
| `GET /admin/groups/{id}/impact`, `PUT /admin/groups/{id}/owners` | permission `administrator` | Groups |
| `POST /admin/spaces/{id}/handover` | permission `administrator` | Personal space owner |
| `GET, PUT, DELETE /admin/permission-sets` | permission `administrator` | Named sets |
| `GET, PUT, DELETE /admin/categories` | permission `administrator` | Categories |
| `GET, PUT /admin/settings/{section}` | permission `administrator` | Typed settings |
| `PUT, DELETE /admin/appearance/logo`, `POST /admin/appearance/reset` | permission `administrator` | Look |
| `POST /admin/mail/test`, `GET /admin/mail/queue` | permission `administrator` | Mail |
| `POST /admin/jobs/{id}/retry`, `/cancel` | permission `administrator` | Jobs |
| `GET /admin/reports/largest`, `GET, POST /admin/storage/unused` | permission `administrator` | Data care |
| `PUT /admin/maintenance` | sensitive, `system_administrator` | Maintenance mode |

`GET` and `PUT /spaces/{id}/grants` change their rule to `space.manage_access`. The router gains `Sensitive(Access)` and `SessionOnly(Access)`, and both show in `docs/api/routes.txt`.

## Data

Migration 0010, expand only:

- `users`: `expires_at`, `last_login_at`, `anonymised_at`.
- `spaces`: `kind` (`team` or `personal`, default `team`), `owner_id`, and a unique index on `owner_id` for personal spaces.
- `sessions`: `reauth_at`, `needs_second_factor`.
- `user_profiles`: job title, department, location, about, picture key, field visibility, locked fields.
- `group_owners`, `api_tokens`, `login_history`, `known_devices`, `permission_sets`, `space_categories`, `space_category_links`, `audit_anchors`.
- `global_grants`: *create personal space* for members who can use the product.
- `CREATE OR REPLACE` for `perm_has_global` and `perm_spaces`, for expired accounts, and a new `perm_visible_people`.

`auth_challenges` gets new kinds, and no new table. Preferences live in the JSON of `user_preferences`, so they need no column. N and N+1 run together: the older release ignores the new columns, and already follows the stricter functions.

## Build order

1. The rules: `space.manage_access`, expiry, `perm_visible_people`, with the matrix first.
2. Router: `Sensitive`, `SessionOnly`, tokens, step-up, background requests.
3. Typed settings, then maintenance mode, features and the appearance routes.
4. Profiles, pictures, `PersonRef`, the people directory, name cards.
5. Account life cycle: reset, invitation, sign-up, password rules, second-factor rule, idle timeout, history.
6. Personal spaces, who has access, Manager in the screens, restrictions dialog.
7. Guests, service accounts, group owners.
8. Preferences, dates, first use.
9. Console: summary, search, users, spaces, audit, reports, jobs, unused files.
10. Personal data and anonymising.
11. Pictures, docs, statuses in `03-functional`, handoff.

## Test plan

**The rule in three places.** The matrix compares a literal rule, `permissions.Evaluate` and the SQL. Each change lands in one commit, in all places at once:

| Change | `Evaluate` and facts | SQL | Matrix test |
|---|---|---|---|
| `space.manage_access` | New constant, in `AllSpacePermissions`, in `worksWhenArchived`, and a case in `actionAllowedBySpace` that also accepts *administer* | None: lists only ask for `space.view`. A comment in the migration says so | New entry in `actions`, a `manager` grant case, and the same rule written out in `literalRule` |
| Expired account | `LoadUser` sets `Active` to false from `expires_at`. `Evaluate` stays as it is | `perm_has_global` and `perm_spaces` add the date to `x.status = 'active'` | New `userCases`: `expired-guest`, `expired-member`, and a guest expiring tomorrow, in all three answers |
| Who may see a person | New pure function `EvaluatePerson` over viewer and target facts | New `perm_visible_people` | New `TestMatrixPeople`: viewer type, *view profiles*, shared by user, group, all members or nothing, target active, disabled or service, literal against function against SQL |
| Personal spaces | No change, on purpose (D1) | No change | An integration test proves the owner holds an ordinary grant, and that losing it means no access |

**Other levels:**

- **Go integration:** every route, each with a refusal. Reset: unknown address, used link, expired link, second factor still asked, sessions ended. Manager cannot reach *administer*. Tokens: scope, revocation, refused on `SessionOnly`. Step-up after 10 minutes. Maintenance refuses a write, also through SCIM. Space audit hides titles. CSV neutralises formulas. Settings refuse bad values, and a refusal leaves no event.
- **Anonymise:** after it, a test walks every text and JSON column of every table through `information_schema`, and fails when the old name or address is found anywhere outside `audit_events`. A new table is covered the moment it exists.
- **Route walk:** the new object routes join `TestEveryObjectRouteIsProtected`, and a new test proves each `Sensitive` route answers `reauth_required`.
- **Mutation:** a Manager grants *administer*, an expired account stays active in SQL, in `LoadUser`, a guest sees everyone, a reset link works twice, a reset skips the second factor, a read token writes, a token reaches `/me/tokens`, `Sensitive` lets anything through, a maintenance write passes, a dangerous CSV cell passes, a failing accent colour is accepted, SVG passes as a logo, a space audit shows a hidden title, polling keeps a session alive.
- **Vitest:** key of a personal space, crop maths, dates per time zone and format, contrast derivation, settings search, CSV preview.
- **Browser:** each section above, keyboard first. Forms are checked with a password filled by paste.
- **Pictures:** People, Profile, Your account with its sections, Access of a space, console Summary, Audit, Appearance, Maintenance banner, in light, dark and narrow where it matters, with short content as well as long.
- **Accessibility:** axe on each new screen, and with the crop, step-up and idle dialogs open.

## Acceptance criteria

- [ ] AC-1 The matrix passes with Manager, expired accounts and the people rule, in all three answers.
- [ ] AC-2 A Manager manages people and content, and cannot reach settings or *administer*.
- [ ] AC-3 A space shows who has access and how, a change shows who gains and loses, and the list can be exported safely.
- [ ] AC-4 A person creates one personal space, which starts private, and it survives their disabling through a hand-over.
- [ ] AC-5 People can be found, and a guest only finds who they share a space with.
- [ ] AC-6 A profile shows what the viewer may see, a person chooses that, and locked fields stay locked.
- [ ] AC-7 A picture can be cropped by keyboard, and leaves no metadata.
- [ ] AC-8 Guests and former colleagues are marked on their names, and absent from pickers.
- [ ] AC-9 Reset, invitation and sign-up work, reveal nothing, and work once.
- [ ] AC-10 A second factor can be required, and the idle warning keeps a draft safe.
- [ ] AC-11 An expired guest has no access from that moment, on every list.
- [ ] AC-12 Tokens stay inside their scope, and never reach account security or the console.
- [ ] AC-13 Sensitive actions ask again after 10 minutes.
- [ ] AC-14 Dates follow a person's time zone and format everywhere.
- [ ] AC-15 Group owners manage members, and deleting a group says what it touches.
- [ ] AC-16 The restrictions dialog shows three states, and nobody locks themselves out.
- [ ] AC-17 The console opens on a summary, finds settings, filters users and audit, and imports a CSV with a preview.
- [ ] AC-18 A space administrator sees their audit, without titles of hidden pages.
- [ ] AC-19 Name, logo, accent pair, login text, banner and reset work, and a failing colour is refused.
- [ ] AC-20 Maintenance mode stops every write and shows why, and text in an open editor survives.
- [ ] AC-21 Unused files are reported before anything is removed.
- [ ] AC-22 A person downloads their data, and anonymising leaves their name in no table but the audit log.
- [ ] AC-23 Everything passes axe, and the new pictures have been looked at.

## Risks

| Risk | Answer |
|---|---|
| A Manager finds a way to *administer* | D4 is tested in both directions, with a mutation |
| A rule change reaches only two of three places | The table above, and one commit per change |
| A field is missed when anonymising | The test walks every column of every table |
| Moving 28 date calls changes screens | One hook, and the pictures show any drift |
| `prompt=login` differs per identity provider | Tested against the fake provider of the test suite, and on Authentik by the product owner |
| An idle timeout logs someone out mid-sentence | The warning, autosave, and typing counts as activity |
| Maintenance mode blocks its own switch-off | The route is on the exception list, and a test covers it |
| Decoding an uploaded picture costs too much memory | Dimensions are read first, with a limit |
| Pruning audit rows breaks the chain | Only the command with the owner's role can do it, it verifies first, and it leaves a signed anchor |
| This wave is very large | The build order is in independent parts, each with its own tests |

## For the product owner to decide

1. **Blocklist of leaked passwords (ACC-014).** The offline check needs a list made by others. Advice: embed the top 100,000 of the SecLists project (MIT), stored as hashes, about 400 KB. It is data, not code, and goes in `docs/dependencies.md`. Without it, only an operator's own file works.
2. **Minimum password length.** It stays at 12. NIST's revision, as summarised by the sources read, says 15 for a password alone. Changing it affects new passwords only.
3. **Place in login history (ACC-017).** A place needs a location database with its own licence and updates. Advice: leave it out, and show the address and device. ACC-017 then stays partly.

## How the product owner tests it

1. Log in as a guest. Open People: you only see those in your spaces, and your name has the mark.
2. Pick Forgot password on the login page, with a real and an unknown address. The answer is the same. Use the link twice.
3. Invite someone, and accept in another browser.
4. In Your account, add a picture, crop it with the keyboard, and hide your e-mail. Look at your profile as someone else.
5. Set your time zone to Tokyo, and look at a date.
6. Create your personal space, then disable that account as an administrator, and hand the space over.
7. Give someone the Manager role. Try to make them administrator as them.
8. Open Who has access on a space, change a role, and read the summary.
9. As administrator, disable a user: it asks for your password after 10 minutes.
10. Set a logo and accent, and try a colour with poor contrast.
11. Turn on maintenance mode with a colleague in the editor. Turn it off, and publish.
12. Anonymise a test user, and search for their name.

## Provenance

Own text from these public sources, read for behaviour only, plus the study behind blueprint 03 (`docs/blueprint/03-functional/sources.md`):

- W3C WCAG 2.2 Understanding 3.3.8 Accessible Authentication (Minimum).
- W3C WAI-ARIA Authoring Practices: alert.
- NIST SP 800-63B, through summaries by Enzoic, Netwrix and Authsignal. The primary text must be read before the minimum length changes.
- Public documentation on anonymising accounts: JetBrains YouTrack and Hub help, Bloomreach docs.
- European Data Protection Board, published Article 17 decision (delay in erasure).
- A public design note on erasure, backups and legal holds (techinterview.org), as a secondary source and not as legal advice.
- The contrast figures were worked out from WCAG's formula and our own tokens.
