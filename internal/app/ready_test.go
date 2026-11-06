// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/diffluence/diffluence/internal/config"
)

func TestOnlyARoleThatConsumesEventsNeedsTheQueueToBeReady(t *testing.T) {
	for role, want := range map[config.Role]bool{config.RoleAPI: false, config.RoleWorker: true, config.RoleAll: true} {
		if got := needsQueueToBeReady(role); got != want {
			t.Errorf("%s: %v", role, got)
		}
	}
}
