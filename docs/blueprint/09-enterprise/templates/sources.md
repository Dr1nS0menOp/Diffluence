# Sources of the study of guided templates

Every source was read on **2026-11-14**, for behaviour or research findings only. No product's source was read. No text, field names, icons or layouts were copied; every requirement is written in Diffluence's own words (`docs/blueprint/02-clean-room-and-licensing.md`).

## The behaviour meant: Templating.app

Templating.app is named in blueprint 08 only to say which behaviour is meant. Its pages describe an app for issues in an issue tracker; this study means the same behaviour for pages.

- [Marketplace listing](https://marketplace.atlassian.com/apps/1224664/templating-app-project-templates-issue-templates-for-jira): templates for hierarchies of items with variables, guided forms with many field types, conditions, and permissions per template.
- [Documentation home](https://docs.templating.app/), reached through the listing's link `http://seibert.biz/templatingdoc`: the kinds of template it offers.
- [Issue templates](https://docs.templating.app/issue-templates/): several items made in one action.
- [Variables](https://docs.templating.app/issue-templates/basic-features/add-variables-to-issue-template/): the kinds of variable, defaults, and a variable used several times.
- [Linked items](https://docs.templating.app/issue-templates/basic-features/add-linked-issues/): links between the items of one template, to a parent, or chosen when applying.
- [Permissions](https://docs.templating.app/issue-templates/general/permissions/): three layers of permission, and restrictions per template where empty means no limit. No approval step is described.
- [Description templates](https://docs.templating.app/description-templates/) and [conditions](https://docs.templating.app/description-templates/features/define-conditions/): "all" or "any" of tests on field values and the user's groups.
- [Editing a template](https://docs.templating.app/issue-templates/basic-features/edit-issue-template/): no versions or history are described.

## Project generators and scaffolding

- Copier, [Configuring a template](https://copier.readthedocs.io/en/stable/configuring/): typed questions, defaults from earlier answers, validators, a condition that skips a question and does not record its answer, conditional files, and updating a project made from an older template version.
- Backstage, [Writing templates](https://backstage.io/docs/features/software-templates/writing-templates/): parameters as JSON Schema in steps, required fields, steps run in series with `if` and `each`, failure handling, and a dry run in the template editor.

## Document assembly

- docassemble, [Interview logic](https://docassemble.org/docs/logic.html): questions asked when a variable is needed, conditional text, and logic written in Python that authors must keep free of side effects.

## Template variables in other wikis and editors

- Notion, [Template buttons](https://www.notion.com/help/template-buttons), [Buttons](https://www.notion.com/help/buttons) and [Database buttons](https://www.notion.com/help/database-buttons): template values, buttons with several actions, and the editor right a button needs.
- Notion developers, [Creating pages from templates](https://developers.notion.com/guides/data-apis/creating-pages-from-templates.md): templates are pages; values such as today are filled in when one is used.
- MediaWiki, [Help:Extension:ParserFunctions](https://www.mediawiki.org/wiki/Help:Extension:ParserFunctions): `#if` and `#switch` as conditional text written in a template language.

## Form builders and safe rules

- SurveyJS, [Conditional logic](https://surveyjs.io/form-library/documentation/design-survey/conditional-logic): visibility, required and enabled rules on questions, pages and choices, evaluated again when an answer changes.
- JsonLogic, as described by [datalogic-rs](https://github.com/json-logic/datalogic-rs) and [go-jsonlogic](https://github.com/IsaacSec/go-jsonlogic): rules as plain JSON, evaluated without running code.
- Common Expression Language, [cel-spec](https://github.com/google/cel-spec) and [cel-go](https://github.com/google/cel-go): an expression language that is linear in time, free of side effects, and not Turing-complete, under Apache 2.0. It is kept for a later need (TPL-1201).

## Research on forms and wizards

- GOV.UK Design System, [Question pages](https://design-system.service.gov.uk/patterns/question-pages): start with one question per page.
- GOV.UK Service Manual, [Structuring forms](https://www.gov.uk/service-manual/design/form-structure): one thing per page, and research decides when to merge.
- GDS Design Notes, [One thing per page](https://designnotes.blog.gov.uk/2015/07/03/one-thing-per-page/) (2015).
- GDS User Research blog, [No more accordions: how to choose a form structure](https://userresearch.blog.gov.uk/2015/08/13/no-more-accordions-how-to-choose-a-form-structure/) (2015): simple question pages worked better than more complex structures.
- Baymard Institute, [Usability testing of inline form validation](https://baymard.com/blog/inline-form-validation) (2024): inline validation helps recovery, if it does not fire too early, and errors clear once fixed.
- Nielsen Norman Group, [Marking required fields in forms](https://www.nngroup.com/videos/required-fields-forms/), and its summary by TPGi, [Doing what's required](https://www.tpgi.com/doing-whats-required-indicating-mandatory-fields-in-an-accessible-way): mark required fields explicitly, and explain the marker.
- Nielsen Norman Group, [Wizards: definition and design recommendations](https://www.nngroup.com/articles/wizards/): wizards suit rare tasks and people new to a domain. Only parts of this page could be read; the rest came through summaries.
- Microsoft, [Wizards (Windows app design guidelines)](https://learn.microsoft.com/en-ie/Windows/Win32/uxguide/win-wizards): wizards are a heavy form of interface, and can hide what they change.

## Standards

- W3C, [Web Content Accessibility Guidelines 2.2](https://www.w3.org/TR/WCAG22/): 1.3.1, 2.1.1, 2.1.2, 2.4.3, 3.2.2, 3.3.1, 3.3.2, 3.3.3, 3.3.7 (redundant entry), 4.1.3.
- OWASP, [Cross Site Scripting Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross_Site_Scripting_Prevention_Cheat_Sheet.html): only safe URL schemes become links.

## Diffluence's own documents and code

- Blueprint 03, ORG-045 to ORG-061, PAG-004, PAG-008, PAG-009, SPC-006, BLK-039 to BLK-042.
- Blueprint 08, "Enterprise document features", and `08-roadmap/02-enterprise-document-needs.md` (RMP-014).
- Blueprint 09, document control, especially DOC-120, DOC-130, DOC-560, DOC-561 and DOC-1404.
- The decision log, ADR-028, ADR-034, ADR-038, ADR-041 and ADR-043.
- Specs 011 (wave D, section D4), 064 and 069.
- The design standard, `docs/design/README.md`.
- The code listed in 1.1 of [01-guided-templates.md](01-guided-templates.md).
