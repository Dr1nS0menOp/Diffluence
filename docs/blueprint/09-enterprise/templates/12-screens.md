# 09.T12: The screens

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

Every screen follows the design standard (`docs/design/README.md`, the Stil look of ADR-034):
- one of the five frames, never a new one;
- the components of its table;
- every state handled;
- every word in `en.json`;
- a picture in `web/e2e/0-design.spec.ts`, in light, dark, narrow and right to left.

WCAG 2.2 AA is the goal. No layout, icon or wording is taken from any other product.

## 12.1 Where things live

- **TPL-1000** · built · A space's templates are in **Space settings, Templates**. Templates of all spaces are in **Administration, Content, Templates**. Both are lists of rows with Edit, Hide, Promote, Export and Delete (spec 011). *Proof:* existing pictures.
- **TPL-1001** · new · Both lists gain:
  - a **state** column, as a `.lozenge` with a word: Draft, Waiting, Approved, Sent back, Withdrawn;
  - a filter of states by dropdown above the list (design standard, "Long lists filter with dropdowns");
  - a section **Waiting for approval** at the top when anything waits, for those who may approve (decided, TPL-1423).

  *Proof:* e2e pictures with no template, one waiting, and fifty.
- **TPL-1002** · new · A person with `template.propose` and no right to manage finds **My templates** in the same Templates section of the space. That person sees only their own drafts and proposals, not the settings of the space; the section shows nothing else to them. *Why:* one place to look, without opening the settings of a space to people who may not change it. *Edge cases:* the settings frame shows such a person only this one section. *Proof:* a matrix test of the sections shown, and a picture.

## 12.2 The template editor

- **TPL-1010** · new · Opening a template shows the **Settings** frame with the template's sections in the sidebar, one at a time at its own address (`?section=`):

  | Section | What it holds |
  |---|---|
  | General | Name, description, category, hidden, promoted, drafts or publish at once, default parent |
  | Pages | The entries of the set (TPL-1011) |
  | Form | Fields and steps (TPL-1012) |
  | Rules | Label rules (TPL-423) |
  | Who can use it | The audience lists and, for templates of all spaces, the spaces offered (TPL-500, TPL-502) |
  | Versions | History, comparison, proposal and decisions (TPL-602) |

  *Why:* a guided template has settings of very different kinds. The settings frame is the standard's answer to "many sections, one at a time". *Proof:* an e2e picture per section.
- **TPL-1011** · new · **Pages** is an indented list of entries, as rows with:
  - the entry's name and kind;
  - its condition as a sentence;
  - Edit text, which opens the entry's text in the **Focus** frame, the editor;
  - Move up, Move down, Indent and Outdent as buttons, with the same keys as the tree.

  There is no dragging (design standard, "Move up and Move down instead of dragging"). A text summary above the list says "This template makes up to 7 pages, 3 levels deep". *Why:* a list of buttons works with a keyboard and a screen reader from the first day (as DOC-1419). *Proof:* an e2e test that builds a three-level set by keyboard only.
- **TPL-1012** · new · **Form** lists the steps, each with its fields as rows: name, kind, required, condition. Opening a field shows its settings in a `Dialog`, with only the settings its kind has (TPL-300 to TPL-311). The validator's messages stand next to the field they concern, before saving (WCAG 3.3.1). A **Try the form** button opens the form exactly as people will see it, with a dry run at the end that makes nothing. *Proof:* e2e tests per kind; an accessibility check of the dialog.
- **TPL-1013** · new · In an entry's text, `/` offers:
  - **Field**, as today;
  - **Value**, for a built-in value or a link to a page of the set (TPL-211);
  - **Section shown when…** (TPL-422).

  A field has a dashed border and instruction text is muted and italic, as today (design standard, "Fields and instruction text"). A conditional section has a quiet outline in `--color-guide`, with its condition as a sentence on its first line. Its settings come from the block's menu in the margin, like every block. *Proof:* e2e pictures of a template with each.
- **TPL-1014** · new · A **condition** is edited as a form of native fields, like a rule's When, If and Then (design standard, "Insight and automation"):
  - "All of" or "Any of";
  - then rows of subject, operator and value, each a native select or input of the right kind;
  - Add test, and Remove on each row.

  A sentence above the rows says what the condition means in words. *Why:* the sentence is what an approver reads. *Proof:* a keyboard e2e test, and a test that the sentence matches the table of TPL-401.

## 12.3 The form for the person who creates

- **TPL-1020** · built · The gallery opens as a `Dialog` from Create or from the empty page, with a list, a search box and a preview (ORG-045). *Proof:* existing pictures.
- **TPL-1021** · new · A set template in the gallery says how many pages it makes, in the preview and in words ("Makes up to 7 pages"). *Proof:* a picture.
- **TPL-1022** · new · The form keeps the `Dialog`:
  - one step at a time;
  - the step's title as its heading;
  - "Step 2 of 4" in a status line (TPL-331);
  - Back as a `quiet` button and Next or Create as the `primary` one.

  Inputs use `Field`, with the label, "required" in words where it applies, the help text, and the error. A person field uses `SubjectPicker`. *Why:* the components of the standard, so the form feels like the rest of the product. *Proof:* pictures of every field kind, empty, filled and in error.
- **TPL-1023** · new · A field or step that a condition shows **appears after** the field that caused it. The change is announced in a polite live region ("1 more question"), and the focus never moves by itself. *Why:* WCAG 3.2.2 (on input) and 4.1.3. A change of context while typing disorients. *Proof:* an accessibility test with a screen-reader query.
- **TPL-1024** · new · The **check step** (TPL-340) shows the answers as a description list with Change links, and the pages as an indented list with titles, labels as plain `#` words and restrictions as the lock icon with its words. The primary button names the action: "Create 7 pages" or "Create 7 drafts". *Why:* the design standard's rule that a button says what it does. *Proof:* a picture.
- **TPL-1025** · new · A **background set** (TPL-121) shows a status line with progress in words ("Making 64 pages…"), and the person can close the dialog. A `useToast` and the bell tell them when it is ready, with a link to the root page. *Proof:* an e2e test with a slowed set.
- **TPL-1026** · new · Errors from the server after Create show in an `ErrorNote` at the top of the check step, with the list of TPL-810 when rights are missing. The answers stay. *Proof:* an e2e test.

## 12.4 Approval

- **TPL-1030** · new · A waiting version opens on its **Versions** section with the comparison to the approved version (TPL-602) and two actions:
  - **Approve**, `primary`;
  - **Send back**, which opens a `Dialog` with a required reason.

  Withdraw is a `danger` action in the template's More menu, behind a `ConfirmDialog` that repeats the action on its button. *Proof:* pictures, and keyboard tests of each action.
- **TPL-1031** · new · The page's **Details** panel shows the origin (TPL-611) as one line with a link. *Proof:* a picture.

## 12.5 Keyboard, language, small screens

- **TPL-1040** · new · Every action has a keyboard path. The form never traps focus: Escape asks before closing when answers exist (TPL-332), and focus returns to the button that opened the dialog. *Why:* WCAG 2.1.1, 2.1.2 and 2.4.3. *Proof:* keyboard e2e tests.
- **TPL-1041** · new · Words people type (names of fields, options, steps and entries) are shown as typed. All other words are translation strings, and the layout uses logical properties only, so the form works right to left. *Proof:* the strings check and an RTL picture of the form.
- **TPL-1042** · new · The form, the check step and approval work at **360 px**. The template editor works at tablet width; on a phone it says it is best used on a larger screen, without hiding any function (as DOC-950). *Proof:* pictures at the narrow width.
- **TPL-1043** · new · Dates in the form use the reader's locale, and date fields offer the browser's own date input with a typed fallback. *Proof:* a test in two locales.
