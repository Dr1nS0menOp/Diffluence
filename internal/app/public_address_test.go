// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/config"
)

// SEC-257: cookies and HSTS follow the public address, so a plain one that others reach is said
// out loud in the console. A trial on one's own machine is not a warning.
func TestTheConsoleWarnsOfAPlainPublicAddressThatOthersReach(t *testing.T) {
	local := newEnv(t)
	if out := admin(local).want(200, "GET", "/api/v1/admin/attention", nil); strings.Contains(out, "public_address_plain") {
		t.Fatalf("a trial on this machine: %s", out)
	}
	e := newEnvWith(t, func(c *config.Config) {
		c.PublicURL, _ = url.Parse("http://wiki.example.com")
	})
	if out := admin(e).want(200, "GET", "/api/v1/admin/attention", nil); !strings.Contains(out, "public_address_plain") {
		t.Fatalf("a plain address on the network, and the console says nothing: %s", out)
	}
}
