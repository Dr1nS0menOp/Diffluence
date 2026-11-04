// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/audit"
)

// Caller is who is performing an action, passed to every module. Modules check
// permissions themselves, so no path through the code can skip them.
type Caller struct {
	UserID uuid.UUID
	System string // set instead of UserID for background and CLI actions
	Meta   RequestMeta
}

// SystemCaller is a caller for background jobs.
func SystemCaller(component string) Caller { return Caller{System: component} }

// CallerFor is a caller for a logged-in user.
func CallerFor(p Principal, meta RequestMeta) Caller { return Caller{UserID: p.User.ID, Meta: meta} }

// Actor returns the audit actor.
func (c Caller) Actor() Actor {
	if c.System != "" {
		return SystemActor(c.System)
	}
	return Actor{Type: audit.ActorUser, ID: c.UserID.String()}
}

// Event builds a successful audit event for this caller.
func (c Caller) Event(action, objectType, objectID, spaceID string, details map[string]any) audit.Event {
	a := c.Actor()
	return audit.Event{ActorType: a.Type, ActorID: a.ID, Action: action, ObjectType: objectType, ObjectID: objectID,
		SpaceID: spaceID, Result: audit.ResultSuccess, IP: c.Meta.IP, UserAgent: c.Meta.UserAgent,
		RequestID: c.Meta.RequestID, Details: details}
}
