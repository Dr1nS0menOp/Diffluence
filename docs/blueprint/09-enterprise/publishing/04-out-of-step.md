# 09.P4: Out of step

How to read this chapter: see [01-draft-and-published-spaces.md](01-draft-and-published-spaces.md).

A draft page is **in step** when releasing it now would change nothing a reader sees. It is **out of step** when a release would change something: its text, its title, its place, its look, its files or its labels. This chapter defines each aspect, says which aspects never count, and gives the statuses a page can have. The aim is that "out of step" is exact: a page is never shown as changed when releasing it would change nothing, and never shown as in step when a reader would see a difference.

## 4.1 The aspects

Each aspect is compared between the draft page now and the draft page as it was at its last release, never with the published page, because the published page can differ by design (rewritten links, restrictions of its own).

- **PUB-300** · new · **Text.** Out of step when the draft page's current version is newer than the version last released and its document differs from that version's document. *Why:* a new version with the same document (for example after a restore of the released version) changes nothing for readers. *Edge cases:* the comparison is of the normalised document (`document.Normalize`), so a version that only reorders attributes in storage is not a change; comment anchors and instruction text are not part of the comparison, since they never cross (PUB-511). *Proof:* table tests: a new version with a change, a new version without one, a restored released version.
- **PUB-301** · new · **Title.** Out of step when the title differs from the released title. A rename makes a version in Diffluence (spec 001), so this is part of the version, but it is shown as its own aspect, because a changed title also changes every link's text and the tree. *Proof:* a test.
- **PUB-302** · new · **Place.** Out of step when the page's parent in the draft space differs from the parent at its last release, or its position among siblings differs. *Why:* the tree is how readers find their way. *Edge cases:* a move of a parent makes the parent out of step, not each child, because the children keep their parent; a sibling's move changes positions of others, so only the moved page is marked, and the release of a parent orders its released children as in the draft (PUB-412). *Proof:* tests for a move, a reorder and a parent's move.
- **PUB-303** · new · **Look.** Out of step when the emoji, the header image, the page width or the page language differ from those released (decided, PUB-1407). *Why:* how a page looks is page data (ADR-025), which readers see. *Proof:* tests per item.
- **PUB-304** · new · **Labels.** Out of step when the page's set of labels differs from the set released (decided, PUB-1407). *Why:* readers find pages by labels, and dynamic blocks list pages by label. *Proof:* a test.
- **PUB-305** · new · **Files.** Out of step when a file shown in the document has a newer version than the one copied, when a file was added to the document, or when one was removed (PUB-320). *Proof:* tests for each.
- **PUB-306** · new · **Links waiting.** Out of step when the page holds a link to a draft page that was not released at the page's last release but is now (PUB-512). *Why:* a release would now turn the link's text back into a working link. *Proof:* a test.
- **PUB-307** · new · A page lists **every** aspect that is out of step, in words: "Text and files changed", "Moved". *Why:* a release manager decides differently on a moved page than on a rewritten one. *Proof:* a test of the list.

## 4.2 What never counts

- **PUB-310** · new · These changes never make a page out of step, because they never cross the pair: comments and their resolution (decided, PUB-1406), reactions, watchers, favourites, the page status (decided, PUB-1407), the page owner, page restrictions (decided, PUB-1405), analytics, read marks, tasks ticked (they make a version, so they count as text; ticking itself is not separate), and the workflow state alone without a new approved version (08). *Why:* each is a working mark of writers, or is decided on the published side. *Proof:* a table test: each change leaves the status unchanged.
- **PUB-311** · new · A **personal draft** or an unsaved edit of a page never makes it out of step (PUB-234). *Proof:* a test.

## 4.3 Files

- **PUB-320** · new · Files are copied by **file pair** (PUB-202): on each release, a file shown in the released document is copied when it has no copy yet or when its current version is newer than the copied version; a file no longer shown is removed from the twin, going to the published space's trash with its page's next version. Old file versions are never copied; the twin has only the versions it was released with. *Why:* only what the reader can open is copied (ADR-022, files are in the text); the copy is independent, so purging in the draft cannot take a reader's file away, and a reader's download can never return a draft file version (sources, Drupal workspaces). *Edge cases:* a file shown on two draft pages is copied once per page, like a page copy today; a file larger than the published space's room stops the run at that page (PUB-1054). *Proof:* tests that an unchanged file is not copied, a new version is, a removed one goes, and a reader of the twin can only download the copied version.
- **PUB-321** · new · A file's comment (the note on a file, migration 0016) is copied with it, as it is shown with the file. *Proof:* a test.

## 4.4 Comments, labels, permissions and titles in short

The task of this study asked what "out of step" means for each of these; the answers above in one table:

| Thing | Crosses on release | Makes a page out of step | Line |
|---|---|---|---|
| Text | Yes, with links rewritten (06) | Yes | PUB-300 |
| Title | Yes | Yes | PUB-301 |
| Place in the tree | Yes | Yes | PUB-302 |
| Look (emoji, header image, width, language) | Yes | Yes | PUB-303 |
| Labels | Yes | Yes | PUB-304 |
| Files shown in the text | Yes, the current version, as new stored files | Yes | PUB-305, PUB-320 |
| Comments (page and inline) | No, never, either way | No | PUB-310, PUB-1406 |
| Page restrictions | No; the published side keeps its own | No | PUB-222, PUB-1405 |
| Space permissions | No; each space has its own | No | PUB-600 |
| Page status, owner, watchers | No | No | PUB-310 |
| Version history | No; the published page has its own versions, one per release | No | PUB-332 |
| Workflow state | No; only the approved version is released | Only through a new approved version | PUB-710 |

## 4.5 The statuses

- **PUB-330** · new · A draft page has exactly one **release status**, shown in words and never by colour alone: **Not released yet**; **Released** (in step); **Changed since release**, with the aspects; **Left out** (PUB-420); **Cannot be released**, with the reason (a kind that is never released, a controlled page never approved, PUB-701); or **Waiting for approval**, for a controlled page whose approved version is released and whose newer version is in review (08). *Why:* one status per page keeps the release screen a simple list to work through. *Proof:* a table test from page facts to status.
- **PUB-331** · new · A published page whose draft page is gone shows, on the draft side's release screen, as **Removed in the draft**, with what happened (deleted, archived, moved to another space, purged), until it is taken off or the next whole-space release mirrors the removal (PUB-1410). *Proof:* tests per cause.
- **PUB-332** · new · The published page's **version history** has one version per release, numbered on its own from 1. Each version's author is the person who released it, or "approval" with the approver as the cause, and its summary is the release note (decided, PUB-1408). People who may view the draft page also see, on each version, which draft version it came from. *Why:* readers see a history of what they were given, not of drafting; the draft space's people can trace each step. *Proof:* tests of the history as reader and as writer.

## 4.6 How the status is kept

- **PUB-340** · new · The facts behind the status are kept in the page pair, updated in the **same transaction** as any change to the draft page: a version, a move, a label, a look change, a file change. The release screen and the band therefore read one row per page, never compute documents. *Why:* comparing documents of thousands of pages on every view would not fit the budget (12). *Edge cases:* a change in an unpaired space writes nothing extra; the space's pair flag is read with the space row already loaded. *Proof:* a query-count test of a publish in a paired space (one more statement) and in an ordinary one (none).
- **PUB-341** · new · A page that has never been released has no page pair yet; its status comes from the absence, so a large draft space costs nothing before its first release. *Proof:* a test.
- **PUB-342** · new · Because "changed" is decided by comparing at write time against stored facts, a change that is later undone (a label added and removed, a page moved and moved back, a version restored to the released one) returns the page to **Released**. *Why:* exact, as the chapter promises. *Proof:* tests for each undo.
- **PUB-343** · new · A **consistency check**, run by `diffluence check publishing` and by the weekly self-check of the health page, recomputes every status of a pair from the versions and compares it with the stored facts; a difference is shown on the health page and fixed by a button that recomputes. *Why:* stored facts can drift through a bug; a check that recomputes finds it before a reader does. *Proof:* a test that corrupts a stored fact and sees the check report and fix it.
- **PUB-344** · new · Comparing the draft page with what readers have is a **compare of versions** of the draft page, the released one against the current, through the existing compare (`DiffDocument`). *Why:* the published page has links rewritten, so comparing across the pair would show every link as changed. *Proof:* a test that a page with links shows no change in a compare after a release.
- **PUB-345** · new · The status of each page is available through the API to those with `space.release` or `page.edit` in the draft space, with the aspects. *Proof:* an API test per kind of caller.
