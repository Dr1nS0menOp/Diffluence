// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
)

// The limits of the sidebar, in rem.
const (
	minSidebarWidth     = 14
	defaultSidebarWidth = 18
	maxSidebarWidth     = 32
)

var (
	themes    = []string{"system", "light", "dark"}
	treeViews = []string{"tree", "visited", "updated", "alphabet"}
	// Empty is what a client sends that does not know this setting yet.
	startScreens = []string{"", "company", "personal"}
)

// Preferences are personal settings that follow a person to other devices. They
// are private and change often, so they are not audited.
type Preferences struct {
	Theme        string `json:"theme"`
	Shortcuts    bool   `json:"shortcuts"`
	Contents     bool   `json:"contents"`
	HoverCards   bool   `json:"hover_cards"`
	SidebarWidth int    `json:"sidebar_width"`
	TreeView     string `json:"tree_view"`
	// StartScreen is company or personal, for an instance with a company home.
	StartScreen string `json:"start_screen"`
}

func defaultPreferences() Preferences {
	return Preferences{Theme: "system", Shortcuts: true, Contents: true, HoverCards: true,
		SidebarWidth: defaultSidebarWidth, TreeView: "tree", StartScreen: "company"}
}

func (p Preferences) validate() error {
	switch {
	case !slices.Contains(themes, p.Theme):
		return apperr.Invalid("A theme is system, light or dark.")
	case !slices.Contains(treeViews, p.TreeView):
		return apperr.Invalid("That is not a view of the page tree.")
	case p.SidebarWidth < minSidebarWidth || p.SidebarWidth > maxSidebarWidth:
		return apperr.Invalid("The sidebar is between 14 and 32 wide.")
	case !slices.Contains(startScreens, p.StartScreen):
		return apperr.Invalid("The start screen is company or personal.")
	}
	return nil
}

// Preferences reads a person's preferences. What they never set has its default.
func (s *Service) Preferences(ctx context.Context, userID uuid.UUID) (Preferences, error) {
	out := defaultPreferences()
	var raw []byte
	err := s.pool.QueryRow(ctx, "SELECT preferences FROM user_preferences WHERE user_id = $1", userID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("identity: read preferences: %v", err)
	}
	return out, nil
}

// SetPreferences stores a person's preferences.
func (s *Service) SetPreferences(ctx context.Context, userID uuid.UUID, p Preferences) error {
	if err := p.validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO user_preferences (user_id, preferences) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET preferences = EXCLUDED.preferences, updated_at = now()`, userID, raw)
	return err
}
