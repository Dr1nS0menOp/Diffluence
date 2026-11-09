# 06.10: Privacy under the GDPR (AVG)

**This chapter is not legal advice.** It says what the software does, and points at the law that seems to apply. Where a line says *lawyer*, a lawyer or data protection officer must check it before anyone relies on it.

## 10.1 Who is who

- **The organisation that runs an instance is the controller.** It decides why people's data is processed. In most cases that is an employer.
- **The Diffluence project** writes software, and does not touch the data of a self-hosted instance. Recital 78 encourages producers of such software to build data protection in, so controllers can meet their duties. It is encouragement, not a duty on us. That is still the bar we hold ourselves to.
- **Whoever hosts Diffluence for someone else** is probably a processor under Article 28, with a contract, instructions, confidentiality, help with rights and with breaches, and deletion at the end. *Lawyer*, if the product owner ever offers hosting.

- **SEC-900** · built · No telemetry, no call home, no outside fonts or scripts, no third-party services by default. An instance works with no internet at all.
- **SEC-901** · new · `docs/compliance/` holds what a controller needs: this chapter, the record below, and the works council text. The blueprint promised it, and it does not exist yet. Belongs before 1.0.

## 10.2 The principles, and what serves them

Article 5 is the frame. Article 25 asks for data protection by design and by default, and by default data must not reach an indefinite number of people.

| Principle (Article 5) | In Diffluence |
|---|---|
| Lawfulness, fairness, transparency | Features that show who did what are visible to the people concerned. A person sees their own sessions and login history |
| Purpose limitation | Audit data is for security. It must not become a performance tool (10.7) |
| Data minimisation | Page views are not logged. Analytics start without names. Mail holds no text by default |
| Accuracy | People can correct names through an administrator or their provider |
| Storage limitation | See 10.4. **The weakest point today** |
| Integrity and confidentiality | Chapters 2 to 9 |
| Accountability | The audit log, this document, and the record below |

## 10.3 What is stored about people

This is the input for a controller's record of processing activities (Article 30). The lawful basis is the controller's to choose. For an employer it will usually be contract or legitimate interest (Article 6). *Lawyer.*

| Data | Where | Why | Who can see it | Today kept | Erasable? |
|---|---|---|---|---|---|
| Name, e-mail address, type, status | `users` | The account | Administrators, and others with *view profiles* | As long as the account | Wave F anonymises |
| Password hash, factors, passkeys | Credential tables | Login | Nobody can read them | As long as the account | Wave F removes |
| Sessions: address, user agent | `sessions` | Security, and the person's own list | The person | Until the session ends | Yes, by itself |
| Actions, address, user agent, sometimes address typed at login | `audit_events` | Security, proof | Administrators | **Forever** | **No**, by design |
| Authorship of versions, comments, files | Content tables | Who wrote what | Readers of that page | As long as the content | Name goes with anonymising |
| Text people wrote about others | Pages, comments | The purpose of a wiki | Readers of that page | As long as the content | By hand, by search |
| Mentions, tasks | Content and index tables | Working together | Readers, the person | As long as the content | Names resolve by id |
| Pages visited | `recent_pages`, last 50 | The person's own list | The person | Until pushed out | Goes with the account |
| Favourites, preferences, drafts | Own tables | The person's own use | The person | As long as the account | Goes with the account |
| Reactions, watches, follows | Wave C tables | Working together | Depends on the feature | As long as the account | Goes with the account |
| Notifications | Wave C | Working together | The person | 90 days | By itself |
| Mail queue | Wave C | Sending | Operators | See SEC-424 | Should go when sent |
| Reading figures | Wave H | Statistics | Those allowed | Counts: no person. Names: see 10.7 | Depends on the mode |
| Login history | Wave F | The person's own check | The person | Spec 013 sets it | Goes with the account |
| Application logs | Standard error | Operations | Operators | By the operator | By the operator |
| Backups | Operator's storage | Recovery | Operators | By the operator | Only when they expire |
| Photo metadata | Attachments | Nothing: it comes along | Readers | As long as the file | See SEC-334 |
| Recent searches, tree state | The browser | Convenience | The device | See SEC-265 | By the person |

- **SEC-905** · built · Special categories of data can be in any page: health, union membership, religion (Article 9). The software cannot know. The guide should tell a controller to think about that, above all for HR spaces, and page restrictions are the tool. Built: in `docs/operations/install.md` (spec 042).
- **SEC-906** · built · Cookies: one session cookie and one short-lived cookie for single sign-on. Both are needed for the service to work, so no consent banner is expected. Browser storage holds preferences. *Lawyer*, under the ePrivacy rules, before the guide says so.

## 10.4 How long

The default today is *forever* for most data. Storage limitation asks for better.

| Data | Default today | Advice |
|---|---|---|
| Audit events | Forever | 12 months, as a setting. Longer with a stated reason |
| Trash | 30 days, setting | Keep |
| Sessions | 8 hours idle, 7 days | See SEC-176 |
| Notifications | 90 days (planned) | Keep |
| Drafts | Forever | One year with a warning (PAG-019) |
| Accounts of people who left | Forever | A reminder to anonymise after a set time |
| Application logs | The operator's | Advise 30 to 90 days in the guide |
| Backups | The operator's | Advise a period, and say that erasure reaches them when they expire |

- **SEC-910** · built · Retention settings for audit events, with the anchor of SEC-830.
- **SEC-911** · built · **Medium.** A default for audit retention (SEC-831). Decision for the product owner. Built: see SEC-831.
- **SEC-912** · built · Old drafts are removed after a warning (PAG-019).
- **SEC-913** · new · A reminder for accounts that were disabled long ago, to hand over content and anonymise. Low. Belongs in wave F.

## 10.5 People's rights

The controller must answer within one month (Article 12). These are the tools the software gives.

| Right | Article | Tool | Status |
|---|---|---|---|
| Access, with a copy | 15 | Download my data | wave F |
| Correction | 16 | A name through an administrator. An address only through the provider | partly, see SEC-921 |
| Erasure | 17 | Anonymise | wave F |
| Restriction | 18 | Disable the account | built |
| Portability | 20 | Download my data, as JSON in a zip: structured, common, machine-readable | wave F |
| Objection | 21 | Decided by the controller, with name mode off by default | built |

- **SEC-915** · wave F · The download covers profile, preferences, groups, sessions, login history, own comments and reactions, and titles of own pages they can still see. A copy must not harm the rights of others (Article 15(4)), so it leaves out what others wrote.
- **SEC-916** · new · The download should also hold the person's own audit events, because Article 15 covers them, or the guide should say how an administrator gives them. Spec 013's list leaves them out. *Lawyer*, on how far this goes. Belongs in wave F.
- **SEC-917** · built · Anonymising replaces name and address, and removes profile, picture, sessions, tokens, passkeys, history, links, follows and preferences. A test walks every column of every table. The dialog says what it cannot reach: audit rows, backups, and names typed in text.
- **SEC-918** · partly · Article 17(3) allows keeping data needed for legal claims, or under a legal obligation. Whether audit rows fit that depends on the controller. Spec 013 relies on it (D10), so a controller needs to know that this is a choice. *Lawyer.* SEC-821 would make it easier.
- **SEC-919** · new · Names typed in pages and comments are not found by anonymising. Search finds them, and the dialog should offer a search for the person's name, so an administrator can look. Low. Belongs in wave F.
- **SEC-920** · built · After a restore from a backup, anonymising must be repeated. A small list of what was anonymised, holding only ids, would make that safe and quick. Spec 013 only says the guide will mention it.
- **SEC-921** · new · The address of a local account cannot be corrected by anyone, though Article 16 gives a right to correction. See SEC-192. Belongs in wave F.

## 10.6 Breaches

A breach must usually reach the supervisory authority within 72 hours (Article 33), with what happened, how many people, the likely effects and what was done. That is the controller's duty. The software should make it possible.

- **SEC-925** · built · The audit log shows who did what, and when.
- **SEC-926** · new · "Who could see this page on that date?" is the first question after a leak. The audit log has the changes, but no tool reads them that way. Medium. Belongs after wave F.
- **SEC-927** · new · The incident runbook (SEC-657) lists what a controller needs for a notification. *Lawyer*, for the text.

## 10.7 Watching colleagues, and the works council

Article 88 lets member states set rules for employment. In the Netherlands, an employer needs the works council's consent for a scheme on processing personal data of staff, and for systems aimed at or **suitable for** monitoring presence, behaviour or performance (Wet op de ondernemingsraden, article 27(1), points k and l). "Suitable for" is wide. *Lawyer.*

Features in Diffluence that are suitable for monitoring:

| Feature | What it could show | Guard |
|---|---|---|
| Audit log | Who did what, and when | Only administrators, and reading it is audited |
| Login history | When someone worked | Only the person |
| Analytics with names | Who read what | Off by default, and the setting names the risk |
| Presence | Who is on a page now | Administrators can turn it off (PAG-120) |
| Contributors, last edited | Who works how much | Normal for a wiki, and only per page |
| Task lists | What someone did not finish | Only on pages one may see |

- **SEC-930** · built · Page views are not in the audit log, and reading is not reported per person by default (deliberate difference 7).
- **SEC-931** · built · Name mode stays off by default, by the product owner's choice. A setting that turns it on shows a text about consent and works councils, and leaves an audit event.
- **SEC-932** · new · A text a controller can give their works council: which data, who sees it, which settings, and what is not done. Belongs in `docs/compliance/`. *Lawyer.*
- **SEC-933** · new · A DPIA (Article 35) may be needed for name mode, or for large HR spaces. Give a starting text in `docs/compliance/`. *Lawyer.*
- **SEC-934** · built · No report ranks people. Keep it that way: no "most active" lists with names.

## 10.8 Data leaving the instance

- **SEC-940** · built · Nothing leaves by itself. Mail, webhooks, a provider and public sites are each set up by the operator.
- **SEC-941** · new · A mail provider or a hosted identity provider outside the EU is an international transfer. That is the controller's call, and the guide should mention it. *Lawyer.*
- **SEC-942** · built · Public sites leave author names out unless chosen, and strip metadata from photos.
- **SEC-943** · built · A bundle carries personal data. It needs *administer*, is audited, and lives 24 hours. Built: see SEC-363. Making a bundle is audited.