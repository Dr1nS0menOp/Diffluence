# 09.10: The workflow editor and the screens, with keyboard and accessibility

How to read this chapter: see [01-document-control.md](01-document-control.md).

Document control is used by quality teams for years at a time, often under pressure of an audit. The screens must be calm, plain and fully usable by keyboard and screen reader. They follow the design standard of Diffluence (`docs/design/README.md`, the Stil look of ADR-034, the settings layout with the list on the left), and WCAG 2.2 at level AA.

## 10.1 Where things live

- **DOC-900** · new · Space settings gain **Workflows** under "Who can do what", beside the existing approval rule, which it replaces once a workflow exists (09.12). The list shows each workflow with the number of pages it governs, its version and when it last changed. *Proof:* an e2e picture.
- **DOC-901** · new · The page gains a **control band** under the byline (DOC-302, DOC-303) and a **review panel** that opens from it, in the same place as today's approval panel. *Proof:* e2e pictures for reader, author and reviewer.
- **DOC-902** · new · Administration gains **Document control** under Content, with the instance templates (DOC-130) and the cross-space reports (DOC-804, DOC-1020). *Proof:* an e2e picture.

## 10.2 The workflow editor

- **DOC-910** · new · The editor is a **form** (decided, DOC-1419): a list of states, each opened to edit its name, kind, description, reviewers, rule, due period and review period; then a list of transitions, each with from, to, button name, who may take it, conditions and requirements. *Why:* a form works with a keyboard and a screen reader from the first day, and it is easy to test. *Proof:* e2e tests that build both example workflows by keyboard only.
- **DOC-911** · new · A **diagram** of the workflow is drawn read-only from the form, as SVG, with each state as a box and each transition as an arrow labelled with its button name. It has a text equivalent: the list of transitions as sentences ("From In review, Approve leads to Approved"). *Why:* WCAG 1.1.1 (non-text content). *Proof:* an accessibility check that the text equivalent exists and matches.
- **DOC-912** · new · The validator of DOC-102 runs while editing and shows problems next to the state they concern, in words, before saving. Saving a broken workflow is not possible. *Why:* WCAG 3.3.1 and 3.3.3 (error identification and suggestion). *Proof:* e2e tests for each rule.
- **DOC-913** · new · Saving shows what changes for running rounds (none, by DOC-110) and for pages in states that are renamed. *Proof:* a test.
- **DOC-914** · later · A drag-and-drop visual editor. *Why:* nice, but a form plus diagram covers the need first.

## 10.3 The control band and review panel

- **DOC-920** · new · The band uses text and an icon, never colour alone, for the state. *Why:* WCAG 1.4.1. *Proof:* an accessibility check.
- **DOC-921** · new · The review panel lists reviewers with their answers as a table with headers, the rule and count as text, and the actions as buttons named by their transition. *Why:* WCAG 1.3.1 (info and relationships). *Proof:* an accessibility check.
- **DOC-922** · new · Answering opens a small dialog with the comment field first, focus on it, and the answer buttons named "Approve" and "Ask for changes" (or the workflow's names). Escape closes it without answering, and focus returns to the button that opened it. *Why:* WCAG 2.1.2 (no keyboard trap) and 2.4.3 (focus order). *Proof:* keyboard e2e tests.
- **DOC-923** · new · The **signing dialog** (DOC-720) names the meaning in its heading and button, asks for the confirmation in the same dialog, and announces the result in a live region. *Why:* WCAG 4.1.3 (status messages). *Proof:* an accessibility test with a screen reader query of the dialog's accessible name.
- **DOC-924** · new · Due dates show as dates in the reader's locale and, for screen readers, with the full date; overdue rounds are marked with text. *Proof:* a test.

## 10.4 Keyboard

- **DOC-930** · new · Every action of document control has a keyboard path, and the page's keyboard shortcuts gain one: open the review panel. It is listed in the shortcuts dialog and can be turned off like the others. *Why:* WCAG 2.1.1 and 2.1.4 (character key shortcuts). *Proof:* a keyboard e2e test.

## 10.5 Language and right to left

- **DOC-940** · new · All texts are translation strings (the UI strings check of `make check`), and the layout uses logical properties so it works right to left. State and transition names typed by people are shown as typed. *Proof:* the existing strings check and an RTL e2e picture.

## 10.6 Small screens

- **DOC-950** · new · The band, the panel and the signing dialog work at 360 px wide; the workflow editor works at tablet width and says on a phone that it is best used on a larger screen, without hiding any function. *Proof:* e2e pictures at the narrow width.
