# 09.P11: Edge cases and failure paths

How to read this chapter: see [01-draft-and-published-spaces.md](01-draft-and-published-spaces.md).

Each line here is a situation where the two spaces could disagree without anyone seeing it, where a reader could get what they should not, or where work could be lost. Each is built and tested before the happy path of its wave, following the rule "failure paths first".

## 11.1 Moves

- **PUB-1000** · built · **Moving a draft page within the draft space** makes it Changed since release (place, PUB-302). The next release moves its twin under the parent's twin; if the new parent has no twin, under the nearest released ancestor (PUB-1426). *Proof:* tests for both.
- **PUB-1001** · new · **Moving a draft page out of the draft space** into another space: the page pair ends, the twin stays and shows Removed in the draft (moved away), and a whole-space release takes it off by default (PUB-1410). The move dialog says this before the move, to those who may see the release screen. *Why:* a page that left the draft space has no source any more; leaving its twin silently would leave readers a page nobody can change. *Proof:* a test.
- **PUB-1002** · new · **Moving a page into the draft space** from another space makes it Not released yet. If it is under a page left out with its children, it is left out too. *Proof:* tests.
- **PUB-1003** · new · **Moving a page of another pair's draft space** into this draft space: the old pair's twin shows Removed in the draft in the old pair; the page is Not released yet here. *Proof:* a test.
- **PUB-1004** · built · **Moving within or into the published space** is refused (PUB-220); moving a twin out of the published space is refused too. *Proof:* tests.
- **PUB-1005** · new · **A page moved in the draft while a run that includes it is going** is released at the place the preview showed (PUB-1409), and stays Changed since release. *Proof:* a test.
- **PUB-1006** · built · **A cycle cannot arise** in the published tree: a twin's parent is always the twin of a draft ancestor, and the draft tree has no cycle; the release path checks it anyway and refuses with a logged error. *Proof:* a unit test of the placement function over random trees.

## 11.2 Deletes and archives

- **PUB-1010** · new · **Deleting a draft page** (to the draft space's trash) leaves its twin in place, marked Removed in the draft (deleted). A whole-space release takes the twin off to the published space's trash by default (PUB-1410); a person can also take it off at once (PUB-440). *Proof:* tests.
- **PUB-1011** · new · **Restoring a draft page from the trash** brings its page pair back if the twin still exists; the page is Released or Changed since release, by comparing (PUB-342). If the twin was taken off meanwhile, the page is Not released yet. *Proof:* tests for both.
- **PUB-1012** · new · **Purging a draft page** ends the page pair; the twin stays, Removed in the draft (purged), until taken off. Its verification (PUB-920) reports "draft version purged". Under document control, signed versions are kept against purge anyway (DOC-770). *Proof:* a test.
- **PUB-1013** · new · **Archiving a draft page** marks it Removed in the draft (archived); a whole-space release archives its twin, rather than taking it off (decided, PUB-1422). **Unarchiving** makes the page Changed since release, and the next release unarchives the twin. *Why:* archived pages stay readable in Diffluence for those who look; mirroring keeps that meaning for readers. *Proof:* tests.
- **PUB-1014** · new · **Deleting a branch** in the draft: each page is handled as above, and the preview of the next whole-space release lists the twins that will leave. *Proof:* a test.
- **PUB-1015** · new · A **twin in the published space's trash** cannot be restored by anyone while the pair is active: restoring is a write (PUB-220). The way back is to release the draft page again (PUB-442), or to restore the draft page and release it. *Why:* a restored twin without a source would be a page nobody can change. *Proof:* a test.
- **PUB-1016** · new · The **draft home page** cannot be deleted in Diffluence; the published home page cannot be taken off (PUB-440). *Proof:* a test.

## 11.3 Renames

- **PUB-1020** · built · **Renaming a draft page** makes it Changed since release (title). Readers keep the old title in the tree, in links and in search until the release (PUB-530). *Proof:* a test.
- **PUB-1021** · built · **Renaming the draft space** or changing its key changes nothing in the published space. Blocks that named the draft space by key were rewritten at release time to the published space (PUB-502), so they keep working. *Proof:* a test.
- **PUB-1022** · built · **Renaming the published space** changes nothing in the pair. *Proof:* a test.

## 11.4 Restores

- **PUB-1030** · built · **Restoring an older version** of a draft page makes a new version (spec 001); the page is Changed since release, unless the restored version equals the released one (PUB-342). Releasing it gives readers that text as a new twin version. *Proof:* tests.
- **PUB-1031** · built · A twin's history cannot be restored from the published space (PUB-220). *Proof:* a test.
- **PUB-1032** · new · **A full restore** of the installation (wave RS) returns both spaces, the pair, the page pairs and runs to the moment of the backup together, because they are in one database. A run that was going at the backup is shown as stopped, and can be resumed with the same versions if they still exist (PUB-1061). *Proof:* a step in the restore drill with a pair and a run in progress.
- **PUB-1033** · new · **Restoring one space from a backup** (roadmap, spec 059 and later) into a paired space is refused while the pair is active, and the screen says to end the pair first. *Why:* a space brought back alone would no longer match its page pairs. *Proof:* a test once that feature exists.
- **PUB-1034** · new · **Restoring a space from the trash** (a deleted space brought back): an ended pair is not brought back by itself; the administrator can link again (PUB-245). *Proof:* a test.

## 11.5 Import and export

- **PUB-1040** · new · **Importing into the draft space** (Markdown, HTML, Word, a bundle, an export of another wiki) makes ordinary draft pages, Not released yet. *Proof:* a test.
- **PUB-1041** · new · **Importing into the published space** is refused (PUB-220). *Proof:* a test.
- **PUB-1042** · new · **Exporting the draft space** as a bundle (an experimental part, ADR-043) carries the left-out marks with their reasons, and not the pair: a pair is between two spaces of one installation. Imported elsewhere, the bundle makes an ordinary space whose pages remember which were left out, so a pair made there later starts with the same choice. *Proof:* a round-trip test.
- **PUB-1043** · new · **Exporting the published space** as a bundle makes an ordinary bundle; imported elsewhere, it is an ordinary space, not managed. *Why:* the managed rule belongs to the pair, which does not travel. *Proof:* a test.
- **PUB-1044** · new · Exports of every other format of either space are those of an ordinary space (PUB-625). *Proof:* covered by PUB-625.
- **PUB-1045** · new · **Copying a whole space** (if offered later) of either side makes an ordinary space. *Proof:* a test when the feature exists.

## 11.6 Spaces of a pair

- **PUB-1046** · new · **Deleting the published space** (by its administrator, with the existing delete of a space, which needs no write of pages) ends the pair, and the draft space's administrators are told. The draft space keeps everything, and its pages become ordinary. *Proof:* a test.
- **PUB-1047** · new · **Deleting the draft space** ends the pair; the published space becomes an ordinary space, which its editors can change again, and its administrators are told. *Why:* the alternative, a published space nobody can ever change, would be a trap. *Proof:* a test.
- **PUB-1048** · new · **Archiving the draft space** stops releases (archived spaces are read-only) and keeps the pair; unarchiving lets them go on. **Archiving the published space** keeps it read-only to readers as any archived space, and refuses runs until unarchived. *Proof:* tests.
- **PUB-1049** · built · A space that is in a pair cannot be made a **personal space**, and a personal space cannot be in a pair. *Why:* a personal space has one writer and no audience to separate. *Proof:* a test.

## 11.7 Two drafts into one, and other shapes

- **PUB-1050** · built · **Two draft spaces into one published space** is refused when the second pair is made, with the reason and two ways to get the result: merge the draft spaces first (move one's pages into the other), or keep two published spaces and link between them (decided, PUB-1402). *Why:* two sources would fight over the published tree, the home page and the order of siblings, and a page would have two places to change it; staging CMSs refuse to merge two sources of one item for the same reason (02). *Proof:* a test of the refusal and its message.
- **PUB-1051** · built · **One draft space into two published spaces** is refused; see PUB-1501. *Proof:* a test.
- **PUB-1052** · built · **A chain**, where a published space is also a draft space of another pair, is refused (PUB-200). *Proof:* a test.
- **PUB-1053** · built · **Swapping the roles** of two paired spaces is not offered; ending the pair and linking the other way needs an empty published space, as always. *Proof:* none beyond PUB-211.

## 11.8 Limits

- **PUB-1054** · new · **Storage**: files copied by a release count against the published space's storage limit (decided, PUB-1419, ADR-038). The preview shows the total against the room; a run that would pass it is refused before it starts, and a run that meets it during a batch (a file grew between preview and run) stops at that batch with what was done. *Proof:* tests of both.
- **PUB-1055** · new · **Size of a run**: at most 50,000 pages, the size of the largest space measured (limits.md); a larger draft space is released by branches. *Why:* every action has a limit; this one is above every measured space. *Proof:* a test of the refusal with a lowered limit.
- **PUB-1056** · built · **Size of a release note**: at most 2,000 characters, refused past it with the limit named. *Why:* it is shown in every twin's history and in notifications. *Proof:* a test.

## 11.9 Concurrency and failures

- **PUB-1060** · built · Two people starting runs on one pair at the same moment on two nodes: the pair row is locked (`SELECT … FOR UPDATE`) while a run is claimed, so exactly one starts and the other queues behind it (PUB-452). *Proof:* a concurrency test with two connections.
- **PUB-1061** · new · A run that **stops halfway** (a node dies, the database restarts, the storage fails) leaves every batch either done or untouched, because each batch is one transaction and files are written before their batch commits; files of a batch that did not commit are removed by the existing orphan clean-up. The run shows as stopped, and resuming continues with the same versions. *Proof:* a fault-injection test that kills a run between batches and inside one.
- **PUB-1062** · built · A **publish in the draft** at the same moment as a run reads that page: the run takes the version of its preview, and the publish makes a newer one; neither waits for the other beyond the page row lock of a moment. *Proof:* a concurrency test.
- **PUB-1063** · new · **Approval while a run goes**: the approval release queues behind the run (PUB-452). *Proof:* a test.
- **PUB-1064** · built · All times of runs are taken from the database server (`now()`), as DOC-1240 does. *Proof:* a test.
- **PUB-1065** · new · A **file that cannot be read** from storage during a run (missing object) stops that page with the reason and skips its children in that run; other pages go on, and the report lists it. *Why:* one broken file should not hold back a guide's release, and the page must not reach readers without its file. *Proof:* a test with a missing object.
- **PUB-1066** · new · A **notification that fails** to send is retried by the outbox and never undoes a run (as DOC-1252). *Proof:* a test with a failing mail sender.
