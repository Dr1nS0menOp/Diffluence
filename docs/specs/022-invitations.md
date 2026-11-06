# 022: Inviting someone without an account

Status: built on branch `invite`, waiting for the product owner's acceptance session
Phase: COL-047, as the product owner decided in ADR-033. Spec 010 left this line for wave F's invitations.

## Problem

A person who shares a page can only reach people who have an account. ADR-033 opens that door with two locks: inviting needs a role that allows it, and an administrator approves every invitation. An ordinary member can never bring in an outsider alone.

## What the study found

**The flow exists.** Wave F has `identity.Invite`, a one-time link, mail through the queue, and a guest type that carries one space. `AcceptInvitation` grants that space in the same transaction as the account. Nothing new is needed after approval.

**A page cannot be given on its own.** A restriction narrows a space and never widens it, so someone who reads a page needs view in its space. What an approved invitation grants is what a guest invitation already grants: a guest account with view on that one space. A restricted page stays closed to them. The console says this before the administrator approves.

**One door was already open.** `POST /spaces/{id}/guests` lets anyone who manages a space invite a guest, at once. Making a space is an ordinary member's right, so that route breaks ADR-033 and has to change with this work.

**Administrators hold no content (PRM-004, ADR-029).** A request is about a page the administrator may not be able to read, so the console shows the space and never the title.

**Addresses of outsiders.** Someone who was never asked has an address in our database. It is needed while the request waits, and not after.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | New global permission `invite_people`, held by nobody by default. It can be given to a person or a group, like every global permission | "A role that allows it". A group is the role |
| D2 | A request is a row in `invitation_requests`. Nothing goes out when it is made | Approval is a state, not a promise |
| D3 | The module checks both locks, so the routes are `object` and `authenticated`, as PRM-039 asks. A refusal is audited as `access.denied` | No caller can skip them |
| D4 | Approve and refuse both need a reason, up to 500 characters. The requester reads it | The product owner's request |
| D5 | Approval, the invitation, the audit line, the notification and the queued mail share one transaction. If mail cannot be queued, the request keeps waiting | A request is never approved with nothing sent |
| D6 | A request can be answered once. Two administrators at once get one answer and one conflict | A refusal stays a refusal |
| D7 | Approval fails when the requester no longer holds the permission, or when invitations are off. The administrator can still refuse | The role is checked when it counts, not only when asked |
| D8 | An address that already has an account is not refused when asked, only when approved | Otherwise the Share dialog tells a requester who has an account |
| D9 | The address is erased from the row when the request is answered. The audit log never holds it | Nothing is kept about an outsider without a reason |
| D10 | The mail is wave F's invitation. The requester's note goes to the administrator only | The recipient gets an invitation, never the content |
| D11 | `POST /spaces/{id}/guests` goes through the same queue for anyone but an administrator, and asks for the home page of the space as its page. It answers 202 | One door, the same two locks. An administrator is the one who approves, so they invite directly |
| D12 | The answer is a notification of the new type `invitation_answer`, delivered in the same transaction, with the address, the outcome and the reason. People can choose its channel like `access_answer` | The event carries ids only, and the words have to reach the requester |
| D13 | Answers on one page are not grouped | Each has its own address and reason |

## User stories

- As someone with the role, I ask for an invitation for an address from the Share dialog, and hear the answer.
- As an ordinary member, I see no such option.
- As an administrator, I see what waits in the console and in the summary, and approve or refuse with a reason.
- As the outsider, I get an invitation, and after making an account I see that one space as a guest.

## What it does

1. In the Share dialog, someone with `invite_people` sees "Invite someone without an account", fills in an address and a note, and sends it. The dialog says it waits for an administrator.
2. The summary of the console says how many wait. Under Invitations each one shows the address, who asked, the space and the note, and says what approving gives.
3. The administrator writes a reason and approves or refuses.
4. On approval the invitation goes out by mail. Without mail the console shows the link, as it does for its own invitations.
5. The requester gets a notification with the answer and the reason.

## Permissions and audit

| Route | Access | Does |
|---|---|---|
| `POST /pages/{id}/invitation-requests` | object `page.view`, and `invite_people` in the module | Ask |
| `POST /spaces/{id}/guests` | object, manage access, then as above unless administrator | Ask for a guest in this space |
| `GET /invitation-requests` | session, `administrator` in the module | What waits |
| `POST /invitation-requests/{id}/approve` | session, `administrator` in the module | Approve, with a reason |
| `POST /invitation-requests/{id}/refuse` | session, `administrator` in the module | Refuse, with a reason |

Audit and outbox events: `invitation.requested`, `invitation.approved`, `invitation.refused`, wave F's `invitation.created`, and `access.denied` for a refusal. They hold ids, never the address. The reason stays in the row, like a share message stays out of the log.

## Acceptance criteria

- [x] A member without `invite_people` gets 403 from the server, and the refusal is audited. The dialog shows no option
- [x] A member who gets the permission through a group can ask
- [x] A request for a page the person cannot see answers 404
- [x] Asking sends nothing, and creates no invitation
- [x] A space manager without the permission cannot invite a guest, and with it only asks
- [x] A non-administrator cannot list, approve or refuse, even the one who asked
- [x] Approval and refusal without a reason are refused
- [x] Approval sends one invitation through the queue. The mail holds no page title and no note
- [x] Refusal sends nothing, and a refused request cannot be approved after all
- [x] The recipient sees the space that was granted and no other, and no restricted page
- [x] Approval fails when the requester lost the permission, when invitations are off, and when the address has an account
- [x] The requester hears the answer and the reason
- [x] The console shows no page title
- [x] The address is gone from the row after the answer, and is never in the audit log
- [x] Anonymising a requester removes their requests

## How the product owner tests it

1. Give a person `invite_people` under Users or Groups.
2. As that person, open a page, choose Share, and ask for an invitation for an address of yours.
3. As an ordinary member, open the same dialog: the option is not there.
4. As administrator, open Invitations, write a reason, and approve. Check the mail: it holds a link and no content.
5. Make the account: you see that space as a guest, and nothing else.
6. As the person who asked, open the notifications and read the answer. Try a refusal too.

## Left out

- The answer does not reach someone who can no longer see the page, because a notification hangs on its page (COL-072).
- A request is removed with its page. Nobody hears about that.
- If a page moves, the request keeps the space it was made for: what the console showed is what is granted.

## Provenance

Own design, from ADR-033 and wave F's invitations.
