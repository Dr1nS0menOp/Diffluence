# 04.6: Live updates and caching

## 6.1 How a screen stays fresh

The old text of this blueprint promised a WebSocket for live updates. Waves C and H chose polling. This section explains why, and when that changes.

| Way | What it costs | What it gives | Fit |
|---|---|---|---|
| **Polling** | One small request every 20 to 30 seconds per visible tab | Up to 30 seconds of delay. Works through every proxy, and needs no state on a node | Bell, presence, status of comments |
| **Server-sent events** | One open connection per tab, and a limit of six per browser and site without HTTP/2 | Instant, one way | Would fit the bell |
| **WebSocket** | An open connection, the write timeout handled per connection, a way to reach the right node, a new Go dependency | Instant, both ways | Needed for editing together only |

**The numbers.** 500 active people, each with one visible tab, polling the count every 30 seconds gives 17 requests a second. Presence every 20 seconds gives 25 more, on the people who read a page. That is about as much as all other traffic together, and still far below what one node handles (chapter 8).

- **ARC-158** · wave C · The count of unread notifications polls every 30 seconds, only in a visible tab, and at once when a tab comes back. Hidden tabs stop by themselves with the Page Visibility API, and browsers would throttle them anyway.
- **ARC-159** · wave C · A poll answers from one small indexed query, a partial index on unread rows.
- **ARC-160** · wave H · Presence and "something changed here" share one call per open page (spec 015 Part 4), so a page has one timer, not three.
- **ARC-161** · wave H · A poll spreads itself with a small random delay, so a thousand tabs opened at nine o'clock do not stay in step.
- **ARC-162** · wave H · Presence is short-lived, so `page_presence` is an `UNLOGGED` table: much cheaper to write, emptied after a crash, and not sent to standbys. All three are fine for "who is here now". On a failover it is empty for 20 seconds.
- **ARC-163** · wave H · The only WebSocket belongs to the service for editing together (ARC-118). Go gains no WebSocket of its own.
- **ARC-164** · later · If people find 30 seconds too slow, server-sent events can replace the bell's timer without changing the rest. HTTP/2 at the reverse proxy lifts the limit of six.

## 6.2 What may be cached

**The rule:** permissions change at any moment, and a page that was restricted a second ago must disappear (NAV-006). So nothing a permission decides is cached between requests, on any node. Going through PostgreSQL every time is the price, and it is what makes nodes stateless.

- **ARC-165** · built · Built assets have a hash in their name, and are cached for a year as `immutable`. `index.html` is `no-store`, so a release reaches people on their next load.
- **ARC-166** · built · API answers are `no-store`.
- **ARC-167** · phase 3 · Files are `private, no-store` today, so an image is downloaded again on every view. Better: `private, no-cache` with an `ETag` from the SHA-256 we already store. RFC 9111 says `no-cache` allows storing and demands a check first. So the browser keeps the bytes, the server checks permission on each use and answers 304, and a person who lost access gets a 404. It saves most of the traffic of images, with no loss of safety.
- **ARC-168** · built · In the browser, TanStack Query keeps answers for five seconds. That is for one screen drawing several parts, not a cache in the real sense.
- **ARC-169** · later · A published version never changes, which makes it the one thing that could be cached on a node without a permission question: the document by page and number, after the check. Only with a measurement that asks for it.
- **ARC-170** · built · Settings are read from PostgreSQL when needed. They are small, and rarely read on a hot path.
- **ARC-171** · built · No CDN and no shared cache sits in front of the API, by design. Only `/assets/` may be cached by a proxy.
