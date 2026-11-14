# 09.T4: Variables and their scope

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

A **variable** is a named value that a template uses in more than one place. Diffluence has two kinds. **Answers** come from the fields of the form (chapter 05). **Built-in values** are known without asking, such as today's date or the space. Both are fixed at the moment the set is made, and written into the pages as plain content. After that, a page holds no variable and no link back to the form.

## 4.1 Kinds of variable

- **TPL-200** · built · **Answers.** Each field of the form is a variable with the field's name. Fields with the same name share one answer (ORG-050). *Proof:* existing tests of `FillFields`.
- **TPL-201** · new · **Built-in values**, the same in every entry of a set:

  | Value | What it holds |
  |---|---|
  | `date` | Today, in the instance's time zone, as the reader's locale shows dates. Today's `{date}`, kept |
  | `space` | The space's name. Today's `{space}`, kept |
  | `creator` | The person who makes the set, as a mention |
  | `set` | The title of the root page, as a link to it |
  | `template` | The template's name, as text |

  *Why:* these are the values people otherwise type again and again (TPL-012). *Edge cases:*
  - `date` is taken from the database server's clock, so nodes with drifting clocks agree (DOC-1240);
  - `creator` as a mention does not notify the creator.

  *Proof:* a table test for each value; a test that `date` comes from the database.
- **TPL-202** · new · **Links to entries** (TPL-110) are variables whose value is the page made from an entry. *Proof:* see TPL-110.
- **TPL-203** · new · There are **no computed variables**: no arithmetic, no joining of text, no "start date plus two weeks" written as a formula. *Why:* a formula is a small language, and this feature has none (TPL-1405). The one common need, a date relative to another, is met by the date field's default (TPL-304), which offers fixed choices. *Proof:* nothing to test; listed in chapter 14 (TPL-1201).

## 4.2 Where variables can be used

- **TPL-210** · new · Variables can be used in:
  - the **title pattern** of every entry, written as `{name}`;
  - the **text** of every entry, as a block inserted from `/`;
  - the **labels** of an entry, through a label rule (TPL-423).

  They cannot be used in restrictions (TPL-131), in file names, or in the template's own name. *Why:* each place listed is one where people asked for the value in the situations of chapter 02; the others would widen what a form answer can do. *Proof:* tests for each place.
- **TPL-211** · new · In the text, an answer is placed by today's `templateField` block, which shows a dashed box while editing (design standard). A built-in value or a link to an entry is placed by a new inline block, `templateValue`, with a source (`date`, `space`, `creator`, `set`, `template` or `entry:<key>`). *Why:* a field is something the form asks; a value is something the template knows. Keeping them apart keeps the form honest about what it asks. *Edge cases:* `Normalize` lets `templateValue` through only in templates, like `templateField` and `instruction` (spec 011). A page draft or a published page that contains one is refused, so a leftover can never reach a reader. *Proof:* a test that a page cannot be published with a `templateValue`, and a mutation that lets one through must be caught.
- **TPL-212** · new · The value placed depends on the field's kind (chapter 05):

  | Kind | What lands in the page |
  |---|---|
  | line, number, choice, link | text |
  | text | paragraphs, one per block of lines |
  | date | a date mark, the existing inline `date` block |
  | yes/no | the words "Yes" or "No" in the space's language, or nothing, as the author chooses |
  | choices | the chosen options, as a list or joined with commas, as the author chooses |
  | person | a mention |
  | people | mentions, joined with commas |
  | page | a page link |
  | label | text; the label itself is added by a label rule (TPL-423) |

  *Why:* the right block makes the value useful afterwards. A date block sorts and shows in the reader's locale, and a mention can be followed. *Proof:* a table test per kind.

## 4.3 Scope

- **TPL-220** · new · The scope of a variable is **one use of one template**. The same name means the same value in every entry of the set, and nothing outside it. *Why:* sharing across the set is the reason sets exist (TPL-012). Sharing beyond a set would need a store of values with its own rights and lifetime. *Proof:* a test that a name used in three entries has one value in all three.
- **TPL-221** · new · Variables are defined **per template**. There is no shared library of variables across templates or for the instance (decided, TPL-1419). *Why:* a library would need its own rights, versions and approval, and the situations of chapter 02 do not need one. Templating.app's documentation mentions no shared library either; other tracker apps offer one in a paid edition. *Proof:* listed in chapter 14 (TPL-1206).
- **TPL-222** · new · A variable's value is **fixed when the set is made**. A page does not change when the person mentioned leaves, or when the space is renamed. *Why:* a page is a record of what was true when it was written. Live values are what dynamic blocks are for (BLK). *Proof:* a test that renaming the space later does not change a page made with `{space}`.

## 4.4 Names

- **TPL-230** · new · A field's **name** is what the form shows as its label and what the title pattern uses. It must be unique in the template, ignoring case, and at most 100 bytes, as today. The editor refuses two fields that differ only in case. *Edge cases:*
  - renaming a field in the template's editor renames every use in every entry, every title pattern and every condition, as one change and one new version. A rename never leaves a condition pointing at nothing;
  - the names `date`, `space`, `creator`, `set` and `template` are reserved for built-in values. A template from before this feature that has a field with one of those names keeps working: its field wins in its own title pattern, as `ExpandTitle` does today, and the editor asks for a new name at the next save.

  *Proof:* a test of a rename across three entries and a condition; a test of a reserved name in an old template.
- **TPL-231** · new · A field has an optional **help text** of at most 300 characters, shown under its label and tied to the input (`aria-describedby`). *Why:* WCAG 3.3.2. *Proof:* an accessibility check.
