// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Warning is a setting that loads but does not work yet, such as a mail server that does
// not answer. It never names an address or a secret.
type Warning struct {
	Setting string // the name without DIFFLUENCE_
	Message string
}

// probeTimeout bounds each probe, so a start never waits long on a system outside.
const probeTimeout = 5 * time.Second

// Probe tries the settings that are otherwise only used later, in an emergency or on the
// first mail, login or publish, without side effects (OPS-101, spec 056 decision 8). A
// failure is a warning: the outside system may be down for its own reasons.
func Probe(ctx context.Context, c Config) []Warning { return ProbeWithin(ctx, c, probeTimeout) }

// ProbeWithin is Probe with another bound for each probe.
func ProbeWithin(ctx context.Context, c Config, timeout time.Duration) []Warning {
	var (
		mu       sync.Mutex
		warnings []Warning
		wg       sync.WaitGroup
	)
	run := func(setting string, probe func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			if err := probe(pctx); err != nil {
				mu.Lock()
				warnings = append(warnings, Warning{Setting: setting, Message: err.Error()})
				mu.Unlock()
			}
		}()
	}
	if c.SMTP.Host != "" {
		run("SMTP_HOST", func(ctx context.Context) error { return probeMail(ctx, c.SMTP) })
	}
	if c.OIDC.Issuer != "" {
		run("OIDC_ISSUER", func(ctx context.Context) error { return probeIssuer(ctx, c.OIDC.Issuer) })
	}
	if c.PublishDir != "" {
		run("PUBLISH_DIR", func(context.Context) error { return probeFolder(c.PublishDir) })
	}
	wg.Wait()
	return warnings
}

// probeMail checks that the name of the mail server resolves and that its port takes a
// connection. It closes the connection at once: no mail, no login.
func probeMail(ctx context.Context, s SMTP) error {
	if _, err := net.DefaultResolver.LookupHost(ctx, s.Host); err != nil {
		return errors.New("the name of the mail server does not resolve")
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(s.Host, strconv.Itoa(s.Port)))
	if err != nil {
		return fmt.Errorf("the mail server does not accept a connection on port %d", s.Port)
	}
	return conn.Close()
}

// probeIssuer loads the identity provider's discovery document and checks that it names
// this issuer, as login will.
func probeIssuer(ctx context.Context, issuer string) error {
	address := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return errors.New("the address of the identity provider is not valid")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.New("the identity provider does not answer")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the identity provider's discovery document does not load (status %d)", resp.StatusCode)
	}
	var doc struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return errors.New("the identity provider's discovery document is not valid JSON")
	}
	if strings.TrimSuffix(doc.Issuer, "/") != strings.TrimSuffix(issuer, "/") {
		return errors.New("the identity provider's discovery document names another issuer; login will refuse it")
	}
	return nil
}

// probeFolder writes a file in the folder and removes it again.
func probeFolder(dir string) error {
	f, err := os.CreateTemp(dir, ".diffluence-probe-*")
	if err != nil {
		return errors.New("the publish folder cannot be written")
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return errors.New("the publish folder cannot be written")
	}
	if err := os.Remove(name); err != nil {
		return errors.New("a test file in the publish folder could not be removed")
	}
	return nil
}
