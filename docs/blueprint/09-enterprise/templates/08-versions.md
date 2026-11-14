# 09.T8: Versions of a template

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

Today each save of a template's text is a version of its hidden page, but the rest of the template (steps, title pattern, labels, default parent) is a row that is overwritten (TPL-007). A guided template is more than text: it has entries, fields, conditions and rules, and approval binds to all of them (TPL-522). So the template as a whole needs versions.

## 8.1 What a version is

- **TPL-600** · built · A **template version** is a numbered, unchangeable snapshot, called a **manifest**, of everything that decides what the template makes:
  - the entries and their order;
  - for each entry, its hidden page and **the page version number** of its text;
  - the title patterns, labels, label rules, restrictions and workflows;
  - the fields, steps and conditions;
  - "drafts or publish at once";
  - the default parent.

  The manifest is stored as JSON in `template_versions`, validated in Go on save. Each save of the template makes one new version, whatever changed. *Why:*
  - a version must be complete, or "approve version 4" would not mean one thing;
  - text already has page versions, so the manifest points to them instead of copying them.

  *Edge cases:* the name, description, category, hidden, promoted and the audience lists are **not** part of the manifest. They change how the template is found and who uses it, not what it makes, and they are audited on their own. *Proof:*
  - a test that every part listed above changes the version;
  - a test that the name does not;
  - a test that an old manifest still gives the old text.
- **TPL-601** · built · A version is never changed or deleted while the template exists. Deleting a template deletes its versions, its hidden pages and its approvals, as today, and the pages made from it keep their origin record with "template deleted" (TPL-612). *Proof:* a test.
- **TPL-602** · new · The **history** of a template lists each version with:
  - its number, author, time and state (draft, waiting, approved, sent back, superseded);
  - who decided and why.

  Two versions can be **compared**:
  - the text of each entry, with `DiffDocument`, as pages are compared;
  - the rest as a readable list of changes: "Field *Team* added, required", "Entry *Equipment* now has the condition *Works on site is no*".

  *Why:* approvers and auditors must see what changed (TPL-522). This is the same need as the workflow history of DOC-112. *Proof:* an API test of the change list for each kind of change.

## 8.2 Using a version

- **TPL-603** · partly · Opening the form **binds** it to the version it shows: the approved version, or the owner's draft when the owner tries it. Submitting makes pages from that version, even if a newer one was approved in between. *Edge cases:*
  - a version that is withdrawn (TPL-524), or a template the person is no longer allowed to use (TPL-503), refuses the submit with a message and a way to start again;
  - a form left open for more than 24 hours asks the person to start again, because the dry run (TPL-341) may no longer be what would happen.

  *Why:* the person must get what they checked (TPL-016). *Proof:* tests across an approval, a withdrawal and the 24-hour limit.
- **TPL-604** · new · An automation rule uses the **approved** version at the moment it runs. *Proof:* a test.

## 8.3 Pages made from an older version

- **TPL-610** · built · Every page made from a template records its **origin** in `page_template_origin`:
  - the set;
  - the template;
  - the template version;
  - the entry key;
  - who made it and when.

  The record is kept while the page exists, and moves, copies and imports treat it as chapter 10 says. *Proof:* a test.
- **TPL-611** · new · The page's **Details** panel shows "Made from *template* version *n* on *date*", with a link to the template's history for those who may see the template, and the other pages of the same set. A reader who may not see the template sees "Made from a template", with no name (TPL-504). *Proof:* tests for both kinds of reader.
- **TPL-612** · new · A page made from an **older version** stays exactly as it is (decided, TPL-1416; ORG-058). There is no "update this page to the new version" and no merge. Details says "a newer version of the template exists" to people who may edit the page, and the template's history can show the comparison between the two versions. *Why:* a page is people's work from the moment it is made. Project generators that update older projects (Copier) need a version control system and a person to settle conflicts by hand. A wiki page has neither. *Proof:* a test that approving a new version changes no existing page and no draft.
- **TPL-613** · new · The template's managers see **how many sets** each version made, and can list the sets of a version, showing only the root pages they may see. *Why:* knowing what an older version produced helps decide whether to tell those teams about a fix. *Proof:* a test that the list never shows a page the manager cannot see, and counts it only as a number.

## 8.4 Before and after this feature

- **TPL-620** · new · Each template that exists at the upgrade gets **version 1**, made from its current row and its hidden page's current text, marked approved (TPL-1101). Pages made before the upgrade have no origin record. *Proof:* the migration test.
