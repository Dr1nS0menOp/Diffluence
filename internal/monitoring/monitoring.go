// SPDX-License-Identifier: Apache-2.0

// Package monitoring holds the Prometheus rules and the Grafana dashboard of this release, so
// the console can offer them as files (spec 075, CSL-585 and CSL-586). They are copies of
// deploy/monitoring/, which a test keeps equal.
package monitoring

import (
	_ "embed" // the files below are part of the binary
)

//go:embed kenwerk-rules.yml
var rules []byte

//go:embed kenwerk-dashboard.json
var dashboard []byte

// File is one file to download.
type File struct {
	Kind        string
	Name        string
	ContentType string
	Body        []byte
}

// Files are the monitoring files, by the kind the console names them with.
func Files() []File {
	return []File{
		{Kind: "rules", Name: "kenwerk-rules.yml", ContentType: "application/yaml", Body: rules},
		{Kind: "dashboard", Name: "kenwerk-dashboard.json", ContentType: "application/json", Body: dashboard},
	}
}

// ByKind is the file of a kind, rules or dashboard.
func ByKind(kind string) (File, bool) {
	for _, f := range Files() {
		if f.Kind == kind {
			return f, true
		}
	}
	return File{}, false
}
