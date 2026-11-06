// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"archive/zip"
	"bytes"
	"testing"
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
