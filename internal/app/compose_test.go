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
