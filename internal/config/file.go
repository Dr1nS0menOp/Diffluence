// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// fileEntry is one setting written in the configuration file.
type fileEntry struct {
	text string // the value as written: never YAML's guess of a type (spec 056, decision 4)
	line int    // where its key stands, for every message about it
	key  string // the key as written, nested keys joined with _
}

// configFile is the configuration file named by DIFFLUENCE_CONFIG_FILE.
type configFile struct {
	path    string
	entries map[string]fileEntry // by the name of the setting, such as SMTP_HOST or SMTP_PASSWORD_FILE
	// private reports whether only the owner of the file may read it, which a secret
	// written in the file itself requires (spec 056, decision 5).
	private bool
}

// keyName is what a key of the file may hold; nesting adds the _.
var keyName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// readConfigFile reads and parses the file. It returns every problem it finds, each with the
// line, rather than stopping at the first.
func readConfigFile(path string) (configFile, []error) {
	f := configFile{path: path, entries: map[string]fileEntry{}}
	info, err := os.Stat(path)
	if err != nil {
		return f, []error{fmt.Errorf("%sCONFIG_FILE: %w", prefix, err)}
	}
	f.private = info.Mode().Perm()&0o077 == 0
	body, err := os.ReadFile(path)
	if err != nil {
		return f, []error{fmt.Errorf("%sCONFIG_FILE: %w", prefix, err)}
	}
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(body))
	switch err := dec.Decode(&doc); {
	case errors.Is(err, io.EOF):
		return f, nil // an empty file, or only comments
	case err != nil:
		return f, []error{fmt.Errorf("config file: %w", err)}
	}
	var more yaml.Node
	if err := dec.Decode(&more); !errors.Is(err, io.EOF) {
		return f, []error{fmt.Errorf("config file line %d: holds more than one document", more.Line)}
	}
	if len(doc.Content) == 0 || isNull(doc.Content[0]) {
		return f, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return f, []error{fmt.Errorf("config file line %d: must be a list of settings, such as log_level: info", root.Line)}
	}
	var errs []error
	f.collect(root, "", &errs)
	return f, errs
}

// collect adds the settings of one mapping, joining the keys of nested mappings with _.
func (f *configFile) collect(m *yaml.Node, outer string, errs *[]error) {
	// A key written twice in one mapping, also in another case, is refused even when its
	// values are mappings whose settings differ (smtp: written twice).
	seen := map[string]int{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Kind != yaml.ScalarNode || !keyName.MatchString(k.Value) {
			*errs = append(*errs, fmt.Errorf("config file line %d: %q is not the name of a setting", k.Line, k.Value))
			continue
		}
		key := k.Value
		if outer != "" {
			key = outer + "_" + key
		}
		if line, ok := seen[strings.ToUpper(k.Value)]; ok {
			*errs = append(*errs, fmt.Errorf("config file line %d: %s is written more than once (also on line %d)", k.Line, key, line))
			continue
		}
		seen[strings.ToUpper(k.Value)] = k.Line
		switch v.Kind {
		case yaml.MappingNode:
			f.collect(v, key, errs)
			continue
		case yaml.ScalarNode:
		default:
			*errs = append(*errs, fmt.Errorf("config file line %d: %s must be one value, not a list or a reference", k.Line, key))
			continue
		}
		name := strings.ToUpper(key)
		if earlier, ok := f.entries[name]; ok {
			*errs = append(*errs, fmt.Errorf("config file line %d: %s is written more than once (also on line %d)", k.Line, key, earlier.line))
			continue
		}
		text := v.Value
		if isNull(v) {
			text = ""
		}
		f.entries[name] = fileEntry{text: text, line: k.Line, key: key}
	}
}

// isNull reports whether a node is YAML's empty value: nothing, ~ or null, unquoted.
func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}
