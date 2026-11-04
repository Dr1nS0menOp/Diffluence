# 03: Functional requirements

Priority: **M** = must for that phase, **S** = should, **L** = later. The phase numbers refer to 08.

## v1: stable document wiki (phases 1 and 2)

### Spaces and pages
- M: Create a space with a key, name, description, icon and home page. Archive and restore a space.
- M: Page tree per space with unlimited depth: create, rename, move (drag and drop), reorder, copy, delete
- M: Breadcrumbs, a page tree sidebar, recently viewed and favourite pages
- M: Stable page URLs that survive renames and moves
- S: Personal space per user

### Editor and content
- M: WYSIWYG editor with Markdown shortcuts (`#`, `**`, `-`, backticks)
- M: Headings, lists, task lists, tables, code blocks with highlighting, quotes, links, images, horizontal rules
- M: Markdown import (`.md` files and zip files with folders) and export (page, page tree, space)
- M: Autosave of drafts, and an explicit "publish" that creates a version
- S: Paste from Word and Google Docs with sensible clean-up

### Versions
- M: Every publish creates a version. View any version, compare any two, restore one.
- M: Shows who changed what and when

### Attachments
- M: Upload files to pages, with a configurable maximum size. Download, replace (versioned) and delete.
- M: Image preview and inline images
- S: PDF preview

### Search
- M: Full-text search over titles, content and attachment names, filtered by space, author and date
- M: Results **only** show what the user is allowed to see

### Identity and access
- M: Local accounts (password plus TOTP), OIDC login (Authentik, Entra ID, Keycloak and so on)
- M: Users, groups, and space roles: viewer, editor, space admin
- M (phase 2): The full permission model from 05, including page restrictions, guests and SCIM 2.0 provisioning
- S: Passkeys and WebAuthn

### Trash and retention
- M: Deleted pages, attachments and spaces go to the trash for one universal retention period (default 30 days), then are permanently removed, including from search and storage

### Audit
- M: Every user action and admin action is recorded (see 06). Admins can filter and export the audit log as CSV.

### Administration
- M: Admin console for users, groups, spaces, authentication settings, retention, storage and system health
- M: All UI text through translation files, English only for v1

## Toward full parity (phases 4 and 5, then after 1.0)

| Area | Features |
|---|---|
| Collaboration | Real-time co-editing with cursors, page comments, inline comments, reactions, @mentions |
| Notifications | In-app and e-mail, watching pages and spaces, daily or weekly digests |
| Structure | Labels, page templates (global and per space), blueprints with a guided form, archiving pages |
| Macros | Table of contents, child pages, include page and excerpts, status labels, info/warning panels, expand, diagrams (Mermaid, Excalidraw-style), embeds |
| Export | PDF, DOCX, ODT, HTML, Markdown, plain text, and a signed Diffluence bundle (for migration). Per page, per tree, per space. |
| Import | Confluence Cloud and Data Center exports (including attachments, comments, versions and permissions where possible), Markdown, HTML, DOCX |
| Automation | Rules made of a *trigger* (page created, label added, schedule…), *conditions* and *actions* (move, label, notify, webhook, create page from template). Runs in the background via the queue. |
| Publishing | Selected spaces or pages published as a static public website on a separate host (see 04) |
| Linked instances | Admin can link instances and migrate or merge spaces, users and history between them, for example after a merger |
| Analytics | Page views and popular content, respecting privacy settings |

## After 1.0 (in order)

1. **AI:** summarise, rewrite, Q&A over the wiki that respects permissions, an MCP server, and provider choice including local models
2. **Platform:** public API, webhooks, OAuth apps, remote apps, WASM macros, then the marketplace
3. **Work module:** issues, boards, sprints and workflows, linked with pages
4. **Mobile apps**

## Non-functional requirements (summary, details in 06 and 07)

- Thousands of users per instance. Page loads under 300 ms (p95) at 5,000 simulated users.
- RPO and RTO at most 4 hours
- Zero-downtime upgrades between consecutive releases
- Runs on Docker, Kubernetes and LXC (Proxmox)
