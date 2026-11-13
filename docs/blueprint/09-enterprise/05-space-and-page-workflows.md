# 09.5: Space and page workflows

How to read this chapter: see [01-document-control.md](01-document-control.md).

Some teams control a whole space (every procedure of a quality system); others control a handful of pages in an otherwise open space (a policy inside a team space). Both must be possible, without ever leaving a page under two procedures at once. ISO 9001 7.5.3 leaves the scope of control to the organisation, so Diffluence offers both scopes and makes the effective one plain.

## 5.1 The space workflow

- **DOC-400** · built · A space can set one **space workflow**, with the kinds of item it governs. Every page of those kinds in the space follows it, including new pages, pages moved in, and pages made from templates. *Why:* a quality system is usually a space of controlled documents; per-page setup would leave gaps. *Edge cases:* folders and links are never governed (they hold no content to approve); a space workflow that governs blog posts holds each post back until approved, which suits announcements that need sign-off. *Proof:* tests for each kind; a test that a new page in a controlled space starts in the start state.
- **DOC-401** · partly · Turning a space workflow on shows the preview of DOC-140 for the whole space and applies it as background work for large spaces. Pages approved under today's approval rounds land in the first approved state (09.12); others land in the start state and disappear for readers until approved, which the preview states in numbers (decided, DOC-1406). *Proof:* a test of the preview and of the landing states.
- **DOC-402** · new · The space workflow can **exempt** pages by label (for example a "working notes" label), set by those with `space.workflows`. Exempt pages are ordinary pages. Adding or removing the exempt label is itself audited, and removing it puts the page in the start state. *Why:* teams keep a few working pages beside their controlled documents. *Edge cases:* an author who adds the exempt label to escape review is visible in the audit; the label change needs `space.workflows`, not only `page.edit`. *Proof:* a test that `page.edit` alone cannot add the exempt label.

## 5.2 Page workflows

- **DOC-410** · built · Where the space has no workflow, a person with the new permission `page.workflow` can set a workflow of the space on one page, or on a page and everything below it (a **branch**). New child pages of a branch inherit it. *Why:* control where it is needed, without forcing a whole space. *Edge cases:* a child page moved out of the branch leaves the workflow and becomes an ordinary page at its approved version (09.13); a page moved into a branch joins it in the start state. *Proof:* tests for set, inherit, move out and move in.
- **DOC-411** · built · A page has **at most one** workflow. Setting a branch workflow on a parent where a child already has its own page workflow asks whether to replace the child's, and lists the pages; without a choice nothing changes. *Why:* two procedures on one document would give two answers to "is this approved?". *Proof:* a test that the effective workflow is unique after every combination.

## 5.3 When both meet

- **DOC-420** · built · A **space workflow overrides** page workflows. Turning on a space workflow in a space with page workflows shows how many pages change workflow and in which state they land. Rounds that run under a page workflow end as "superseded by the space workflow", and their record stays. *Why:* the space owner's decision is the broader control and must win; the record of what happened before must stay. *Proof:* a test of the override and of the superseded rounds.
- **DOC-421** · built · Turning off a space workflow does not bring back the page workflows it replaced (decided, DOC-1412). Pages become ordinary pages at their approved versions (DOC-141); a person with `page.workflow` can set page workflows again. *Why:* silently re-activating old procedures would surprise everyone. *Proof:* a test.
- **DOC-422** · built · The page always shows which workflow governs it and why ("space workflow" or "set on this branch by …"). *Proof:* an e2e picture.

## 5.4 Across spaces

- **DOC-430** · new · A workflow belongs to one space; a space cannot use another space's workflow directly, only copy it, or copy an instance template (DOC-130). *Why:* a space owner must own the procedure of the space. *Proof:* an API test that assigning a foreign workflow is refused.
- **DOC-431** · later · Draft and published spaces (the next enterprise feature, spec 048) publish only approved versions from a draft space into a published space. They reuse the visibility rule of DOC-300 and are specified in their own study.
