// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"context"
	"sync"
	"time"
)

// appearanceFreshFor is how long a node answers the public appearance from its copy, as it
// keeps the health facts. A change on the node drops the copy; another node's change shows
// within this time (spec 090).
const appearanceFreshFor = 30 * time.Second

// Appearance is what the public appearance answer reads from the settings: everything but
// maintenance mode, which keeps a copy of its own.
type Appearance struct {
	Font     Font
	HasFont  bool
	Look     Look
	Logo     Logo
	HasLogo  bool
	General  General
	Features Features
}

// appearanceCache holds the last Appearance of this node. Its lock is held while it reads, so
// a change that drops it waits for a read in flight and never leaves an older copy behind.
type appearanceCache struct {
	mu     sync.Mutex
	value  Appearance
	readAt time.Time
}

// Appearance gives what the public appearance answer shows, from this node's copy while it is
// fresh.
func (s *Service) Appearance(ctx context.Context) (Appearance, error) {
	s.appearance.mu.Lock()
	defer s.appearance.mu.Unlock()
	if !s.appearance.readAt.IsZero() && time.Since(s.appearance.readAt) < appearanceFreshFor {
		return s.appearance.value, nil
	}
	v, err := s.readAppearance(ctx)
	if err != nil {
		return v, err
	}
	s.appearance.value, s.appearance.readAt = v, time.Now()
	return v, nil
}

func (s *Service) readAppearance(ctx context.Context) (Appearance, error) {
	var v Appearance
	var err error
	if v.Font, v.HasFont, err = s.CurrentFont(ctx); err != nil {
		return v, err
	}
	if err = s.ReadSection(ctx, SectionLook, &v.Look); err != nil {
		return v, err
	}
	if v.Logo, v.HasLogo, err = s.CurrentLogo(ctx); err != nil {
		return v, err
	}
	v.General = DefaultGeneral
	if err = s.ReadSection(ctx, SectionGeneral, &v.General); err != nil {
		return v, err
	}
	v.Features = DefaultFeatures
	err = s.ReadSection(ctx, SectionFeatures, &v.Features)
	return v, err
}

// forgetAppearance drops this node's copy after a change, so the next answer reads it anew.
func (s *Service) forgetAppearance() {
	s.appearance.mu.Lock()
	defer s.appearance.mu.Unlock()
	s.appearance.readAt = time.Time{}
}
