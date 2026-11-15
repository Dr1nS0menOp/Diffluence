// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func signedExample() SignedContent {
	return SignedContent{Title: "Cleaning procedure B", SchemaVersion: 3, Document: `{"content":[{"text":"wash","type":"text"}],"type":"doc"}`,
		Files: []SignedFile{
			{ID: uuid.MustParse("01900000-0000-7000-8000-000000000002"), Version: 2, Filename: "form.pdf", SHA256: strings.Repeat("ab", 32)},
			{ID: uuid.MustParse("01900000-0000-7000-8000-000000000001"), Version: 1, Filename: "plan.png", SHA256: strings.Repeat("cd", 32)},
		}}
}

// DOC-711: any change to the title, the document or a file a version shows changes the hash,
// and so does the version of the document format.
func TestAnyChangeToTheSignedContentChangesItsHash(t *testing.T) {
	base := signedExample().Hash()
	changes := map[string]func(*SignedContent){
		"title":        func(c *SignedContent) { c.Title += "." },
		"body":         func(c *SignedContent) { c.Document = strings.Replace(c.Document, "wash", "rinse", 1) },
		"schema":       func(c *SignedContent) { c.SchemaVersion++ },
		"file version": func(c *SignedContent) { c.Files[0].Version++ },
		"file name":    func(c *SignedContent) { c.Files[0].Filename = "form2.pdf" },
		"file bytes":   func(c *SignedContent) { c.Files[1].SHA256 = strings.Repeat("ef", 32) },
		"file added": func(c *SignedContent) {
			c.Files = append(c.Files, SignedFile{ID: uuid.New(), Version: 1, Filename: "x", SHA256: "00"})
		},
		"file taken out": func(c *SignedContent) { c.Files = c.Files[:1] },
		// A title cannot pose as part of the document: each part carries its length.
		"parts shifted": func(c *SignedContent) { c.Title += "\ndocument 1\n" },
	}
	for name, change := range changes {
		c := signedExample()
		change(&c)
		if bytes.Equal(c.Hash(), base) {
			t.Errorf("a change of the %s kept the hash", name)
		}
	}
	// The order in which files are listed is not content.
	c := signedExample()
	c.Files[0], c.Files[1] = c.Files[1], c.Files[0]
	if !bytes.Equal(c.Hash(), base) {
		t.Error("the order of the files changed the hash")
	}
}

// The canonical document keeps numbers as stored, sorts keys and escapes no HTML.
func TestTheCanonicalDocumentIsStable(t *testing.T) {
	got, err := canonicalJSON([]byte(`{"type": "doc", "attrs": {"level": 1.50, "big": 12345678901234567890}, "text": "<b>&"}`))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"attrs":{"big":12345678901234567890,"level":1.50},"text":"<b>&","type":"doc"}`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// DOC-1303: signing costs one hash of the canonical form; display compares stored hashes.
func BenchmarkSignedContentHash(b *testing.B) {
	c := signedExample()
	c.Document = `{"content":[` + strings.Repeat(`{"type":"paragraph","content":[{"type":"text","text":"a line of a long procedure"}]},`, 2000) + `{"type":"text"}],"type":"doc"}`
	b.SetBytes(int64(len(c.CanonicalForm())))
	for b.Loop() {
		c.Hash()
	}
}
