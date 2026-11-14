# 09.T2: What people need

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

A template saves the person who starts a page from a blank sheet. A guided template goes further. It knows that some work always takes **several pages**, that the same facts (a name, a date, a customer) appear in each of them, and that the right text depends on a few answers. The people who use it are often new to the work, so the form must lead them without teaching them a language. This chapter names the situations the study started from and turns them into requirement lines. Chapters 03 to 13 say how each is met.

## 2.1 Situations

These are typical uses of a wiki in a mid-sized organisation. They are written for Diffluence; no product's examples were used.

| Situation | Pages in one go | Answers that vary | Content that depends on an answer |
|---|---|---|---|
| A new colleague starts | A welcome page, a first-week plan, an equipment checklist, a page for the manager only | Name, start date, team, manager, works on site or remote | Remote workers get the equipment shipping section; a manager page only for some teams |
| A customer project begins | A project home, a decision log, meeting notes, a risk list, a hand-over page | Customer, project lead, start and end date, contract type | A fixed-price contract adds a change-request page |
| An incident is reviewed | A review page, a timeline, a list of actions | Severity, services hit, date, owner | A severe incident adds a page for the customer statement |
| A quarterly planning round | A page per team, under one round page, plus a summary | Quarter, year, the teams taking part | Only the teams chosen get a page (later, TPL-1202) |
| A procedure is written in a controlled space | One page, in the space's workflow (DOC-560) | Owner, scope, review period | A regulated scope adds a signature section |
| A team meeting | One page, as today | Date, attendees | None |

## 2.2 Needs

- **TPL-010** · new · One action creates **all the pages** a piece of work needs, in the right place in the tree, with their labels, so nobody forgets one and every instance of the work looks the same. *Why:* project generators exist for this reason. Copier, Backstage software templates and Templating.app all create a whole structure from one run, and Templating.app creates a hierarchy of issues in one action. *Met in:* chapter 03.
- **TPL-011** · new · The pages of a set **point to each other**: the home links to its parts, and each part links back. The links work from the first moment, with no fixing by hand. *Why:* a set whose pages do not know each other is just several templates. Templating.app links the issues it creates, to each other and to a parent. *Met in:* TPL-110 to TPL-114.
- **TPL-012** · new · A fact asked **once** appears wherever it is needed: in titles, in text and in every page of the set. *Why:* WCAG 2.2 success criterion 3.3.7 (redundant entry) asks that information entered before is not asked again in the same process. Today, fields with the same name already share one answer (ORG-050). *Met in:* chapter 04.
- **TPL-013** · new · The form asks for each fact in **the right kind of field**: a date picker for a date, a person picker for a person, a number field for a number, a choice for a fixed list. Each field says whether it is required and helps with a short text. *Why:* WCAG 3.3.2 (labels or instructions). Research on forms finds fewer errors when the input fits the data and required fields are marked plainly (Nielsen Norman Group, Baymard). Templating.app offers ten kinds of variable, among them text, number, person, date and single and multiple choice. *Met in:* chapter 05.
- **TPL-014** · new · Mistakes are caught **at the field**, in words, before anything is created, and nothing typed is lost. *Why:* WCAG 3.3.1 and 3.3.3 (error identification and suggestion). Baymard's 2024 testing found that inline validation helps people recover from errors faster, if it does not fire too early. *Met in:* TPL-320 to TPL-325.
- **TPL-015** · new · Text and whole pages appear **only when they apply**, decided by the answers, without the template's author writing code. *Why:* document assembly tools build documents from conditional text. When the condition is code (docassemble's logic is Python), authors must reason about re-execution and side effects, which few page authors can. Form builders such as SurveyJS show a field only when a condition on earlier answers holds. *Met in:* chapter 06.
- **TPL-016** · new · The person creating sees **what will be made** before it is made: the pages, their titles and their place, and the answers given, with a way back to change any of them. *Why:* the "check answers" pattern of the GOV.UK Design System. Wizards can hide what an action will change, which experienced users find rigid (Microsoft's wizard guidelines); a summary before the last step answers that. *Met in:* TPL-340.
- **TPL-017** · new · A set is made **completely or not at all**. *Why:* a half-made onboarding is worse than none, because nobody knows which pages are missing. Backstage marks a run as failed and leaves cleaning up to steps the author writes; a wiki can do better with one database transaction. *Met in:* TPL-120.
- **TPL-018** · new · A template can be meant for **some people only**: HR's onboarding template for HR, the incident template for the on-call team. Others do not see it in the gallery. *Why:* Templating.app restricts who may browse, apply and edit a template, beside the project's own rights. *Met in:* TPL-500 to TPL-503.
- **TPL-019** · new · A template that many people will use is **checked by someone responsible** before they can use it. A change to it is checked again, while the checked version stays in use. *Why:* the roadmap asks for it. A template is a multiplier: a mistake in it is copied into every page made from it. *Met in:* TPL-510 to TPL-524.
- **TPL-021** · new · A template can be **improved without breaking** anything: the pages made from it stay as they are, and people mid-way through the form are not surprised. *Why:* ORG-058, and the rule of DOC-110 that a running process keeps its version. *Met in:* chapter 08.
- **TPL-022** · new · Everything works by **keyboard and screen reader**, on a phone, and right to left, like the rest of Diffluence. *Why:* WCAG 2.2 AA is the goal of the design standard. *Met in:* chapter 12.
- **TPL-023** · new · Creating a large set must **not slow down** the space for others. *Why:* the load rule of ARC-238, and the headroom the product keeps. *Met in:* chapter 11.
- **TPL-024** · new · Templates that exist today keep **working unchanged**. *Why:* the upgrade promise of blueprint 07. *Met in:* chapter 13.
- **TPL-025** · new · A person can tell **where a page came from**: which template, which version, made by whom and when, and which pages were made with it. *Why:* teams that look after templates need to know what an older version produced. *Met in:* TPL-610 to TPL-612.

## 2.3 What people do not need

These were considered and are left out, with reasons, in chapter 14:
- a programming language in templates;
- pages that stay linked to the template and change with it;
- forms that send data anywhere else;
- a page per item of a list, which is later.

Templating.app also offers templates for whole projects and boards. The closest thing in Diffluence is the space template, which stays as it is (TPL-1203).
