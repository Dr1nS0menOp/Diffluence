// SPDX-License-Identifier: Apache-2.0

// Package mail sends mail in the background (spec 010, C11): a queue in PostgreSQL,
// a composer for the message, and SMTP through the standard library. A slow or broken
// mail server never touches a request. What a mail says is decided when it is sent,
// by a Renderer, so a person who lost access gets nothing.
package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"
)

// Message is one mail to one person.
type Message struct {
	To          string
	Subject     string
	Text        string
	HTML        string
	Reason      string // why it came, so people can filter on it
	Unsubscribe string // the HTTPS address of RFC 8058, empty for a test mail
}

// oneLine keeps a value on its own header line: a title with a line break must not
// be able to add a header.
func oneLine(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == '\r' || r == '\n' || r == 0 }), " ")
}

func encoded(s string) string { return mime.QEncoding.Encode("utf-8", oneLine(s)) }

func writeBody(b *bytes.Buffer, contentType, body string) error {
	fmt.Fprintf(b, "Content-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", contentType)
	w := quotedprintable.NewWriter(b)
	if _, err := w.Write([]byte(body)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	b.WriteString("\r\n")
	return nil
}

func boundary() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "diffluence-" + hex.EncodeToString(raw), nil
}

// Compose builds the bytes of a message.
func Compose(from, replyTo, listID string, m Message, at time.Time) ([]byte, error) {
	sender, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("mail: the from address is not valid: %v", err)
	}
	to, err := mail.ParseAddress(oneLine(m.To))
	if err != nil {
		return nil, fmt.Errorf("mail: the address of a person is not valid")
	}
	mark, err := boundary()
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	header := func(name, value string) { fmt.Fprintf(&b, "%s: %s\r\n", name, value) }
	header("From", sender.String())
	header("To", to.String())
	if replyTo != "" {
		header("Reply-To", oneLine(replyTo))
	}
	header("Subject", encoded(m.Subject))
	header("Date", at.Format(time.RFC1123Z))
	header("MIME-Version", "1.0")
	// RFC 3834: stops auto-replies and mail loops.
	header("Auto-Submitted", "auto-generated")
	header("List-Id", oneLine(listID))
	if m.Reason != "" {
		header("X-Diffluence-Reason", oneLine(m.Reason))
	}
	if m.Unsubscribe != "" {
		header("List-Unsubscribe", "<"+oneLine(m.Unsubscribe)+">")
		header("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", mark)
	fmt.Fprintf(&b, "--%s\r\n", mark)
	if err := writeBody(&b, "text/plain", m.Text); err != nil {
		return nil, err
	}
	fmt.Fprintf(&b, "--%s\r\n", mark)
	if err := writeBody(&b, "text/html", m.HTML); err != nil {
		return nil, err
	}
	fmt.Fprintf(&b, "--%s--\r\n", mark)
	return b.Bytes(), nil
}
