# 05: Data and permission model

## Core entities

| Entity | Key fields | Notes |
|---|---|---|
| `instance` | id, name, public key | This installation. Used when linking instances later. |
| `user` | id, type (`member` / `guest` / `service`), status, auth links | A local password, OIDC subject or SCIM id can be linked to a user |
| `group` | id, name, source (`local` / `scim` / `oidc`) | Groups from the IdP are read-only in the UI |
| `space` | id, key, name, status (active / archived / trashed) | The unit of ownership and permissions |
| `page` | id, space_id, parent_id, position, title, status | The tree uses `parent_id` plus a **closure table** for fast ancestor and descendant lookups |
| `page_version` | id, page_id, number, document (JSON), author, created_at | Immutable. The current page points to its latest version. |
| `draft` | page_id, user_id, document | One draft per user per page, autosaved |
| `attachment` | id, page_id, object_key, hash, size, encryption key id | The file itself lives in object storage, encrypted |
| `grant` | subject (user / group / guest-all / members-all), scope (global / space), permission | Space and global permissions |
| `restriction` | page_id, kind (`view` / `edit`), subject | Page-level limits |
| `trash_item` | object, deleted_by, deleted_at, purge_after | One universal retention period |
| `audit_event` | see 06 | Append-only |
| `outbox_event` | id, type, payload, created_at, published_at | Events waiting to go to NATS |

**IDs:** UUIDv7 everywhere. They are globally unique, so merging instances later never clashes. **Every row** that holds content carries `origin_instance_id`.

**Document format (ADR-004):** the source of truth is a **versioned JSON document** in our own schema, based on the ProseMirror model. Markdown is a first-class way to type, import and export, but not the stored format. Plain Markdown cannot hold Confluence-grade features (merged table cells, macros, mentions, status labels), and real-time co-editing needs a structured document. The schema has a `schema_version`, and old versions are upgraded on read.

## Permission model

The goal is the depth of an enterprise wiki, with rules simple enough to test fully.

### Layer 1: global permissions
Granted to users or groups: *can use the product*, *can create spaces*, *can view user profiles*, *administrator* (settings, users, audit), *system administrator* (infrastructure, backups, keys).

### Layer 2: space permissions
A matrix per space, granted to users, groups, "all members" or "all guests":

| Category | Permissions |
|---|---|
| Space | view, export, administer (manage permissions and settings) |
| Pages | create, edit, archive, delete (own), delete (any) |
| Comments | add, delete (own), delete (any) |
| Attachments | add, delete (own), delete (any) |
| Restrictions | add or remove page restrictions |

Space admins can apply a **permission template** (for example "Team space", "Read-only knowledge base") instead of ticking boxes.

### Layer 3: page restrictions
- **View restriction:** only the listed users or groups can see this page **and everything below it**. It inherits down the tree, and it can only narrow access, never widen it.
- **Edit restriction:** only the listed users or groups can edit **this page**. It does not inherit, so child pages keep their own settings.

### The rule (one sentence, fully tested)
A user may perform action *A* on page *P* when:
**global "use product"** AND **space permission for A** AND **for every ancestor of P including P: no view restriction, or the user matches it** AND (for edits) **no edit restriction on P, or the user matches it**.

### Design rules
- **Allow-only.** There are no explicit "deny" entries. Experience with file servers shows that deny rules make effective rights impossible to reason about. To limit access, you narrow it with a restriction.
- **Deny by default.** No grant means no access. That includes admins for content in restricted pages, unless they use an explicit "recover access" action, which is audited.
- **Guests** are a user type that can be granted access to **any** space, just like members, but are never included in "all members". They can't see user lists or create spaces unless explicitly granted.
- **One central function** evaluates permissions, `permissions.Can(user, action, object)`, and every query that lists content filters through the same rules in SQL. Search, exports, AI, the API and notifications all use it. No second path exists.
- **Effective permissions** are cached per user and invalidated by events (grant changed, group membership changed, page moved).
- **"Why can/can't I?"** An admin view shows which grant or restriction caused the result.

### How it is tested
A generated test matrix of user type × group membership × space grant × restriction position × action, checked against the rule above. That gives thousands of cases in CI. Each bug found later adds a case.
