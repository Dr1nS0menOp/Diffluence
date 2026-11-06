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

var accountWords = map[string]accountWord{
	identity.MailReset: {"A new password for %s",
		"Someone asked for a new password for this address on %s. This link works once, for an hour:\n\n%s\n\nDid you not ask for it? Then you can ignore this mail."},
	identity.MailInvitation: {"You are invited to %s",
		"You are invited to %s. This link lets you choose a name and a password, and works for 7 days:\n\n%s"},
	identity.MailSignup: {"Confirm your address for %s",
		"Someone signed up for %s with this address. This link finishes that, and works once:\n\n%s\n\nDid you not ask for it? Then you can ignore this mail."},
	identity.MailNewDevice: {"A new device logged in to %s",
		"A device we have not seen before logged in to your account on %s. Was that not you? Change your password now.%s"},
}

// AccountMessage writes a mail about an account. It carries no unsubscribe link: these
// mails come from something a person did, or are about their safety.
func AccountMessage(m identity.AccountMail, instance string) mail.Message {
	words := accountWords[m.Kind]
	text := fmt.Sprintf(words.body, instance, m.Link)
	return mail.Message{To: m.To, Subject: fmt.Sprintf(words.subject, instance), Text: text,
		HTML: "<pre>" + html.EscapeString(text) + "</pre>", Reason: "account"}
}
