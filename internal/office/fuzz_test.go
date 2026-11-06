// SPDX-License-Identifier: Apache-2.0

package office

import (
	"bytes"
	"testing"

	"github.com/diffluence/diffluence/internal/document"
)

// FuzzReadDOCX reads hostile files. It may refuse them, never crash on them.
func FuzzReadDOCX(f *testing.F) {
	var b bytes.Buffer
	doc := document.Node{Type: document.TypeDoc, Content: []document.Node{{Type: document.TypeParagraph,
		Content: []document.Node{{Type: document.TypeText, Text: "seed"}}}}}
	if err := WriteDOCX(&b, []Section{{Title: "Seed", Document: doc}}, Options{}); err != nil {
		f.Fatal(err)
	}
	f.Add(b.Bytes())
	f.Add([]byte("PK"))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _ = ReadDOCX(data, nil)
	})
}
