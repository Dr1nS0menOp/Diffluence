# 09.T9: Search, audit and notifications

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

## 9.1 Search

- **TPL-700** · built · Templates are **never indexed** in the page search (spec 011), and neither is the text of any entry. A word that exists only in a template never finds anything in search. This stays. *Proof:* the existing test, extended to every entry of a set and to `conditional` sections.
- **TPL-701** · new · The **gallery** searches templates by name, description and category, and by the names of the entries. It searches on the server when a space offers more than 100 templates, and in the browser below that, as today. It only ever returns templates the person may use (TPL-504). *Why:* an instance with many teams can have hundreds of templates; the browser list was built for tens. *Proof:* a test with 150 templates and an audience list; a test that the server search never returns a template outside the audience.
- **TPL-702** · new · Pages made from a set are indexed like any page, when they are published, and never while they are the creator's drafts. *Proof:* covered by the existing draft tests; one test for a set.
- **TPL-703** · new · The search box offers a filter **made from template** (`template:` in the search syntax), which finds pages by their origin (TPL-610) among pages the reader may see. *Why:* "all incident reviews" should not depend on everyone using the same label. *Edge cases:* a reader who may not see the template cannot use its name as a filter; the filter by an unknown or hidden name finds nothing and gives no error (ORG-032). *Proof:* tests for both.

## 9.2 Audit

Each event goes through the existing audit log: written to `audit_pending` in the same transaction as the change, then sealed into the chain (ADR-041). No event holds an answer's value (TPL-1409).

- **TPL-710** · built · `template.created`, `template.updated` and `template.deleted` exist today. *Proof:* existing tests.
- **TPL-711** · new · New events:

  | Event | When | Details kept |
  |---|---|---|
  | `template.version_saved` | A new version is saved | Version number, the readable change list (TPL-602) |
  | `template.proposed` | A version is proposed | Version, scope |
  | `template.approved` | A version is approved | Version, approver, comment |
  | `template.sent_back` | A version is sent back | Version, reason |
  | `template.withdrawn` | An approved template is withdrawn | Version, reason |
  | `template.rights_changed` | An audience list or the spaces offered change | Before and after, as ids |
  | `template.setting_changed` | "Every change needs a second person" is turned on or off | Scope, new value |
  | `template.set_created` | A set is made | Template, version, space, root page, every page id, the field names answered (not the values), whether published |
  | `template.set_failed` | A background set fails (TPL-121) | Template, version, space, the cause in words without answers |

  *Why:*
  - approval is a person's decision about what many others will receive, so it must be on record;
  - the set event ties the pages together for an auditor.

  *Edge cases:* each page of a set also writes the events a page made from a template writes today. A set of 100 pages therefore writes 101 events or more, all sealed by the normal sealer (budget in TPL-903). *Proof:* an audit test per event; a test that no answer value appears in any event of a set.
- **TPL-712** · new · `space.audit` ("See the audit") shows the template events of its space, and the instance audit shows those of templates of all spaces. *Proof:* matrix cases.

## 9.3 Notifications

Notifications go through the existing outbox, so a failing mail sender never blocks a set or an approval (as DOC-1252).

- **TPL-720** · new · When a set is made **as drafts**, nobody is told except the creator: on screen, or by the bell for a background set (TPL-121). When it is **published**, by being made with "publish at once" or by "Publish all" (TPL-150), the watchers of the parent page and of the space get **one** notification for the set: "*Person* added *Root title* and *n* pages below it". They do not get one notification per page. *Why:* a hundred notifications for one action is noise that teaches people to ignore the bell. *Edge cases:*
  - a watcher who may not see some pages of the set is told only the number of pages they may see, and is not told at all if they may see none;
  - in a space under document control, the readers' rule of DOC-603 applies.

  *Proof:* tests of one notification per set, and of a watcher with partial access.
- **TPL-721** · new · People **mentioned** through person fields are told as for any mention: when the page is published, not while it is a draft. One person mentioned in several pages of one set gets **one** notification that lists the pages. *Proof:* a test with a person mentioned in three pages.
- **TPL-722** · new · Approval notifications:
  - a proposal tells the people who may approve it (TPL-520, TPL-521), by the bell and by mail according to their preferences;
  - a decision tells the author, with the reason when sent back;
  - a withdrawal tells the author and the managers of the template's scope.

  *Edge cases:* a proposal to a scope with many approvers tells each once, and the first decision closes it for all. *Proof:* tests for each, and a test that two approvers deciding at the same moment produce one decision (TPL-855).
- **TPL-723** · new · No notification and no mail holds an answer's value, except the page titles that people who may see the pages receive. *Why:* answers can hold personal data, and mail leaves the installation. *Proof:* a test that renders each notification kind for a set whose answers are marked.
