// SPDX-License-Identifier: Apache-2.0

package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"time"

	"github.com/diffluence/diffluence/internal/config"
)

const smtpTimeout = 30 * time.Second

// Sender delivers one message.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// SMTP delivers through a mail server. net/smtp's SendMail only moves to TLS "if
// possible", so this drives the client by hand: with STARTTLS configured, a server
// that does not offer it gets nothing (spec 010, D13).
type SMTP struct {
	Config    config.SMTP
	ListID    string
	TLSConfig *tls.Config // tests set this. Empty means the system's trust store.
}

func (s SMTP) tlsConfig() *tls.Config {
	if s.TLSConfig != nil {
		return s.TLSConfig
	}
	return &tls.Config{ServerName: s.Config.Host, MinVersion: tls.VersionTLS12}
}

func (s SMTP) dial(ctx context.Context) (net.Conn, error) {
	address := net.JoinHostPort(s.Config.Host, strconv.Itoa(s.Config.Port))
	dialer := &net.Dialer{Timeout: smtpTimeout}
	if s.Config.TLS == config.SMTPTLSImplicit {
		return (&tls.Dialer{NetDialer: dialer, Config: s.tlsConfig()}).DialContext(ctx, "tcp", address)
	}
	return dialer.DialContext(ctx, "tcp", address)
}

func (s SMTP) authenticate(c *smtp.Client) error {
	if s.Config.Username == "" {
		return nil
	}
	return c.Auth(smtp.PlainAuth("", s.Config.Username, s.Config.Password, s.Config.Host))
}

func (s SMTP) secure(c *smtp.Client) error {
	if s.Config.TLS != config.SMTPTLSStart {
		return nil
	}
	if ok, _ := c.Extension("STARTTLS"); !ok {
		return errors.New("mail: the server does not offer STARTTLS")
	}
	return c.StartTLS(s.tlsConfig())
}

// Send delivers one message.
func (s SMTP) Send(ctx context.Context, m Message) error {
	body, err := Compose(s.Config.From, s.Config.ReplyTo, s.ListID, m, time.Now())
	if err != nil {
		return err
	}
	from, err := mail.ParseAddress(s.Config.From)
	if err != nil {
		return err
	}
	to, err := mail.ParseAddress(oneLine(m.To))
	if err != nil {
		return errors.New("mail: the address of a person is not valid")
	}
	conn, err := s.dial(ctx)
	if err != nil {
		return err
	}
	if err := conn.SetDeadline(time.Now().Add(smtpTimeout)); err != nil {
		_ = conn.Close()
		return err
	}
	c, err := smtp.NewClient(conn, s.Config.Host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = c.Close() }()
	if err := s.secure(c); err != nil {
		return err
	}
	if err := s.authenticate(c); err != nil {
		return err
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(to.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// safeError says what went wrong without an address or a word of the mail, because
// it ends up in the admin console and in logs.
func safeError(err error) string {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return "The mail server could not be reached."
	}
	var proto interface{ Temporary() bool }
	if errors.As(err, &proto) {
		return fmt.Sprintf("The mail server answered with an error (temporary: %t).", proto.Temporary())
	}
	return "The mail could not be delivered."
}
