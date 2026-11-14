# 09.T3: Sets of linked pages

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

A **set template** makes several pages in one action. Today a template is one hidden page (TPL-001). A set template is a template with more than one **entry**. Each entry is a page to be made, with its own text, title pattern, labels and place in the set. A template with one entry is exactly today's template, so there is one concept, not two.

## 3.1 The structure

- **TPL-100** · built · A template has one or more **entries**, ordered as a tree with one **root entry**. Each entry has:
  - a stable key, made by Diffluence and never shown as text to fill in;
  - a name for the template's editor;
  - a kind: page or folder (ADR-028);
  - its own text, a title pattern and labels;
  - optionally a condition (TPL-420) and restrictions (TPL-130).

  The text of every entry is a hidden page of kind `template`, as today (spec 011, D6). Each entry therefore gets the editor, versions, files and comparison for free. *Why:* reuse the one rule that keeps template text away from readers, instead of adding a second store. *Edge cases:*
  - a folder entry has no text (ADR-028), only a title, labels and restrictions;
  - an entry's key survives renaming and moving, so conditions and links keep pointing to it.

  *Proof:* a test that a two-entry template stores two hidden pages, and that neither appears in any list, search or route for any person (the matrix of spec 011, D5, extended).
- **TPL-101** · built · The root entry is placed where the person chooses, under the template's default parent, or at the top of the space. Every other entry goes under its parent entry, in the order the template gives. The whole set lands as **one subtree of one space** (decided, TPL-1402). *Why:* one space means one tree lock and one transaction (TPL-120). Rights across spaces would make "all or nothing" depend on many rule sets. *Edge cases:* there is no entry kind for blog posts, databases or link entries. Posts sit outside the tree, and databases are an experimental part of their own. *Proof:* a test that a set of three levels lands in the given order under the chosen parent.
- **TPL-102** · built · A set has **at most 100 entries** and at most 6 levels below the root (decided, TPL-1403). The editor counts entries as they are added and refuses the 101st with a message. The limit counts entries in the template, not the pages a given set produces after conditions. *Why:* the load budget (TPL-901). Half the space-template limit of 200 keeps the transaction short. *Proof:* validator tests at 100 and 101 entries, and at 6 and 7 levels.
- **TPL-103** · built · An entry may hold its own **files**: images and attachments in its text, copied to the new page, as today (ORG-052). Copies count against the space's storage limit (ADR-038). The whole set is checked **before** anything is made. *Proof:* a test that a set whose files do not fit is refused with nothing created.

## 3.2 Links between the pages of a set

- **TPL-110** · built · The text of an entry can hold a **link to another entry** of the same template, inserted from `/` as "Link to a page of this set". The link names the entry's key. When the set is made, each link becomes an ordinary page link (`pageLink`) to the page made from that entry. *Why:* the links must work from the first moment (TPL-011). *Edge cases:*
  - a link to an entry that a condition left out becomes plain text: the title the page would have had, without a link (TPL-420);
  - a link to the entry's own page is allowed.

  *Proof:* a test that a set of three pages, linking in a ring, has three working links. A test that a link to a left-out entry becomes text.
- **TPL-111** · built · Page ids are made (UUIDv7) **before** any text is written, so every link can be resolved in one pass inside the transaction. There is no second pass that edits pages after they exist. *Why:* a second pass would add a version to each page, and could fail halfway. *Proof:* a test that each new page has exactly one draft, or one version when published at once, and no extra version.
- **TPL-112** · new · Dynamic blocks in the text keep working as they do in any page. For example, the list of child pages or a facts report over the set's label (BLK-040). The template's author can use them to build the set's overview without links. *Proof:* a test that a child-pages block on the root lists the set's pages.
- **TPL-113** · built · A link to an **existing** page comes from a page field (TPL-306). It is never written into the template by key. *Why:* a template that points to a fixed page of one space breaks when that page moves or its reader lacks access. A field is checked against the creator's rights (TPL-306). *Edge cases:* a page link typed into a template's text by hand is kept as it is. A reader without access sees it as a link they cannot follow, as with any page link. *Proof:* a test of a hand-typed link.
- **TPL-114** · built · Every page of a set records its **origin**: the set, the template, the template version and the entry key (TPL-610). That record is not a link a reader sees; the page details show it (TPL-611).

## 3.3 Making a set: all or nothing

- **TPL-120** · built · A set is made **in one database transaction**, which:
  - checks every right (TPL-503 and TPL-130);
  - takes the tree lock of the space;
  - makes every page;
  - copies every file;
  - adds every label and restriction;
  - writes the origin rows and the audit entries.

  Any failure rolls back everything, and the person sees one message that names the first cause and the entry it concerns. *Why:* a half-made set is worse than none (TPL-017). Today's single page is already made this way (`StartFromTemplate`), so this extends a working pattern, not a new one. *Edge cases:* see chapter 10. *Proof:*
  - fault-injection tests that fail at the first page, a middle page, the last page, a file copy, a label and the audit write, and find no trace of the set;
  - a mutation that commits per page must be caught.
- **TPL-121** · partly · Sets of **more than 25 pages** are made as background work, in the jobs table with `SKIP LOCKED`, still in **one** transaction (decided, TPL-1403). The person sees progress in a status line, and can leave the dialog; the bell tells them when the set is ready or why it failed. *Why:* a long request can be cut by a proxy, while a job finishes. One transaction keeps "all or nothing" true. *Edge cases:*
  - a worker that dies mid-set rolls back, and the job is picked up again from the start, because nothing was committed;
  - the job carries the answers only until it ends, and then they are deleted (TPL-1409).

  *Proof:* a test that kills the worker halfway and finds no pages, then one complete set after the retry.
- **TPL-122** · built · The **empty page** that was waiting for a template (PAG-004) is removed in the **same** transaction that makes the set, not in a second call from the browser as today (TPL-006). *Why:* today a failure between the two calls leaves an empty page behind. *Proof:* a test that a refused set keeps the empty page, and a made set removes it.
- **TPL-123** · built · Using a template twice with the same answers makes two sets. A request carries a one-time key made when the form opens, so a double click or a resent request makes **one** set. *Proof:* a test that sends the same request twice.

## 3.4 Naming

- **TPL-140** · built · Every entry has a **title pattern**, filled with the variables of chapter 04 (TPL-210). An empty pattern takes the entry's name. *Edge cases:*
  - a title that comes out empty after filling, because the answers were empty, takes the entry's name;
  - a title over 255 characters (PAG-009) is cut at a word boundary, with an ellipsis;
  - leading and trailing spaces are removed.

  *Proof:* table tests of the title filler for each case.
- **TPL-141** · partly · Titles need not be unique (PAG-008). The step that checks the answers (TPL-340) shows each title the set will make, and marks one that already exists in the space with a link to the existing page, as the editor does. *Why:* two pages "Onboarding: Sam" may be right, or may be a mistake; the person decides. *Proof:* an e2e test with an existing title.
- **TPL-142** · built · Answers are put into titles **once**, as text. An answer that contains `{date}` or the name of another field is not filled in again (today's single-pass `ExpandTitle`, kept). *Why:* an answer must never be read as part of the pattern. *Proof:* a test with an answer of `{space}`.
- **TPL-143** · partly · The set's **name** for history and the bell is the title of its root page. *Proof:* covered by the notification tests (TPL-720).

## 3.5 Restrictions in a set

- **TPL-130** · built · An entry may carry **restrictions**: who may view and who may edit the page made from it, as people and groups. A typical use is a page for the manager only. Making a set with restricted entries needs `restriction.manage` in the space. Without it, the whole set is refused before anything is made, and the message names the entry (decided, TPL-1417). *Why:* a page meant to be restricted must never exist without its restriction, not even for the moment between creation and a second step. *Edge cases:*
  - a group named in a restriction that was deleted since the template was approved refuses the set, with the entry and the missing group named. The set is never made with a looser restriction;
  - view restrictions inherit downward, as today, so the children of a restricted entry are restricted too, and the review step says so.

  *Proof:*
  - a test that a person without `restriction.manage` gets no pages at all;
  - a test with a deleted group;
  - a test that the restricted page is never visible to an outsider, from any route, at any moment of the transaction (read from a second connection).
- **TPL-131** · built · Restrictions are **only** taken from the template. A field cannot choose who may see a page in this feature (TPL-1205). *Why:* rights from a form answer are a new door. The need is not shown yet.

## 3.6 Drafts or published

- **TPL-150** · partly · By default every page of a set is made as an **unpublished page with a draft** of its creator, as one page is today (decided, TPL-1404). Only the creator sees the pages until they publish them. *Edge cases:* a set of drafts shows the creator a note on the root page: "Made from template X with N pages, not yet published", with **Publish all**, which publishes every page of the set they still hold as a draft, in one transaction. *Proof:* a test that a reader sees none of the set; a test that "Publish all" is all or nothing.
- **TPL-151** · built · A template can say **publish at once**. The set is then published as it is made. This needs every required field answered, and the template's text must hold no unfilled field, because `ForPublish` refuses those. The editor warns the author when an entry would be refused this way. *Why:* routine sets, such as meeting pages, need no review before others see them. *Edge cases:* in a space or branch under document control, every page enters the workflow's start state (DOC-560), so "published" means a working version that readers do not see until it is approved. *Proof:* a test of both, and a test in a controlled space that readers see nothing.
- **TPL-152** · built · A template may name a **workflow** for the pages it makes, per entry (DOC-560). A space workflow wins (DOC-420). Choosing a workflow needs `page.workflow` (DOC-011); without it, the set is refused, never made outside control. *Proof:* tests for each case.
