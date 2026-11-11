// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes a configuration file with the given mode and returns its path.
func writeConfig(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "diffluence.yaml")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

// withFile is an environment that names the file and nothing else, unless more is given.
func withFile(path string, more map[string]string) LookupFunc {
	m := map[string]string{"DIFFLUENCE_CONFIG_FILE": path}
	for k, v := range more {
		m[k] = v
	}
	return env(m)
}

func mustFail(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want an error mentioning %q, got none", parts)
	}
	for _, p := range parts {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error does not mention %q: %v", p, err)
		}
	}
}

func TestAnUnknownKeyInTheFileIsRefusedWithItsLine(t *testing.T) {
	p := writeConfig(t, "database_url: postgres://x\n\nsmtp_hots: mail.example.com\n", 0o600)
	_, err := Load(withFile(p, nil))
	mustFail(t, err, "smtp_hots", "line 3", "unknown")
}

func TestAKeyWithThePrefixIsNamedAsSuch(t *testing.T) {
	p := writeConfig(t, "database_url: postgres://x\nDIFFLUENCE_LOG_LEVEL: debug\n", 0o600)
	_, err := Load(withFile(p, nil))
	mustFail(t, err, "line 2", "without DIFFLUENCE_")
}

func TestASettingWrittenTwiceIsRefused(t *testing.T) {
	for name, body := range map[string]string{
		"twice flat":      "database_url: postgres://x\nlog_level: info\nlog_level: debug\n",
		"flat and nested": "database_url: postgres://x\nsmtp_host: a.example.com\nsmtp:\n  host: b.example.com\n",
		"different case":  "database_url: postgres://x\nlog_level: info\nLOG_LEVEL: debug\n",
		"nested and flat": "database_url: postgres://x\nsmtp:\n  host: a.example.com\nsmtp_host: b.example.com\n",
		"nested twice":    "database_url: postgres://x\nsmtp:\n  host: a.example.com\nsmtp:\n  port: 25\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(withFile(writeConfig(t, body, 0o600), nil))
			mustFail(t, err, "more than once")
		})
	}
}

func TestANestedKeyIsTheFlatName(t *testing.T) {
	p := writeConfig(t, "database_url: postgres://x\nsmtp:\n  host: mail.example.com\n  from: wiki@example.com\n  port: 25\n", 0o600)
	c, err := Load(withFile(p, nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.SMTP.Host != "mail.example.com" || c.SMTP.From != "wiki@example.com" || c.SMTP.Port != 25 {
		t.Errorf("smtp = %+v", c.SMTP)
	}
}

func TestAListWhereOneValueBelongsIsRefused(t *testing.T) {
	p := writeConfig(t, "database_url: postgres://x\ntrusted_proxies:\n  - 10.0.0.0/8\n", 0o600)
	_, err := Load(withFile(p, nil))
	mustFail(t, err, "trusted_proxies", "line 2", "one value")
}

// YAML 1.1 reads no and off as false and 012 as octal. Settings read the text written.
func TestYAMLGuessesAreNotTaken(t *testing.T) {
	p := writeConfig(t, "database_url: postgres://x\nexample_space: no\n", 0o600)
	_, err := Load(withFile(p, nil))
	mustFail(t, err, "EXAMPLE_SPACE", "true or false", `"no"`, "line 2")

	p = writeConfig(t, "database_url: postgres://x\ninstance_name: 012\nsmtp_port: 0587\n", 0o600)
	c, err := Load(withFile(p, nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.InstanceName != "012" {
		t.Errorf("instance name = %q, want the text 012", c.InstanceName)
	}
	if c.SMTP.Port != 587 {
		t.Errorf("port = %d, want 587, not an octal number", c.SMTP.Port)
	}
}

func TestAFileThatIsNotYAMLIsRefusedWithItsLine(t *testing.T) {
	p := writeConfig(t, "database_url: postgres://x\nlog_level: [unclosed\n", 0o600)
	_, err := Load(withFile(p, nil))
	mustFail(t, err, "config file", "line")
}

func TestAMissingFileIsRefused(t *testing.T) {
	_, err := Load(withFile(filepath.Join(t.TempDir(), "absent.yaml"), map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x"}))
	mustFail(t, err, "CONFIG_FILE")
}

func TestAnEmptyFileChangesNothing(t *testing.T) {
	p := writeConfig(t, "# only a comment\n", 0o600)
	c, err := Load(withFile(p, map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8080" {
		t.Errorf("addr = %q", c.HTTPAddr)
	}
}

func TestTheFileWinsOverTheDefault(t *testing.T) {
	p := writeConfig(t, "database_url: postgres://x\nhttp_addr: \":9090\"\n", 0o600)
	c, err := Load(withFile(p, nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":9090" {
		t.Errorf("addr = %q, want the file's", c.HTTPAddr)
	}
}

func TestTheEnvironmentWinsOverTheFile(t *testing.T) {
	p := writeConfig(t, "database_url: postgres://file\nlog_level: debug\n", 0o600)
	c, err := Load(withFile(p, map[string]string{"DIFFLUENCE_LOG_LEVEL": "warn"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.LogLevel.String() != "WARN" || c.DatabaseURL != "postgres://file" {
		t.Errorf("level %v, database %q", c.LogLevel, c.DatabaseURL)
	}
}

// Compose passes most settings as an empty variable; empty has always meant the default, so
// the file applies.
func TestAnEmptyVariableLeavesTheFileInEffect(t *testing.T) {
	p := writeConfig(t, "database_url: postgres://x\nlog_level: debug\n", 0o600)
	c, settings, err := Inspect(withFile(p, map[string]string{"DIFFLUENCE_LOG_LEVEL": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if c.LogLevel.String() != "DEBUG" {
		t.Errorf("level %v, want the file's", c.LogLevel)
	}
	for _, s := range settings {
		if s.Name == "LOG_LEVEL" && s.Source != "file, line 2" {
			t.Errorf("source %q", s.Source)
		}
	}
}

// The environment's secret file wins over the file.
func TestTheEnvironmentsFileVariantWinsOverTheFile(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "db")
	if err := os.WriteFile(secret, []byte("postgres://from-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := writeConfig(t, "database_url: postgres://file\n", 0o600)
	c, err := Load(withFile(p, map[string]string{"DIFFLUENCE_DATABASE_URL_FILE": secret}))
	if err != nil {
		t.Fatal(err)
	}
	if c.DatabaseURL != "postgres://from-secret" {
		t.Errorf("database %q", c.DatabaseURL)
	}
}

func TestASecretFromAPathInTheFile(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "smtp")
	if err := os.WriteFile(secret, []byte("hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The file itself may be read by others: it holds a path, not the secret.
	p := writeConfig(t, "database_url_file: "+secret+"\nsmtp:\n  host: mail.example.com\n  from: w@example.com\n  password_file: "+secret+"\n", 0o644)
	c, err := Load(withFile(p, nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.SMTP.Password != "hunter2" {
		t.Errorf("password = %q", c.SMTP.Password)
	}
}

func TestAKeyAndItsFileVariantBothInTheFileAreRefused(t *testing.T) {
	p := writeConfig(t, "database_url: postgres://x\nmetrics_token: a\nmetrics_token_file: /run/secrets/m\n", 0o600)
	_, err := Load(withFile(p, nil))
	mustFail(t, err, "METRICS_TOKEN", "not both")
}

func TestASecretInAFileOthersCanReadIsRefused(t *testing.T) {
	body := "database_url: postgres://x\nmetrics_token: s3cret-token\n"
	_, err := Load(withFile(writeConfig(t, body, 0o644), nil))
	mustFail(t, err, "metrics_token", "chmod 600", "_file")
	if err != nil && strings.Contains(err.Error(), "s3cret-token") {
		t.Errorf("the error shows the secret: %v", err)
	}
	if _, err := Load(withFile(writeConfig(t, body, 0o600), nil)); err != nil {
		t.Errorf("the same file read by its owner alone must be accepted: %v", err)
	}
}

// The database address holds a password too.
func TestADatabaseAddressInAFileOthersCanReadIsRefused(t *testing.T) {
	_, err := Load(withFile(writeConfig(t, "database_url: postgres://u:pw@db/x\n", 0o640), nil))
	mustFail(t, err, "database_url", "chmod 600")
}

func TestAnEmptySecretNeedsNoPrivateFile(t *testing.T) {
	if _, err := Load(withFile(writeConfig(t, "database_url_file: /dev/null\nmetrics_token:\n", 0o644), nil)); err != nil &&
		strings.Contains(err.Error(), "chmod") {
		t.Errorf("an empty secret is no secret: %v", err)
	}
}

func TestTheReportNamesEverySourceAndHidesSecrets(t *testing.T) {
	p := writeConfig(t, "database_url: postgres://owner:dbpass@db:5432/wiki\nlog_level: debug\nmetrics_token: s3cret-token\nsmtp:\n  host: mail.example.com\n  from: w@example.com\n", 0o600)
	_, settings, err := Inspect(withFile(p, map[string]string{"DIFFLUENCE_LOG_LEVEL": "warn", "DIFFLUENCE_PDF_PASSWORD": "pdfpass"}))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Setting{}
	for _, s := range settings {
		got[s.Name] = s
		for _, secret := range []string{"dbpass", "s3cret-token", "pdfpass"} {
			if strings.Contains(s.Value, secret) {
				t.Errorf("%s shows a secret: %q", s.Name, s.Value)
			}
		}
	}
	for name, want := range map[string]struct{ value, source string }{
		"HTTP_ADDR":     {":8080", "default"},
		"SMTP_HOST":     {"mail.example.com", "file, line 5"},
		"LOG_LEVEL":     {"warn", "environment, over the file"},
		"METRICS_TOKEN": {"(set)", "file, line 3"},
		"PDF_PASSWORD":  {"(set)", "environment"},
		"COLLAB_TOKEN":  {"(not set)", "default"},
	} {
		s, ok := got[name]
		if !ok {
			t.Errorf("%s is missing from the report", name)
			continue
		}
		if s.Value != want.value || s.Source != want.source {
			t.Errorf("%s = %q from %q, want %q from %q", name, s.Value, s.Source, want.value, want.source)
		}
	}
	if db := got["DATABASE_URL"].Value; !strings.Contains(db, "db:5432") || strings.Contains(db, "dbpass") {
		t.Errorf("database address = %q, want the host without the password", db)
	}
}

// The example that ships must load as written, with its secrets in files of their own.
func TestTheExampleFileLoads(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "deploy", "diffluence.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	secrets := t.TempDir()
	for _, name := range []string{"database_url", "migrate_database_url", "smtp_password"} {
		if err := os.WriteFile(filepath.Join(secrets, name), []byte("postgres://x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	text := strings.ReplaceAll(string(body), "/run/secrets/", filepath.ToSlash(secrets)+"/")
	// The example may be read by others: it holds paths, not secrets.
	c, err := Load(withFile(writeConfig(t, text, 0o644), nil))
	if err != nil {
		t.Fatalf("the example is refused: %v", err)
	}
	if c.SMTP.Host != "mail.example.com" || c.PublicOrigin() != "https://wiki.example.com" {
		t.Errorf("the example reads as %+v", c)
	}
}

// Every setting Load asks for is a key of the file; there is no second list to forget.
func TestEverySettingIsAKeyOfTheFile(t *testing.T) {
	_, settings, err := Inspect(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) < 30 {
		t.Fatalf("only %d settings in the report", len(settings))
	}
	var body strings.Builder
	for _, s := range settings {
		if s.Name == "DATABASE_URL" {
			continue
		}
		body.WriteString(strings.ToLower(s.Name) + ":\n")
	}
	body.WriteString("database_url: postgres://x\n")
	if _, err := Load(withFile(writeConfig(t, body.String(), 0o600), nil)); err != nil {
		t.Errorf("a file naming every setting is refused: %v", err)
	}
}
