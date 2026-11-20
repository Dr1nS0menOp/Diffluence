# Draft and published spaces

Operator guide for the experimental part `publishing` (spec 073, PUB-1312). Writers work in a **draft space**; readers have the **published space**, which only a **release** changes. This guide says what the part is, that it starts off, how to turn it on, what it refuses, how to check it and how to end a pair. For the people who use it, the space settings section Published space explains each step on screen.

## What it is

Two spaces of one installation are linked as a **pair**. A release copies a page, a branch or the whole draft space to the published space, after a preview; under a workflow, an approval can release by itself. A page in the published space (a **twin**) is an ordinary page for readers, so search, the feed, exports and permissions treat it as any page. The draft space and its pages never reach a reader of the published space.

A pair is made in two ways:

- **Link** an empty space, or a new one, as the published space of a draft space (Space settings, Published space, Link a published space).
- **Start a draft space** from a space that already has content (Space settings, Published space, Start a draft space from this space). The space becomes the published space; a new draft space gets a copy of every page, folder and link with its files, labels, look and restrictions, each paired with its source, in step. Only the grants that write move along: people who only read the old space never see the draft space. Blog posts, databases, archived pages and pages never published stay as they are. A space with pages under a workflow cannot be started from: end their workflows first.

Starting runs in the background in one transaction: editing the old space waits until it is done, and a start that fails leaves nothing. Until the first release it can be undone (Undo the start), which ends the pair and sends the draft space to the trash.

## It starts off

The part is experimental (ADR-043) and **off** on a new installation and after an upgrade. While it is off every route of the part answers 404 `experimental_off`, its screens are gone, approval releases wait, and published spaces stay read-only. Two things stay open while it is off: **ending a pair**, so a published space can always be given back to its editors, and the take-off of a page that a workflow withdrew, so a withdrawn procedure never stays in front of readers.

## Turning it on

Under Administration, Organising, Experimental parts, turn on Draft and published spaces. Finish the upgrade of every node first: runs, starts and checks are work of the worker of this release. Turning it on again after it was off brings back every pair as it was, with the statuses as stored, and the consistency check runs once within the hour.

Who may do what is decided in the spaces:

| Act | Needs |
|---|---|
| Link two spaces, end a pair | Run the space (`space.administer`) in both spaces |
| Start a draft space from a space | Run the space, and Create spaces |
| Release, leave out, take off | Release to the published space (`space.release`) in the draft space, and view of the published space |
| Verify, export the release history | See the audit (`space.audit`) of the draft space |
| Check and fix every pair | Administrator, on the health page |

## What it refuses

- **Nobody writes the published space by hand**, not even its administrators: no page, edit, file, move, label, template, workflow or automation there. The permission rule refuses it, and a trigger in the database refuses a write of page content that is not a release, also from a node of the previous release. Someone who could otherwise edit sees "This page is changed by releases" under the byline.
- **One pair per space**, on one side: no second published space for a draft space, no two drafts into one published space, no chain, no personal space.
- **Linking needs an empty published space.** A space with content becomes a published space by starting a draft space from it.
- **A preview older than an hour** is not confirmed; a run takes the versions its preview showed.

## Checking it

- **The health page** (Administration, System health) has the line Published spaces: what the last consistency check found. It runs by itself once a week and after the part is turned on again; Check now runs it at once, and Fix what it found recomputes every status from the versions.
- **The command line**: `docker compose exec kenwerk kenwerk check publishing` checks every pair; `--fix` recomputes what differs.
- **Verification**: on a draft page, Verify the published text checks its twin; in the settings section of a draft space, Verify every published page checks them all in the background and lists the pages that differ, whose draft version changed, that cannot be verified yet or whose draft was purged. A twin that differs was changed past the application, in the database: treat it as an incident ([incidents.md](incidents.md)).

## Backups and restores

A pair lives in the same database as both spaces, so a full restore brings both spaces, the pair, the page pairs and the runs back together. A run that was going when the backup was made shows as stopped after the restore once its sign of life is two minutes old; resume it from the release screen, and it releases the rest with the versions of its preview. The restore drill (`scripts/restore-drill.sh`) proves this with a pair and a run going.

A bundle of a draft space carries which pages are left out of releases, never the pair; a bundle of a published space makes an ordinary space where it is imported.

## Ending a pair

In the space settings of either space, Published space, End the link. It needs Run the space in both spaces, and works while the part is off. The published space becomes an ordinary space its editors can change again; the draft space stays as it is. Linking the same two spaces again later brings their page pairs back. Deleting either space ends its pair too, and the administrators of the other space are told.
