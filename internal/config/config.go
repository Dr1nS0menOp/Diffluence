// SPDX-License-Identifier: Apache-2.0

// Package config loads settings from environment variables. Every variable can
// instead be read from a file by appending _FILE (for Docker/Kubernetes secrets).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Role decides which parts of the binary run in this process.
type Role string

const (
	RoleAll    Role = "all"    // api + worker in one process (small installs)
	RoleAPI    Role = "api"    // HTTP API only
	RoleWorker Role = "worker" // background jobs only
)

// RunsAPI reports whether this process serves the HTTP API.
func (r Role) RunsAPI() bool { return r == RoleAll || r == RoleAPI }

// RunsWorker reports whether this process runs background jobs.
func (r Role) RunsWorker() bool { return r == RoleAll || r == RoleWorker }

// Config is the complete runtime configuration.
type Config struct {
	Role               Role
	HTTPAddr           string
	PublicURL          *url.URL
	InstanceName       string
	DatabaseURL        string
	MigrateDatabaseURL string // owner of the tables; empty = use DatabaseURL
	NATSURL            string // empty = embedded NATS (only allowed with role "all")
	DataDir            string
	MigrateOnStart     bool
	LogLevel           slog.Level
	SessionIdleTimeout time.Duration
	SessionMaxAge      time.Duration
	TrustedProxies     []netip.Prefix
	ShutdownDelay      time.Duration // time between "not ready" and closing connections
	MasterKeyFile      string        // key-encryption key; empty = DATA_DIR/master.key
	// PasswordBlocklistFile holds known bad passwords, one per line. Empty = none.
	PasswordBlocklistFile string
	ExampleSpace          bool // make the example space on a new instance
	OIDC                  OIDC
	SMTP                  SMTP
}

// The ways a connection to a mail server is encrypted.
const (
	SMTPTLSStart    = "starttls"
	SMTPTLSImplicit = "implicit"
	SMTPTLSNone     = "none"
)

// SMTP is the mail server. Host empty = mail is off, and waits in the queue.
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	ReplyTo  string
	TLS      string
}

// OIDC configures login through an identity provider. Issuer empty = off.
type OIDC struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	DisplayName  string
	AutoCreate   bool
	GroupsClaim  string
}

// SecureCookies reports whether cookies must carry the Secure flag.
func (c Config) SecureCookies() bool { return c.PublicURL.Scheme == "https" }

// KeyFile is where the master key lives.
func (c Config) KeyFile() string {
	if c.MasterKeyFile != "" {
		return c.MasterKeyFile
	}
	return filepath.Join(c.DataDir, "master.key")
}

// PublicOrigin returns scheme://host[:port] of the public URL.
func (c Config) PublicOrigin() string { return c.PublicURL.Scheme + "://" + c.PublicURL.Host }

// LookupFunc matches os.LookupEnv. Tests pass a map-backed function.
type LookupFunc func(key string) (string, bool)

const prefix = "DIFFLUENCE_"

// FromEnvironment loads the configuration from the process environment.
func FromEnvironment() (Config, error) { return Load(os.LookupEnv) }

// Load builds a Config, collecting every problem instead of stopping at the first.
func Load(lookup LookupFunc) (Config, error) {
	l := loader{lookup: lookup}
	c := Config{
		Role:                  Role(l.str("ROLE", string(RoleAll))),
		HTTPAddr:              l.str("HTTP_ADDR", ":8080"),
		InstanceName:          l.str("INSTANCE_NAME", "Diffluence"),
		DatabaseURL:           l.str("DATABASE_URL", ""),
		MigrateDatabaseURL:    l.str("MIGRATE_DATABASE_URL", ""),
		NATSURL:               l.str("NATS_URL", ""),
		DataDir:               l.str("DATA_DIR", "./data"),
		MigrateOnStart:        l.boolean("MIGRATE_ON_START", true),
		SessionIdleTimeout:    l.duration("SESSION_IDLE_TIMEOUT", 8*time.Hour),
		SessionMaxAge:         l.duration("SESSION_MAX_AGE", 7*24*time.Hour),
		ShutdownDelay:         l.duration("SHUTDOWN_DELAY", 5*time.Second),
		MasterKeyFile:         l.str("MASTER_KEY_FILE", ""),
		PasswordBlocklistFile: l.str("PASSWORD_BLOCKLIST_FILE", ""),
		ExampleSpace:          l.boolean("EXAMPLE_SPACE", true),
		OIDC: OIDC{
			Issuer:       l.str("OIDC_ISSUER", ""),
			ClientID:     l.str("OIDC_CLIENT_ID", ""),
			ClientSecret: l.str("OIDC_CLIENT_SECRET", ""),
			DisplayName:  l.str("OIDC_DISPLAY_NAME", "Single sign-on"),
			AutoCreate:   l.boolean("OIDC_AUTO_CREATE", true),
			GroupsClaim:  l.str("OIDC_GROUPS_CLAIM", ""),
		},
		SMTP: SMTP{
			Host:     l.str("SMTP_HOST", ""),
			Port:     l.integer("SMTP_PORT", 587),
			Username: l.str("SMTP_USERNAME", ""),
			Password: l.str("SMTP_PASSWORD", ""),
			From:     l.str("SMTP_FROM", ""),
			ReplyTo:  l.str("SMTP_REPLY_TO", ""),
			TLS:      l.str("SMTP_TLS", SMTPTLSStart),
		},
	}
	if c.SMTP.Host != "" && c.SMTP.From == "" {
		l.fail("SMTP_HOST", errors.New("needs SMTP_FROM as well"))
	}
	switch c.SMTP.TLS {
	case SMTPTLSStart, SMTPTLSImplicit, SMTPTLSNone:
	default:
		l.fail("SMTP_TLS", fmt.Errorf("must be starttls, implicit or none, got %q", c.SMTP.TLS))
	}
	if c.OIDC.Issuer != "" && (c.OIDC.ClientID == "" || c.OIDC.ClientSecret == "") {
		l.fail("OIDC_ISSUER", errors.New("needs OIDC_CLIENT_ID and OIDC_CLIENT_SECRET as well"))
	}
	if c.MasterKeyFile == "" && c.Role != RoleAll {
		l.fail("MASTER_KEY_FILE", errors.New("is required when ROLE is api or worker: every node needs the same key"))
	}

	switch c.Role {
	case RoleAll, RoleAPI, RoleWorker:
	default:
		l.fail("ROLE", fmt.Errorf("must be all, api or worker, got %q", c.Role))
	}
	if c.DatabaseURL == "" {
		l.fail("DATABASE_URL", errors.New("is required"))
	}
	if c.NATSURL == "" && c.Role != RoleAll {
		l.fail("NATS_URL", errors.New("is required when ROLE is api or worker (embedded NATS only works with ROLE=all)"))
	}
	if c.SessionIdleTimeout <= 0 || c.SessionMaxAge <= 0 || c.SessionIdleTimeout > c.SessionMaxAge {
		l.fail("SESSION_IDLE_TIMEOUT", errors.New("must be positive and not longer than SESSION_MAX_AGE"))
	}
	if c.ShutdownDelay < 0 {
		l.fail("SHUTDOWN_DELAY", errors.New("must not be negative"))
	}

	pub := l.str("PUBLIC_URL", "http://localhost:8080")
	u, err := url.Parse(pub)
	switch {
	case err != nil:
		l.fail("PUBLIC_URL", err)
	case (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.User != nil:
		l.fail("PUBLIC_URL", errors.New("must look like https://wiki.example.com (no path, query or credentials)"))
	default:
		c.PublicURL = u
	}

	switch lvl := strings.ToLower(l.str("LOG_LEVEL", "info")); lvl {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		l.fail("LOG_LEVEL", fmt.Errorf("must be debug, info, warn or error, got %q", lvl))
	}

	for _, s := range strings.Split(l.str("TRUSTED_PROXIES", ""), ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			l.fail("TRUSTED_PROXIES", fmt.Errorf("%q is not a CIDR range: %w", s, err))
			continue
		}
		c.TrustedProxies = append(c.TrustedProxies, p.Masked())
	}

	if len(l.errs) > 0 {
		return Config{}, errors.Join(l.errs...)
	}
	return c, nil
}

type loader struct {
	lookup LookupFunc
	errs   []error
}

func (l *loader) fail(key string, err error) {
	l.errs = append(l.errs, fmt.Errorf("%s%s: %w", prefix, key, err))
}

// raw returns the value of KEY or the contents of the file named by KEY_FILE.
func (l *loader) raw(key string) (string, bool) {
	v, ok := l.lookup(prefix + key)
	f, fok := l.lookup(prefix + key + "_FILE")
	if ok && fok {
		l.fail(key, errors.New("set either the variable or its _FILE variant, not both"))
		return "", false
	}
	if fok {
		b, err := os.ReadFile(f)
		if err != nil {
			l.fail(key+"_FILE", err)
			return "", false
		}
		return strings.TrimRight(string(b), "\r\n"), true
	}
	return v, ok
}

func (l *loader) str(key, def string) string {
	if v, ok := l.raw(key); ok && v != "" {
		return v
	}
	return def
}

func (l *loader) boolean(key string, def bool) bool {
	v, ok := l.raw(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.fail(key, fmt.Errorf("must be true or false, got %q", v))
		return def
	}
	return b
}

func (l *loader) integer(key string, def int) int {
	v, ok := l.raw(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.fail(key, fmt.Errorf("must be a number, got %q", v))
		return def
	}
	return n
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	v, ok := l.raw(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.fail(key, fmt.Errorf("must be a duration like 30s or 8h, got %q", v))
		return def
	}
	return d
}
