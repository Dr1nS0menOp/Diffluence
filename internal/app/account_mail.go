// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/mail"
	"github.com/diffluence/diffluence/internal/notify"
)

// accountMailContext binds sealed account mail to the queue.
const accountMailContext = "account mail"

// accountMailer puts account mail in the mail queue. A link is a secret, so the queue
// only ever holds it sealed, and drops it once the mail has left.
type accountMailer struct {
	keyring *keys.Keyring
}

// Send queues one mail about an account.
func (m accountMailer) Send(ctx context.Context, q db.Querier, a identity.AccountMail) error {
	body, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("account mail: %w", err)
	}
	sealed, err := m.keyring.Seal(body, accountMailContext)
	if err != nil {
		return fmt.Errorf("account mail: %w", err)
	}
	return mail.EnqueueAccount(ctx, q, sealed)
}

// renderMail writes what the queue sends. Account mail is written here, because only
// this package holds the key. The rest is up to notify.
func (a *App) renderMail(ctx context.Context, j mail.Job) (mail.Rendered, error) {
	if j.Kind != mail.KindAccount {
		return a.notify.Render(ctx, j)
	}
	plain, err := a.keyring.Open(j.Sealed, accountMailContext)
	if err != nil {
		return mail.Rendered{}, fmt.Errorf("account mail: %w", err)
	}
	var m identity.AccountMail
	if err := json.Unmarshal(plain, &m); err != nil {
		return mail.Rendered{}, fmt.Errorf("account mail: %w", err)
	}
	return mail.Rendered{Message: notify.AccountMessage(m, a.cfg.InstanceName)}, nil
}
