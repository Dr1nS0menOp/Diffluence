// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"bytes"
	"fmt"
	"strings"
)

const frontMatterFence = "---"

// frontMatter is what a Markdown file says about itself (D21).
type frontMatter struct {
	title  string
	labels []string
	status string
}

// splitFrontMatter reads a small subset: "key: value" and simple lists. It is not
// critical, so it needs no YAML module, and what it does not know is a warning.
func splitFrontMatter(data []byte) (frontMatter, []byte, []string) {
	var out frontMatter
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, frontMatterFence+"\n") {
		return out, data, nil
	}
	head, body, found := strings.Cut(text[len(frontMatterFence)+1:], "\n"+frontMatterFence)
	if !found {
		return out, data, nil
	}
	if rest, ok := strings.CutPrefix(body, "\n"); ok {
		body = rest
	} else if body != "" {
		return out, data, nil
	}
	var warnings []string
	listKey := ""
	for _, line := range strings.Split(head, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if item, isItem := strings.CutPrefix(trimmed, "- "); isItem && listKey != "" {
			out.add(listKey, cleanValue(item))
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			warnings = append(warnings, fmt.Sprintf("The line %q in the front matter was not understood.", truncate(trimmed, maxReportedName)))
			continue
		}
		listKey = ""
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		switch key {
		case "title", "status", "created", "updated":
			out.set(key, cleanValue(value))
		case "labels", "tags":
			listKey = "labels"
			for _, item := range splitList(value) {
				out.add(listKey, item)
			}
		default:
			warnings = append(warnings, fmt.Sprintf("The front matter key %q was left out.", truncate(key, maxReportedName)))
		}
	}
	return out, []byte(body), warnings
}

func (f *frontMatter) set(key, value string) {
	switch key {
	case "title":
		f.title = value
	case "status":
		f.status = value
	}
}

func (f *frontMatter) add(key, value string) {
	if key == "labels" && value != "" {
		f.labels = append(f.labels, value)
	}
}

func cleanValue(value string) string { return strings.Trim(strings.TrimSpace(value), `"'`) }

func splitList(value string) []string {
	value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = cleanValue(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
