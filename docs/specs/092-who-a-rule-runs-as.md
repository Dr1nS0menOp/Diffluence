# 092: Who a rule runs as

Status: built
Phase: hardening before 1.0 (wave RV12)

## Problem

A rule acts as its account (spec 015 D10, PLT-023): the person who made it, or a service account an administrator chose. Everyone who may manage automation where a rule lives may open it. PLT-082 says that by default only the owner of a rule changes it, and PLT-083 that someone else who manages it can always switch it off, and can take it over. This spec builds that part, and settles two questions around it:

- What a test run looks at when the person testing is not the rule's account.
- What conditions mean when a run has no page to judge them on: a rule on a schedule, an incoming webhook, a group or a space trigger.

## Study

**What a rule lends.** The account of a rule is the set of rights its steps use: pages it can see, spaces it can move or copy into, restrictions it may set. Whoever chooses the steps of a rule chooses what is done with those rights. So the person who decides the steps must be the person whose rights they use, or an administrator who chose a service account for the rule.

**Who holds a rule.** The holder is the rule's account when that is a person. For a service account the holder is the owner who chose it, as long as that owner is still an administrator, because only an administrator may lend a service account (spec 015). The check compares both columns, `owner_id` and `account_id`, so a row where the two point at different people never makes the owner a holder of rights that are not theirs.

**What a manager still needs without taking over.** Switching a rule off at once (PLT-083), deleting it, reading it, its log and a test run. None of these lends anyone's rights to new steps. Switching a rule on does lend them, at a moment the holder did not choose, so it stays with the holder.

**Test runs.** A test run reads pages as if it ran (spec 015 D11): conditions read the page, a Find pages step lists what it would act on, and each step says whether it would be allowed. Run as the account, a test by someone else answers questions about pages that person may not see. A test is a question the caller asks, so it runs as the caller; only the holder tests as the rule's account, which for a service account is the point of the test.

**Conditions without a page.** Conditions look at a page: its labels, status, title, text, author, age, place. A schedule, an incoming webhook, a group or a space trigger starts a run without one. Until now those runs skipped the conditions and ran every step, so a rule meant for a few pages acted on every page a Find pages step returns, archive or delete included. Deny by default is the safe reading: a condition that cannot be judged does not hold.

**Old and new nodes side by side.** The rule is checked in the service of the node that takes the request. A node of the previous release keeps the account of a rule when a change names none, and never changes the owner, so it keeps what a take-over set; and it never runs a rule that is off, because every run reads `enabled` from the row when it starts. Such a node still accepts a change from someone who does not hold the rule, as before, until it is upgraded, and a change that names an account sets it to the person saving, as before. A node of this release saves a change only while the row still has the owner and account it read (`409 rule_changed` otherwise), so two saves at once never undo a take-over.

## Decision

Taken on the product owner's behalf on 16 November 2026, open to change:

- **D1.** Only the holder of a rule changes its name, trigger, conditions, actions, spaces or account, or switches it on (PLT-082, default "only the owner"). Anyone else gets `409 rule_held`. The choice of PLT-082 to let every manager change a rule is not built.
- **D2.** Without taking it over, someone else who may manage the rule may switch it off and may delete it, as before (PLT-083).
- **D3.** Someone who may manage a rule takes it over explicitly, with `take_over: true` on `PUT /api/v1/rules/{id}`. The rule then belongs to them and acts as them (or as a service account they may lend as an administrator), it is switched off, its incoming webhook address ends, the previous holder is told, and the audit log has `rule.taken_over` with who took it over, from whom, and when. The same request may carry the new steps. The reason of PLT-083 is not built.
- **D4.** A test run runs as the caller, unless the caller holds the rule; then it runs as the rule's account.
- **D5.** A condition needs a page. A run without a page whose rule has conditions does nothing, and its log says `conditions_need_page`. A rule with conditions under a trigger that brings no page has a problem on its conditions (PLT-077, PLT-078): it can be saved switched off and cannot be switched on. A Find pages step does not count: conditions are judged once, before any step.
- **D6.** Instance rules follow the same rules, among administrators.

## Behaviour

- `automation.Service.Update` loads the rule and works out whether the caller holds it. A holder changes it as before. Someone else who sends `take_over` takes it over (D3). Someone else who does not may only send the same name and definition with `enabled: false`; anything else is `409 rule_held`. A rule that is already off and is saved unchanged by someone else is a change of nothing, and passes.
- Taking over sets `owner_id` and `account_id` to the caller (or the chosen service account), `enabled` to false, ends the pause, deletes the row of `automation_hooks`, records `rule.taken_over` with `from_owner` and `from_account`, and leaves a notification of type `rule` with `what: taken_over` for the previous owner and the previous account, when they are people other than the caller. As other notifications of a rule, it hangs on the home page of the space; an instance rule has none, and shows the change on its own screen and in the audit log.
- `Test` runs as the caller unless the caller holds the rule. Find pages, the facts of conditions and the permission of each step all use the account of the run, not the column of the rule.
- `perform` and `Test` answer `conditions_need_page` for a rule with conditions and a run without a page, before any step runs.
- `Definition.Problems` adds the problem "Conditions look at a page, and this trigger brings none." on step -2 for a rule with conditions whose trigger brings no page.

## Load budget

A change of a rule by its holder reads one more row, the type of the account, only when the account is not the caller. A take-over writes the rule, deletes one hook row, and adds one audit entry, one outbox event and at most two notifications. Runs do one comparison more. No new query on a page view or a run.

## Acceptance criteria

- [x] AC-1 Someone who manages automation but does not hold a rule cannot change its steps, name or account, or switch it on: `409 rule_held`, and the rule is as it was.
- [x] AC-2 That person can switch the rule off and delete it without taking it over.
- [x] AC-3 A take-over makes the rule act as the person who took it over, switches it off, ends its webhook address, tells the previous holder and is audited with both.
- [x] AC-4 A test run by someone else reads pages as that person: a page only the holder may see is `not_visible`, and Find pages does not list it.
- [x] AC-5 Instance rules follow AC-1 among administrators.
- [x] AC-6 A rule with conditions under a schedule cannot be switched on and shows the problem on its conditions.
- [x] AC-7 A rule of that shape that is already on does nothing when it runs, and its log says `conditions_need_page`; a test run says the same.
- [x] AC-8 Mutations that drop the holder check, run a test as the rule's account, or skip conditions without a page are caught.

## How the product owner tests it

1. As a space administrator, make a rule in a space that labels commented pages, and switch it on.
2. Grant a second person Space administrator in that space. As them, open the rule and change the label: saving says the rule belongs to someone else.
3. As the second person, switch the rule off from the list: that works.
4. As the second person, send `PUT /api/v1/rules/{id}` with `take_over: true` and the rule as it is: the rule is now theirs and off, the first person has a notification, and the audit log has "rule.taken_over".
5. Make a rule on a schedule with a Find pages step and a condition: it can be saved only switched off, with a note on its conditions.

## Provenance

Own design, from spec 015 (D10, D11, H5), blueprint 03 chapter 12 (PLT-023, PLT-077, PLT-078, PLT-082, PLT-083).
