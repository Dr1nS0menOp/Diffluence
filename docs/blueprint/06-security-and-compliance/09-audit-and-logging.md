# 06.9: The audit log and other logs

Two different things. The **audit log** is a record of who did what, kept in the database, tamper-evident. The **application log** is what the process writes for operators.

## 9.1 What the audit log holds

**What can go wrong:** an action leaves no trace, a trace can be changed afterwards, the log holds more than it should, nobody can tell a token from a person.

- **SEC-800** · built · Every state change writes an audit event in the same transaction, so a change and its trace commit together. A refused change leaves none (ASVS 16.3.3).
- **SEC-801** · built · Logins are recorded with their result and method, and failed ones with a reason (ASVS 16.3.1).
- **SEC-802** · built · Refused access is recorded (ASVS 16.3.2).
- **SEC-803** · built · An event has the time in UTC, actor, action, object, space, result, address, user agent, request id and details (ASVS 16.2.1, 16.2.2).
- **SEC-804** · built · Page views are not audit events. Downloads are.
- **SEC-805** · partly · Changes record a short summary, not always before and after. Permission changes do. Settings record the new values only. Add the old value where it helps an investigation. Low.
- **SEC-806** · wave F · An event says which token acted (SEC-230). `on-behalf-of` for apps comes with phase 7.
- **SEC-807** · new · The earlier blueprint promised an event for every API call made with a token. Nothing records that: changes, refusals and downloads leave a trace, and other reads do not. A read through a stolen token would leave no audit event. Record reads by token, or change the promise. Medium. Belongs in wave H, with SEC-230.

## 9.2 Can it be trusted?

**What can go wrong:** someone with database access rewrites history, and nothing notices.

- **SEC-810** · built · Each event holds a hash of the one before it, so changing, removing or reordering a row breaks the chain. A verification walks the whole chain, including the copied columns.
- **SEC-811** · built · Triggers refuse update, delete and truncate, and the application role has no such rights, so even a bug in the app cannot change the log (ASVS 16.4.2).
- **SEC-812** · built · A daily job checks the chain, and records the result.
- **SEC-813** · partly · A daily job signs the head of the chain with Ed25519. A chain alone can be rewritten end to end by someone with the database, and only a signature stops that.
- **SEC-814** · built · **High.** Nothing checks those signatures. `VerifyCheckpoints` is only called from a test: not by the daily job, not by the admin route, not by `restore.sh` and not from the command line. The one defence against a full rewrite never runs. The daily job, the Verify button and the restore script must all call it, and a failure must reach a person (SEC-655). Belongs in the next commit that touches `internal/app`, with a mutation line.
- **SEC-815** · partly · Whoever has both the database and `master.key` can forge checkpoints too. Only a copy outside the instance stops that (SEC-833). The guide should say what the log can and cannot prove.
- **SEC-816** · partly · One lock keeps the chain in order, so all audited writes take turns. That is fine at wiki scale, and phase 3 measures it.

## 9.3 What it should not hold

**What can go wrong:** the log is immutable, so what goes in cannot come out, and it holds more personal data than it needs.

- **SEC-820** · built · **High.** A failed login stores the typed address in `details.email`, also when no account exists. People sometimes type a password in that field, which then stays in a log that cannot be changed. For an unknown account store a keyed hash of the address, which still shows credential stuffing. Only a known account gets its id. Belongs in wave F.
- **SEC-821** · new · **Medium.** Events hold personal data for good: e-mail addresses in `user.created`, page titles (which can be names), filenames, and the address and user agent of every action. Spec 013 accepts this until retention ends. That works, but the hash covers the whole payload, so nothing can be blanked for one person. A better shape for new events: identifying fields in a separate part that the hash covers only through a digest. That part can be removed later, and the chain still holds. Decision for the product owner, because it changes the format.
- **SEC-822** · partly · Titles of restricted pages in the audit log: see SEC-606.
- **SEC-823** · built · No passwords, tokens or document text go into audit details on purpose. SEC-820 is the one way a typed password can end up there by accident.
- **SEC-824** · built · CSV exports neutralise formulas.

## 9.4 Keeping and moving it

- **SEC-830** · wave F · Audit retention is a setting. A prune runs with the owner's role, verifies the chain, leaves a signed anchor and removes older rows, so the app still cannot delete (spec 013, D11).
- **SEC-831** · new · Retention needs a default. Without one the log grows forever and holds personal data forever, against the GDPR's storage limitation. Advice in chapter 10: 12 months for most events, longer only with a reason. Decision for the product owner.
- **SEC-832** · later · Sending events to a SIEM by syslog, CEF, GELF or Sentinel (ASVS 16.4.3 at Level 2). The product owner mentioned a SIEM in ADR-026. A SIEM is also the real answer to SEC-815, so this should move up to phase 3.
- **SEC-833** · new · Medium. Sending the signed head of the chain to something outside the instance, even a daily mail or a file an operator collects, makes a full rewrite visible. It is small, and could come with SEC-832.
- **SEC-834** · built · Audit retention is separate from content retention. Events hold ids as plain text, with no link to the page, so deleting a page never deletes its trail.

## 9.5 The application log

ASVS 16.1.1 asks for an inventory. This is it.

| What | Where | Holds | Does not hold | Kept |
|---|---|---|---|---|
| Request log | JSON on standard error | Method, path, status, duration, address, request id | Query strings, bodies, cookies, headers | By the operator |
| Errors | JSON on standard error | Message and request id | Tokens and passwords | By the operator |
| Panics | JSON on standard error | Stack trace | Request bodies | By the operator |
| Jobs | JSON on standard error | Job id, kind, error | Document text | By the operator |
| Audit verification | JSON and the audit log | First bad row | Personal data | Log: operator, audit: see above |

- **SEC-840** · built · Logs are structured JSON, so a line break in a value cannot fake a line (ASVS 16.2.4, 16.4.1).
- **SEC-841** · built · Query strings are left out, because they may hold tokens. See SEC-422 for tokens in paths.
- **SEC-842** · built · A panic becomes a 500 with a request id, and the stack stays in the log (ASVS 16.5.1, 16.5.4).
- **SEC-843** · partly · Addresses are personal data, and the log has them. The operator decides how long logs stay. The guide should advise a period. Low.
- **SEC-844** · built · A failed outside call or job is logged, and does not stop the rest (ASVS 16.3.4).
