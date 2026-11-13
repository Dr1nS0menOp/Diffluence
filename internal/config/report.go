// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Setting is one setting as diffluence config check shows it: the value in effect, with a
// secret shown as (set), and where the value came from.
type Setting struct {
	Name   string // the name without DIFFLUENCE_, such as SMTP_HOST
	Value  string
	Source string // default, "file, line N", environment, or "environment, over the file"
}

// secretKeys are the settings whose value must never be shown or written in a file
// others may read. The database addresses hold a password.
var secretKeys = map[string]bool{
	"DATABASE_URL": true, "MIGRATE_DATABASE_URL": true, "LISTEN_DATABASE_URL": true, "SMTP_PASSWORD": true,
	"OIDC_CLIENT_SECRET": true, "COLLAB_TOKEN": true, "METRICS_TOKEN": true, "PDF_PASSWORD": true,
}

// addressKeys are the secrets whose value is an address worth seeing without its password.
var addressKeys = map[string]bool{"DATABASE_URL": true, "MIGRATE_DATABASE_URL": true, "LISTEN_DATABASE_URL": true}

// shown is the value of a setting as the report may show it.
func shown(key, value string) string {
	if !secretKeys[key] {
		return value
	}
	if value == "" {
		return "(not set)"
	}
	if addressKeys[key] {
		u, err := url.Parse(value)
		// A password can also stand in the query of a PostgreSQL address; then nothing shows.
		if err == nil && u.Scheme != "" && u.Host != "" && !u.Query().Has("password") {
			return u.Redacted()
		}
	}
	return "(set)"
}

func (o origin) describe() string {
	switch o.source {
	case sourceFile:
		return fmt.Sprintf("file, line %d", o.line)
	case sourceEnvironment:
		if o.overruns {
			return "environment, over the file"
		}
		return sourceEnvironment
	}
	return sourceDefault
}

// checkFile refuses what the configuration file holds that no setting reads, and a secret
// written in a file others may read (spec 056, decisions 2 and 5).
func (l *loader) checkFile() {
	if l.file == nil {
		return
	}
	names := make([]string, 0, len(l.file.entries))
	for name := range l.file.entries {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return l.file.entries[names[i]].line < l.file.entries[names[j]].line })
	for _, name := range names {
		e := l.file.entries[name]
		if !l.asked[name] {
			hint := ""
			if strings.HasPrefix(name, prefix) {
				hint = ": write the names in the file without " + prefix
			}
			l.errs = append(l.errs, fmt.Errorf("config file line %d: unknown setting %s%s", e.line, e.key, hint))
			continue
		}
		if secretKeys[name] && e.text != "" && !l.file.private {
			// The message names the key and never the value.
			l.errs = append(l.errs, fmt.Errorf("config file line %d: %s is a secret in a file others may read: "+
				"run chmod 600 on the file, or give the secret as %s_file", e.line, e.key, e.key))
		}
	}
}
