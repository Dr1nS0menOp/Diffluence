// SPDX-License-Identifier: Apache-2.0

package document

import "slices"

// The attributes of a document that name a page, a space or a file. A release of a page
// into a published space rewrites every one of them (spec 073, PUB-502, decision D7), so
// they are one list, next to LinkedPages; a test over the block schema fails when an
// attribute that holds an id is added without being listed here.

// pageAttributes names, per node type, the attribute that holds the id of a page.
var pageAttributes = map[string]string{
	TypePageLink: "page_id",
	TypePageCard: "page_id",
}

// PageAttribute is the attribute of a node type that holds a page id, if it has one.
func PageAttribute(nodeType string) (string, bool) {
	key, ok := pageAttributes[nodeType]
	return key, ok
}

// FileAttribute is the attribute that holds the id of a file, on every node that shows one.
const FileAttribute = "attachment_id"

// fileNodes are the node types that show a file.
var fileNodes = []string{TypeImage, TypeInlineImage, TypeFile, TypeMedia}

// ShowsFile reports whether a node type points at a file through FileAttribute.
func ShowsFile(nodeType string) bool { return slices.Contains(fileNodes, nodeType) }

// otherIDAttributes hold ids that name neither a page nor a space nor a file: a task and a
// person or group. A release keeps them as they are (PUB-533).
var otherIDAttributes = map[string][]string{
	TypeTaskItem: {"id"},
	TypeMention:  {"id"},
}

// spaceKeySettings are the settings of dynamic blocks that list spaces by key.
var spaceKeySettings = []string{"spaces"}

// DynamicPageSettings lists the settings of a kind of dynamic block that hold one page, and
// those that hold a list of pages. They are read from the settings of the kind, so a new
// setting of a page is in the list the moment it exists.
func DynamicPageSettings(kind string) (single, lists []string) {
	for key, rule := range dynamicKinds[kind] {
		switch rule.kind {
		case settingPage:
			single = append(single, key)
		case settingPages:
			lists = append(lists, key)
		}
	}
	slices.Sort(single)
	slices.Sort(lists)
	return single, lists
}

// DynamicSpaceSettings lists the settings of a kind of dynamic block that name spaces by key.
func DynamicSpaceSettings(kind string) []string {
	var out []string
	for _, key := range spaceKeySettings {
		if _, ok := dynamicKinds[kind][key]; ok {
			out = append(out, key)
		}
	}
	return out
}
