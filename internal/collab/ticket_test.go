// SPDX-License-Identifier: Apache-2.0

package collab

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/keys"
)

func testKeyring(t *testing.T, fill byte) *keys.Keyring {
	t.Helper()
	k, err := keys.New(bytes.Repeat([]byte{fill}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return k
}

var issued = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

func sampleTicket() ticket {
	return ticket{SessionID: uuid.New(), UserID: uuid.New(), PageID: uuid.New(), Expires: issued.Add(ticketLifetime)}
}

func TestATicketOpensAsItWasSealed(t *testing.T) {
	k := testKeyring(t, 1)
	want := sampleTicket()
	sealed, err := sealTicket(k, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := openTicket(k, sealed, issued)
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != want.SessionID || got.UserID != want.UserID || got.PageID != want.PageID || !got.Expires.Equal(want.Expires) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestATicketPastItsExpiryIsRefused(t *testing.T) {
	k := testKeyring(t, 1)
	sealed, err := sealTicket(k, sampleTicket())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openTicket(k, sealed, issued.Add(ticketLifetime)); !errors.Is(err, errBadTicket) {
		t.Fatalf("at the expiry: %v", err)
	}
	if _, err := openTicket(k, sealed, issued.Add(ticketLifetime-time.Second)); err != nil {
		t.Fatalf("a second before the expiry: %v", err)
	}
}

func TestATicketWithAnotherPersonInItIsRefused(t *testing.T) {
	k := testKeyring(t, 1)
	original := sampleTicket()
	sealed, err := sealTicket(k, original)
	if err != nil {
		t.Fatal(err)
	}
	_, signature, _ := strings.Cut(sealed, ".")
	forged := original
	forged.UserID = uuid.New()
	payload := base64.RawURLEncoding.EncodeToString(forged.payload())
	if _, err := openTicket(k, payload+"."+signature, issued); !errors.Is(err, errBadTicket) {
		t.Fatalf("a changed person kept its signature: %v", err)
	}
}

func TestATicketFromAnotherKeyIsRefused(t *testing.T) {
	sealed, err := sealTicket(testKeyring(t, 2), sampleTicket())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openTicket(testKeyring(t, 1), sealed, issued); !errors.Is(err, errBadTicket) {
		t.Fatalf("got %v", err)
	}
}

func TestAMalformedTicketIsRefused(t *testing.T) {
	k := testKeyring(t, 1)
	sealed, err := sealTicket(k, sampleTicket())
	if err != nil {
		t.Fatal(err)
	}
	payload, signature, _ := strings.Cut(sealed, ".")
	unsigned := base64.RawURLEncoding.EncodeToString([]byte("v1|x|y|z|1"))
	for name, value := range map[string]string{
		"empty":             "",
		"no signature":      payload,
		"empty signature":   payload + ".",
		"signature only":    "." + signature,
		"not base64":        "!!!." + signature,
		"too long":          strings.Repeat("a", maxTicketBytes+1),
		"a cut signature":   payload + "." + signature[:len(signature)-2],
		"fields not parsed": unsigned + "." + signature,
	} {
		if _, err := openTicket(k, value, issued); !errors.Is(err, errBadTicket) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestTheFieldsOfASignedTicketAreChecked(t *testing.T) {
	k := testKeyring(t, 1)
	good := sampleTicket()
	for name, payload := range map[string]string{
		"old version":   "v0|" + good.SessionID.String() + "|" + good.UserID.String() + "|" + good.PageID.String() + "|1893456000",
		"three fields":  "v1|" + good.SessionID.String() + "|" + good.UserID.String(),
		"no session":    "v1|x|" + good.UserID.String() + "|" + good.PageID.String() + "|1893456000",
		"no person":     "v1|" + good.SessionID.String() + "|x|" + good.PageID.String() + "|1893456000",
		"no page":       "v1|" + good.SessionID.String() + "|" + good.UserID.String() + "|x|1893456000",
		"no expiry":     "v1|" + good.SessionID.String() + "|" + good.UserID.String() + "|" + good.PageID.String() + "|soon",
		"a sixth field": "v1|" + good.SessionID.String() + "|" + good.UserID.String() + "|" + good.PageID.String() + "|1893456000|x",
	} {
		// Signed with the right key, so only the reading of the fields can refuse it.
		mac, err := k.Fingerprint(ticketPurpose, []byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		raw := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac)
		if _, err := openTicket(k, raw, issued); !errors.Is(err, errBadTicket) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}
