// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

func commandEnv(t *testing.T) string {
	t.Helper()
	tdb := testutil.Database(t)
	t.Setenv("DIFFLUENCE_DATABASE_URL", tdb.URL)
	t.Setenv("DIFFLUENCE_DATA_DIR", t.TempDir())
	t.Setenv("DIFFLUENCE_ROLE", "all")
	return tdb.URL
}

func TestSetupCodeIsPrintedUntilAnAdministratorExists(t *testing.T) {
	commandEnv(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"setup-code"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("setup-code: %d %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "/setup and enter the code ") {
		t.Fatalf("no code printed: %q", out.String())
	}
	out.Reset()
	if code := run([]string{"user", "create-admin", "--email", "first@example.com", "--name", "First"},
		strings.NewReader("correct horse battery staple\n"), &out, &errOut); code != 0 {
		t.Fatalf("create-admin: %d %s", code, errOut.String())
	}
	errOut.Reset()
	if code := run([]string{"setup-code"}, strings.NewReader(""), &out, &errOut); code == 0 {
		t.Fatal("once an administrator exists, there is no setup code")
	}
	if !strings.Contains(errOut.String(), "setup is over") {
		t.Fatalf("the refusal says why: %q", errOut.String())
	}
}

func TestCreateAdminStoresTheOrganisationName(t *testing.T) {
	url := commandEnv(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"user", "create-admin", "--email", "first@example.com", "--name", "First",
		"--organisation", "Acme & Sons"}, strings.NewReader("correct horse battery staple\n"), &out, &errOut); code != 0 {
		t.Fatalf("create-admin: %d %s", code, errOut.String())
	}
	pool, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var look string
	if err := pool.QueryRow(context.Background(), "SELECT value::text FROM settings WHERE key = 'section.look'").Scan(&look); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(look, `"organisation_name": "Acme & Sons"`) && !strings.Contains(look, `"organisation_name":"Acme & Sons"`) {
		t.Fatalf("the name must be stored: %s", look)
	}

	out.Reset()
	errOut.Reset()
	if code := run([]string{"user", "create-admin", "--email", "second@example.com", "--name", "Second",
		"--organisation", "<b>bold</b>"}, strings.NewReader("correct horse battery staple\n"), &out, &errOut); code == 0 {
		t.Fatal("a name with markup is refused")
	}
}
