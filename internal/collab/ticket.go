// SPDX-License-Identifier: Apache-2.0

package collab

import (
	"crypto/hmac"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/keys"
)

// TicketHeader carries a ticket from the API to the service, in place of the cookies of
// the browser, which the API takes off the socket (spec 067).
const TicketHeader = "X-Diffluence-Collab-Ticket"

// ticketLifetime is how long a ticket opens the service. The service asks again every
// minute and gets a new ticket each time, so a ticket outlives one missed recheck and no
// more.
const ticketLifetime = 2 * time.Minute

// ticketPurpose keeps the key that signs tickets apart from every other key derived from
// the key file.
const ticketPurpose = "collab ticket v1"

// ticketVersion starts every payload, so a later layout cannot be read as this one.
const ticketVersion = "v1"

// maxTicketBytes is far above a real ticket, and stops a large header before decoding.
const maxTicketBytes = 512

var errBadTicket = errors.New("collab: the ticket is not valid")

// ticket names a session, its person, one page and when it ends.
type ticket struct {
	SessionID uuid.UUID
	UserID    uuid.UUID
	PageID    uuid.UUID
	Expires   time.Time
}

func (t ticket) payload() []byte {
	return []byte(strings.Join([]string{ticketVersion, t.SessionID.String(), t.UserID.String(), t.PageID.String(),
		strconv.FormatInt(t.Expires.Unix(), 10)}, "|"))
}

// sealTicket signs a ticket with a key derived from the key file.
func sealTicket(k *keys.Keyring, t ticket) (string, error) {
	payload := t.payload()
	mac, err := k.Fingerprint(ticketPurpose, payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac), nil
}

// openTicket checks the signature and the expiry of a ticket and reads it. Every fault is
// errBadTicket: the holder learns nothing about which part failed.
func openTicket(k *keys.Keyring, raw string, now time.Time) (ticket, error) {
	if len(raw) > maxTicketBytes {
		return ticket{}, errBadTicket
	}
	encoded, signature, found := strings.Cut(raw, ".")
	if !found {
		return ticket{}, errBadTicket
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return ticket{}, errBadTicket
	}
	got, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return ticket{}, errBadTicket
	}
	want, err := k.Fingerprint(ticketPurpose, payload)
	if err != nil {
		return ticket{}, err
	}
	if !hmac.Equal(got, want) {
		return ticket{}, errBadTicket
	}
	t, err := parseTicket(string(payload))
	if err != nil || !now.Before(t.Expires) {
		return ticket{}, errBadTicket
	}
	return t, nil
}

func parseTicket(payload string) (ticket, error) {
	fields := strings.Split(payload, "|")
	if len(fields) != 5 || fields[0] != ticketVersion {
		return ticket{}, errBadTicket
	}
	var t ticket
	var err error
	if t.SessionID, err = uuid.Parse(fields[1]); err != nil {
		return ticket{}, errBadTicket
	}
	if t.UserID, err = uuid.Parse(fields[2]); err != nil {
		return ticket{}, errBadTicket
	}
	if t.PageID, err = uuid.Parse(fields[3]); err != nil {
		return ticket{}, errBadTicket
	}
	expires, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil {
		return ticket{}, errBadTicket
	}
	t.Expires = time.Unix(expires, 0)
	return t, nil
}
