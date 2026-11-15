# 03: Sources of the functional study

The provenance log that `docs/blueprint/02-clean-room-and-licensing.md` asks for. All sources are public help pages, read on 2026-11-05 for **behaviour only**, through a tool that returns a summary. No product was used, no account was logged in to, no source code was read, and no text, icon, template or layout was copied. Community and issue-tracker results in a search were not used as a basis for requirements.

Base address: `https://support.atlassian.com/confluence-cloud/docs/`, followed by:

**Editing:** `insert-elements-into-a-page`, `keyboard-shortcuts-markdown-and-autocomplete`, `format-text`, `create-and-manage-layouts`, `differences-using-confluence-legacy-editor-and-cloud-editor`, `transform-content-with-the-block-menu`, `how-to-use-the-new-toolbar-and-insert-elements`, `insert-links-and-anchors`, `simplify-data-with-tables`, `charts-and-connected-data`, `use-symbols-emojis-and-special-characters`, `insert-the-expand-macro`, `insert-the-code-block-macro`, `insert-the-panel-macro`, `make-your-page-and-its-title-more-memorable`.

**Files:** `display-files-and-images`, `manage-uploaded-files`.

**Pages:** `create-edit-and-publish-a-page`, `view-a-page`, `move-copy-and-hide-pages`, `archive-pages`, `delete-restore-or-purge-a-page`, `add-a-status-to-your-page-or-blog`, `request-and-manage-approvals-in-confluence`, `save-a-page-for-later`, `create-and-collaborate-in-real-time-with-live-docs`.

**Dynamic blocks:** `use-macros-to-show-confluence-content-on-pages`, `use-macros-to-add-navigation-on-confluence-pages`, `use-macros-to-add-visuals-and-images-to-confluence-pages`, `use-macros-to-include-media-on-confluence-pages`, `insert-the-table-of-contents-macro`, `insert-the-task-report-macro`, `insert-the-page-properties-macro`, `insert-the-page-properties-report-macro`, `insert-the-filter-by-label-macro`, `insert-the-excerpt-macro`, `insert-the-include-page-macro`, `insert-the-children-display-macro`, `insert-the-recently-updated-macro`, `insert-the-attachments-macro`, `insert-the-contributors-macro`.

**Working together:** `comment-on-pages-and-blog-posts`, `add-a-reaction-to-any-content`, `mention-a-person-or-team`, `add-action-items-and-mentions`, `share-a-page-or-blog-post`, `watch-pages-spaces-and-blogs`, `manage-watchers`, `view-your-notifications`, `subscribe-to-email-notifications`.

**Finding and organising:** `search-for-pages-and-posts`, `confluence-search-syntax`, `use-labels-to-organize-your-content`, `create-a-template`, `edit-a-template`, `create-a-global-template`, `organize-your-space`, `decide-how-you-see-pages-in-a-space`, `what-is-the-new-navigation-in-confluence`.

**Spaces and access:** `manage-your-content-tree`, `what-are-space-permissions`, `add-or-remove-page-restrictions`, `archive-a-space`, `delete-a-space`, `edit-features-for-a-space`, `customize-your-personal-space`, `configure-shortcut-links`, `manage-global-permissions`, `make-a-space-public`, `share-content-externally-with-public-links`, `invite-guests-for-external-collaboration`.

**Administration and exchange:** `view-the-audit-log`, `customize-a-sites-color-schemes`, `export-content-to-word-pdf-html-and-xml`, `import-content-into-confluence-cloud`.

**Analytics:** `view-insights-on-pages`, `view-insights-on-spaces`, `view-analytics-to-see-how-content-is-performing`, `turn-on-privacy-mode-for-analytics-data`.

**Automation:** `https://support.atlassian.com/cloud-automation/docs/actions-in-confluence-automation/`, `https://support.atlassian.com/cloud-automation/docs/triggers-in-confluence-automation/`.

## Enterprise document features in the roadmap

Read on 2026-11-06 for behaviour only, through a tool that returns a summary, to describe these features in `08-roadmap.md`. No requirement in chapters 1 to 13 comes from them. Each feature gets its own study and sources before it is built.

- **Document control:** the public Comala Document Control and Document Management pages at `https://appfire.atlassian.net/wiki/spaces/CDCCL/` and `https://appfire.atlassian.net/wiki/spaces/COMALACDCLS/`.
- **Draft and published spaces:** `https://appfire.atlassian.net/wiki/spaces/AHP/pages/649954539` and `https://marketplace.atlassian.com/apps/143/comala-publishing`.
- **Guided templates:** `https://marketplace.atlassian.com/apps/1224664/templating-app-project-templates-issue-templates-for-jira`.

## Pictures of another product's screens (ADR-045)

On 2026-11-13 and 2026-11-14 the chapters were checked against 154 pictures of the screens of a running instance of another wiki product, provided by the product owner, under the exception and the conditions in `02-clean-room-and-licensing.md` ("One exception"). The pictures show pages and the page tree, the editor and its catalogue of insertable blocks, the settings of a space, the administration of the instance, an organisation-level console of a vendor of several products, personal account settings, and an automation builder.

How the study ran: one analyst described every element of every picture in a run of eleven (regions, items in order, states, formatting as roles); a second analyst checked each run again with enlarged halves of every wide picture; a third compared each run with this blueprint and proposed lines; one writer per chapter turned the proposals into lines; a last pass removed duplicates across chapters and checked numbers, contradictions and the clean-room rules. The pictures, the inventories and the comparisons stay outside the repository. Every line the study added has the status **new** or **later**.

Screens of the organisation-level console that only a vendor of several products or organisations needs (billing, a switcher between products, proving a domain to the vendor, choosing where the vendor stores data, deleting the organisation) were left out, under ADR-008.

Where a picture showed only the name of an entry or setting, the writers read these public help pages on 2026-11-15, for behaviour only, through a tool that returns a summary:

- `https://support.atlassian.com/confluence-cloud/docs/` followed by `improved-confluence-navigation`, `configure-the-administrator-contact-page`, `configure-the-site-homepage`, `customize-a-sites-color-schemes`, `apply-watermarks` and `export-content-to-word-pdf-html-and-xml`.
- `https://support.atlassian.com/navigation/docs/what-is-the-new-navigation-in-confluence/`
- `https://confluence.atlassian.com/doc/code-block-macro-139390.html`
- `https://support.atlassian.com/cloud-automation/docs/what-is-the-automation-audit-log/`

A few web searches limited to those help sites were used to find these pages. Where a page gave a number, ours stays our own choice (for example the run log keeps 30 days, spec 015).

## What came from elsewhere

Names, numbers and defaults in chapters 1 to 13 are our own choices unless the behaviour itself required them. Standards are cited where used: WCAG 2.2, RFC 6238, OpenID Connect, SCIM 2.0, WebAuthn, CommonMark, iCalendar and WebDAV. The chapters on identity, permissions, audit and operations come from our own blueprint and specs.
