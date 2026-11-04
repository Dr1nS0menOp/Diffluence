// SPDX-License-Identifier: Apache-2.0

// Package ids generates identifiers. Every entity uses UUIDv7: globally unique
// (so instances can be merged later) and time-ordered (index friendly).
package ids

import "github.com/google/uuid"

// New returns a new UUIDv7. It panics only if the system random source fails,
// in which case nothing secure can be done anyway.
func New() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic("ids: cannot generate UUIDv7: " + err.Error())
	}
	return id
}
