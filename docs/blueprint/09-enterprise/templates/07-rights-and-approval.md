# 09.T7: Rights per template, and approval

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

A template is a multiplier: whatever is in it ends up in every page made from it. Two questions follow:
- **who may use and change a template**, beyond the space's own rights;
- **who checks a template before others may use it**.

Templating.app answers the first with three layers:
- the tracker's project rights;
- app-wide permissions;
- restrictions per template on browsing, editing and applying, where an empty restriction limits nothing.

Its public documentation describes no approval step, so the second answer is Diffluence's own.

## 7.1 Rights per template

- **TPL-500** · new · A template has two optional **audience lists**, each of people and groups:
  - **Who may use it**: who sees the template in the gallery and may make pages from it;
  - **Who may edit it**: which of the template's managers (TPL-060) may change it.

  An empty list adds no limit, as page restrictions do today. *Why:* HR's onboarding template is for HR (TPL-018). A template that took a team weeks to agree should not be changed by every space administrator. *Proof:* matrix tests for each list, empty and set.
- **TPL-501** · new · Audience lists **only narrow** (decided, TPL-1415):
  - using a template still needs `page.create` in the target space;
  - editing it still needs `space.templates`, or administrator for templates of all spaces;
  - a person on the list without those rights gets nothing.

  *Why:* deny by default. A template must never become a way into a space or a way around a role. *Edge cases:*
  - an editors list that would leave nobody able to edit is refused when saved. The person setting it is always on it;
  - an administrator can always edit a template of all spaces, and a holder of `space.administer` can always clear the lists of a template in their space, so a template is never locked away for good. Both are audited.

  *Proof:*
  - a test that a listed person without `page.create` cannot use the template;
  - a test that a lists-only person cannot edit;
  - a test of the lock-out refusal.
- **TPL-502** · new · A template **of all spaces** is offered in **every space** or in **chosen spaces**, shown as chips (design standard, "A rule of the instance says where it acts"; decided, TPL-1424). Today it is offered everywhere, and that stays the default. *Edge cases:* a chosen space that is deleted drops off the list. A template whose list becomes empty is offered nowhere and says so in the administration. *Proof:* tests for each case.
- **TPL-503** · new · Every right is checked **again on submit**, inside the transaction that makes the set (TPL-120): the audience, `page.create`, the parent, restrictions and the workflow. *Why:* rights can change while a form is open. *Proof:* a test that removes a person from the audience between opening the form and submitting it.
- **TPL-504** · new · A person **outside** a template's audience does not see it anywhere: not in the gallery, not in a search of templates, not in the automation editor. A direct request answers "not found", never "forbidden". *Why:* a name can say more than people should know ("Dismissal procedure"). *Proof:* tests on each route.
- **TPL-505** · new · An **automation rule** that uses a template (`new_from_template`) uses it with the rights of its owner, as today, including the audience lists. A rule whose owner leaves the audience fails its next run with a clear message in the rule's history. *Proof:* a test.

## 7.2 Who writes templates

- **TPL-510** · new · A template has an **owner**: the person who made it. It has a **state**:

  | State | Who sees it | Offered in the gallery |
  |---|---|---|
  | Draft | The owner, and the managers once it was proposed | Only to the owner, marked "Draft", to try it |
  | Waiting | The owner and those who may approve it (TPL-520, TPL-521) | Only to the owner |
  | Approved | Everyone it is offered to | Yes |
  | Withdrawn | The owner and the managers | No |

  Today's templates are **Approved** (TPL-1101). *Why:* the owner must be able to test a draft for real before asking anyone to look at it. A test use makes real pages, as drafts of the owner only (TPL-150). *Proof:* a matrix test of who sees which state.
- **TPL-511** · new · People who may **manage** templates in a scope publish their own templates and changes **directly**, without approval: `space.templates` for a space, administrator for all spaces (decided, TPL-1411). A space can turn on **"every template change needs a second person"**, and so can the instance for templates of all spaces. Then every new template and every new version needs approval by someone other than its author. The setting is off by default (decided, TPL-1412). *Why:*
  - approval exists so that a template written by someone else is checked by someone responsible;
  - managers are that someone;
  - regulated teams that want four eyes on every change turn it on.

  *Edge cases:* turning the setting on does not withdraw approved templates; it applies to the next change. *Proof:* tests with the setting off and on, and a test that the author cannot approve their own version when it is on.
- **TPL-512** · new · People with `template.propose` (TPL-061), who do not manage templates, make **drafts** and **propose** them for the space where they made them. Only a manager or an administrator can widen a template to all spaces, by approving it there (TPL-521). *Proof:* tests that a proposer cannot approve, cannot publish directly, and cannot make a template of all spaces.

## 7.3 Approval

- **TPL-520** · new · A template **proposed for a space** is approved or sent back by someone with `space.templates` in that space, and on the template's editors list if it has one (TPL-500). The approver is never the author of the version. *Edge cases:* a space where nobody else holds `space.templates` still has its space administrators, who hold it by the split of wave PM; if the author is the only one, the proposal waits, and the screen says that nobody else can approve it. *Proof:* a matrix test.
- **TPL-521** · new · A template **proposed for all spaces**, or a template of a space proposed to be widened to all spaces, is approved by an **administrator** (instance permission `administrator`). This is the "administrator approves templates that others may use" of the roadmap. *Edge cases:* an installation with one administrator who is also the author can only publish directly (TPL-511), and with the four-eyes setting on cannot publish at all until a second administrator exists. The setting's screen says so. *Proof:* tests for both.
- **TPL-522** · new · Approving is **one decision by one person** (decided, TPL-1413):
  - **Approve**, with an optional comment;
  - **Send back**, with a required reason that the author sees.

  The decision is about **one version** (chapter 08): approving version 4 approves exactly that text, those fields and those conditions. *Why:* a template is not a controlled document (DOC-561), and one step needs no workflow. A space that wants a regulated procedure for its templates can keep templates as controlled pages and save them as templates after approval. *Edge cases:*
  - a version that changes after it was proposed is a new version and must be proposed again;
  - the decision screen shows the comparison with the approved version (TPL-602).

  *Proof:* a test that approving binds to the version number, and that a change after proposal withdraws the proposal.
- **TPL-523** · new · While a new version **waits**, the **approved** version stays offered and is the one people use (decided, TPL-1414). When the new version is approved, it replaces the old one for forms opened from then on. A form already open keeps its version (TPL-603). *Why:* users are never left without a template while its author improves it, the same idea as readers keeping the approved version in document control (DOC-300). *Proof:* a test that a form opened before approval makes pages from the version it showed.
- **TPL-524** · new · An approver can **withdraw** an approved template, with a reason. It leaves the gallery at once; forms already open are refused on submit with "this template was withdrawn". Pages made from it stay as they are (ORG-058). Withdrawn templates can be proposed again. *Proof:* tests.
- **TPL-525** · new · Approving and withdrawing **re-check** the template with the validators of chapters 03 to 06, and with the forbidden-content checks of chapter 10 (TPL-840). A template that no longer passes cannot be approved; the screen says what to fix. *Why:* the world changes under a template: groups are deleted, options removed, the space's fixed labels change. *Proof:* a test that approval is refused for a template naming a deleted group.

## 7.4 What approval does not do

- **TPL-530** · new · Approval checks the **template**, not the pages made from it. Those pages follow the space's rules, including document control when the space has it (DOC-560). *Proof:* nothing beyond TPL-151.
- **TPL-531** · new · The starter templates (ORG-053) ship approved. An administrator can hide them, as today (ORG-054). *Proof:* a test after the migration.
