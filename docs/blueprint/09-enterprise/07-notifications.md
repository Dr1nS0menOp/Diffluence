# 09.7: Notifications and watching

How to read this chapter: see [01-document-control.md](01-document-control.md).

Review only works when people know they are needed, and readers only trust a procedure when they learn that it changed. At the same time a notification is a distribution channel: a mail about a working change sent to a reader leaks the change. This chapter decides who is told what, and when.

- **DOC-600** · new · **Reviewers** are notified when a round asks them, with the page, the change summary, the rule, the due date and whether a signature is needed. *Proof:* a test of the notification content.
- **DOC-601** · new · Reviewers are **reminded** before the due date and on it (DOC-230), and the escalation target is notified when added (DOC-231). Reminders stop the moment the reviewer answers or the round ends. *Proof:* tests with a fixed clock.
- **DOC-602** · new · **Authors** are notified of each answer (approve or changes, with the comment) and of the end of the round. *Proof:* a test.
- **DOC-603** · new · **Watchers** of a controlled page who are readers are notified only when a **new version is approved**, never for working changes (decided, DOC-1414). Watchers who are authors receive both. *Why:* readers are told about what they can see; telling them about a draft leaks it. *Edge cases:* a watcher who loses `page.edit` stops receiving working-change notifications at once. *Proof:* tests per kind of watcher; a mutation that sends a working change to a reader must be caught.
- **DOC-604** · new · A mail about a working version never contains its text or excerpt when sent to someone who may not see it; this is enforced by building every notification through the visibility rule of DOC-300, not by each notification kind. *Proof:* a test that renders every notification kind for a reader of a controlled page.
- **DOC-605** · new · **Digests** keep one item per controlled page per day, naming the newest approved version. *Proof:* a digest test.
- **DOC-606** · new · @mentions in a working version notify the mentioned person only if they may see the working version; otherwise the mention is held until the version is approved, and then delivered with the approved text (decided, DOC-1415). *Why:* a mention would leak the draft. *Proof:* a test.
- **DOC-607** · new · People can choose, per space, to be told about every approval in the space ("approvals digest"), which suits quality leads. *Proof:* a test.
- **DOC-608** · new · When a reviewer is removed, delegated or replaced by a stand-in, both sides are notified. *Proof:* a test.
- **DOC-609** · new · Notifications for document control use the existing mail and in-app channels and their rate limits; they add no new channel. *Why:* keep one system to run. *Proof:* no new sender in the code (review).
