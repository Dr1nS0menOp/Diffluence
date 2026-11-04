# 01: Vision and scope

## The problem

Organisations want a team wiki that they **own**. It should be self-hosted, secure, stable, and free of per-user licence fees and vendor lock-in. The existing open-source options are either simple (BookStack, Wiki.js), copyleft-licensed (Docmost, Plane, OpenProject), or built on large dependency trees that are hard to keep patched. None of them combines a permissive licence, enterprise-grade permissions, an audit trail and painless upgrades.

## The vision

**Diffluence is an open-source (Apache 2.0), self-hosted knowledge platform.** Over time it matches what teams use in commercial enterprise wikis, but its foundation is security, stability and upgrades that just work. Later it grows into a platform with AI, apps and a work-management module.

## Who it is for

- **Primary:** organisations with roughly 50 to 5,000 users, run by an IT team that can operate Docker, Kubernetes or Proxmox, and that cares about data sovereignty, security and compliance (government, education, healthcare, non-profits, SMEs).
- **Secondary:** homelab users and small teams who want something that is solid by default.
- **Target scale for now:** thousands of users per instance. Tens of thousands is a later goal and must not be blocked by today's design.

## Guiding principles

1. **Secure by default.** Every feature ships with its permission checks, audit events and tests. Nothing is "added later".
2. **Boring technology.** Proven, permissively licensed components. As few moving parts as possible.
3. **Upgrades are trivial.** One command, no downtime, no manual migration steps. This decides between design options.
4. **API-first.** Our own frontend uses only the public, versioned API, the same one that apps will use.
5. **Global from day 1.** No hard-coded UI text, and right-to-left (RTL) layouts supported. English first, 60+ languages later.
6. **Accessible.** Aim for WCAG 2.2 AA.
7. **Clean-room.** Behaviour can be familiar. Code, text and designs are our own (see 02).
8. **Small to large with the same software.** One binary runs on a single LXC container and in an HA cluster.

## Scope of version 1 (the MVP)

v1 is a **stable wiki for creating and organising documents**:

- A GUI editor with Markdown shortcuts, plus Markdown import and export
- Spaces and a page tree (create, move, reorder, delete)
- Version history with compare and restore
- Attachments and images
- Local accounts, OIDC login and basic space roles
- Search
- Trash with one universal retention period
- An audit log of all actions with CSV export
- Runs on Docker Compose and LXC, with documented backup and restore

## Explicitly *not* in v1

Real-time co-editing, inline comments, macros, automations, Confluence import, public publishing, SCIM, HA, AI, apps/marketplace, a Jira-like module, mobile apps, and translations beyond English. They are all on the roadmap (08), and v1's design must leave room for them.

## Long-term order (after 1.0)

AI, then the app platform and marketplace, then a work-management (Jira-like) module, then mobile apps.

## What success looks like for v1

- Runs on a test server for **4 weeks** of daily use without data loss or a restart caused by a crash
- An upgrade from one build to the next with users online causes **no noticeable interruption**
- A restore from backup on a clean machine succeeds within **4 hours**
- No open security findings rated high or critical
