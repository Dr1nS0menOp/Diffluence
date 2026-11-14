# 09.P7: Permissions, and every door a draft could leak through

How to read this chapter: see [01-draft-and-published-spaces.md](01-draft-and-published-spaces.md).

The promise of this feature is that a draft cannot reach readers through a page setting. In one space, that promise rests on every page's restrictions being right. With a pair, it rests on two facts that do not depend on any page: draft content lives only in the draft space, which readers do not have; and the published space only changes by a release, which a person with `space.release` starts, after a preview, or which an approval starts. This chapter states the rules, and then goes door by door, as chapter 06 of document control did, through every way content leaves a space.

## 7.1 Who may do what

- **PUB-600** · built · Each space of a pair keeps its **own** grants, roles and restrictions; a pair copies none of them, and nothing granted in one space means anything in the other. *Why:* who writes and who reads are different groups (02); ISO 9001 7.5.3.2 b asks for control of access. *Proof:* matrix tests that a grant in one space changes no decision in the other.
- **PUB-601** · partly · The rights of this feature, in one table:

| Action | Needs |
|---|---|
| Link two spaces, end a pair | `space.administer` in both (or in the draft alone when the published space is gone) |
| Change the pair's settings | `space.administer` in the draft space |
| See the release screen, the statuses and the preview | `space.release` in the draft space |
| Release, take off, leave out, include | `space.release` in the draft space, view of the pages, `space.view` in the published space |
| See a draft page's status in its band | `page.edit` or `space.release` in the draft space |
| See that a published page has a draft twin, and open it | View of the draft page |
| Set "release on approval" | `space.administer` and `space.workflows` in the draft space (08) |
| Read the pair's audit | `space.audit` in the draft space; in the published space, its own `space.audit` shows the effects without draft names |

*Proof:* a matrix test of this table, for an instance administrator, both spaces' administrators, a release manager, a writer, a reader of the published space only, a person in both, and a guest.
- **PUB-602** · built · **Instance administrators** get no way around the managed rule (PUB-220) and no view of the draft space beyond what their grants give, as everywhere in Diffluence (ADR-009). *Proof:* matrix cases.
- **PUB-603** · built · A person who may view **both** spaces sees each page as what it is: in the draft space, a band with the release status; in the published space, a quiet line "Released from a draft" with a link to the draft page (PUB-1230). They are never shown draft content inside the published space. *Proof:* tests for a person in both.

## 7.2 What linking checks

- **PUB-610** · built · Linking **withdraws** the draft space's public site and public links, and from then on `site.publish` in a draft space is refused (decided, PUB-1417). The preview names them. *Why:* public exposure of drafts is the widest leak there is. *Proof:* a test that a draft space's site is withdrawn and cannot be built again.
- **PUB-611** · built · When the draft space grants `space.view` to **all members**, **all guests**, or to a group or role that also reads the published space, the preview of linking says so in plain words ("Everyone who can read the published space can also read the drafts"), and the health line of PUB-241 keeps saying it. It is a warning, not a refusal. *Why:* some teams draft in the open on purpose; but the person must know the separation is then only between finished and unfinished, not between audiences. *Proof:* tests of the warning for each kind of grant.
- **PUB-612** · new · **Guests** can be readers of the published space and writers in the draft space, by their grants, like members. *Why:* external writers and external readers both exist; guests are limited by their own rules (spec 022) already. *Proof:* matrix cases.

## 7.3 The doors

Each door gets the content of the space it reads, and the published space never holds draft content, so most doors need no new rule. They are listed so that the tests cover each.

- **PUB-620** · built · **Pages, the tree, the content screen and the API** of the published space read only published pages; the draft twin's id appears nowhere in them for a reader. *Proof:* tests that render each for a reader and search the output for the draft space's id, key and name.
- **PUB-621** · built · **Version history** of a twin shows only released versions; their summaries are release notes, never draft summaries (PUB-1408); "released from draft version N" shows only to those who may view the draft page. *Proof:* a test.
- **PUB-622** · built · **Search** finds draft pages only for those who may view the draft space, as for any space; the twin is indexed when its version lands. A word that exists only in the draft is never found in the published space. *Proof:* a test with a word only in the draft.
- **PUB-623** · built · **Links and cards** in the published space never hold a draft id (PUB-510); a card's excerpt comes from the twin. *Proof:* the fuzz test of PUB-534.
- **PUB-624** · built · **Files** of a twin are separate stored copies (PUB-320); a twin's file address never serves a draft file version. *Proof:* a test that downloads by every file route as a reader.
- **PUB-625** · built · **Exports** (PDF, Word, ODT, HTML, Markdown, bundle) of the published space hold only twins. *Proof:* tests per format.
- **PUB-626** · built · **Public sites and public links** come only from the published space, through `mayBePublic`, unchanged. *Proof:* a test.
- **PUB-627** · built · **Notifications, the feed and webhooks** about the published space carry only twin content and the release note (09). *Proof:* tests that render each notification kind for a reader.
- **PUB-628** · built · **The audit view of the published space** shows releases as changes by the releaser, without draft titles or ids; the pair's audit with both sides is in the draft space (10). *Proof:* a test.
- **PUB-629** · built · **Comments** on a twin are the published space's own; draft comments never cross (PUB-1406). *Proof:* a test.
- **PUB-630** · built · **Editing together** has no session for a twin, because nobody may edit it. *Proof:* a test that opening a collab ticket for a twin is refused.
- **PUB-631** · new · **Copying** a twin into another space is allowed for those who may copy, like any page they may see; the copy is an ordinary page and holds only the twin's content. *Why:* readers copy what they read. *Proof:* a test.
- **PUB-632** · partly · **Moving** a page out of the draft space into an ordinary space is the existing move with its checks (`page.move_out`); its twin stays and shows Removed in the draft (PUB-331). Moving into the published space is refused (PUB-220). *Proof:* tests.
- **PUB-633** · built · **The release note** is free text written by the releaser for readers and shown in the twin's history and in notifications; it is the one way draft words can cross besides the released content, and it is written for that purpose. *Why:* the person releasing decides what readers read. *Proof:* a test that a release note is visible to readers, so the screen labels it "Note for readers" (13).
- **PUB-634** · new · **Restrictions on a twin** are the published space's choice (PUB-222). A release into a branch under a twin restricted to a group inherits that restriction, as any child does. *Proof:* a test.
- **PUB-635** · built · A **mutation** for each door: release copies a comment; a link keeps the draft id; the history shows a draft summary; a file route serves the draft file; a draft space builds a site; the managed rule lets an administrator edit. Each must be caught. *Proof:* the mutation check.
