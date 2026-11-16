// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/webui"
)

// fontCacheSeconds can be long: a replaced font gets a new versioned address.
const fontCacheSeconds = "31536000"

// appearanceCallsPerWindow is how often one address may ask for the appearance without a login
// in appearanceWindow: far more than the pages one person opens, and the answer comes from the
// node's copy, so the limit only stops a flood.
const (
	appearanceCallsPerWindow = 600
	appearanceWindow         = time.Minute
)

func (s *Server) registerAppearanceRoutes() {
	r := s.router
	// Public: the login page shows the installation's font before anyone is logged
	// in. It reveals only which font an administrator imported.
	r.HandleFunc("GET /api/v1/appearance", Public, s.handleAppearance)
	// Public: same reason; a font file holds no data of the installation.
	r.HandleFunc("GET /api/v1/appearance/font", Public, s.handleAppearanceFont)

	admin := SessionOnly(Permission(identity.PermAdministrator))
	r.HandleFunc("PUT /api/v1/admin/appearance/font", admin, s.handleFontImport)
	r.HandleFunc("DELETE /api/v1/admin/appearance/font", admin, s.handleFontRemove)
}

type fontJSON struct {
	Name    string `json:"name"`
	Format  string `json:"format"`
	Version string `json:"version"`
}

func (s *Server) handleAppearance(w http.ResponseWriter, r *http.Request) {
	// Anyone may ask without a login, so an address is limited the way the login limits count
	// addresses; someone logged in is not (spec 090).
	if PrincipalFrom(r.Context()) == nil && !s.appearanceCalls.allow(addressKey(metaFrom(r).IP)) {
		w.Header().Set("Retry-After", strconv.Itoa(int(appearanceWindow.Seconds())))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests. Try again later.")
		return
	}
	a, err := s.deps.Settings.Appearance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	maintenance, err := s.deps.Settings.Maintenance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	look := a.Look
	out := map[string]any{"font": nil, "name": look.InstanceName, "organisation": look.OrganisationName, "accent": settings.DerivedAccent(look.AccentLight, look.AccentDark),
		"login_welcome": look.LoginWelcome, "terms_url": look.TermsURL, "banner": activeBanner(look), "welcome_page": look.WelcomePage,
		"help_url": a.General.HelpURL, "maintenance": maintenance, "logo": "", "mail_ready": s.deps.Identity.MailReady(),
		"self_signup": a.Features.SelfSignup,
		// The id of the frontend this server ships, so an open tab notices a newer version (spec 041).
		"build": webui.Build()}
	if a.HasFont {
		out["font"] = fontJSON{Name: a.Font.Name, Format: a.Font.Format, Version: a.Font.Version}
	}
	if a.HasLogo {
		out["logo"] = "/api/v1/appearance/logo?v=" + a.Logo.Version
	}
	writeJSON(w, http.StatusOK, out)
}

// activeBanner leaves out a banner past its end date.
func activeBanner(look settings.Look) any {
	if look.BannerText == "" || (look.BannerEnds != nil && time.Now().After(*look.BannerEnds)) {
		return nil
	}
	return map[string]string{"text": look.BannerText, "level": look.BannerLevel}
}

func (s *Server) handleAppearanceFont(w http.ResponseWriter, r *http.Request) {
	font, file, err := s.deps.Settings.OpenFont(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	h := w.Header()
	h.Set("Content-Type", "font/"+font.Format)
	h.Set("X-Content-Type-Options", "nosniff")
	if r.URL.Query().Get("v") == font.Version {
		h.Set("Cache-Control", "public, max-age="+fontCacheSeconds+", immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, "", time.Time{}, file)
}

func (s *Server) handleFontImport(w http.ResponseWriter, r *http.Request) {
	font, err := s.deps.Settings.ImportFont(r.Context(), s.caller(r), r.URL.Query().Get("filename"), r.Body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, fontJSON{Name: font.Name, Format: font.Format, Version: font.Version})
}

func (s *Server) handleFontRemove(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Settings.RemoveFont(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
