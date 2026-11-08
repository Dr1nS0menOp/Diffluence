// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"time"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/settings"
)

// fontCacheSeconds can be long: a replaced font gets a new versioned address.
const fontCacheSeconds = "31536000"

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
	font, found, err := s.deps.Settings.CurrentFont(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var look settings.Look
	if err := s.deps.Settings.ReadSection(r.Context(), settings.SectionLook, &look); err != nil {
		s.fail(w, r, err)
		return
	}
	logo, hasLogo, err := s.deps.Settings.CurrentLogo(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	maintenance, err := s.deps.Settings.Maintenance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	general := settings.DefaultGeneral
	if err := s.deps.Settings.ReadSection(r.Context(), settings.SectionGeneral, &general); err != nil {
		s.fail(w, r, err)
		return
	}
	features, err := s.features(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := map[string]any{"font": nil, "name": look.InstanceName, "organisation": look.OrganisationName, "accent": settings.DerivedAccent(look.AccentLight, look.AccentDark),
		"login_welcome": look.LoginWelcome, "terms_url": look.TermsURL, "banner": activeBanner(look), "welcome_page": look.WelcomePage,
		"help_url": general.HelpURL, "maintenance": maintenance, "logo": "", "mail_ready": s.deps.Identity.MailReady(),
		"self_signup": features.SelfSignup}
	if found {
		out["font"] = fontJSON{Name: font.Name, Format: font.Format, Version: font.Version}
	}
	if hasLogo {
		out["logo"] = "/api/v1/appearance/logo?v=" + logo.Version
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
