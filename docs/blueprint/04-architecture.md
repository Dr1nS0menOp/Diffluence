# 04: Architecture

## Overview

```
            Browser (React + TypeScript SPA)
                     │ HTTPS (REST /api/v1, WebSocket)
              ┌──────▼──────┐
              │ Reverse proxy│  Traefik / nginx / Ingress (TLS)
              └──┬────────┬──┘
     ┌───────────▼─┐    ┌─▼──────────────┐
     │ diffluence  │    │ collab (Node)  │  phase 4: real-time editing only
     │ role: api   │◄───┤ auth via api   │
     └──┬───┬───┬──┘    └────────────────┘
        │   │   │ publish events (outbox)
        │   │ ┌─▼──────────────┐     ┌───────────────────┐
        │   │ │ NATS JetStream │◄───►│ diffluence        │
        │   │ └────────────────┘     │ role: worker      │
        │   │                        └──┬────────────────┘
   ┌────▼───▼─────┐   ┌──────────────┐ │  exports, imports, indexing,
   │ PostgreSQL   │   │ Object store │◄┘  automations, retention, mail
   │ (+PgBouncer) │   │ S3 or disk   │
   └──────────────┘   └──────────────┘
```

## Components

| Component | Technology | Purpose |
|---|---|---|
| **diffluence** binary | Go | One binary, several roles: `api` (HTTP and WebSocket), `worker` (background jobs), `all` (both, for small installs). It also serves the built frontend. |
| Frontend | TypeScript, React, Vite, TipTap editor | An SPA that only talks to the public API |
| Database | PostgreSQL 16+ | The only required database: content, permissions, audit, outbox |
| Message queue | NATS JetStream | Events and jobs between api and worker. **Embedded** in the binary for small installs, an external 3-node cluster for HA. |
| Object storage | S3-compatible (Garage, SeaweedFS, Ceph, MinIO, AWS) or local disk | Attachments, exports, backup bundles |
| Search | PostgreSQL full-text search (v1) | Pluggable interface so OpenSearch or Meilisearch can be added later for very large instances |
| collab service (phase 4) | Node.js, Hocuspocus/Yjs | Only merges simultaneous edits. Has no business logic and checks every connection with the API. |
| Export helpers | Headless Chromium (PDF), Pandoc (DOCX/ODT) | Separate optional sidecar containers, called by the worker |

## Why this shape

- **Modular monolith.** One Go codebase split into strict internal modules: `identity`, `spaces`, `pages`, `permissions`, `audit`, `search`, `storage`, `export`, `import`, `automation`. Modules talk via interfaces and events, never through each other's tables. That gives the simplicity of one binary with clear boundaries, and lets us split out a service later if it is ever really needed.
- **One binary from homelab to cluster.** Small: one LXC container with `role=all`, embedded NATS, local disk and a local Postgres. Large: several `api` and `worker` replicas, a NATS cluster, Patroni Postgres behind PgBouncer, and S3.
- **Transactional outbox.** Every change writes its event in the **same database transaction** as the change itself. A relay publishes it to NATS. So events are never lost and never sent for a change that was rolled back. Audit, search indexing, notifications and automations all work from these events.
- **Stateless API nodes.** Sessions live in Postgres (hashed tokens), files in object storage, jobs in NATS. Any node can be killed at any time.

## API

- REST with JSON under `/api/v1`, **spec-first** with OpenAPI. The TypeScript client is generated from the spec.
- WebSocket for live updates (tree changes, notifications)
- Versioning: new fields are added, never removed within `v1`. Breaking changes go into `v2`, and both run side by side for at least 12 months.
- Every endpoint declares the permission it needs, and CI fails if one doesn't.

## Go stack (proposed, all permissive licences)

Standard library `net/http` (routing), `pgx` (Postgres driver), `sqlc` (type-safe SQL, no ORM), `pgroll` or our own expand/contract migration runner (see 07), `nats.go` plus embedded `nats-server`, `coreos/go-oidc`, `go-webauthn`, `bluemonday` (HTML sanitiser), `log/slog` with JSON logs, OpenTelemetry, Prometheus client.

## Frontend stack (proposed)

React and TypeScript, Vite, TanStack Query and Router, TipTap/ProseMirror (editor), a Radix-based component library (MIT), FormatJS or i18next with ICU messages, CSS logical properties (for RTL), Playwright (end-to-end tests).

## Public publishing (phase 5)

Public pages **never** come from the internal instance. The worker renders chosen spaces to a **static website** (HTML, CSS and a client-side search index), signs the bundle, and pushes it **outbound only** to a separate host (any web server, object storage bucket or CDN). The public side has no database, no accounts, no write API and no network path back. Its attack surface is that of a static file server.

## Linked instances (later)

All IDs are UUIDv7 (globally unique, time-ordered), and every content row records its `origin_instance_id`. Instances trust each other through mutual TLS and exchange signed, versioned bundles. Users are mapped by IdP subject or e-mail. That makes merging instances after an acquisition a data operation, not a rewrite.
