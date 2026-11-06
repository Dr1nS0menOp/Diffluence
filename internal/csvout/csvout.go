// SPDX-License-Identifier: Apache-2.0

// Package csvout writes CSV that a spreadsheet cannot run. Titles and search terms
// are written by people, and a cell starting with = runs as a formula (spec 015, D8).
package csvout

import (
	"encoding/csv"
	"io"
	"strings"
	"unicode/utf8"
)

// dangerousStarts are what a spreadsheet treats as the start of a formula, also in
// their full-width forms.
const dangerousStarts = "=+-@\t\r\n＝＋－＠"

// Cell makes one value safe to open in a spreadsheet.
func Cell(s string) string {
	first, size := utf8.DecodeRuneInString(s)
	if size > 0 && strings.ContainsRune(dangerousStarts, first) {
		return "'" + s
	}
	return s
}

// Writer writes guarded rows.
type Writer struct{ w *csv.Writer }

// New starts a CSV with its header.
func New(w io.Writer, header ...string) (*Writer, error) {
	out := &Writer{w: csv.NewWriter(w)}
	return out, out.Row(header...)
}

// Row writes one row.
func (w *Writer) Row(cells ...string) error {
	safe := make([]string, len(cells))
	for i, c := range cells {
		safe[i] = Cell(c)
	}
	return w.w.Write(safe)
}

// Close flushes what is left, and reports what went wrong on the way.
func (w *Writer) Close() error {
	w.w.Flush()
	return w.w.Error()
}
