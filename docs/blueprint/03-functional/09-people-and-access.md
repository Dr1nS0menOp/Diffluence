# 03.9: People and access

## 9.1 Accounts and login

- **ACC-001** · built · A local account has an e-mail address, a display name and a password hashed with Argon2id.
- **ACC-002** · built · Login is by password, by password with an authenticator code, by passkey, or by single sign-on with OpenID Connect.
- **ACC-003** · built · The login page shows only the methods that are configured.
- **ACC-004** · built · Failed logins are rate limited, and the wording never reveals whether an account exists.
- **ACC-005** · built · A person changes their own password with the current one. All other sessions then end.
- **ACC-006** · built · A person sees their sessions, and can end any of them.
- **ACC-007** · built · A person sets up an authenticator app and receives 10 recovery codes, each usable once.
- **ACC-008** · built · A person registers, lists and removes passkeys.
- **ACC-009** · built · An administrator resets a password or the second factor of a person, and it is audited.
- **ACC-010** · built · The first administrator is created from the command line.
- **ACC-011** · built · **Forgot password:** a person asks for a link by e-mail, which works once and expires within an hour. It works only when e-mail is configured, and never for accounts of an identity provider.
- **ACC-012** · built · **Invitation:** an administrator invites by e-mail. The person chooses their own password and name, and the link expires.
- **ACC-013** · built · **Self sign-up:** optional, limited to e-mail domains on a list. Off by default.
- **ACC-014** · built · **Password rules:** a minimum length, with a check against lists of leaked passwords that works offline, and no rules that force odd characters.
- **ACC-015** · built · An administrator can require a second factor for all members, or for administrators only.
- **ACC-016** · built · A session ends after a configurable idle time, with a warning first, and the draft stays safe.
- **ACC-017** · built · A person sees recent logins with place and device, and is told by e-mail of a login from a new device.
- **ACC-018** · built · Single sign-on links accounts on stable ids, and never takes over a local account by e-mail alone.
- **ACC-019** · built · **Personal API tokens:** a person creates tokens for scripts, with a name, an expiry and a limited scope. Tokens are shown once, stored hashed, and can be revoked. An administrator can turn the feature off.

## 9.2 Profile and preferences

- **ACC-020** · built · A profile screen shows the picture, name, job title, department, location, local time, e-mail and a short text, within what the viewer may see.
- **ACC-021** · built · It lists recent pages, spaces and blog posts by the person, filtered for the viewer.
- **ACC-022** · built · A person uploads and crops a profile picture. Without one, the initials show on a tint chosen from the name.
- **ACC-023** · built · A person chooses which fields are visible to others. An identity provider can fill and lock some fields.
- **ACC-024** · partly · **Your account** holds security, sessions and passkeys. Built: profile, preferences, security, tokens, data. Still open: notifications and watches (wave C).
- **ACC-025** · partly · **Preferences:** language, time zone, date format, theme, keyboard shortcuts on or off, hover cards on or off, typing replacements on or off, toolbar pinned or not, editor width. Built: language, time zone, date format, first day, theme, shortcuts, cards. Still open: typing replacements, toolbar pinned, editor width.
- **ACC-026** · built · Preferences are stored with the account, not in the browser, so they follow a person.
- **ACC-027** · built · A person can download all their personal data, and ask an administrator for removal, for GDPR requests.
- **ACC-028** · built · A disabled person's name stays on their work, marked as no longer active. An administrator can anonymise the name for a GDPR removal, with the content untouched.

## 9.3 Groups

- **ACC-029** · built · An administrator creates, renames and deletes groups, and adds and removes members.
- **ACC-030** · built · Groups from SCIM or single sign-on are read-only in the screens.
- **ACC-031** · built · Groups hold global permissions and space permissions, and appear on restrictions.
- **ACC-032** · built · A group has an owner who may manage its members without being an administrator.
- **ACC-033** · built · A person sees the groups they belong to on their profile.
- **ACC-034** · built · Deleting a group shows which spaces and restrictions use it, and what changes. A group that would leave a space with no administrator cannot be deleted.

## 9.4 Guests and other kinds of account

- **ACC-035** · built · A guest is a user type. Guests are never part of "all members".
- **ACC-036** · built · Guests see only what is granted to them directly, and cannot browse people unless granted *view profiles*.
- **ACC-037** · built · Guests can be granted any space, but start with none.
- **ACC-038** · built · A guest has a mark on their name, so others know they are talking to someone outside.
- **ACC-039** · built · A guest account can expire on a date, and an administrator gets a reminder before that.
- **ACC-040** · partly · An administrator can list guests with their spaces and last login. Built: guests by filter, with last login. Still open: their spaces in the list.
- **ACC-041** · built · A space administrator can invite a guest to their space, when an administrator allows it.
- **ACC-042** · built · A guest can be turned into a member, and back, with a clear warning about what changes.
- **ACC-043** · built · A service account is a user type for automation.

## 9.5 Provisioning

- **ACC-044** · built · SCIM 2.0 provisions users and groups from Authentik, Entra ID and others, with tokens from the admin console.
- **ACC-045** · built · A user removed through SCIM is disabled, and keeps their content.
- **ACC-046** · built · A `groups` claim can keep group membership in step at login.
- **ACC-047** · later · LDAP directory sync for instances without SCIM.
- **ACC-048** · later · SAML 2.0 single sign-on, for identity providers without OpenID Connect.

## 9.6 The permission model

- **ACC-049** · built · Three layers: global permissions, space permissions and page restrictions.
- **ACC-050** · built · **The rule:** a person may do something on a page when they may use the product, and their space permission allows it, and no view restriction above or on the page excludes them, and for edits no edit restriction on the page excludes them.
- **ACC-051** · built · Permissions only allow. There is no explicit deny.
- **ACC-052** · built · No grant means no access, also for administrators.
- **ACC-053** · built · A view restriction applies to the page and everything below it. An edit restriction applies to that page only.
- **ACC-054** · built · Restrictions can only narrow. A person on a restriction list with no space access gets nothing.
- **ACC-055** · built · The same rule is used in one function, and in the SQL for lists. Thousands of generated cases compare them.
- **ACC-056** · built · A person who may not view gets "not found". A person who may view but not act gets "forbidden".
- **ACC-057** · built · Every refusal is audited as denied.
- **ACC-058** · built · Global permissions are: use the product, create spaces, view profiles, administrator, system administrator.
- **ACC-059** · built · A global permission **create personal space**.
- **ACC-060** · built · Global permission screens show who has each permission through which group.

## 9.7 Page restrictions

- **ACC-061** · built · The lock opens the restrictions of the page: who can view it and everything below it, and who can edit it.
- **ACC-062** · built · The dialog shows restrictions inherited from pages above.
- **ACC-063** · built · Restrictions are managed by people with *manage restrictions*, who can see the page.
- **ACC-064** · built · Three clear states at the top of the dialog: **Open** (everyone in the space can view and edit), **View only for most** (everyone can view, chosen people can edit), and **Private** (only chosen people).
- **ACC-065** · built · A person setting a restriction is added to it, so nobody locks themselves out by accident.
- **ACC-066** · partly · The lock icon shows the state: open or closed. Everyone who can see the page can see that it is restricted. Built: the word Restricted for everyone who can see the page. Still open: an open and a closed lock.
- **ACC-067** · built · A list in space settings shows all restricted pages (SPC-065). It is the flag `restricted` on the Content screen of a space (SPC-065).
- **ACC-068** · built · The dialog says how many pages below are affected.

## 9.8 When something is off

- **ACC-069** · built · **Why can or can't I?** Space administrators and administrators ask, for any person and action on a page, for the result and the layer that decided it. It uses the same evaluation as the real check.
- **ACC-070** · built · **Recover access:** an administrator gives themselves access to a space or page, with a reason. It is always audited.
- **ACC-071** · built · The author of a page is notified when an administrator recovers access to it.
- **ACC-072** · new · Share has an **inspect** entry that opens the same explanation for a person who asked for access.
- **ACC-073** · partly · When the last person on a restriction is disabled, a space administrator is told, and can recover the page. Built: the console lists such a page without its title, and an administrator can recover it. Still open: a notification to the space administrator, because a notification about a page they cannot see does not show.
