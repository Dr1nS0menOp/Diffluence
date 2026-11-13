// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A request that ends while it waits for a hash stops waiting, so a rush of sign-ins whose
// callers gave up does not keep the queue full.
func TestAWaitForAHashEndsWithItsRequest(t *testing.T) {
	for range cap(hashSlots) {
		hashSlots <- struct{}{}
	}
	defer func() {
		for range cap(hashSlots) {
			<-hashSlots
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// Each runs on its own, so a wait that does not end fails the test instead of hanging it;
	// the slots given back at the end let it finish.
	waits := map[string]func() error{
		"a new hash": func() error {
			_, err := HashPassword(ctx, "correct horse battery staple", fastParams)
			return err
		},
		"a check of a password": func() error {
			_, _, err := VerifyPassword(ctx, "$argon2id$v=19$m=8,t=1,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA", "anything at all", fastParams)
			return err
		},
	}
	for what, wait := range waits {
		done := make(chan error, 1)
		go func() { done <- wait() }()
		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("%s: %v", what, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s went on waiting past its request", what)
		}
	}
}

func TestNoMoreHashesRunAtOnceThanTheLimit(t *testing.T) {
	for range cap(hashSlots) {
		hashSlots <- struct{}{}
	}
	done := make(chan struct{})
	go func() {
		_, _ = HashPassword(context.Background(), "correct horse battery staple", DefaultParams)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("a hash ran while every slot was taken")
	case <-time.After(200 * time.Millisecond):
	}
	for range cap(hashSlots) {
		<-hashSlots
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the hash never ran after the slots were free")
	}
}
