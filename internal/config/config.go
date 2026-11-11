// SPDX-License-Identifier: Apache-2.0

// Package config loads settings from environment variables and, when
// DIFFLUENCE_CONFIG_FILE names one, a YAML file. Every setting can instead be read from
// a file of its own by appending _FILE (for Docker/Kubernetes secrets). The order is the
// default, then the file, then the environment (spec 056).
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
	// UpgradeBackupMaxAge is how old the last noted backup may be before a node refuses to
	// migrate the database (blueprint 07, rule 4; ADR-040). It is 24 hours unless set. Zero
	// means the operator switched the check off with "off": only a warning in the log.
	UpgradeBackupMaxAge time.Duration
	// ContractGrace is how long every live node must have run a release before its contract
	// migrations run, the window in which a rollback stays possible (spec 048 D1).
	ContractGrace time.Duration
	// PasswordBlocklistFile holds known bad passwords, one per line. Empty = none.
	PasswordBlocklistFile string
	ExampleSpace          bool // make the example space on a new instance
	// OutwardAllowPrivate lets outgoing calls reach private addresses and plain http,
	// for an instance whose outside systems live on its own network.
	OutwardAllowPrivate bool
	// CollabURL and CollabToken point at the service for editing together. Empty
	// means it is not installed, and pages keep private drafts.
	CollabURL   string
	CollabToken string
	// MetricsToken is the secret a scraper sends. Empty means no /metrics at all.
	MetricsToken string
	// PDFURL is the helper process that makes PDF files. Empty = no PDF.
	PDFURL string
	// PDFPassword is what the PDF helper asks for, sent as basic authentication. Empty =
	// the helper asks nothing, for installs from before it could.
	PDFPassword string
	// CollaboraURL is an office service. It opens office files as a PDF, in place of the
	// LibreOffice of the PDF helper. Empty = that helper, if there is one.
	CollaboraURL string
	// PublishDir is where public sites go. It is configuration and not a setting,
	// because a person must not be able to write anywhere on the server. Empty = no sites.
	PublishDir string
	// DrawingURL is the optional draw.io service. Nil = off, and nothing is framed.
	DrawingURL *url.URL
	OIDC       OIDC
	SMTP       SMTP
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

// PlainToOthers reports whether the public address is plain http on a host other than this
// machine. Cookies then go without Secure and no HSTS is sent, which is only right for a trial
// on one's own machine (SEC-257).
func (c Config) PlainToOthers() bool {
	if c.SecureCookies() {
		return false
	}
	// Load refuses an IP address here, so a name is all that can come: only localhost
	// stays on this machine.
	return !strings.EqualFold(c.PublicURL.Hostname(), "localhost")
}

// isAddress reports whether a host is an IP address rather than a name.
func isAddress(host string) bool {
	_, err := netip.ParseAddr(host)
	return err == nil
}

// KeyFile is where the master key lives.
func (c Config) KeyFile() string {
	if c.MasterKeyFile != "" {
		return c.MasterKeyFile
	}
	return filepath.Join(c.DataDir, "master.key")
}

// PublicOrigin returns scheme://host[:port] of the public URL.
func (c Config) PublicOrigin() string { return c.PublicURL.Scheme + "://" + c.PublicURL.Host }

// DrawingOrigin returns scheme://host[:port] of the drawing service, or empty when off.
func (c Config) DrawingOrigin() string {
	if c.DrawingURL == nil {
		return ""
	}
	return c.DrawingURL.Scheme + "://" + c.DrawingURL.Host
}

// LookupFunc matches os.LookupEnv. Tests pass a map-backed function.
type LookupFunc func(key string) (string, bool)

const prefix = "DIFFLUENCE_"

// FromEnvironment loads the configuration from the process environment.
func FromEnvironment() (Config, error) { return Load(os.LookupEnv) }

// Load builds a Config, collecting every problem instead of stopping at the first.
func Load(lookup LookupFunc) (Config, error) {
	c, _, err := Inspect(lookup)
	return c, err
}

// Inspect builds a Config as Load does, and also says which value each setting has and
// where it came from, with secrets hidden, for diffluence config check. Load and Inspect
// are one validator, so the server and the check cannot disagree.
func Inspect(lookup LookupFunc) (Config, []Setting, error) {
	l := loader{lookup: lookup, asked: map[string]bool{}, origins: map[string]origin{}}
	if path, ok := lookup(prefix + "CONFIG_FILE"); ok && path != "" {
		f, errs := readConfigFile(path)
		l.file, l.errs = &f, append(l.errs, errs...)
	}
	c := l.load()
	l.checkFile()
	if len(l.errs) > 0 {
		return Config{}, l.settings, errors.Join(l.errs...)
	}
	return c, l.settings, nil
}

// load reads every setting. Every setting it asks for is a key the file may hold.
func (l *loader) load() Config {
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
		UpgradeBackupMaxAge:   l.backupAge("UPGRADE_BACKUP_MAX_AGE"),
		ContractGrace:         l.notNegative("CONTRACT_GRACE", defaultContractGrace),
		PasswordBlocklistFile: l.str("PASSWORD_BLOCKLIST_FILE", ""),
		ExampleSpace:          l.boolean("EXAMPLE_SPACE", true),
		OutwardAllowPrivate:   l.boolean("OUTWARD_ALLOW_PRIVATE", false),
		CollabURL:             l.str("COLLAB_URL", ""),
		CollabToken:           l.str("COLLAB_TOKEN", ""),
		MetricsToken:          l.str("METRICS_TOKEN", ""),
		PDFURL:                l.str("PDF_URL", ""),
		PDFPassword:           l.str("PDF_PASSWORD", ""),
		CollaboraURL:          l.str("COLLABORA_URL", ""),
		PublishDir:            l.str("PUBLISH_DIR", ""),
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
	case isAddress(u.Hostname()):
		// Passkeys are bound to a name, and a browser refuses an address as that name, so
		// an address here would only fail later, in a loop of restarts (spec 050).
		l.fail("PUBLIC_URL", errors.New("must use a host name, not an IP address: passkeys are bound to a name. "+
			"On this machine use http://localhost:8080; on a network, a name from DNS or the hosts file"))
	default:
		c.PublicURL = u
	}

	if address := l.str("DRAWING_URL", ""); address != "" {
		drawing, err := parseDrawingURL(address, c.PublicURL)
		if err != nil {
			l.fail("DRAWING_URL", err)
		}
		c.DrawingURL = drawing
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
	return c
}

type loader struct {
	lookup   LookupFunc
	file     *configFile // nil = no configuration file
	errs     []error
	asked    map[string]bool   // every setting load asked for, and its _FILE variant
	origins  map[string]origin // where the value of each setting came from
	settings []Setting         // in the order load asked, for the report
}

// origin is where the value of a setting came from.
type origin struct {
	source   string // sourceDefault, sourceFile or sourceEnvironment
	line     int    // the line in the configuration file, when it came from there
	overruns bool   // the environment set it and the file did too
}

const (
	sourceDefault     = "default"
	sourceFile        = "file"
	sourceEnvironment = "environment"
)

func (l *loader) fail(key string, err error) {
	if o := l.origins[key]; o.source == sourceFile {
		// The variable name says which setting, the line where to look in the file.
		l.errs = append(l.errs, fmt.Errorf("%s%s (config file line %d): %w", prefix, key, o.line, err))
		return
	}
	l.errs = append(l.errs, fmt.Errorf("%s%s: %w", prefix, key, err))
}

// raw returns the value of a setting: from the environment, KEY or the file named by
// KEY_FILE, or else from the configuration file, key or key_file.
func (l *loader) raw(key string) (string, bool) {
	l.asked[key], l.asked[key+"_FILE"] = true, true
	v, ok := l.lookup(prefix + key)
	f, fok := l.lookup(prefix + key + "_FILE")
	if ok && fok {
		l.fail(key, errors.New("set either the variable or its _FILE variant, not both"))
		return "", false
	}
	// An empty variable has always meant the default, and Compose passes most settings as
	// one, so it leaves the file in effect.
	if (ok && v != "") || fok {
		l.origins[key] = origin{source: sourceEnvironment, overruns: l.fileHas(key)}
		if fok {
			return l.readSecret(key, f)
		}
		return v, ok
	}
	if l.file == nil {
		return "", false
	}
	e, has := l.file.entries[key]
	ef, hasFile := l.file.entries[key+"_FILE"]
	switch {
	case has && hasFile:
		l.origins[key] = origin{source: sourceFile, line: ef.line}
		l.fail(key, fmt.Errorf("set either %s or %s in the config file, not both", e.key, ef.key))
		return "", false
	case hasFile:
		l.origins[key] = origin{source: sourceFile, line: ef.line}
		if ef.text == "" {
			return "", false
		}
		return l.readSecret(key, ef.text)
	case has:
		l.origins[key] = origin{source: sourceFile, line: e.line}
		return e.text, true
	}
	return "", false
}

// fileHas reports whether the configuration file sets key, directly or through key_file.
func (l *loader) fileHas(key string) bool {
	if l.file == nil {
		return false
	}
	_, has := l.file.entries[key]
	_, hasFile := l.file.entries[key+"_FILE"]
	return has || hasFile
}

// readSecret reads the value of a setting from the file at path.
func (l *loader) readSecret(key, path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		l.fail(key+"_FILE", err)
		return "", false
	}
	return strings.TrimRight(string(b), "\r\n"), true
}

// note keeps the value in effect of a setting for the report.
func (l *loader) note(key, value string) {
	o, ok := l.origins[key]
	if !ok {
		o.source = sourceDefault
	}
	l.settings = append(l.settings, Setting{Name: key, Value: shown(key, value), Source: o.describe()})
}

func (l *loader) str(key, def string) string {
	v, ok := l.raw(key)
	if !ok || v == "" {
		v = def
		delete(l.origins, key) // an empty value means the default, wherever it was written
	}
	l.note(key, v)
	return v
}

func (l *loader) boolean(key string, def bool) bool {
	b := def
	if v, ok := l.raw(key); ok && v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			l.fail(key, fmt.Errorf("must be true or false, got %q", v))
		} else {
			b = parsed
		}
	} else {
		delete(l.origins, key)
	}
	l.note(key, strconv.FormatBool(b))
	return b
}

func (l *loader) integer(key string, def int) int {
	n := def
	if v, ok := l.raw(key); ok && v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			l.fail(key, fmt.Errorf("must be a number, got %q", v))
		} else {
			n = parsed
		}
	} else {
		delete(l.origins, key)
	}
	l.note(key, strconv.Itoa(n))
	return n
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	d := def
	if v, ok := l.raw(key); ok && v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			l.fail(key, fmt.Errorf("must be a duration like 30s or 8h, got %q", v))
		} else {
			d = parsed
		}
	} else {
		delete(l.origins, key)
	}
	l.note(key, d.String())
	return d
}

// defaultContractGrace leaves a working day to notice a bad upgrade and roll it back
// before a contract migration removes what the previous release needs (spec 048 D1).
const defaultContractGrace = 24 * time.Hour

// notNegative reads a duration that may be zero but not below.
func (l *loader) notNegative(key string, def time.Duration) time.Duration {
	d := l.duration(key, def)
	if d < 0 {
		l.fail(key, fmt.Errorf("must not be negative, got %v", d))
		return def
	}
	return d
}

// defaultUpgradeBackupMaxAge is how old the last backup may be before an upgrade, unless the
// operator says otherwise (ADR-040).
const defaultUpgradeBackupMaxAge = 24 * time.Hour

// backupOff is the value that switches the backup check before an upgrade off on purpose.
const backupOff = "off"

// backupAge reads the backup age for an upgrade: a positive duration, or "off", which
// gives zero. A zero or negative duration is refused rather than read as "off", so the
// check is never switched off by a typing mistake.
func (l *loader) backupAge(key string) time.Duration {
	v, ok := l.raw(key)
	if !ok || v == "" {
		delete(l.origins, key)
		l.note(key, defaultUpgradeBackupMaxAge.String())
		return defaultUpgradeBackupMaxAge
	}
	if strings.EqualFold(strings.TrimSpace(v), backupOff) {
		l.note(key, backupOff)
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		l.fail(key, fmt.Errorf("must be a positive duration like 24h, or %s to upgrade without a recent backup, got %q", backupOff, v))
		d = defaultUpgradeBackupMaxAge
	}
	l.note(key, d.String())
	return d
}

// parseDrawingURL checks the address of the drawing service. A frame on our own origin
// would reach our session, so the service must live on another one (SEC-521).
func parseDrawingURL(address string, public *url.URL) (*url.URL, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.User != nil {
		return nil, errors.New("must look like https://draw.example.com (no path, query or credentials)")
	}
	if public != nil && strings.EqualFold(u.Scheme, public.Scheme) && strings.EqualFold(u.Host, public.Host) {
		return nil, errors.New("must be another origin than PUBLIC_URL")
	}
	return &url.URL{Scheme: u.Scheme, Host: strings.ToLower(u.Host)}, nil
}
