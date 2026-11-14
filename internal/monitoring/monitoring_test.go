// SPDX-License-Identifier: Apache-2.0

package monitoring

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The files the console offers are the files of deploy/monitoring/ of the release that runs
// (spec 075, CSL-1214): a change there without a change here fails.
func TestTheEmbeddedFilesEqualDeployMonitoring(t *testing.T) {
	files := Files()
	if len(files) != 2 {
		t.Fatalf("two files, got %d", len(files))
	}
	for _, f := range files {
		want, err := os.ReadFile(filepath.Join("..", "..", "deploy", "monitoring", f.Name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(f.Body, want) {
			t.Errorf("%s differs from deploy/monitoring/%s: copy it into internal/monitoring", f.Name, f.Name)
		}
		if f.ContentType == "" || len(f.Body) == 0 {
			t.Errorf("%s: %+v", f.Name, f.ContentType)
		}
	}
}

func TestAFileIsFoundByItsKindOnly(t *testing.T) {
	for kind, name := range map[string]string{"rules": "diffluence-rules.yml", "dashboard": "diffluence-dashboard.json"} {
		f, ok := ByKind(kind)
		if !ok || f.Name != name {
			t.Errorf("%s: %+v %v", kind, f.Name, ok)
		}
	}
	for _, kind := range []string{"", "../go.mod", "diffluence-rules.yml", "README"} {
		if _, ok := ByKind(kind); ok {
			t.Errorf("%q found a file", kind)
		}
	}
}
