// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"testing"
	"time"
)

func TestNoMoreHashesRunAtOnceThanTheLimit(t *testing.T) {
	for range cap(hashSlots) {
		hashSlots <- struct{}{}
	}
	done := make(chan struct{})
	go func() {
		_, _ = HashPassword("correct horse battery staple", DefaultParams)
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
