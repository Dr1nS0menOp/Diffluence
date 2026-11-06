// SPDX-License-Identifier: Apache-2.0

// Package outward is the one way this server calls other servers (spec 015, D15).
// Calling an address that people chose is a classic way into the inside network, so
// a call needs an allowed host, https, a public address, and gets no redirects.
package outward

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
)

const (
	callTimeout  = 10 * time.Second
	maxReadBytes = 64 << 10
	maxHostLen   = 253
	maxHosts     = 200
)

// ErrRefused says a call was not made, and why it must not be tried again as it is.
var ErrRefused = errors.New("outward: refused")

// Options change what a call may reach. Private is for an instance whose outside
// systems live on its own network, and is off unless an operator sets it.
type Options struct {
	AllowPrivate bool
}

// Service keeps the list of allowed hosts and makes calls.
type Service struct {
	pool    *pgxpool.Pool
	options Options
	client  *http.Client
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, o Options) *Service {
	return &Service{pool: pool, options: o, client: newClient(o)}
}

// isPublic refuses every kind of address that is not on the open internet.
func isPublic(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsValid() && a.IsGlobalUnicast() && !a.IsPrivate() && !a.IsLoopback() && !a.IsLinkLocalUnicast() &&
		!isSharedOrReserved(a)
}

var reserved = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func isSharedOrReserved(a netip.Addr) bool {
	for _, p := range reserved {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// newClient checks the address at the moment of connecting: a name that changes
// between a check and the call gains nothing, and every address a name gives passes
// through here.
func newClient(o Options) *http.Client {
	dialer := &net.Dialer{Timeout: callTimeout, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return ErrRefused
		}
		a, err := netip.ParseAddr(host)
		if err != nil || (!o.AllowPrivate && !isPublic(a)) {
			return fmt.Errorf("%w: address not allowed", ErrRefused)
		}
		return nil
	}}
	return &http.Client{
		Timeout: callTimeout,
		Transport: &http.Transport{
			DialContext:       dialer.DialContext,
			DisableKeepAlives: true,
			Proxy:             nil,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Target checks an address before it is stored or called.
func (s *Service) Target(ctx context.Context, address string) (*url.URL, error) {
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" || u.User != nil {
		return nil, apperr.Invalid("That is not an address.")
	}
	if u.Scheme != "https" && !(s.options.AllowPrivate && u.Scheme == "http") {
		return nil, apperr.Invalid("Calls go over https.")
	}
	if port := u.Port(); port != "" && port != "443" && !s.options.AllowPrivate {
		return nil, apperr.Invalid("Calls go to the normal https port.")
	}
	allowed, err := s.allowed(ctx, u.Hostname())
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, apperr.Invalid("An administrator must allow this host first, under Outgoing calls.")
	}
	return u, nil
}

func (s *Service) allowed(ctx context.Context, host string) (bool, error) {
	var found bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM outgoing_hosts WHERE host = lower($1))", host).Scan(&found)
	return found, err
}

// Answer is what came back. Only the status and the start of the body are kept.
type Answer struct {
	Status int
	Body   []byte
}

// Post sends a JSON body to an allowed address.
func (s *Service) Post(ctx context.Context, address string, body []byte, headers map[string]string) (Answer, error) {
	u, err := s.Target(ctx, address)
	if err != nil {
		return Answer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Diffluence")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return Answer{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	read, err := io.ReadAll(io.LimitReader(resp.Body, maxReadBytes))
	if err != nil {
		return Answer{Status: resp.StatusCode}, err
	}
	return Answer{Status: resp.StatusCode, Body: read}, nil
}

// Hosts lists the allowed hosts.
func (s *Service) Hosts(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, "SELECT host FROM outgoing_hosts ORDER BY host")
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if out == nil {
		out = []string{}
	}
	return out, err
}

func cleanHost(host string) (string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || len(host) > maxHostLen || strings.ContainsAny(host, "/:@ \t\r\n*") {
		return "", apperr.Invalid("A host is a name such as hooks.example.com, without a path.")
	}
	return host, nil
}

// SetHosts replaces the list. The route requires administrator.
func (s *Service) SetHosts(ctx context.Context, c identity.Caller, hosts []string) error {
	if len(hosts) > maxHosts {
		return apperr.Invalid("The list holds at most 200 hosts.")
	}
	clean := make([]string, 0, len(hosts))
	for _, h := range hosts {
		v, err := cleanHost(h)
		if err != nil {
			return err
		}
		clean = append(clean, v)
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM outgoing_hosts WHERE NOT host = ANY($1)", clean); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO outgoing_hosts (host, added_by)
			SELECT h, $2 FROM unnest($1::text[]) AS h ON CONFLICT DO NOTHING`, clean, c.UserID); err != nil {
			return err
		}
		_, err := audit.Record(ctx, tx, c.Event("outward.hosts_changed", "settings", "", "", map[string]any{"hosts": clean}))
		return err
	})
}
