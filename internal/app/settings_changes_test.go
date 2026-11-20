// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kenwerk/kenwerk/internal/audit"
)

// lastSettingsChange gives the details of the newest settings.updated event.
func (e *env) lastSettingsChange() map[string]any {
	e.t.Helper()
	evs, err := audit.List(context.Background(), e.app.Pool(), audit.Filter{Action: "settings.updated", Limit: 1})
	if err != nil {
		e.t.Fatal(err)
	}
	if len(evs) == 0 {
		e.t.Fatal("no settings.updated event")
	}
	return evs[0].Details
}

// changesOf gives what the newest settings change recorded per field.
func (e *env) changesOf() map[string]any {
	e.t.Helper()
	changes, ok := e.lastSettingsChange()["changes"].(map[string]any)
	if !ok {
		e.t.Fatalf("the change records no fields: %v", e.lastSettingsChange())
	}
	return changes
}

func TestASettingsChangeRecordsEachChangedFieldBeforeAndAfter(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	general := root.ok("GET", "/api/v1/admin/settings/general", nil)
	general["default_time_zone"] = "Europe/Amsterdam"
	general["help_url"] = "https://help.example.com"
	root.ok("PUT", "/api/v1/admin/settings/general", general)

	want := map[string]any{
		"default_time_zone": map[string]any{"before": "UTC", "after": "Europe/Amsterdam"},
		"help_url":          map[string]any{"before": "", "after": "https://help.example.com"},
	}
	if got := e.changesOf(); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes %v, want %v", got, want)
	}
	if e.lastSettingsChange()["section"] != "general" {
		t.Fatalf("the section must stay named: %v", e.lastSettingsChange())
	}

	general["default_time_zone"] = "Europe/Berlin"
	root.ok("PUT", "/api/v1/admin/settings/general", general)
	want = map[string]any{"default_time_zone": map[string]any{"before": "Europe/Amsterdam", "after": "Europe/Berlin"}}
	if got := e.changesOf(); !reflect.DeepEqual(got, want) {
		t.Fatalf("a second change records only itself: %v", got)
	}
}

func TestTheOrganisationNameIsRecordedAsChangedWithoutItsValue(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	root.ok("PUT", "/api/v1/admin/settings/look", map[string]any{"organisation_name": "Jane Example", "instance_name": "Wiki"})
	changes := e.changesOf()
	if got := changes["organisation_name"]; !reflect.DeepEqual(got, map[string]any{"redacted": true}) {
		t.Fatalf("organisation name recorded as %v", got)
	}
	if got := changes["instance_name"]; !reflect.DeepEqual(got, map[string]any{"before": "", "after": "Wiki"}) {
		t.Fatalf("instance name recorded as %v", got)
	}
	if text := fmt.Sprint(e.lastSettingsChange()); strings.Contains(text, "Jane") {
		t.Fatalf("the name reached the audit row: %s", text)
	}
}

func TestAResetOfTheLookRecordsWhatItReset(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	root.ok("PUT", "/api/v1/admin/settings/look", map[string]any{"instance_name": "Team wiki", "banner_text": "Note", "banner_level": "info"})
	root.ok("POST", "/api/v1/admin/appearance/reset", nil)
	details := e.lastSettingsChange()
	if details["reset"] != true {
		t.Fatalf("a reset says so: %v", details)
	}
	want := map[string]any{
		"instance_name": map[string]any{"before": "Team wiki", "after": ""},
		"banner_text":   map[string]any{"before": "Note", "after": ""},
		"banner_level":  map[string]any{"before": "info", "after": ""},
	}
	if got := e.changesOf(); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes %v, want %v", got, want)
	}
}

func TestAStoredSettingRecordsItsValueBeforeAndAfter(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	values := root.ok("GET", "/api/v1/admin/organising", nil)
	values["draft_days"] = 60
	root.ok("PUT", "/api/v1/admin/organising", values)
	// The route writes three values; the default permission set comes last.
	evs, err := audit.List(context.Background(), e.app.Pool(), audit.Filter{Action: "settings.updated", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range evs {
		changes, _ := ev.Details["changes"].(map[string]any)
		if got, ok := changes["value"]; ok && ev.ObjectID == "draft_retention_days" {
			found = true
			if !reflect.DeepEqual(got, map[string]any{"before": nil, "after": float64(60)}) {
				t.Fatalf("draft retention recorded as %v", got)
			}
		}
	}
	if !found {
		t.Fatalf("no change of the draft retention: %v", evs)
	}
}

func TestTheGeneralValuesRecordWhatChanged(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	values := root.ok("GET", "/api/v1/admin/settings", nil)
	values["trash_retention_days"] = 7
	root.ok("PUT", "/api/v1/admin/settings", values)
	want := map[string]any{"trash_retention_days": map[string]any{"before": float64(30), "after": float64(7)}}
	if got := e.changesOf(); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes %v, want %v", got, want)
	}
}

func TestTheBackupScheduleRecordsWhatChanged(t *testing.T) {
	e := newEnv(t)
	d := heldDefaults(t)
	checkedIn(t, e, d)
	root := admin(e)
	root.ok("PUT", "/api/v1/admin/settings/backups", map[string]any{"interval_hours": 2, "keep": 60})
	want := map[string]any{
		"interval_hours": map[string]any{"before": float64(0), "after": float64(2)},
		"keep":           map[string]any{"before": float64(0), "after": float64(60)},
	}
	if got := e.changesOf(); !reflect.DeepEqual(got, want) {
		t.Fatalf("changes %v, want %v", got, want)
	}
}

func TestTheAppearanceAnswerIsKeptUntilASettingChanges(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	root.ok("PUT", "/api/v1/admin/settings/look", map[string]any{"instance_name": "First"})
	if got := root.ok("GET", "/api/v1/appearance", nil)["name"]; got != "First" {
		t.Fatalf("name %v", got)
	}
	// Behind the service's back, as another node would: this node keeps its copy.
	if _, err := e.app.Pool().Exec(context.Background(),
		`UPDATE settings SET value = jsonb_set(value, '{instance_name}', '"Elsewhere"') WHERE key = 'section.look'`); err != nil {
		t.Fatal(err)
	}
	if got := root.ok("GET", "/api/v1/appearance", nil)["name"]; got != "First" {
		t.Fatalf("the answer must come from the node's copy: %v", got)
	}
	root.ok("PUT", "/api/v1/admin/settings/look", map[string]any{"instance_name": "Second"})
	if got := root.ok("GET", "/api/v1/appearance", nil)["name"]; got != "Second" {
		t.Fatalf("a change on this node shows at once: %v", got)
	}
	root.ok("POST", "/api/v1/admin/appearance/reset", nil)
	if got := root.ok("GET", "/api/v1/appearance", nil)["name"]; got != "" {
		t.Fatalf("a reset shows at once: %v", got)
	}
}

func TestTheAppearanceAnswerIsLimitedPerAddressWithoutALogin(t *testing.T) {
	e := newEnvBehindProxy(t)
	anonymous := e.client()
	last := 0
	for range 601 {
		resp, _ := e.do(anonymous, "GET", "/api/v1/appearance", "", "X-Forwarded-For", "198.51.100.4")
		last = resp.StatusCode
		if last != http.StatusOK {
			if resp.Header.Get("Retry-After") == "" {
				t.Fatal("a refusal says when to try again")
			}
			break
		}
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("after 600 answers from one address: %d, want 429", last)
	}
	if resp, _ := e.do(anonymous, "GET", "/api/v1/appearance", "", "X-Forwarded-For", "192.0.2.8"); resp.StatusCode != http.StatusOK {
		t.Fatalf("another address: %d, want 200", resp.StatusCode)
	}
	root := admin(e)
	if status, _ := root.do("GET", "/api/v1/appearance", nil); status != http.StatusOK {
		t.Fatalf("someone logged in: %d, want 200", status)
	}
}

func TestATermsOrHelpAddressMayNotCarryAUserOrPassword(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	root.want(400, "PUT", "/api/v1/admin/settings/look", map[string]any{"terms_url": "https://name:secret@terms.example.com"})
	root.want(400, "PUT", "/api/v1/admin/settings/look", map[string]any{"terms_url": "https://bank.example@terms.example.com"})
	general := root.ok("GET", "/api/v1/admin/settings/general", nil)
	general["help_url"] = "https://name:secret@help.example.com"
	root.want(400, "PUT", "/api/v1/admin/settings/general", general)
	// An internal site without a certificate stays possible.
	general["help_url"] = "http://intranet/help"
	root.ok("PUT", "/api/v1/admin/settings/general", general)
	root.ok("PUT", "/api/v1/admin/settings/look", map[string]any{"terms_url": "http://intranet/terms"})
}
