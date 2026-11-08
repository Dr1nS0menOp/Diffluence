# When something went wrong

Written for the operator of an instance. Do the steps in this order, and write down what you did and when: you need that record afterwards.

## 1. Stop it from getting worse

- **One account is taken over:** run `diffluence session end --email address` to log that person out everywhere without disabling them, or disable the account in Administration, Users, which also stops its tokens. Then reset its password and factors.
- **Many accounts, or you do not know which:** run `diffluence session end --all`. Everyone logs in again, and nobody is disabled. Both are in the audit log.
- **The server itself is in doubt:** turn on Maintenance mode in the administration, so it only reads, and take a backup before changing anything.

## 2. Replace what may be known

| Secret | Where it lives | How to replace it |
|---|---|---|
| `master.key` | Data directory, or `DIFFLUENCE_MASTER_KEY_FILE` | `diffluence key add`, then `promote`, `reseal`, `check` and `retire`. See [backup and restore](backup-restore.md#replace-it). It runs while people work |
| Database passwords | `secrets/` | Change the role's password in PostgreSQL, update the file, restart |
| OIDC client secret | `DIFFLUENCE_OIDC_CLIENT_SECRET` | Make a new one at the provider, set it, restart, then remove the old one there |
| SCIM tokens | Administration, Provisioning | Make a new token, set it at the provider, revoke the old one |
| Personal API tokens | Each person's account | Revoke them in Administration, Users, or ask people to |
| Mail password | `DIFFLUENCE_SMTP_PASSWORD` | Change it at the mail server, set it, restart |
| Helper secrets | `DIFFLUENCE_PDF_PASSWORD`, `DIFFLUENCE_COLLAB_TOKEN` | Set a new value for both sides, restart both |
| Webhook secrets | Administration, Webhooks | Rotate per endpoint: both signatures go out for a day |
| Metrics token | `DIFFLUENCE_METRICS_TOKEN` | Set a new one for Diffluence and the scraper |

Replace a secret on a schedule as well, not only after an incident: at least once a year, and whenever someone who knew it leaves.

## 3. Find out what happened

- **The audit log** shows who did what, and its chain shows whether it was changed: run `diffluence audit verify`.
- **Refused requests and failed logins** are audit events too. Filter on them for the time in question.
- **The request log** of the server holds addresses and times, never tokens or passwords.

## 4. Restore, when data was damaged

Restore the last good backup, then run `diffluence key check --deep` to prove every file opens. Anonymise again the people who were anonymised after that backup. See [backup and restore](backup-restore.md).

## 5. Tell the people who must know

If personal data may have been seen by someone who should not have seen it, the organisation that runs the instance may have to report it to its data protection authority, often within 72 hours, and to the people concerned. That is the organisation's call, with its own advice; this guide gives no legal advice.

## Afterwards

Write down what happened, what it touched, how it was stopped and what changes now. If Diffluence itself was at fault, report it privately as [SECURITY.md](../../SECURITY.md) describes.
