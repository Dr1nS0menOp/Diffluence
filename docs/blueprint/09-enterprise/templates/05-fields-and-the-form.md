# 09.T5: Fields and the form

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

The form is the only part of a guided template that most people see. It decides whether they trust the result. The research behind this chapter agrees on a few points:
- one clear question at a time works better than a dense page (GOV.UK "one thing per page", and their finding that simple question pages beat accordions);
- required fields are marked in words or with a marker that is explained (Nielsen Norman Group);
- errors are shown next to the field, after the person has left it and not while they type (Baymard, 2024);
- wizards suit tasks done rarely by people who do not know the domain, but must not hide what they will do (Nielsen Norman Group, Microsoft's wizard guidelines).

## 5.1 Field types

Today there are three kinds: `line`, `text` and `choice` (TPL-002). This feature adds nine. Each kind is defined once in Go, with its validation, and once in TypeScript for the form, both checked against one shared table of cases (TPL-325).

- **TPL-300** · built · **Line:** one line of text, at most 2,000 characters. New: a minimum and maximum length. *Proof:* existing tests, plus the length table.
- **TPL-301** · built · **Text:** several lines, at most 2,000 characters. New: a minimum and maximum length, and paragraphs in the page (TPL-212). *Proof:* as above.
- **TPL-302** · built · **Choice:** one of a list of options, at most 50 options of at most 200 characters each. New: shown as radio buttons up to 6 options and as a list above that. *Why:* radio buttons show every option at once, which is quicker for short lists. *Proof:* a test of both renderings and of an answer that is not an option, which the server refuses.
- **TPL-303** · new · **Number:** a whole or decimal number, with an optional minimum and maximum and a number of decimals (0 to 4). The form accepts the reader's decimal separator, and the page shows the number as the reader's locale writes it at creation. *Edge cases:* a number field is a text input with `inputmode="decimal"`, not `type="number"`, so a scroll wheel never changes it by accident. *Proof:* table tests with comma and point separators, the bounds, and too many decimals.
- **TPL-304** · new · **Date:** a calendar date, with an optional earliest and latest date. The default is either none, a fixed date, "the day the set is made", or that day plus or minus a number of days or weeks. *Why:* the one relative value people ask for (TPL-203), without a formula. *Edge cases:* dates are dates, not times, so time zones do not shift them. Bounds may also be relative ("not before today"). *Proof:* table tests across a month end and a leap day.
- **TPL-305** · new · **Yes/no:** a single checkbox, with the question as its label. It is never required, because "no" is an answer. *Why:* the common input for conditions ("works remotely"). *Proof:* a test that a required flag on a yes/no field is refused by the validator.
- **TPL-306** · new · **Page:** a page picker that offers only pages the creator may view, in any space they may view. A space can be set as a limit. The page link lands in the page (TPL-212). *Edge cases:*
  - the server checks the chosen page again on submit, so a page id sent by hand that the creator may not see is refused as "not found", never with its title;
  - a page that is deleted between choosing and submitting refuses the set.

  *Proof:* a test with a page id the creator cannot see.
- **TPL-307** · new · **Person:** one person, chosen with `SubjectPicker` among people the creator may see (`profiles.view`). The default may be "the person filling in the form". *Edge cases:*
  - a guest can be chosen only if the creator may see guests;
  - a disabled person cannot be chosen;
  - the server checks the person again on submit.

  *Proof:* tests for a guest, a disabled person, and a person the creator cannot see.
- **TPL-308** · new · **People:** as Person, for up to 20 people. *Proof:* as above, and the limit.
- **TPL-309** · new · **Choices:** several of a list of options, as checkboxes. It has a minimum and maximum number to choose. *Proof:* table tests.
- **TPL-310** · new · **Link:** a web address, `https` or `http` only, at most 2,000 characters, which lands as text the editor links as it links typed addresses. *Why:* a typed `javascript:` address must never become a link (OWASP). *Proof:* tests with other schemes, which are refused.
- **TPL-311** · new · **Label:** one label from the space's labels, or a new one if the space has no fixed list (ORG-043). It is used by label rules (TPL-423). *Proof:* a test with a fixed list.
- **TPL-312** · new · The number of fields per template rises from 20 to **40** across all entries of a set. Fields with the same name count once. *Why:* a set asks for more than one page does. Forty is still a form people finish; more belongs in the pages themselves. *Proof:* validator tests at 40 and 41.
- **TPL-313** · new · There is **no** rich-text field, file upload field, signature field or field that asks for a password. *Why:* answers stay plain text (spec 011): files belong in the page after it is made, signatures belong to document control, and secrets never belong in a wiki form. *Proof:* listed in chapter 14 (TPL-1207).

## 5.2 Required fields and defaults

- **TPL-315** · built · A field can be **required** (ORG-050). The form marks a required field with the word "required" in its label, not with colour or a symbol alone. *Why:* the design standard's rule that state never rests on colour, and the research on marking required fields. *Proof:* an accessibility check.
- **TPL-316** · new · A field can have a **default**, of its own kind, which the form shows filled in. A default counts as an answer if the person leaves it, so a required field with a default never blocks. *Edge cases:*
  - a default that is no longer valid is refused when the template is saved, for example a choice option that was removed or a person who left;
  - a default person who leaves after the save is shown empty in the form, with the note "the suggested person is no longer active".

  *Proof:* tests of a removed option and a person who left.
- **TPL-317** · new · A field can have a **placeholder**, but the placeholder is never the label. *Why:* a placeholder disappears when typing starts, which hurts memory and accessibility. *Proof:* an accessibility check that every input has a label.

## 5.3 Validation

- **TPL-320** · new · Validation runs **in the browser** when the person leaves a field and on Next, and **on the server** on submit. The server decides; the browser only helps. *Why:* a request can be sent without the browser. *Proof:* API tests that send every invalid value the browser would stop.
- **TPL-321** · new · An error is shown **next to its field**, in words that say what to do ("Enter a date on or after 1 March 2027"). Pressing Next or Create with errors moves the focus to an **error summary** at the top of the step, which links to each field. *Why:* WCAG 3.3.1 and 3.3.3, and the GOV.UK error summary pattern. *Proof:* keyboard e2e tests.
- **TPL-322** · new · An error is not shown while the person is still typing in an empty field. It disappears as soon as the value is valid. *Why:* Baymard's findings on premature validation. *Proof:* an e2e test.
- **TPL-323** · new · Validation is limited to: required, length, number range and decimals, date range, the number of choices, the kinds of TPL-306 to TPL-311, and **named formats** for a line: email address, whole number, and letters and digits only. There are no regular expressions (decided, TPL-1408). *Why:* a regular expression behaves differently in the browser and in Go, can be slow in the browser, and is hard for a template's author to get right. *Proof:* table tests per format.
- **TPL-324** · new · Answers that fail on the server come back with the field's name and the message. The form shows them in place without losing other answers. *Proof:* an e2e test that the server refuses a value the browser accepted, using a mocked browser check.
- **TPL-325** · new · Go and TypeScript validate from **one shared table of cases** (a JSON file in the repository read by both test suites), so the two never disagree on a case the table holds. *Proof:* both suites run the table, and a mutation in either side is caught.

## 5.4 Steps

- **TPL-330** · partly · Fields are grouped into **steps**, at most 10, each with a title (ORG-061). New: writing steps on the template screen; a step may have a condition (TPL-421); a step with no visible field is skipped. *Proof:* tests for each.
- **TPL-331** · new · The form says where the person is ("Step 2 of 4") in a status line. The count changes when a condition shows or hides a step, and the change is announced. *Why:* WCAG 4.1.3. *Proof:* an accessibility check.
- **TPL-332** · new · Back keeps every answer. Closing the dialog asks first when anything was typed, with `ConfirmDialog`. *Why:* nothing typed is lost (TPL-014). *Proof:* an e2e test.
- **TPL-333** · partly · The **last step** sets the parent of the root page and shows the labels the set will get (ORG-061, still open today). *Proof:* an e2e test.

## 5.5 Checking the answers

- **TPL-340** · new · Before anything is made, a **check step** shows:
  - every answer, grouped by step, each with a Change link back to its field;
  - the **pages that will be made**, as an indented list with their titles and their place;
  - the labels and the restrictions of each page;
  - whether the pages will be drafts or published;
  - any title that already exists in the space (TPL-141).

  A set of one page with no fields skips this step, as today. *Why:* TPL-016. *Proof:* an e2e test of a set where a condition leaves out one page, which is then not in the list.
- **TPL-341** · new · The check step is computed **by the server**, through a dry run of the same code that makes the set. The dry run makes nothing and returns the list. *Why:* the list must be exactly what will happen, conditions included. Backstage offers a dry run for template authors; here it serves every user. *Proof:* a test that the dry run and the real run give the same pages for 200 random answer sets (a property test).
