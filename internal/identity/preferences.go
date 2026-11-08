// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
	_ "time/tzdata" // the zones must be known, whatever the host has installed

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
)

// The limits of the sidebar, in rem.
const (
	minSidebarWidth     = 14
	defaultSidebarWidth = 18
	maxSidebarWidth     = 32
	minSidebarPart      = 4
	maxSidebarPart      = 40
)

var (
	themes      = []string{"system", "light", "dark"}
	treeViews   = []string{"tree", "visited", "updated", "alphabet"}
	languages   = []string{"en"}
	dateFormats = []string{"short", "medium", "long", "iso"}
	// Empty is what a client sends that does not know this setting yet.
	startScreens = []string{"", "company", "personal"}
	// sidebarParts are the parts of the sidebar that fold: of a space, and of the home page.
	sidebarParts = []string{"space", "you", "spaces"}
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
	Language     string `json:"language"`
	TimeZone     string `json:"time_zone"`
	DateFormat   string `json:"date_format"`
	// FirstDay is 0 for Sunday and 1 for Monday. It is kept for a calendar, which
	// is the first thing that can use it (spec 013, D20).
	FirstDay         int  `json:"first_day"`
	WelcomeDismissed bool `json:"welcome_dismissed"`
	TourSeen         bool `json:"tour_seen"`
	// StartScreen is company or personal, for an instance with a company home.
	StartScreen string `json:"start_screen"`
	// ShowPresence lets others see this person on a page.
	ShowPresence bool `json:"show_presence"`
	// HideFollows keeps followers and who this person follows off their profile.
	HideFollows bool `json:"hide_follows"`
	// TypingHelp turns what people type, such as :) and ->, into the real character.
	TypingHelp bool `json:"typing_help"`
	// SidebarShut names the parts of the sidebar this person folded shut (spec 033).
	SidebarShut []string `json:"sidebar_shut"`
	// SidebarLinksHeight is the height in rem this person dragged the links of a space
	// to, or 0 for as tall as they are.
	SidebarLinksHeight int `json:"sidebar_links_height"`
}

// DefaultPreferences is what someone has who never chose anything.
func DefaultPreferences() Preferences {
	return Preferences{Theme: "system", Shortcuts: true, Contents: true, HoverCards: true,
		SidebarWidth: defaultSidebarWidth, TreeView: "tree", Language: "en", TimeZone: "", DateFormat: "medium", FirstDay: 1, StartScreen: "company", ShowPresence: true, TypingHelp: true}
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
	case !slices.Contains(languages, p.Language):
		return apperr.Invalid("English is the only language so far.")
	case !slices.Contains(dateFormats, p.DateFormat):
		return apperr.Invalid("That is not a date format.")
	case p.FirstDay != 0 && p.FirstDay != 1 && p.FirstDay != 6:
		return apperr.Invalid("A week starts on Saturday, Sunday or Monday.")
	case p.SidebarLinksHeight != 0 && (p.SidebarLinksHeight < minSidebarPart || p.SidebarLinksHeight > maxSidebarPart):
		return apperr.Invalid("A part of the sidebar is between 4 and 40 high.")
	}
	for _, part := range p.SidebarShut {
		if !slices.Contains(sidebarParts, part) {
			return apperr.Invalid("That is not a part of the sidebar.")
		}
	}
	if p.TimeZone == "" {
		return nil // the time zone of the browser
	}
	if _, err := time.LoadLocation(p.TimeZone); err != nil {
		return apperr.Invalid("That is not a time zone.")
	}
	return nil
}

// Preferences reads a person's preferences. What they never set has its default.
func (s *Service) Preferences(ctx context.Context, userID uuid.UUID) (Preferences, error) {
	out := DefaultPreferences()
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
