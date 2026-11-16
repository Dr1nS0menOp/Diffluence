# Guided templates, for administrators

Guided templates (spec 074) let one template make a set of pages from a form, with conditions, rights per template and approval. They are an experimental part: off on a new installation and after an upgrade. This guide says how to turn them on, who may do what, and what approval does and does not check.

## Turning the part on and off

- **Administration, Organising, Experimental parts**, then turn on **Guided templates** (`guided_templates`, through `PUT /api/v1/admin/experimental`).
- It is refused while a node of an older release still runs (`older_release_running`). Finish the upgrade of every node first; a node that has not been seen for a minute no longer counts.
- Turning it **off** keeps every template, version, approval, decision and the origin of every page. Guided templates leave the gallery, and their routes answer `experimental_off`; plain templates work as before. Managers still list, change and delete every template.
- A template with a users list or chosen spaces counts as guided, so it is hidden while the part is off, never opened to everyone.
- Turning it **on** again brings everything back as it was.

## Who may do what

| Who | What |
|---|---|
| Whoever may create pages in a space | Uses the templates offered there, unless a template's users list leaves them out |
| Holders of **Propose templates** (`template.propose`) | Write templates in **My templates**, try them alone, and propose them. Every role with Create pages has it after the upgrade; take it away to narrow who writes templates |
| Holders of **Manage templates** (`space.templates`) | Change, hide, promote, export, import and delete the templates of the space, and approve or send back proposals, unless a template's editors list leaves them out |
| Whoever runs the space (`space.administer`) | Turns the four-eyes setting on and off, and may always clear a template's lists |
| Administrators | All of the above for templates of all spaces, which no editors list locks them out of |
| Whoever reads the audit of a space (`space.audit`) | Sees the template events of that space; the instance audit holds those of templates of all spaces |

A users list and an editors list only narrow: an empty list limits nothing, and being on a list gives no right the space does not give.

## The four-eyes setting

**Every template change needs a second person** is off by default, per space and once for templates of all spaces. While it is off, a manager's change is published at once. While it is on, every change waits as a draft until someone other than its author approves it, and the approved version stays in use meanwhile. Turning it on withdraws nothing. When a space has one manager of templates and the setting is on, nothing they write can be published until someone else may approve; the screen says so.

## What approval checks, and what it does not

Approval runs every check a save runs, again, against the installation as it is now:
- the shape and limits of the template, its texts and links between its pages;
- every question's settings, and that a suggested person is still active;
- every condition, and that the groups it tests still exist;
- every restriction, and that the people and groups it names still exist, and that nothing is still to be chosen again after an import.

Approval does **not** judge the words of a template: whether they are right, kind, or free of personal data and secrets. That is the approver's to judge, and the approval note says so (TPL-844).

## Export and import

- **Export** gives the version in use as one file (format 2). It holds the pages, texts, questions, steps, conditions, label rules and publishing at once, and the groups of restrictions by id and name with a count of the people left out. It never holds the users or editors list, chosen spaces, state, history, owner, reviewers, people, workflows, the usual place, or files.
- **Import** makes a draft of the importer, whoever imports it, which follows the approval rule of its scope. The people of a restriction are left out; when that empties a list, the importer stands in, so the page stays restricted. A group that is not here stays and matches nobody. Either way the page is marked to choose again, and no version is approved until its lists change. Tests of groups that are not here are false until other groups are chosen, and suggested people and pages are left out. The screen lists what was left out.
- A file from before guided templates imports as one page. The previous release refuses a file of format 2 whole.

## Automation

A rule's action **new page from a template** uses the version approved when the rule runs, with the rights of the rule's owner. A run that makes nothing records why: `template_asks` (a required question with no suggestion, which a rule cannot answer), `template_not_approved`, or `template_not_offered` (for example, the owner left the users list). The rule's test run warns of the same.

## Finding pages made from a template

The Details panel of a page says what it was made from. In search, `template:"Name"` finds the pages made from that template, among the pages the reader may see; a template the reader may not see finds nothing.

## Backups and restore

A full restore returns templates, versions, approvals and the origins of pages to the moment of the backup. A background set asked for after the backup is gone, with its job. A background set that waited at the backup runs after the restore and checks its version again, so a version withdrawn before the backup makes nothing. The restore drill (`scripts/restore-drill.sh`) proves both.

## What a later release may drop

A plain template saved by a release from before versions gets its next version on first read, and turning the part on is refused while such a release runs (TPL-1103). A later contract migration may drop both once no supported release writes templates without a manifest (TPL-1108).
