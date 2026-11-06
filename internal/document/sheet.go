// SPDX-License-Identifier: Apache-2.0

package document

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// TypeSheet is the one block of a database: a simple spreadsheet (spec 019).
const TypeSheet = "sheet"

// Limits of a sheet. A document has its own limit of 5 MiB on top of these.
const (
	maxSheetColumns  = 100
	maxSheetRows     = 5000
	maxSheetCell     = 2000
	maxSheetName     = 100
	maxSheetOptions  = 50
	sheetTrue        = "true"
	sheetColumnText  = "text"
	sheetColumnLink  = "link"
	sheetColumnDate  = "date"
	sheetColumnCheck = "check"
)

var (
	sheetColumnTypes = choice{sheetColumnText: true, "number": true, sheetColumnDate: true, sheetColumnCheck: true, "choice": true, sheetColumnLink: true}
	sheetID          = regexp.MustCompile(`^[a-zA-Z0-9-]{1,40}$`)
)

type sheetColumn struct {
	id, name, kind string
}

func cleanSheetName(v any) (string, error) {
	s, _ := v.(string)
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > maxSheetName || strings.ContainsAny(s, "\r\n\x00") {
		return "", errors.New("a column needs a name of at most 100 characters")
	}
	return s, nil
}

func cleanSheetOptions(v any) ([]any, error) {
	list, _ := v.([]any)
	if len(list) > maxSheetOptions {
		return nil, errors.New("a choice column has at most 50 options")
	}
	out := make([]any, 0, len(list))
	for _, item := range list {
		name, err := cleanSheetName(item)
		if err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, nil
}

func cleanSheetColumns(v any) ([]any, []sheetColumn, error) {
	list, _ := v.([]any)
	if len(list) == 0 || len(list) > maxSheetColumns {
		return nil, nil, errors.New("a sheet has 1 to 100 columns")
	}
	out := make([]any, 0, len(list))
	columns := make([]sheetColumn, 0, len(list))
	seen := map[string]bool{}
	for _, item := range list {
		in, _ := item.(map[string]any)
		id, _ := in["id"].(string)
		kind, _ := in["type"].(string)
		if !sheetID.MatchString(id) || seen[id] || !sheetColumnTypes[kind] {
			return nil, nil, errors.New("a column needs its own id and a known type")
		}
		seen[id] = true
		name, err := cleanSheetName(in["name"])
		if err != nil {
			return nil, nil, err
		}
		column := map[string]any{"id": id, "name": name, "type": kind}
		if in["options"] != nil {
			options, err := cleanSheetOptions(in["options"])
			if err != nil {
				return nil, nil, err
			}
			column["options"] = options
		}
		out = append(out, column)
		columns = append(columns, sheetColumn{id: id, name: name, kind: kind})
	}
	return out, columns, nil
}

// cleanSheetCell checks one value. A value starting with = is a formula, which is
// only text here: the browser works it out, and it can run nothing.
func cleanSheetCell(column sheetColumn, v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", errors.New("a cell holds text")
	}
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxSheetCell || strings.Contains(s, "\x00") {
		return "", errors.New("a cell holds at most 2000 characters")
	}
	if s == "" || strings.HasPrefix(s, "=") && column.kind != sheetColumnLink {
		return s, nil
	}
	switch column.kind {
	case sheetColumnLink:
		href, err := SafeHref(s)
		if err != nil {
			return "", fmt.Errorf("column %s: %w", column.name, err)
		}
		return href, nil
	case sheetColumnDate:
		if _, err := time.Parse(time.DateOnly, s); err != nil {
			return "", fmt.Errorf("column %s: a date is written as 2026-11-06", column.name)
		}
	case sheetColumnCheck:
		if s != sheetTrue {
			return "", fmt.Errorf("column %s: a tick is true or empty", column.name)
		}
	}
	return s, nil
}

func cleanSheetRow(columns []sheetColumn, v any, seen map[string]bool) (map[string]any, error) {
	in, _ := v.(map[string]any)
	id, _ := in["id"].(string)
	if !sheetID.MatchString(id) || seen[id] {
		return nil, errors.New("a row needs its own id")
	}
	seen[id] = true
	given, _ := in["cells"].(map[string]any)
	cells := map[string]any{}
	for _, column := range columns {
		value, found := given[column.id]
		if !found {
			continue
		}
		s, err := cleanSheetCell(column, value)
		if err != nil {
			return nil, err
		}
		if s != "" {
			cells[column.id] = s
		}
	}
	return map[string]any{"id": id, "cells": cells}, nil
}

func cleanSheet(in, out map[string]any) error {
	columns, known, err := cleanSheetColumns(in["columns"])
	if err != nil {
		return err
	}
	list, _ := in["rows"].([]any)
	if len(list) > maxSheetRows {
		return errors.New("a sheet has at most 5000 rows")
	}
	rows := make([]any, 0, len(list))
	seen := map[string]bool{}
	for _, item := range list {
		row, err := cleanSheetRow(known, item, seen)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	out["columns"], out["rows"] = columns, rows
	return nil
}

// HasSheet reports whether a document holds a sheet. A sheet only sits at the top.
func HasSheet(n Node) bool {
	return slices.ContainsFunc(n.Content, func(block Node) bool { return block.Type == TypeSheet })
}

// IsSheetDocument reports whether a document is one sheet and nothing else, which
// is what a database holds.
func IsSheetDocument(n Node) bool {
	return len(n.Content) == 1 && n.Content[0].Type == TypeSheet
}

// EmptySheet is the document of a new database: two columns and three rows to type in.
func EmptySheet(name, value string) Node {
	row := func(id string) any { return map[string]any{"id": id, "cells": map[string]any{}} }
	return Node{Type: TypeDoc, Content: []Node{{Type: TypeSheet, Attrs: map[string]any{
		"columns": []any{
			map[string]any{"id": "a", "name": name, "type": sheetColumnText},
			map[string]any{"id": "b", "name": value, "type": sheetColumnText},
		},
		"rows": []any{row("r1"), row("r2"), row("r3")},
	}}}}
}

// sheetLines gives the header and the rows of a sheet as text, one slice per line.
func sheetLines(n Node) [][]string {
	columns, _ := n.Attrs["columns"].([]any)
	rows, _ := n.Attrs["rows"].([]any)
	ids := make([]string, 0, len(columns))
	header := make([]string, 0, len(columns))
	for _, c := range columns {
		column, _ := c.(map[string]any)
		id, _ := column["id"].(string)
		name, _ := column["name"].(string)
		ids = append(ids, id)
		header = append(header, name)
	}
	lines := [][]string{header}
	for _, r := range rows {
		row, _ := r.(map[string]any)
		cells, _ := row["cells"].(map[string]any)
		line := make([]string, 0, len(ids))
		for _, id := range ids {
			value, _ := cells[id].(string)
			line = append(line, value)
		}
		lines = append(lines, line)
	}
	return lines
}

func sheetText(n Node) string {
	var b strings.Builder
	for _, line := range sheetLines(n) {
		b.WriteString(strings.Join(line, " "))
		b.WriteByte('\n')
	}
	return b.String()
}

func sheetMarkdown(n Node) string {
	var b strings.Builder
	escape := strings.NewReplacer("|", `\|`, "\n", " ")
	for i, line := range sheetLines(n) {
		b.WriteString("|")
		for _, cell := range line {
			b.WriteString(" " + escape.Replace(cell) + " |")
		}
		b.WriteByte('\n')
		if i == 0 {
			b.WriteString("|" + strings.Repeat(" --- |", len(line)) + "\n")
		}
	}
	return b.String()
}
