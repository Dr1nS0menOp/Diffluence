# 09.T6: Conditions

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

A **condition** decides whether part of a template applies, from the answers and a few facts about the moment. The roadmap asks for "content filled in by conditions, without scripting".

Tools in this field take one of two roads:
- **Code or an expression language**, which is powerful and hard to review. Document assembly with Python, the `#if` and `#switch` functions of MediaWiki templates, Jinja conditions in project generators, and the `{name}` expressions of form libraries all take this road.
- **Structured rules picked from lists**, which are limited and readable. Templating.app's conditions ("all" or "any" of tests on field values and the user's groups) take this road.

Diffluence takes the second road, as document control did for transitions (DOC-1404).

## 6.1 What a condition is

- **TPL-400** · new · A condition is **"all of"** or **"any of"** a list of at most 10 **tests**. A test has:
  - a subject: a field, the space, or the creator's groups;
  - an operator, picked from those allowed for the subject's kind;
  - a value, picked or typed in a field of the subject's kind.

  There is no nesting, no "not" around a group, and no text that is parsed (decided, TPL-1405). *Why:*
  - one level of all or any covers every situation of chapter 02;
  - a reader of the template can say in one sentence what a condition does;
  - a structure picked from lists cannot hold a syntax error or an injection.

  *Edge cases:* a need for "A and (B or C)" is met by two conditions on two blocks, or by splitting the content. Chapter 14 says nesting is left out (TPL-1201). *Proof:* validator tests that refuse nesting, an 11th test, and an operator not allowed for the kind.
- **TPL-401** · new · The operators, by the subject's kind:

  | Subject | Operators |
  |---|---|
  | line, text, link | is empty, is not empty, is, is not, contains (ignoring case) |
  | number | is empty, is not empty, =, ≠, <, ≤, >, ≥ |
  | date | is empty, is not empty, is, before, after, on or before, on or after; the value may be a date or "the day the set is made" |
  | yes/no | is yes, is no |
  | choice | is empty, is, is not, is one of |
  | choices | is empty, includes, does not include, includes all of |
  | person, people, page | is empty, is not empty |
  | label | is, is not, is one of |
  | the space | is one of (chosen spaces), for templates of all spaces |
  | the creator's groups | is a member of, is not a member of |

  *Why:* each operator is total and cheap, and defined for every value, including an empty one (TPL-403). Person and page tests stop at "is empty". Testing who was chosen would make the template depend on people's identities, which change. *Proof:* a table test per operator and kind, shared by Go and TypeScript (TPL-430).
- **TPL-402** · new · A test on **the creator's groups** is allowed, and the condition editor says next to it, in words: "This changes the text people get. It does not stop anyone reading the template" (decided, TPL-1421). *Why:* different text for contractors and employees is a real need. But the template's text stays readable by everyone who may edit it, and a person could pick a different template, so this is never a way to protect content. *Edge cases:*
  - the groups are those of the creator at submit time, as the permission rule resolves them;
  - a deleted group makes the test false, and the template's managers see a warning in the template list.

  *Proof:* a test with a member and a non-member; a test with a deleted group.
- **TPL-403** · new · **Empty values** are defined for every operator:
  - an unanswered optional field is empty;
  - every test on an empty value is false, except "is empty", "is not" and "does not include", which are true.

  *Why:* a condition must never fail with an error at the moment of creating. Every test always has an answer (fail closed for content that is added: an empty value never adds a section by surprise). *Proof:* the table of TPL-401 includes an empty value for each operator.

## 6.2 What a condition switches

- **TPL-420** · new · An **entry** of a set may have a condition. When it is false, the page is not made, nor any page below it. *Why:* "a severe incident adds a page for the customer statement" (chapter 02). *Edge cases:* the root entry cannot have a condition, because a set needs its root. Links to a left-out entry become plain text (TPL-110). *Proof:* tests of a left-out entry with children and with links to it.
- **TPL-421** · new · A **step** or a **field** of the form may have a condition. When it is false, the step or field is not shown. Its answer is dropped and never stored or used, and a required field that is hidden is not required (decided, TPL-1407). *Why:* a person cannot be bound by an answer to a question they never saw. SurveyJS and Copier skip a question whose condition is false, and Copier also does not record its answer. *Edge cases:*
  - a condition on a field may only test fields of **earlier** steps, or earlier fields of the same step, so the form never has a cycle and never hides a field the person is typing in;
  - a hidden field's variable is empty everywhere it is used.

  *Proof:*
  - validator tests that refuse a test on a later field, and so any cycle;
  - an API test that an answer sent for a hidden field is ignored;
  - a test that a hidden required field does not block.
- **TPL-422** · new · A **section of text** may have a condition. It is a new block, `conditional`, that holds other blocks. While editing, it shows as a quiet outline with its condition written as a sentence above it, such as "Only when Works on site is no". When a page is made, a true section is unwrapped: its blocks stay and the wrapper goes. A false section is removed with everything in it. *Why:* this is the "content filled in by conditions" of the roadmap. Unwrapping leaves no trace of the template in the page. *Edge cases:*
  - `Normalize` lets `conditional` through only in templates, so it never reaches a reader (as TPL-211);
  - a conditional section may hold another conditional section, up to 3 deep. Each has its own one-level condition, so nesting text is allowed, but nesting logic is not;
  - files inside a removed section are not copied.

  *Proof:*
  - a test of true, false and nested sections;
  - a test that files in a removed section are not copied;
  - a test that publishing a page with `conditional` is refused.
- **TPL-423** · new · A **label rule** adds a label to an entry's page when its condition holds, or adds the label chosen in a label field. *Why:* "fixed-price projects get the label `fixed-price`", so reports over labels find them. *Edge cases:* a label off the space's fixed list is left out and reported, as today (ORG-043). *Proof:* tests of both rules and of a fixed list.
- **TPL-424** · new · A condition never changes **rights**, the space, the parent or the workflow of a page. *Why:* conditions shape content. Rights come from the template and the space (TPL-131). *Proof:* nothing to test; the validator offers no such target.

## 6.3 Evaluating safely

- **TPL-430** · new · Conditions are evaluated by one small Go package, `internal/conditions`. Its input is the condition, the answers and a fixed context (today's date from the database, the space, the creator's groups). Its output is true or false. It never fails at evaluation time, because the validator refused every malformed condition at save. *Why:*
  - the evaluator is a pure function, with no loops over user data and no calls out;
  - its cost is at most 10 tests per condition times the conditions of a template, a few thousand operations.

  A structure that cannot express a loop cannot hang. *Proof:*
  - unit tests from the shared table;
  - a fuzz target over random conditions and answers that never panics and never takes more than a fixed number of steps;
  - a mutation in each operator must be caught.
- **TPL-431** · new · The browser runs the **same table** in TypeScript to show and hide steps and fields while the person types. The server evaluates everything again on submit and in the dry run (TPL-341), and its answer is the only one that counts (decided, TPL-1406). *Proof:* the shared table in both suites. An API test sends answers that the browser would have hidden, and they are ignored.
- **TPL-432** · new · The validator runs when a template is **saved**, and refuses:
  - a test on a field that does not exist;
  - a value of the wrong kind;
  - a choice option that does not exist;
  - an operator not allowed for the kind;
  - a test on a later field (TPL-421);
  - a group that does not exist.

  The editor shows each problem next to the condition, in words, before saving (WCAG 3.3.1). *Why:* errors belong to the author, never to the person filling in the form. *Edge cases:* a field renamed in the editor renames its tests too (TPL-230). *Proof:* a validator test per rule.
- **TPL-433** · new · Document control's transition conditions (DOC-120) test pages, not answers, but they share the shape "all or any of tests". Whichever lands first puts the shared part (the all-or-any combinator and the "never fails" rule) in `internal/conditions`, and the other reuses it. *Why:* one way to read and test a condition across the product. *Proof:* a code review note in whichever wave comes second.
- **TPL-434** · later · Conditions that compare **two fields** with each other, such as "end date after start date". *Why:* useful, but it is a cross-field validation more than a condition, and it doubles the editor's choices. For now, a date field's bounds cover the common case (TPL-304).
