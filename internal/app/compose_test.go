// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// A helper reads hostile files, so it gets no way to the database or the internet (ARC-074).
func TestHelpersHaveNoWayOut(t *testing.T) {
	raw, err := os.ReadFile("../../compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !regexp.MustCompile(`(?m)^  helpers:\n    internal: true$`).MatchString(text) {
		t.Fatal("the network of the helpers must be internal")
	}
	services := regexp.MustCompile(`(?m)^  (pdf|collab):\n(?:(?:    .*|)\n)*`).FindAllString(text, -1)
	if len(services) != 2 {
		t.Fatalf("found %d helpers", len(services))
	}
	for _, service := range services {
		if !strings.Contains(service, "    networks: [helpers]\n") {
			t.Errorf("a helper can reach more than the app:\n%s", service)
		}
	}
}

// The app speaks plain http, so by default only a proxy on the same machine reaches it (SEC-627).
func TestEveryPublishedPortListensOnThisMachineByDefault(t *testing.T) {
	raw, err := os.ReadFile("../../compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ports := regexp.MustCompile(`(?m)^    ports: \[(.*)\]$`).FindAllStringSubmatch(string(raw), -1)
	if len(ports) == 0 {
		t.Fatal("no published port found")
	}
	for _, p := range ports {
		if !strings.HasPrefix(p[1], `"${DIFFLUENCE_BIND:-127.0.0.1}:`) {
			t.Errorf("a port listens on every address by default: %s", p[1])
		}
	}
}
