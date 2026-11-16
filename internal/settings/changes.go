// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"encoding/json"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"unicode/utf8"
)

// What an audit row keeps of a settings change: each changed field with its value before and
// after, for the fields listed as safe; every other field only as changed (spec 090).

// SafeFields names the fields of a stored setting whose values an audit row may keep. A field
// it does not name is recorded as changed, without its values: deny by default.
type SafeFields []string

// WholeValue is the one field of a setting stored as a single value, such as a number.
const WholeValue = "value"

// maxAuditValueRunes keeps an audit row small: a longer value is cut there and marked.
const maxAuditValueRunes = 200

// The fields of each section whose values an audit row keeps.
var sectionSafeFields = map[string]SafeFields{
	SectionGeneral:   {"default_language", "default_time_zone", "help_url", "embed_hosts", "working_days"},
	SectionFeatures:  {"comments", "reactions", "blogs", "invitations", "personal_spaces", "self_signup", "analytics", "automation", "tokens", "presence"},
	SectionAccess:    {"second_factor", "idle_minutes", "signup_domains", "guest_invites"},
	SectionRetention: {"audit_days", "longer_because", "lowering"},
	SectionLook: {"instance_name", "accent_light", "accent_dark", "login_welcome", "terms_url", "banner_text", "banner_level",
		"banner_ends", "welcome_page"},
	SectionBackups: {"interval_hours", "keep"},
	SectionHealth:  {"disk_warn_percent", "disk_refuse_percent"},
}

// sectionRedactedFields are the fields of a section recorded as changed without their values
// on purpose. The organisation name may be the name of the person an instance is for.
var sectionRedactedFields = map[string][]string{
	SectionLook: {"organisation_name"},
}

// valuesSafeFields are the console's general values, none of which holds a secret or a name.
var valuesSafeFields = SafeFields{keyTrashRetentionDays, keyMaxUploadMiB, keyMailEnabled, keyMailExcerpts,
	keyReactionsEnabled, keyGroupMentions, keyBlockedFileTypes}

// SectionChanges is Changes with the safe fields of a section.
func SectionChanges(name string, before, after any) (map[string]any, error) {
	return Changes(before, after, sectionSafeFields[name])
}

// Changes compares two values of a setting as their JSON gives them, and returns what an audit
// row keeps of each field that differs: {"before", "after"} for a safe field, {"redacted": true}
// for any other. A value that is no object counts as one field named WholeValue; a nil before
// is a setting that was not stored yet.
func Changes(before, after any, safe SafeFields) (map[string]any, error) {
	old, err := jsonFieldsOf(before)
	if err != nil {
		return nil, err
	}
	fresh, err := jsonFieldsOf(after)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(old)+len(fresh))
	for name := range old {
		names = append(names, name)
	}
	for name := range fresh {
		if _, ok := old[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := map[string]any{}
	for _, name := range names {
		was, is := old[name], fresh[name]
		if reflect.DeepEqual(was, is) {
			continue
		}
		if !slices.Contains(safe, name) {
			out[name] = map[string]any{"redacted": true}
			continue
		}
		change := map[string]any{}
		var cutBefore, cutAfter bool
		change["before"], cutBefore = auditValue(was)
		change["after"], cutAfter = auditValue(is)
		if cutBefore || cutAfter {
			change["truncated"] = true
		}
		out[name] = change
	}
	return out, nil
}

// jsonFieldsOf decodes a value through its JSON, so a stored value and a struct compare alike.
func jsonFieldsOf(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{}, nil
	}
	raw, ok := v.([]byte)
	if !ok {
		var err error
		if raw, err = json.Marshal(v); err != nil {
			return nil, err
		}
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	if fields, ok := decoded.(map[string]any); ok {
		return fields, nil
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	return map[string]any{WholeValue: decoded}, nil
}

// auditValue gives a value as an audit row keeps it, and whether it was cut: an address
// without its user and password, and nothing longer than maxAuditValueRunes.
func auditValue(v any) (any, bool) {
	if s, ok := v.(string); ok {
		return cutText(withoutCredentials(s))
	}
	if v == nil {
		return nil, false
	}
	raw, err := json.Marshal(v)
	if err != nil || utf8.RuneCount(raw) <= maxAuditValueRunes {
		return v, false
	}
	return cutText(string(raw))
}

func cutText(s string) (string, bool) {
	if utf8.RuneCountInString(s) <= maxAuditValueRunes {
		return s, false
	}
	return string([]rune(s)[:maxAuditValueRunes]) + "…", true
}

// withoutCredentials removes a user name and password from an address; other text stays.
func withoutCredentials(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.User == nil || u.Host == "" {
		return s
	}
	u.User = nil
	return u.String()
}
