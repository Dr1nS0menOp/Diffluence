// SPDX-License-Identifier: Apache-2.0

package publishing

import "github.com/google/uuid"

// placement is where a twin goes in the published tree.
type placement struct {
	// Parent is the twin to put it under, or nil for the top of the published space.
	Parent *uuid.UUID
	// Higher says that the draft parent has no twin, so the page goes under its nearest
	// released ancestor instead, and the report says so (PUB-400, PUB-1426).
	Higher bool
}

// placeTwin finds the parent of a twin: the twin of the nearest ancestor of the draft page
// that has one, ancestors being nearest first. A twin's parent is therefore always the twin
// of a draft ancestor, and the draft tree has no cycle, so the published tree cannot get one
// (PUB-1006).
func placeTwin(ancestors []uuid.UUID, twins map[uuid.UUID]uuid.UUID) placement {
	for i, a := range ancestors {
		if twin, ok := twins[a]; ok {
			return placement{Parent: &twin, Higher: i > 0}
		}
	}
	return placement{Higher: len(ancestors) > 0}
}
