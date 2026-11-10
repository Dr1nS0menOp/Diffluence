// SPDX-License-Identifier: Apache-2.0

package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/testutil"
)

type sentMail struct{ mails []identity.AccountMail }

func (m *sentMail) Send(_ context.Context, mail identity.AccountMail) error {
	m.mails = append(m.mails, mail)
	return nil
}

// ADR-038, NIST SP 800-63B: 100 wrong passwords in a row stop password login, the stop
// outlives a restart, and the ways in that never use the password keep working.
func TestAHundredWrongPasswordsStopPasswordLoginAcrossARestart(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	options := identity.Options{Params: fast, SessionIdleTimeout: time.Hour, SessionMaxAge: 8 * time.Hour}
	svc := identity.NewService(tdb.Pool, ids.New(), options)
	mails := &sentMail{}
	svc.SetMailer(mails, "http://example.test")
	u, err := svc.CreateLocalUser(ctx, identity.NewUser{Email: "ann@example.com", DisplayName: "Ann",
		Password: "a long enough password", GlobalPermissions: []string{identity.PermUseProduct}},
		identity.SystemActor("test"), identity.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	for i := range identity.PasswordLockAfter {
		if _, _, err := svc.Login(ctx, "ann@example.com", "not the password", identity.RequestMeta{}); !errors.Is(err, identity.ErrInvalidCredentials) {
			t.Fatalf("wrong password %d: %v", i+1, err)
		}
		if i == identity.PasswordLockAfter-2 && len(mails.mails) != 0 {
			t.Fatal("told before the stop")
		}
	}
	if len(mails.mails) != 1 || mails.mails[0].Kind != identity.MailPasswordLocked || mails.mails[0].To != "ann@example.com" {
		t.Fatalf("mails: %+v", mails.mails)
	}

	restarted := identity.NewService(tdb.Pool, ids.New(), options)
	if _, _, err := restarted.Login(ctx, "ann@example.com", "a long enough password", identity.RequestMeta{}); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("the right password after a restart: %v, want the stop to hold", err)
	}
	// More wrong passwords after the stop write nothing and send nothing more.
	if _, _, err := restarted.Login(ctx, "ann@example.com", "not the password", identity.RequestMeta{}); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatal(err)
	}
	if len(mails.mails) != 1 {
		t.Fatalf("told again: %d mails", len(mails.mails))
	}

	// A passkey and the identity provider open a session through OpenSession, which the stop does not touch.
	err = pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error {
		_, _, err := restarted.OpenSession(ctx, tx, u.ID, "passkey", identity.RequestMeta{})
		return err
	})
	if err != nil {
		t.Fatalf("a passkey after the stop: %v", err)
	}
}
