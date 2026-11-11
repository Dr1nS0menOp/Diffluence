// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"fmt"
	"html"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/mail"
)

// accountWord holds the words of one kind of account mail. Body has room for the
// instance name and, when there is one, the link.
type accountWord struct {
	subject string
	body    string
}

// The kinds of the backup mails, as package backups names them; spelled out here because
// backups depends on notify's sibling identity, not the other way round.
const (
	mailBackupFailed       = "backup_failed"
	mailRestoreCheckFailed = "restore_check_failed"
)

var accountWords = map[string]accountWord{
	identity.MailReset: {"A new password for %s",
		"Someone asked for a new password for this address on %s. This link works once, for an hour:\n\n%s\n\nDid you not ask for it? Then you can ignore this mail."},
	identity.MailInvitation: {"You are invited to %s",
		"You are invited to %s. This link lets you choose a name and a password, and works for 7 days:\n\n%s"},
	identity.MailSignup: {"Confirm your address for %s",
		"Someone signed up for %s with this address. This link finishes that, and works once:\n\n%s\n\nDid you not ask for it? Then you can ignore this mail."},
	identity.MailNewDevice: {"A new device logged in to %s",
		"A device we have not seen before logged in to your account on %s. Was that not you? Change your password now.%s"},
	identity.MailSignInChanged: {"Your sign-in changed on %s",
		"A way into your account on %s changed just now: a password, a second factor or a passkey. Was that not you? Ask an administrator to lock your account now.%s"},
	// The link of the two backup mails is the backup page of the console (spec 058).
	mailBackupFailed: {"A backup of %s failed",
		"The latest backup of %s failed, so the newest backup is older than planned. The backup page says which step failed:\n\n%s\n\nYou get this mail at most twice a day while backups keep failing."},
	mailRestoreCheckFailed: {"A test restore of %s failed",
		"The weekly test of %s restored its newest backup into a throw-away copy, and that failed. A backup that does not restore is no backup. The backup page has the result:\n\n%s"},
	identity.MailPasswordLocked: {"Password login to %s stopped",
		"Someone typed a wrong password for your account on %s 100 times in a row, so logging in with a password stopped. It may be someone guessing your password.\n\nA passkey and your organisation's login still work. To use your password again, choose a new one with \"Forgot your password?\" on the login page, or ask an administrator to lift the stop.%s"},
}

// AccountMessage writes a mail about an account. It carries no unsubscribe link: these
// mails come from something a person did, or are about their safety.
func AccountMessage(m identity.AccountMail, instance string) mail.Message {
	words := accountWords[m.Kind]
	text := fmt.Sprintf(words.body, instance, m.Link)
	return mail.Message{To: m.To, Subject: fmt.Sprintf(words.subject, instance), Text: text,
		HTML: "<pre>" + html.EscapeString(text) + "</pre>", Reason: "account"}
}
