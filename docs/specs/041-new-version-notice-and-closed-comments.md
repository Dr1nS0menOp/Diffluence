# 041: A notice for a new version, and who decides that comments are closed

Status: built on branch `wave-n2`.

Two small points: one the product owner ran into, one the module rule asked for.

## 1. A notice when a newer version runs

**What happened.** The product owner rebuilt the app and went on working in a tab that was opened before the rebuild. The tab kept running the old frontend, and nothing said so: the Roles screen of the new version stayed out of sight until a manual reload.

**What the person gets.** When the server ships a newer frontend than the tab runs, a calm note appears under the top bar: "A newer version of Diffluence is running. Reload when it suits you; text you are writing stays." It has one button, Reload. It never reloads by itself, takes no focus and opens no dialog, so typing goes on. A screen reader hears it once, as a status.

**How the tab knows.**

| Option | Verdict |
| --- | --- |
| `version.Version` from `/system/info` | Not enough: a build from source says `dev` every time, so a rebuild changes nothing |
| A hash of the frontend inside the binary | Chosen: `index.html` names every asset by its content, so any frontend change gives a new id |
| A new route | Not needed: `/api/v1/appearance` is already public and polled every 30 seconds for the maintenance note |

`webui.Build()` is the first 8 bytes of SHA-256 over the embedded `index.html`, in hex, worked out once. `/api/v1/appearance` carries it as `build`. It says nothing a visitor cannot already download, because `index.html` itself is public. A binary built without a frontend gives an empty id, and then no note ever shows.

The tab keeps the id of its first answer. When a later answer differs, the note shows. Besides the poll, the tab asks again when a person comes back to it (`visibilitychange`), the likely moment after a rebuild.

**Reload without losing text.** Before, a reload in the middle of typing lost nothing either, but the words came back only through "Put it back", one click more. Now Reload sends one event, `diffluence:before-reload`; an open editor answers by saving its draft at once, and the tab reloads when the save is done, or after 5 seconds at most. The editor's existing browser backup on `pagehide` stays as the safety net when the save fails. The event carries the list of promises, so there is no shared state.

**Load budget (ARC-238).** Reading: no new request, no database query; one more field on an answer that is polled anyway, plus one extra appearance request when a person returns to the tab. Saving: on Reload, one draft save that the editor would have made a moment later anyway.

## 2. Who decides that comments are closed

**The rule.** A module may read another module's columns to show them, never to decide (blueprint 04, section 2). `comments` decided from `pages.comments_closed` in `checkOpen`, and read it again for the comment list.

**Options.**

| Option | Verdict |
| --- | --- |
| `comments` imports `pages` | Impossible: `pages` imports `comments` (publishing releases comments meant for editors; removing a version blanks quotes), so this would be a cycle |
| An event from `pages` to `comments` | A second copy of the switch, which can be stale for a moment; closing must count at once |
| Move the decision into `pages` | `pages` would have to know about comment rules |
| A small interface owned by `comments`, handed in by `app` | Chosen: one source of truth, read in the caller's transaction, no cycle |

`comments.PageGate` has one method, `CommentsClosed(ctx, q, pageID)`. `pages.Service` implements it and `app` hands it in. `checkOpen` and the comment list both ask it, so `comments` reads no column of `pages` any more.

**Proof.** `TestClosingCommentsStopsNewOnesAtOnce` was written first and passed on the old code, then on the new: closing refuses a new comment and a reply at once, keeps the old comments, and opening again lets comments in. Two mutations each break one half, and both are caught.

**Load budget.** Unchanged: the same one query per new comment and per list.

## Not proven

- A real rebuild under a tab in a browser. The browser test changes the answered id, which is what a rebuild does to it.
- The 5-second limit when a draft save hangs; the backup in the browser covers it, as before.
- Collaborative editing has no draft of its own; the shared document service keeps its text, and the note does not wait for it.
