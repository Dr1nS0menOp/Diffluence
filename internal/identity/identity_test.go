// SPDX-License-Identifier: Apache-2.0

package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/testutil"
)

var fast = identity.Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func setup(t *testing.T) (*identity.Service, *clock) {
	t.Helper()
	tdb := testutil.Database(t)
	clk := &clock{t: time.Now()}
	svc := identity.NewService(tdb.Pool, ids.New(), identity.Options{Params: fast,
		SessionIdleTimeout: time.Hour, SessionMaxAge: 8 * time.Hour, Now: clk.now})
	_, err := svc.CreateLocalUser(context.Background(), identity.NewUser{
		Email: "Ann@Example.com", DisplayName: "Ann", Password: "a long enough password",
		GlobalPermissions: []string{identity.PermUseProduct},
	}, identity.SystemActor("test"), identity.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	return svc, clk
}

func TestDuplicateEmailIsRejected(t *testing.T) {
	svc, _ := setup(t)
	_, err := svc.CreateLocalUser(context.Background(), identity.NewUser{
		Email: "ann@example.com", DisplayName: "Ann 2", Password: "another long password",
	}, identity.SystemActor("test"), identity.RequestMeta{})
	if !errors.Is(err, identity.ErrEmailTaken) {
		t.Fatalf("want ErrEmailTaken, got %v", err)
	}
}

func TestInvalidInput(t *testing.T) {
	svc, _ := setup(t)
	for _, in := range []identity.NewUser{
		{Email: "not-an-email", DisplayName: "X", Password: "a long enough password"},
		{Email: "x@example.com", DisplayName: "", Password: "a long enough password"},
		{Email: "x@example.com", DisplayName: "X", Password: "short"},
		{Email: "x@example.com", DisplayName: "X", Password: "a long enough password", GlobalPermissions: []string{"god_mode"}},
		{Email: "x@example.com", DisplayName: "X", Password: "a long enough password", Type: "service"},
	} {
		if _, err := svc.CreateLocalUser(context.Background(), in, identity.SystemActor("test"), identity.RequestMeta{}); err == nil {
			t.Errorf("accepted invalid input %+v", in)
		}
	}
}

func TestSessionLifecycle(t *testing.T) {
	svc, clk := setup(t)
	ctx := context.Background()
	token, p, err := svc.Login(ctx, "ann@example.com", "a long enough password", identity.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuthenticateSession(ctx, token); err != nil {
		t.Fatalf("fresh session: %v", err)
	}
	if _, err := svc.AuthenticateSession(ctx, token+"x"); !errors.Is(err, identity.ErrNoSession) {
		t.Fatalf("forged token: %v", err)
	}

	// Activity keeps the session alive...
	clk.t = clk.t.Add(50 * time.Minute)
	if _, err := svc.AuthenticateSession(ctx, token); err != nil {
		t.Fatalf("active session: %v", err)
	}
	// ...but inactivity beyond the idle timeout ends it.
	clk.t = clk.t.Add(61 * time.Minute)
	if _, err := svc.AuthenticateSession(ctx, token); !errors.Is(err, identity.ErrNoSession) {
		t.Fatalf("idle session: %v", err)
	}

	// Absolute maximum age, even when active.
	token, _, _ = svc.Login(ctx, "ann@example.com", "a long enough password", identity.RequestMeta{})
	for range 9 {
		clk.t = clk.t.Add(55 * time.Minute)
		_, err = svc.AuthenticateSession(ctx, token)
	}
	if !errors.Is(err, identity.ErrNoSession) {
		t.Fatalf("session older than max age: %v", err)
	}
	_ = p
}

func TestPasswordIsRehashedWhenParamsIncrease(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	weak := identity.NewService(tdb.Pool, ids.New(), identity.Options{Params: fast, SessionIdleTimeout: time.Hour, SessionMaxAge: time.Hour})
	if _, err := weak.CreateLocalUser(ctx, identity.NewUser{Email: "bo@example.com", DisplayName: "Bo",
		Password: "a long enough password"}, identity.SystemActor("test"), identity.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	stronger := fast
	stronger.Time = 2
	strong := identity.NewService(tdb.Pool, ids.New(), identity.Options{Params: stronger, SessionIdleTimeout: time.Hour, SessionMaxAge: time.Hour})
	if _, _, err := strong.Login(ctx, "bo@example.com", "a long enough password", identity.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	var hash string
	_ = tdb.Pool.QueryRow(ctx, "SELECT password_hash FROM local_credentials").Scan(&hash)
	if want := "$argon2id$v=19$m=64,t=2,p=1$"; hash[:len(want)] != want {
		t.Fatalf("hash not upgraded: %s", hash)
	}
}
