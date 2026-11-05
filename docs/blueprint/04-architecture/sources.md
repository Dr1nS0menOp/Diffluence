# 04: Sources

Read on 2026-11-05 for this study, in our own words. Diffluence's own code, specs and ADRs are the first source. The others are for Go, PostgreSQL, NATS and the web platform.

## NATS

- [Consumers](https://docs.nats.io/nats-concepts/jetstream/consumers): pull consumers, workers sharing one, `MaxAckPending` of 1000 for the whole pool.
- [Ack responses and redelivery](https://docs.nats.io/learn/jetstream/acknowledgment), [Consumer details](https://docs.nats.io/using-nats/developer/develop_jetstream/consumers): `AckWait`, `MaxDeliver` of -1 by default, `BackOff`.
- [Streams](https://docs.nats.io/nats-concepts/jetstream/streams): retention, `Nats-Msg-Id`, the duplicates window of two minutes.
- [JetStream clustering](https://docs.nats.io/running-a-nats-service/configuration/clustering/jetstream_clustering): odd numbers of servers, replicas, what a lost majority does.

## PostgreSQL and its tools

- [SELECT, the locking clause](https://www.postgresql.org/docs/16/sql-select.html): `SKIP LOCKED` for queue-like tables.
- [CREATE TABLE](https://www.postgresql.org/docs/16/sql-createtable.html): `UNLOGGED`.
- [Text search indexes](https://www.postgresql.org/docs/16/textsearch-indexes.html): GIN is preferred.
- [Table partitioning](https://www.postgresql.org/docs/16/ddl-partitioning.html): when it pays, and how many partitions.
- [PgBouncer features](https://www.pgbouncer.org/features.html): what transaction pooling never supports.
- [pgx](https://pkg.go.dev/github.com/jackc/pgx/v5): query modes with PgBouncer.
- [pgxpool](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool): the default size of a pool.

## Go

- [net/http](https://pkg.go.dev/net/http): `ResponseController`, `CrossOriginProtection`.
- [net/smtp](https://pkg.go.dev/net/smtp): the package is frozen.

## The web platform

- [Using server-sent events, MDN](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events): the limit of six connections without HTTP/2.
- [Page Visibility API, MDN](https://developer.mozilla.org/en-US/docs/Web/API/Page_Visibility_API): polling and hidden tabs.
- [CSP frame-src, MDN](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy/frame-src), [CSP script-src, MDN](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Content-Security-Policy/script-src): frames, and `'wasm-unsafe-eval'`.
- [Content Security Policy Level 3, W3C](https://www.w3.org/TR/CSP3/): `'self'` and `wss:`.
- [The iframe element, MDN](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/iframe): `sandbox`, and the warning about `allow-scripts` with `allow-same-origin`.
- [Window.postMessage, MDN](https://developer.mozilla.org/en-US/docs/Web/API/Window/postMessage): exact origins, checking what arrives.
- [RFC 9111, HTTP Caching](https://www.rfc-editor.org/rfc/rfc9111): `no-store`, `private`, `no-cache`.
- [RFC 8058](https://www.rfc-editor.org/rfc/rfc8058): one-click unsubscribe.

## The optional services

- [Gotenberg documentation](https://gotenberg.dev/docs/getting-started/introduction), [HTML to PDF](https://gotenberg.dev/docs/convert-with-chromium/convert-html-to-pdf), [Outbound URL filtering](https://gotenberg.dev/docs/outbound-url-filtering), [Installation](https://gotenberg.dev/docs/getting-started/installation).
- [Collabora conversion API](https://sdk.collaboraonline.com/docs/conversion_api.html): the page refused automated reading, so only its search summary was seen, that `convert-to` is limited to listed hosts. Forum posts gave `post_allow`. **Confirm at build time.**
- [Euro-Office at heise](https://heise.de/-11322160), [announcement](https://heise.de/-11228123): what it is, and its licence. Nothing read says how it connects.
- [draw.io embed mode](https://www.drawio.com/docs/reference/embed-mode/), [Embedding, project wiki](https://github.com/jgraph/drawio/wiki/Embedding), [FAQ](https://github.com/jgraph/drawio/wiki/FAQ): messages, `xmlpng`, self-hosting, licences.
- [PdfCraft](https://getartcraft.com/apps/pdfcraft), [its repository](https://github.com/storytold/pdfcraft): alpha, MIT or Apache-2.0, a static web build.
- [Hocuspocus hooks](https://tiptap.dev/docs/hocuspocus/server/hooks), [Redis extension](https://tiptap.dev/docs/hocuspocus/server/extensions/redis), [Persistence](https://tiptap.dev/docs/hocuspocus/guides/persistence), [Configuration](https://tiptap.dev/docs/hocuspocus/server/configuration).
- [Redis 8 licence, Phoronix](https://www.phoronix.com/news/Redis-8.0-Goes-AGPLv3), [Valkey, OVHcloud](https://www.ovhcloud.com/en/public-cloud/uc-public-cloud-best-open-source-alternative-to-redis/): secondary sources. Check the projects' own files before use.
- [Amazon S3 consistency](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Welcome.html): read after write.
- [Mailpit](https://pkg.go.dev/github.com/axllent/mailpit-test): licence and ports.
