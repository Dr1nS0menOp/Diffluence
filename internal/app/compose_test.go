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

// The server holds only the application role; the backup service alone holds the owner of
// the tables, and migrates (spec 070).
func TestOnlyTheBackupServiceHoldsTheOwnerOfTheTables(t *testing.T) {
	raw, err := os.ReadFile("../../compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	service := func(name string) string {
		found := regexp.MustCompile(`(?m)^  ` + name + `:\n(?:(?:    .*|\s*)\n)*`).FindString(string(raw))
		if found == "" {
			t.Fatalf("no service %s", name)
		}
		return found
	}
	server, backup := service("diffluence"), service("backup")
	if strings.Contains(server, "migrate_database_url") || strings.Contains(server, "MIGRATE_DATABASE_URL") {
		t.Errorf("the server holds the owner of the tables:\n%s", server)
	}
	if !strings.Contains(server, `DIFFLUENCE_MIGRATE_ON_START: "false"`) {
		t.Error("the server would try to migrate as the application role")
	}
	if !strings.Contains(backup, "migrate_database_url") {
		t.Error("the backup service cannot migrate")
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
