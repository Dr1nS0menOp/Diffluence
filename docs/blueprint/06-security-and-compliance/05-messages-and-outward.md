# 06.5: Mail, notifications, webhooks and calls to other servers

All of this is planned in waves C and H. The study below is for what those waves must not forget. Nothing here is built yet, unless the line says so.

## 5.1 Mentions, notifications and comments

**What can go wrong:** a title reaches someone who may not see the page, a mention reveals that a page exists, text removed from a page lives on in a comment, one person floods everyone.

- **SEC-400** · wave C · A notification holds ids, a type and a group key. Titles are read when it is shown (spec 010, D10).
- **SEC-401** · wave C · A notification is checked against the access rule when it is made, when it is shown and when a mail goes out. If the person lost access it disappears (COL-072).
- **SEC-402** · wave C · A person without access gets nothing from a mention, and the writer is warned, with a choice to share (COL-032, 033).
- **SEC-403** · wave C · Limits per share, per group and per minute, and group mentions are off by default and stop at 50 members.
- **SEC-404** · wave C · People pickers for mentions follow the rules of the directory: `GET /pages/{id}/mention-candidates` must not show a guest everyone in the company.
- **SEC-405** · wave C · Comments pass an allow-list of their own, and search finds them only through the access rule.
- **SEC-406** · wave C · A comment made while editing stays visible to editors only until the page is published.
- **SEC-407** · new · **Medium.** An inline comment stores the words it points at, as the W3C annotation model does, and shows them after they are gone (COL-018). The model itself warns about copying text. When someone removes a secret from a page, or an administrator removes a version (PAG-096), that text lives on in the anchors. Removing a version must also blank the anchors that quote it. Belongs in waves C and D.
- **SEC-408** · wave C · Share never grants access by itself. A person with the right permission can grant it, and that is audited (COL-043).
- **SEC-409** · wave C · An access request reveals nothing about a page a person cannot see, unless a space chooses to allow that (COL-049). Every step is audited.

## 5.2 Mail

**What can go wrong:** mail goes out in the clear, a title with a line break adds a header, a mail holds what the person may no longer see, a slow server stops the app, someone's mailbox is flooded, a mail is forged.

- **SEC-415** · wave C · TLS is required, by STARTTLS or from the first byte. Plain text is only for a local relay, by explicit setting. A server that does not offer STARTTLS fails, and never falls back (ASVS 12.2.1, 12.3.1).
- **SEC-416** · wave C · The certificate of the mail server is checked (ASVS 12.3.2). Spec 010 does not say so. It must be a test: a fake server with a wrong certificate.
- **SEC-417** · wave C · Names and titles lose line breaks, and headers are encoded, so nothing can add a header (ASVS 1.3.11). `html/template` escapes the body.
- **SEC-418** · wave C · By default mail holds a title and a link, no excerpt. An instance with sensitive content keeps it that way.
- **SEC-419** · wave C · The worker sends through a queue, so a broken mail server never touches a request.
- **SEC-420** · wave C · The password comes from the environment or a file, is never shown, and is not logged. Mail errors are logged without addresses.
- **SEC-421** · wave C · One-click unsubscribe follows RFC 8058: a `POST` with no session, a token that identifies the person and what to stop, and `GET` changes nothing, because mail scanners follow links.
- **SEC-422** · new · **Medium.** The unsubscribe token sits in the URL path, and the request log writes `r.URL.Path`. The comment there only keeps query strings out. So tokens in paths reach the logs: `/api/v1/mail/unsubscribe/<token>`, `/unsubscribe/<token>` and `/hooks/{token}`. Log the route pattern instead of the path, or mask those paths. Belongs in wave C, before the first such route.
- **SEC-423** · new · RFC 8058 requires a DKIM signature that covers the unsubscribe headers. That is the mail server's job, so the mail guide must tell operators to set up SPF, DKIM and DMARC. Low. Belongs in wave C.
- **SEC-424** · new · `mail_queue` holds whole messages with names and titles. Remove a mail when it is sent, and keep only that it was sent. Belongs in wave C, with a retention period.
- **SEC-425** · wave C · Mailpit is for testing only, and catches all mail in a screen. Its screen has no login by default, so it must listen on the local machine only.
- **SEC-426** · wave C · `net/smtp` is frozen, which is fine for this use: it takes no new features, and it refuses to send a password without TLS, except to the local machine. No dependency is needed.

## 5.3 Calls to other servers

**What can go wrong:** the server is made to call itself or the internal network (SSRF), a name that resolves to a private address, an answer that never ends, a redirect to somewhere else.

- **SEC-430** · built · Today the server calls no other web server but the identity provider, which an administrator sets.
- **SEC-431** · wave H · One client for all other calls, `internal/outward`: https only, an allow-list that starts empty, every resolved address checked, the connection made to the checked address, no redirects, 10 seconds, 64 KiB (ASVS 1.3.6, 13.2.4, 13.2.5, 15.3.2).
- **SEC-432** · wave H · Each case in the OWASP cheat sheet is a test: private ranges, link-local and the cloud metadata address `169.254.169.254`, IPv6 local ranges, addresses mapped into IPv6, a name with one good and one bad address, a name that changes between check and connect.
- **SEC-433** · new · The cheat sheet also asks for a firewall on the way out. The operations guide should give the rule: the app may reach PostgreSQL, the mail server, the identity provider, allowed hosts and the optional services, and nothing else. Medium. Belongs in wave H.
- **SEC-434** · new · OIDC discovery uses the library's own client and follows redirects. An administrator sets that address, so the risk is small. Keep it out of `internal/outward`, and say so in the code. Low.

## 5.4 Webhooks and automation

**What can go wrong:** a webhook carries content to someone who lost access, a delivery is replayed, a secret leaks, a rule loops forever, a rule does what its owner may not.

- **SEC-440** · wave H · Webhooks are signed the way the Standard Webhooks specification describes: HMAC-SHA256 over id, timestamp and body, with two signatures during a rotation. Receivers check the time, which stops a replay.
- **SEC-441** · wave H · Secrets are sealed by `internal/keys`, and shown once.
- **SEC-442** · wave H · Each delivery checks the access of the webhook's owner, so a person who lost access stops getting content.
- **SEC-443** · new · The body of a webhook should hold ids and a little context, not page text, unless an administrator chooses more. A receiver is a copy outside our rules. Medium. Spec 015 should say what is in a body.
- **SEC-444** · wave H · An endpoint that keeps failing is switched off, and the owner is told.
- **SEC-445** · wave H · An incoming trigger is a public route with a 256-bit token stored hashed, which can only start that one rule. See SEC-422 for the log.
- **SEC-446** · wave H · A rule acts with the rights of its owner, checked when it runs, and stops when the owner loses them or is disabled. A rule that triggers itself is stopped. Every action is audited.
- **SEC-447** · wave H · Rules are data with a fixed list of actions, never code.

## 5.5 Analytics

**What can go wrong:** a tool for reading statistics becomes a tool for watching colleagues.

- **SEC-450** · wave H · Analytics are off by default. In counts mode a reader is an HMAC of their id with a salt for that day, and salt and hashes go when the day closes, so nothing can be traced back.
- **SEC-451** · wave H · Name mode stays off by default, by the product owner's choice, and the setting names the risk (see chapter 10).
- **SEC-452** · built · Page views are not audit events, and the recent list is only for the person themselves.
