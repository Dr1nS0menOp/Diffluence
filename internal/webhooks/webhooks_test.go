// SPDX-License-Identifier: Apache-2.0

package webhooks

import (
	"encoding/base64"
	"testing"
	"time"
)

// The example of the Standard Webhooks specification.
func TestSignatureMatchesTheExampleOfTheSpecification(t *testing.T) {
	secret, err := base64.StdEncoding.DecodeString("MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"test": 2432232314}`)
	got := Sign(secret, "msg_p5jXN8AQM9LWM0D4loKWxJek", time.Unix(1614265330, 0), body)
	if got != "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=" {
		t.Fatalf("got %s", got)
	}
}

func TestFiveAttemptsSpanADay(t *testing.T) {
	var total time.Duration
	for _, w := range waits {
		total += w
	}
	if len(waits) != maxAttempts-1 || total < 24*time.Hour || total > 25*time.Hour {
		t.Fatalf("%d waits over %s", len(waits), total)
	}
}
