// SPDX-License-Identifier: Apache-2.0

package permissions_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/testutil"
)

// interfaceWords reads the names and lines of every permission from the interface, keyed
// by the permission itself.
func interfaceWords(t *testing.T) map[string][2]string {
	t.Helper()
	raw, err := os.ReadFile("../../web/src/locales/en.json")
	if err != nil {
		t.Fatal(err)
	}
	var words struct {
		Permissions map[string]string `json:"permissions"`
	}
	if err := json.Unmarshal(raw, &words); err != nil {
		t.Fatal(err)
	}
	out := map[string][2]string{}
	for _, p := range permissions.AllSpacePermissions {
		k := strings.ReplaceAll(p, ".", "_")
		out[p] = [2]string{words.Permissions[k], words.Permissions[k+"_help"]}
	}
	for _, p := range identity.AllGlobalPermissions {
		k := "global_" + p
		out[p] = [2]string{words.Permissions[k], words.Permissions[k+"_help"]}
	}
	return out
}

// TestEveryPermissionHasPlainWords keeps keys off the screen: each permission has a name
// and one line that says what it allows, in the words of the interface (spec 036).
func TestEveryPermissionHasPlainWords(t *testing.T) {
	for p, w := range interfaceWords(t) {
		if strings.TrimSpace(w[0]) == "" || strings.TrimSpace(w[1]) == "" {
			t.Errorf("%s has no name or no line in en.json: %q", p, w)
		}
	}
}

// TestRoleNamesUseTheWordsOfTheInterface: a role the database names after its
// permissions uses the same words as the screens.
func TestRoleNamesUseTheWordsOfTheInterface(t *testing.T) {
	tdb := testutil.Database(t)
	for p, w := range interfaceWords(t) {
		var got string
		if err := tdb.Pool.QueryRow(context.Background(), "SELECT rbac_words(ARRAY[$1::text])", p).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != w[0] {
			t.Errorf("the database calls %s %q, the interface %q", p, got, w[0])
		}
	}
}
