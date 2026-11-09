// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/net/html"

	"github.com/diffluence/diffluence/internal/document"
)

func FuzzFrontMatter(f *testing.F) {
	f.Add([]byte("---\ntitle: x\nlabels: [a, b]\n---\nbody"))
	f.Add([]byte("---\nlabels:\n- a\n"))
	f.Fuzz(func(_ *testing.T, data []byte) {
		_, _, _ = splitFrontMatter(data)
	})
}

// FuzzVerifyBundle reads hostile bundles. Nothing unsigned may get through.
func FuzzVerifyBundle(f *testing.F) {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, err := zw.Create(manifestFile)
	if err != nil {
		f.Fatal(err)
	}
	if _, err := w.Write([]byte(`{"format":"diffluence-bundle"}`)); err != nil {
		f.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		f.Fatal(err)
	}
	f.Add(b.Bytes())
	f.Fuzz(func(t *testing.T, data []byte) {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		if _, err := verifyBundle(zr); err == nil && len(zr.File) < 2 {
			t.Fatal("a bundle without a signature got through")
		}
	})
}

// FuzzHTMLImport reads hostile HTML (SEC-351). Whatever comes out must pass the document gate,
// and nothing may reach for a file the archive does not hold.
func FuzzHTMLImport(f *testing.F) {
	f.Add([]byte(`<html><head><title>T</title></head><body><h1>T</h1><p>a <a href="javascript:alert(1)">b</a></p>` +
		`<script>alert(1)</script><img src="x.png" onerror="alert(1)"><form><input></form></body></html>`))
	f.Add([]byte(`<table><tr><td><ul><li><ol><li><pre>x</pre></li></ol></li></ul></td></tr></table><a href="other.html#top">x</a>`))
	f.Add([]byte(`<details><summary>s</summary><blockquote><div><div><div>deep</div></div></div></blockquote></details>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		root, err := html.Parse(bytes.NewReader(data))
		if err != nil {
			return
		}
		r := &htmlReader{m: &importer{}, store: func(string) (uuid.UUID, bool) { return uuid.Nil, false }}
		doc, _ := r.document(root)
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := document.Normalize(raw); err != nil {
			t.Fatalf("an imported page fails the gate: %v\n%s", err, raw)
		}
	})
}

// FuzzImportArchive reads hostile archives (SEC-351). No name may climb out or hide, and the
// claimed sizes never pass the limit.
func FuzzImportArchive(f *testing.F) {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, name := range []string{"Space/Page.md", "../escape.md", `a\..\..\b.md`, ".hidden/x.md", "__MACOSX/y.md"} {
		if _, err := zw.Create(name); err != nil {
			f.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		f.Fatal(err)
	}
	f.Add(b.Bytes())
	const limit = 1 << 20
	f.Fuzz(func(t *testing.T, data []byte) {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		list, err := zipEntries(zr, emptyResult(), limit)
		if err != nil {
			return
		}
		var total uint64
		for _, e := range list {
			total += e.size
			if e.name == "" || strings.HasPrefix(e.name, "/") || strings.Contains(e.name, `\`) {
				t.Fatalf("an unsafe name got through: %q", e.name)
			}
			for _, part := range strings.Split(e.name, "/") {
				if part == ".." || strings.HasPrefix(part, ".") {
					t.Fatalf("a name climbs out or hides: %q", e.name)
				}
			}
		}
		if total > limit {
			t.Fatalf("%d bytes passed a limit of %d", total, limit)
		}
	})
}

func TestFrontMatterReadsTheSubset(t *testing.T) {
	got, body, warnings := splitFrontMatter([]byte("---\ntitle: \"A\"\ntags:\n- x\n- y\nstatus: Done\nodd: 1\n---\nrest\n"))
	if got.title != "A" || len(got.labels) != 2 || got.status != "Done" || string(body) != "rest\n" || len(warnings) != 1 {
		t.Fatalf("%+v %q %v", got, body, warnings)
	}
}

func TestATextWithoutFrontMatterStaysWhole(t *testing.T) {
	for _, text := range []string{"# hi", "---\nno end", "---\ntitle: x\n---more"} {
		_, body, _ := splitFrontMatter([]byte(text))
		if string(body) != text {
			t.Errorf("%q became %q", text, body)
		}
	}
}
