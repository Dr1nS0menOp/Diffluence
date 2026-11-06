# Handoff 2026-11-06: invitations

Branch `invite`. See spec 022 for every line.

## What is built

- **Role:** global permission `invite_people`, held by nobody at first.
- **Asking:** a section in the Share dialog, for people with the role. `POST /pages/{id}/invitation-requests`. The module checks the role, and nothing goes out.
- **Answering:** under Invitations in the console, approve or refuse with a reason. The summary says how many wait. Approval, the invitation, the audit line, the notification and the queued mail are one transaction.
- **Recipient:** wave F's invitation, as a guest with view on the space of the page. The mail holds no title and no note.
- **Requester:** a notification of type `invitation_answer`, with the address, the outcome and the reason.

## Changed beyond the request

`POST /spaces/{id}/guests` let anyone who manages a space invite a guest at once, which breaks ADR-033. It now needs `invite_people` and goes through the same queue, unless the caller is an administrator. The route answers 202 then.

## For the product owner

- **Upgrade:** nobody holds `invite_people`, so space managers who invited guests need it first.
- **What is granted:** a whole space, as a guest with view. A page cannot be given on its own, because a restriction narrows a space and never widens it. The console says so before approval.
- **Mail about an answer:** it says that there is an answer. The outcome and the reason show in the notification in the app.

## Not proven

- Two administrators answering at the same moment: the row lock decides, and no test runs them in parallel.