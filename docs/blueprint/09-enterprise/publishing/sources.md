# Sources of the study of draft and published spaces

All sources were consulted on **2026-11-14**, for behaviour and findings only. No text, workflow names, state names, icons or layouts were copied, and no product's source code was read (`docs/blueprint/02-clean-room-and-licensing.md`). "Read" means the page itself was opened; "through search" means only the search engine's summary of the page was available, because the page did not render in full.

## Standards and guidelines

- ISO: [ISO 9001:2015, Quality management systems, Requirements](https://www.iso.org/standard/62085.html), clause 7.5 "Documented information": 7.5.2 a (identification, including a version), 7.5.3.1 (available and protected), 7.5.3.2 (distribution, access, control of changes, retention, prevention of unintended use of obsolete information). As cited in blueprint 09.1.
- US FDA: [21 CFR Part 11](https://www.ecfr.gov/current/title-21/chapter-I/subchapter-A/part-11): §11.10(b) copies of records, §11.10(e) audit trails, §11.50(b) signature manifestations in human-readable forms, §11.70 signature and record linking. As cited in blueprint 09.1.
- W3C: [Web Content Accessibility Guidelines 2.2](https://www.w3.org/TR/WCAG22/): 1.3.1, 1.4.1, 2.1.1, 2.1.2, 2.4.3, 3.3.1, 4.1.3.

## The behaviour blueprint 08 names

The roadmap names a publishing app for a commercial wiki only to say which behaviour is meant. Its public documentation, read for behaviour:

- [Reduce the spread of misinformation](https://support.appfire.com/space/CPCL/646055614.md) (read): two spaces linked as draft source and published target in one site; the target is meant to start empty; separate version numbers; separate permissions per space; publishing through a workflow.
- [What is not published?](https://support.appfire.com/space/CPCL/646088074.md) (read): what is and is not copied (title, body, attachments, ordinary labels and content properties are; comments, page permissions, page status, personal marks, version history, the home page and blog posts are not); archived target pages block republishing; pages in folders go to the top.
- [Overview](https://support.appfire.com/space/CPCL/646120872.md) (read): first publish as version 1; hierarchy kept where possible, sibling order may differ; a publishing report per page in both spaces; publishing on a workflow's final state or events; one target per source.
- [Maintaining the page hierarchy](https://support.appfire.com/space/CPCL/646775362.md) (read): a page whose parent is missing goes to the top with a warning; moves in the source mark both sides out of sync; moves in the target are not undone by republishing; a page deleted in the source stays in the target marked removed; a page deleted in the target sets the source back to new.
- [Space publishing](https://appfire.atlassian.net/wiki/spaces/CPCL/pages/646578843/Space+publishing) (read, partly truncated): a space publish sends new and out-of-sync pages; it can be limited to pages in a workflow's final state; other publishing waits while it runs.
- [How Comala Publishing works](https://appfire.atlassian.net/wiki/spaces/AHP/pages/649954539) (read, truncated): ways to link a new empty target or start a new draft from a populated space; out of sync until published.
- [Getting Started with Comala Publishing](https://appfire.atlassian.net/wiki/spaces/AHP/pages/649856333) (read, truncated, and through search): per-page states new, out of sync, published.
- [Using publish and sync with the Comala Document Management family of apps](https://appfire.atlassian.net/wiki/spaces/AHP/pages/649561187) (read, truncated): publishing through a workflow trigger or the latest approved final state.
- [Publishing using a Comala workflow](https://appfire.atlassian.net/wiki/spaces/CPCL/pages/1091043351/Publishing+using+a+Comala+workflow) (read, truncated).
- [What is published?](https://appfire.atlassian.net/wiki/spaces/CPCL/pages/697469115/What+is+published) (read, truncated).
- [Publish and syncing a page with in-page links](https://appfire.atlassian.net/wiki/spaces/CPCL/pages/1286602822/Publish+and+syncing+a+page+with+in-page+links) (through search): links within the source space point to the target's pages; links to other spaces, outside sites and short links stay; card links are updated; embedded links to pages not yet published are updated when those are published.
- [Comala Publishing on the Atlassian Marketplace](https://marketplace.atlassian.com/apps/143/comala-publishing) (through search): the listing, for the scope of the product only.

## Other tools that keep draft and live content apart

- Drupal: [Drupal Workspaces FAQ](https://www.tag1.com/white-paper/drupal-workspaces-faq/), Tag1 (read): workspaces layered over live, published as a unit; an item changed in one workspace is locked for others; no automatic merging; public files reachable by address whatever the state.
- Drupal: [The Basics of Drupal Revisions and Content Moderation](https://www.lullabot.com/articles/basics-drupal-revisions-and-content-moderation), Lullabot (through search): a published default revision stays live while a newer draft revision waits.
- Plone: [How a folder's workflow state affects its content](https://5.docs.plone.org/working-with-content/collaboration-and-workflow/folder-workflow-state.html) (read): an item published inside a private folder is still found by search and opened by its address.
- Plone: [Basic Publication States](https://5.docs.plone.org/working-with-content/collaboration-and-workflow/basic-publication-states.html) (through search): private, pending and published.
- Adobe Experience Manager: [Publishing pages](https://experienceleague.adobe.com/en/docs/experience-manager-cloud-service/content/sites/authoring/sites-console/publishing-pages) (through search): a quick publish of one page with its unpublished references; a managed publication that lists references to untick, includes children and schedules; unpublishing does not unpublish references.
- Adobe Experience Manager: [Replication](https://experienceleague.adobe.com/en/docs/experience-manager-65-lts/content/implementing/deploying/configuring/replication) (through search): separate author and publish systems, activation copies content.
- Sanity: [Content Releases user guide](https://www.sanity.io/docs/user-guides/content-releases) and [Perspectives](https://www.sanity.io/docs/content-lake/perspectives) (through search): changes to many documents grouped, previewed and published together, on demand or scheduled.
- Strapi: [Draft & Publish](https://docs.strapi.io/cms/features/draft-and-publish) (through search): a draft and a published version per entry; the API returns the draft unless asked otherwise.
- XWiki: [Publication Workflow Application](https://extensions.xwiki.org/xwiki/bin/view/Extension/XWiki+Publication+Workflow+Application) (through search): a draft document and a published document; the published one changes only by publishing; unpublishing removes it; an open request to rewrite links between nested pages ([XAWORKFLOW-37](https://jira.xwiki.org/browse/XAWORKFLOW-37)).
- MediaWiki: [Help:FlaggedRevs](https://www.mediawiki.org/wiki/Help:FlaggedRevs) (through search): readers see the latest reviewed version; lists of pending changes and of pages never reviewed.
- Docs as code: [Docs as code](https://read-the-docs-website.readthedocs-hosted.com/docs-as-code), Read the Docs, and [Preview and publish](https://buildwithfern.com/learn/docs/preview-publish/overview), Fern (through search): a preview built for every change; production deployed by a deliberate step, sometimes through staging.

## Research

- C. Tran, K. Champion, B. M. Hill, R. Greenstadt, "The Risks, Benefits, and Consequences of Prepublication Moderation: Evidence from 17 Wikipedia Language Editions", Proc. ACM Hum.-Comput. Interact. 6 (CSCW2), 2022 ([record](https://dblp1.uni-trier.de/rec/journals/pacmhci/TranCHG22.html), [text](https://arxiv.org/abs/2202.05548)) (through search): prepublication review kept low-quality edits from readers very effectively, with at most moderate effects on the amount and quality of contributions, and some sign of discouraging newcomers.
- C. Tran, E. Take, K. Champion, B. M. Hill, R. Greenstadt, "Challenges in Restructuring Community-based Moderation", 2024 ([text](https://arxiv.org/abs/2402.17880)) (through search): support for the flagged-revisions system waned over time; its further deployment was halted.

## Diffluence

- Code read on 2026-11-14: `internal/pages/tree.go` (copy), `internal/pages/links.go`, `internal/document` (links), `internal/attachments/attachments.go` (`CopyAll`), `internal/sites/links.go`, `internal/features`, `internal/health/health.go`, migrations `0002`, `0051` and `0054`.
- Documents: blueprint 09.1 to 09.14 and their sources, spec 064 and the report of wave DC1, blueprints 02, 04.8 and 08, the decision log (ADR-009, ADR-010, ADR-025, ADR-034 to ADR-043), the design standard, `docs/operations/limits.md`, and spec 069.
