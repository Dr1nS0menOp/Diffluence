// SPDX-License-Identifier: Apache-2.0

package document

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Node types of wave E (spec 012).
const (
	TypeDynamic   = "dynamic"
	TypePageFacts = "pageFacts"
	TypeExcerpt   = "excerpt"
	TypeAnchor    = "anchor"
	TypeChart     = "chart"
	TypeMedia     = "media"
)

// Kinds of dynamic block that reuse text from another page.
const (
	KindShowExcerpt  = "show_excerpt"
	KindEmbeddedPage = "embedded_page"
	KindSyncedBlock  = "synced_block"
)

const (
	maxSettingBytes = 200
	maxSettingItems = 50
	maxNameBytes    = 64
	maxFactKeyBytes = 200
	maxFactBytes    = 1000
	maxChartSeries  = 12
)

// setting says what one setting of a dynamic block may hold. A setting that is not
// on the list of its kind is dropped, so a document can only carry data we know.
type setting struct {
	kind     settingKind
	min, max int
	choices  choice
}

type settingKind int

const (
	settingText settingKind = iota
	settingList
	settingNumber
	settingSwitch
	settingPage
	settingPages
	settingChoice
	settingDate
)

func number(lo, hi int) setting { return setting{kind: settingNumber, min: lo, max: hi} }

func oneOf(names ...string) setting {
	return setting{kind: settingChoice, choices: choices(names...)}
}

var (
	freeText  = setting{kind: settingText}
	day       = setting{kind: settingDate}
	list      = setting{kind: settingList}
	on        = setting{kind: settingSwitch}
	page      = setting{kind: settingPage}
	pageList  = setting{kind: settingPages}
	sortOrder = oneOf("tree", "title", "created", "changed")
	listStyle = oneOf("list", "cards")
)

// pageFilter is shared by every block that lists pages (spec 012, E5).
func pageFilter(extra map[string]setting) map[string]setting {
	out := map[string]setting{
		"labels": list, "exclude_labels": list, "title": freeText, "below": page, "this_space": on,
		"sort": sortOrder, "descending": on, "max": number(1, 500), "hide_empty": on, "heading": freeText,
		"spaces": list, "content_type": oneOf("page", "post", "all"), "text": freeText, "creator": freeText,
		"contributor": freeText, "mentioned": freeText, "created_after": day, "changed_after": day, "direct": on,
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

// dynamicKinds lists every kind of dynamic block and its settings. A new kind is a
// line here and a line in the blocks package.
var dynamicKinds = map[string]map[string]setting{
	"toc": {"from": number(1, 6), "to": number(1, 6), "style": oneOf("list", "line"),
		"numbered": on, "exclude": freeText, "no_print": on},
	"section_contents": {"start": freeText, "end": freeText, "section": freeText, "from": number(1, 6), "to": number(1, 6)},
	"back_to_top":      {},
	"search_box": {"placeholder": freeText, "this_space": on, "below": page,
		"content_type": oneOf("page", "post"), "labels": list},
	"child_pages": {"page": page, "max": number(1, 500), "levels": number(1, 9), "sort": sortOrder,
		"descending": on, "preview": on, "hide_empty": on},
	"page_tree":   {"page": page},
	"space_index": {},
	"spaces_list": {"favourites": on, "style": listStyle, "categories": list},
	"breadcrumbs": {"page": page},
	"recent_changes": {"max": number(1, 200), "this_space": on, "hide_empty": on, "heading": freeText,
		"spaces": list},
	"activity_tabs":      {"max": number(1, 200), "scope": oneOf("all", "favourites", "network")},
	"popular_pages":      {"max": number(1, 50), "days": number(1, 365), "this_space": on},
	"my_favourites":      {"max": number(1, 200)},
	"my_drafts":          {"max": number(1, 200)},
	"my_tasks":           {"max": number(1, 200)},
	"filter_list":        pageFilter(map[string]setting{"preview": on}),
	"report_table":       pageFilter(map[string]setting{"columns": list}),
	"labels_list":        {"this_space": on, "max": number(1, 500)},
	"popular_labels":     {"this_space": on, "max": number(1, 100), "style": oneOf("list", "cloud")},
	"related_labels":     {"page": page, "max": number(1, 100)},
	"recent_labels":      {"this_space": on, "max": number(1, 100)},
	"facts_report":       pageFilter(map[string]setting{"facts_id": freeText, "columns": list, "sort_key": freeText}),
	"decision_report":    pageFilter(map[string]setting{"page": page, "person": freeText}),
	"task_report":        pageFilter(map[string]setting{"mine": on, "done": on, "created_by": freeText}),
	KindShowExcerpt:      {"page": page, "name": freeText, "frame": on, "values": list},
	KindSyncedBlock:      {"page": page, "name": freeText, "values": list},
	KindEmbeddedPage:     {"page": page},
	"contributors":       {"page": page, "max": number(1, 100), "kinds": list},
	"contribution_table": {"page": page, "by": oneOf("person", "page"), "max": number(1, 100)},
	"person_card":        {"person": freeText},
	"people_list":        {"group": freeText, "people": list, "style": oneOf("list", "grid"), "max": number(1, 100)},
	"history_here":       {"page": page, "max": number(1, 50)},
	"attachments_here": {"page": page, "pattern": freeText, "max": number(1, 200),
		"sort": oneOf("name", "date", "size"), "labels": list},
	"space_files": {"labels": list, "file_type": freeText, "max": number(1, 200)},
	"blog_posts": {"max": number(1, 50), "this_space": on, "labels": list, "days": number(1, 3650),
		"authors": list},
	"create_from_template": {"template": page, "parent": page, "label": freeText},
	"link_cards":           pageFilter(map[string]setting{"pages": pageList}),
	"rotating_cards":       pageFilter(map[string]setting{"pages": pageList}),
	"timeline":             {"items": list},
}

// DynamicKinds lists the kinds of dynamic block.
func DynamicKinds() []string {
	out := make([]string, 0, len(dynamicKinds))
	for name := range dynamicKinds {
		out = append(out, name)
	}
	return out
}

// IsDynamicKind reports whether a kind exists.
func IsDynamicKind(kind string) bool {
	_, ok := dynamicKinds[kind]
	return ok
}

func cleanSetting(rule setting, v any) (any, bool) {
	switch rule.kind {
	case settingText:
		s, ok := v.(string)
		if !ok || s == "" || !utf8.ValidString(s) {
			return nil, false
		}
		return truncate(s, maxSettingBytes), true
	case settingList:
		return cleanStrings(v)
	case settingNumber:
		n, ok := intAttr(v)
		return n, ok && n >= rule.min && n <= rule.max
	case settingSwitch:
		b, _ := v.(bool)
		return true, b
	case settingPage:
		s, _ := v.(string)
		id, err := uuid.Parse(s)
		return id.String(), err == nil
	case settingPages:
		return cleanPageIDs(v)
	case settingChoice:
		s, _ := v.(string)
		return s, rule.choices[s]
	case settingDate:
		s, _ := v.(string)
		_, err := time.Parse(time.DateOnly, s)
		return s, err == nil
	}
	return nil, false
}

func cleanStrings(v any) (any, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := []any{}
	for _, item := range items {
		s, ok := item.(string)
		if ok && s != "" && utf8.ValidString(s) && len(out) < maxSettingItems {
			out = append(out, truncate(s, maxSettingBytes))
		}
	}
	return out, len(out) > 0
}

func cleanPageIDs(v any) (any, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := []any{}
	for _, item := range items {
		s, _ := item.(string)
		if id, err := uuid.Parse(s); err == nil && len(out) < maxSettingItems {
			out = append(out, id.String())
		}
	}
	return out, len(out) > 0
}

// CleanSettings keeps the settings a kind knows, with valid values. The blocks route
// uses it too, so an answer never rests on a value the gate would refuse.
func CleanSettings(kind string, in map[string]any) map[string]any {
	out := map[string]any{}
	for key, rule := range dynamicKinds[kind] {
		if v, present := in[key]; present {
			if clean, ok := cleanSetting(rule, v); ok {
				out[key] = clean
			}
		}
	}
	return out
}

func cleanDynamic(in, out map[string]any) error {
	kind, _ := in["kind"].(string)
	if !IsDynamicKind(kind) {
		return errors.New("that is not a kind of dynamic block")
	}
	out["kind"] = kind
	settings, _ := in["settings"].(map[string]any)
	if clean := CleanSettings(kind, settings); len(clean) > 0 {
		out["settings"] = clean
	}
	blockWidths.keep(in, out, "width")
	return nil
}

var blockWidths = choices("wide", "full")

var excerptName = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} _-]{0,63}$`)

func cleanName(in, out map[string]any, what string) error {
	name, _ := in["name"].(string)
	if !excerptName.MatchString(name) {
		return errors.New(what + " needs a short name of letters and digits")
	}
	out["name"] = name
	return nil
}

func cleanExcerpt(in, out map[string]any) error {
	if err := cleanName(in, out, "an excerpt"); err != nil {
		return err
	}
	keepTrue(in, out, "hidden")
	keepTrue(in, out, "synced")
	return nil
}

// AnchorName is how a name becomes an address: lower case, spaces become dashes.
func AnchorName(name string) string {
	return truncate(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "-"), maxNameBytes)
}

func cleanAnchor(in, out map[string]any) error {
	if err := cleanName(in, out, "an anchor"); err != nil {
		return err
	}
	out["name"] = AnchorName(out["name"].(string))
	return nil
}

func cleanPageFacts(in, out map[string]any) error {
	keepText(in, out, "facts_id", maxNameBytes)
	keepTrue(in, out, "hidden")
	return nil
}

var (
	chartKinds   = choices("bar", "line", "pie", "area")
	chartTotals  = choices("sum", "average", "count")
	chartHeights = choices("small", "large")
	chartLegends = choices("top", "bottom", "start", "end")
	mediaKinds   = choices("video", "audio")
	blockIDRule  = regexp.MustCompile(`^[a-z0-9]{8,12}$`)
)

func cleanChart(in, out map[string]any) error {
	table, _ := in["table"].(string)
	if !blockIDRule.MatchString(table) {
		return errors.New("a chart must point to a table")
	}
	out["table"] = table
	out["kind"] = "bar"
	chartKinds.keep(in, out, "kind")
	keepText(in, out, "title", maxTitleBytes)
	keepTrue(in, out, "legend")
	keepTrue(in, out, "no_legend")
	keepTrue(in, out, "smooth")
	keepTrue(in, out, "points")
	chartTotals.keep(in, out, "total")
	chartHeights.keep(in, out, "height")
	chartLegends.keep(in, out, "legend_position")
	keepText(in, out, "x_title", maxTitleBytes)
	keepText(in, out, "y_title", maxTitleBytes)
	if axis, ok := intAttr(in["axis"]); ok && axis >= 0 && axis < 1000 {
		out["axis"] = axis
	}
	if series := columnNumbers(in["series"]); series != nil {
		out["series"] = series
	}
	return nil
}

func columnNumbers(v any) []any {
	items, ok := v.([]any)
	if !ok || len(items) == 0 || len(items) > maxChartSeries {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		n, ok := intAttr(item)
		if !ok || n < 0 || n >= 1000 {
			return nil
		}
		out = append(out, n)
	}
	return out
}

func cleanMedia(in, out map[string]any) error {
	id, ok := attachmentID(in)
	if !ok {
		return errors.New("media must point to an attachment")
	}
	out["attachment_id"] = id
	mediaKinds.keep(in, out, "kind")
	if out["kind"] == nil {
		return errors.New("media is video or audio")
	}
	keepText(in, out, "caption", maxCaptionBytes)
	return nil
}

func keepBlockID(in, out map[string]any) {
	if id, _ := in["block_id"].(string); blockIDRule.MatchString(id) {
		out["block_id"] = id
	}
}

// Fact is one row of a page facts block.
type Fact struct {
	FactsID string
	Key     string
	Text    string
	Date    *time.Time
	Number  *float64
}

func factValue(cell Node) Fact {
	f := Fact{Text: truncate(strings.TrimSpace(BlockText(cell)), maxFactBytes)}
	Walk(cell, func(c Node) {
		if c.Type != TypeDate {
			return
		}
		if value, _ := c.Attrs["value"].(string); value != "" {
			if d, err := time.Parse(time.DateOnly, value); err == nil {
				f.Date = &d
			}
		}
	})
	if n, err := strconv.ParseFloat(strings.ReplaceAll(f.Text, ",", "."), 64); err == nil {
		f.Number = &n
	}
	return f
}

// Facts lists the rows of all page facts blocks in a document, in order.
func Facts(doc Node) []Fact {
	out := []Fact{}
	Walk(doc, func(n Node) {
		if n.Type != TypePageFacts {
			return
		}
		id, _ := n.Attrs["facts_id"].(string)
		Walk(n, func(row Node) {
			if row.Type != TypeTableRow || len(row.Content) < 2 {
				return
			}
			key := truncate(strings.TrimSpace(BlockText(row.Content[0])), maxFactKeyBytes)
			if key == "" {
				return
			}
			f := factValue(row.Content[1])
			f.FactsID, f.Key = id, key
			out = append(out, f)
		})
	})
	return out
}

// Decisions lists the words of each decision in a document.
func Decisions(doc Node) []string {
	out := []string{}
	Walk(doc, func(n Node) {
		if n.Type == TypeDecision {
			if s := strings.TrimSpace(BlockText(n)); s != "" {
				out = append(out, s)
			}
		}
	})
	return out
}

// Include is one place where a document shows text from another page.
type Include struct {
	Page uuid.UUID
	Name string
}

// IncludeOf reads what a dynamic block shows from another page, if anything.
func IncludeOf(n Node) (Include, bool) {
	kind, _ := n.Attrs["kind"].(string)
	if n.Type != TypeDynamic || (kind != KindShowExcerpt && kind != KindEmbeddedPage && kind != KindSyncedBlock) {
		return Include{}, false
	}
	settings, _ := n.Attrs["settings"].(map[string]any)
	s, _ := settings["page"].(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return Include{}, false
	}
	name, _ := settings["name"].(string)
	return Include{Page: id, Name: name}, true
}

// Includes lists every include of a document.
func Includes(doc Node) []Include {
	out := []Include{}
	Walk(doc, func(n Node) {
		if include, ok := IncludeOf(n); ok {
			out = append(out, include)
		}
	})
	return out
}

// Excerpt returns the blocks of the first excerpt with that name.
func Excerpt(doc Node, name string) ([]Node, bool) {
	var found []Node
	ok := false
	Walk(doc, func(n Node) {
		if !ok && n.Type == TypeExcerpt && n.Attrs["name"] == name {
			found, ok = n.Content, true
		}
	})
	return found, ok
}
