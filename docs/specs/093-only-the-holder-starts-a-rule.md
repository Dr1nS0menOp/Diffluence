# 093: Only the holder starts a rule outside its trigger

Status: built
Phase: hardening before 1.0 (wave RV13)

## Problem

Spec 092 settled that the steps of a rule use the rights of its account, so only the holder of a rule (the person it acts as, or the administrator who chose its service account) changes it or switches it on. Two other requests also make a rule run with those rights at a moment its holder did not choose:

- `POST /api/v1/rules/{id}/hook` makes a new incoming webhook address for a rule, and ends the old one. Whoever calls the new address starts the rule.
- `POST /api/v1/rules/{id}/run` starts a rule by hand on a page. For a rule with a manual trigger that is what the rule is for; for any other trigger it was open to everyone who manages automation where the rule lives.

This spec decides who may make those requests, and looks for other ways to start a rule as its holder.

## Study

**An address is a key.** The incoming address is the only permission an outside system needs (spec 015): a 256-bit token, stored hashed. Making a new address hands out a key that starts the rule with its account's rights, and takes away the key its holder gave out. Neither is a choice for someone else to make for the holder. The body of an incoming call is not read, so a caller of the address chooses when the rule runs, not what its steps do.

**By hand.** A rule with a manual trigger is offered in the More menu of every page it reaches; its holder chose to offer it to everyone who can see those pages, and the run uses the account's rights within the steps the holder chose. A rule with another trigger was not offered: starting it by hand runs the holder's steps on a page of the caller's choosing, at a moment of the caller's choosing.

**Spaces of an instance rule.** The More menu lists an instance rule only on pages in the spaces its holder chose (`Definition.reaches`), and every event run checks the same. A request to `POST /api/v1/rules/{id}/run` did not: anyone who can see a page in another space could start a manual instance rule there.

**Other ways in, looked at.**

- Copying, duplicating, exporting or importing rules: none exists. Only `internal/automation` reads or writes the tables of rules (`internal/db/boundary_list_test.go`), and the routes of `handlers_rules.go` are its only callers.
- Changing the schedule, the trigger, the spaces or the account by another route: none. `next_run_at` is written only by a save of the holder, a take-over (which switches the rule off) and the scheduler.
- Switching a rule on after a take-over: only the new holder (spec 092 D1, D3).
- An address made before a take-over: the take-over ends it (spec 092 D3).
- Instance rules: the same service, among administrators (spec 092 D6).
- A test run: changes nothing, and runs as its caller unless the caller holds the rule (spec 092 D4).
- Delete and switch off: neither starts a run.

**Old and new nodes side by side.** The check is made by the node that takes the request. Until every node runs this release, a node of the previous release still makes a new address for someone who manages a rule without holding it, and still starts a rule of another trigger by hand for them, and still starts a manual instance rule on a page outside its spaces.

## Decision

Taken on the product owner's behalf on 16 November 2026, may be overruled:

- **D1.** Only the holder of a rule makes a new incoming webhook address for it. Anyone else who manages it gets `409 rule_held`, and the address its holder gave out keeps working. To give it an address of their own, they take the rule over first (spec 092 D3).
- **D2.** Only the holder starts a rule by hand when its trigger is not manual. Anyone else who manages it gets `409 rule_held` and nothing runs; someone who does not manage it gets `403`, as before.
- **D3.** A rule with a manual trigger stays open to everyone who can see the page, as spec 015 built it, and starts only on a page its holder chose to reach: an instance rule answers `404` on a page outside its spaces.
- **D4.** No other path was found that lets someone start a rule as its holder; nothing else changes.

## Behaviour

- `automation.Service.requireHolder` answers `409 rule_held` unless the caller holds the rule (`holds`, spec 092).
- `NewHook` checks `requireHolder` after `Get` (which checks the right to manage), before anything is written.
- `RunByHand` checks `mayManage` and then `requireHolder` for a rule whose trigger is not manual, and answers `404` when the rule does not reach the space of the page.

## Load budget

A new address or a run by hand of a non-manual rule reads at most one more row: the type of the account, only when the caller is the owner and not the account (`mayLend`). A run by hand compares the space with the rule's list in memory. Nothing changes on a page view, an event run or an incoming call.

## Acceptance criteria

- [x] AC-1 Someone who manages a rule but does not hold it cannot make a new incoming address for it: `409 rule_held`, and the holder's address still starts the rule.
- [x] AC-2 After taking the rule over, they can.
- [x] AC-3 Someone who manages a rule with a trigger other than manual but does not hold it cannot start it by hand: `409 rule_held`, and no run is recorded. The holder can.
- [x] AC-4 AC-3 holds among administrators for an instance rule.
- [x] AC-5 A manual instance rule cannot be started by hand on a page outside its spaces.
- [x] AC-6 Mutations that drop either holder check, let the holder check pass everyone, or drop the spaces check are caught.

## How the product owner tests it

1. As a space administrator, make a rule with an incoming webhook in a space, give it an address, and switch it on.
2. Grant a second person Space administrator in that space. As them, open the rule and choose to make a new address: it says the rule belongs to someone else. The first address still works.
3. As the second person, send `POST /api/v1/rules/{id}/run` with a page for a rule on comments: it says the rule belongs to someone else, and its log stays empty.
4. As the second person, take the rule over (spec 092); now making an address works.

## Provenance

Own design, from spec 015 (D10, H5) and spec 092; blueprint 03 chapter 12 (PLT-023, PLT-082, PLT-083).
