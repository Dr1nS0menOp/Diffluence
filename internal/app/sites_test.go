// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const siteName = "Open docs"

type siteEnv struct {
	*env
	dir   string
	owner *user
	admin *user
	space string
	home  string
	site  string
}

func siteSetup(t *testing.T) *siteEnv {
	t.Helper()
	dir := t.TempDir()
	e := newEnvWith(t, func(c *config.Config) { c.PublishDir = dir })
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	admin.want(204, "PUT", "/api/v1/admin/export-policy", map[string]any{"rules": []any{}, "limits": map[string]any{
		"max_export_mib": 100, "max_import_mib": 100, "public_sites_enabled": true, "public_links_enabled": true}})
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("PUB")
	home := owner.page(space, nil, "Welcome", "welcome words")
	site := owner.want(201, "POST", "/api/v1/spaces/"+space+"/sites", map[string]any{"name": siteName, "address": "https://docs.example.org"})
	id := regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(site)[1]
	return &siteEnv{env: e, dir: dir, owner: owner, admin: admin, space: space, home: home, site: id}
}

func (s *siteEnv) choose(pages ...string) {
	s.t.Helper()
	s.owner.want(204, "PUT", "/api/v1/sites/"+s.site+"/pages", map[string]any{"page_ids": pages})
}

func (s *siteEnv) publish() {
	s.t.Helper()
	s.owner.want(202, "POST", "/api/v1/sites/"+s.site+"/publish", map[string]any{"confirm": siteName})
	s.runJobs()
}

// live reads a file of the live site. An empty answer means it is not there.
func (s *siteEnv) live(name string) string {
	s.t.Helper()
	var target string
	if err := s.app.Pool().QueryRow(context.Background(), `SELECT target FROM public_sites WHERE id = $1`, s.site).Scan(&target); err != nil {
		s.t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.dir, target, "live", name))
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *siteEnv) deliver() {
	s.t.Helper()
	if err := s.app.DeliverEvents(context.Background()); err != nil {
		s.t.Fatal(err)
	}
}

func TestASiteHoldsExactlyTheChosenPagesAndThePreviewSaidSo(t *testing.T) {
	s := siteSetup(t)
	other := s.owner.page(s.space, nil, "Internal", "kept inside")
	s.choose(s.home)

	preview := s.owner.ok("POST", "/api/v1/sites/"+s.site+"/preview", map[string]any{})
	if added := preview["added"].([]any); len(added) != 1 || added[0] != "Welcome" {
		t.Fatalf("%+v", preview)
	}
	s.owner.want(400, "POST", "/api/v1/sites/"+s.site+"/publish", map[string]any{"confirm": "wrong"})
	s.publish()

	if !strings.Contains(s.live("welcome.html"), "welcome words") {
		t.Fatalf("no page: %q", s.live("welcome.html"))
	}
	if s.live("internal.html") != "" || strings.Contains(s.live("index.html"), "Internal") {
		t.Fatal("a page nobody chose is on the site")
	}
	for _, want := range []string{`lang="en"`, `class="skip"`, "<main", "<nav"} {
		if !strings.Contains(s.live("welcome.html"), want) {
			t.Errorf("missing %s", want)
		}
	}
	if s.auditCount("site.published") != 1 {
		t.Fatal("a build must be audited")
	}
	_ = other
}

func (s *siteEnv) auditCount(action string) int { return s.auditActions(action) }

func TestARestrictedPageCannotBeChosen(t *testing.T) {
	s := siteSetup(t)
	secret := s.owner.page(s.space, nil, "Secret", "secret words")
	child := s.owner.page(s.space, secret, "Below", "below words")
	s.owner.restrict(secret, "view", s.owner.id)
	s.owner.want(409, "PUT", "/api/v1/sites/"+s.site+"/pages", map[string]any{"page_ids": []string{secret}})
	s.owner.want(409, "PUT", "/api/v1/sites/"+s.site+"/pages", map[string]any{"page_ids": []string{child}})
}

func TestEveryEventThatCanHideAPageTakesItOffTheSite(t *testing.T) {
	steps := map[string]func(s *siteEnv, page string){
		"a restriction on it": func(s *siteEnv, page string) { s.owner.restrict(page, "view", s.owner.id) },
		"a restriction above it": func(s *siteEnv, page string) {
			s.owner.restrict(s.home, "view", s.owner.id)
		},
		"a move below a restricted page": func(s *siteEnv, page string) {
			closed := s.owner.page(s.space, nil, "Closed", "x")
			s.owner.restrict(closed, "view", s.owner.id)
			s.owner.ok("POST", "/api/v1/pages/"+page+"/move", map[string]any{"parent_id": closed})
		},
		"archiving it":        func(s *siteEnv, page string) { s.owner.ok("POST", "/api/v1/pages/"+page+"/archive", map[string]any{}) },
		"deleting it":         func(s *siteEnv, page string) { s.owner.ok("DELETE", "/api/v1/pages/"+page, nil) },
		"archiving the space": func(s *siteEnv, _ string) { s.owner.ok("POST", "/api/v1/spaces/"+s.space+"/archive", map[string]any{}) },
		"deleting the space":  func(s *siteEnv, _ string) { s.owner.ok("DELETE", "/api/v1/spaces/"+s.space, nil) },
	}
	for name, step := range steps {
		t.Run(name, func(t *testing.T) {
			s := siteSetup(t)
			page := s.owner.page(s.space, s.home, "Leaving", "leaving words")
			s.choose(page)
			s.publish()
			if !strings.Contains(s.live("leaving.html"), "leaving words") {
				t.Fatal("not on the site")
			}
			step(s, page)
			s.deliver()
			if strings.Contains(s.live("leaving.html"), "leaving words") || strings.Contains(s.live("search.json"), "leaving words") {
				t.Fatal("still on the site")
			}
		})
	}
}

func TestRemovingTheShownVersionTakesItOffTheSite(t *testing.T) {
	s := siteSetup(t)
	page := s.owner.page(s.space, nil, "Versioned", "a secret slipped in")
	s.choose(page)
	s.publish()
	s.owner.publish(page, "Versioned", 1, docWith("clean words"))
	s.admin.want(204, "DELETE", "/api/v1/admin/pages/"+page+"/versions/1", map[string]any{"reason": "a secret"})
	s.deliver()
	if strings.Contains(s.live("versioned.html"), "secret") {
		t.Fatal("the removed version is still public")
	}
	if !strings.Contains(s.live("versioned.html"), "clean words") {
		t.Fatalf("got %q", s.live("versioned.html"))
	}
}

func mentionDoc(id string) map[string]any {
	return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{
		map[string]any{"type": "text", "text": "Ask "},
		map[string]any{"type": "mention", "attrs": map[string]any{"id": id, "kind": "user"}},
	}}}}
}

func TestAMentionIsNeutralUnlessTheSiteShowsNames(t *testing.T) {
	s := siteSetup(t)
	if _, err := s.app.Pool().Exec(context.Background(), `UPDATE users SET display_name = 'Anna Visible' WHERE id = $1`, s.owner.id); err != nil {
		t.Fatal(err)
	}
	page := s.owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": s.space, "title": "Team"})["id"].(string)
	s.owner.publish(page, "Team", 0, mentionDoc(s.owner.id))
	s.choose(page)
	s.publish()
	for _, file := range []string{"team.html", "search.json"} {
		if strings.Contains(s.live(file), "Anna") {
			t.Fatalf("%s names a person: %s", file, s.live(file))
		}
	}
	if !strings.Contains(s.live("team.html"), "a colleague") {
		t.Fatalf("no neutral text: %s", s.live("team.html"))
	}
	s.owner.ok("PATCH", "/api/v1/sites/"+s.site, map[string]any{"name": siteName, "show_names": true})
	s.publish()
	if !strings.Contains(s.live("team.html"), "Anna") {
		t.Fatalf("names were chosen: %s", s.live("team.html"))
	}
}

func TestSearchOnlyHoldsWhatIsOnTheSite(t *testing.T) {
	s := siteSetup(t)
	s.owner.page(s.space, nil, "Left out", "plantedword")
	s.choose(s.home)
	s.publish()
	if strings.Contains(s.live("search.json"), "plantedword") {
		t.Fatal("the index holds a page that is left out")
	}
	if !strings.Contains(s.live("search.json"), "welcome words") {
		t.Fatalf("index: %s", s.live("search.json"))
	}
}

func TestALinkToAPageOffTheSiteBecomesText(t *testing.T) {
	s := siteSetup(t)
	hidden := s.owner.page(s.space, nil, "Hidden", "x")
	page := s.owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": s.space, "title": "Linking"})["id"].(string)
	s.owner.publish(page, "Linking", 0, map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{
		map[string]any{"type": "text", "text": "see that", "marks": []any{map[string]any{"type": "link", "attrs": map[string]any{"href": "/pages/" + hidden}}}}}}}})
	s.choose(page)
	preview := s.owner.ok("POST", "/api/v1/sites/"+s.site+"/preview", map[string]any{})
	if len(preview["dead_links"].([]any)) != 1 {
		t.Fatalf("%+v", preview)
	}
	s.publish()
	if got := s.live("linking.html"); strings.Contains(got, hidden) || strings.Contains(got, "<a href=\"/pages") || !strings.Contains(got, "see that") {
		t.Fatalf("%s", got)
	}
}

func TestTheScrubStopsABuildAndKeepsTheOldSite(t *testing.T) {
	cases := map[string]func(s *siteEnv) string{
		"the instance address": func(*siteEnv) string { return "see http://localhost/x" },
		"an API path":          func(*siteEnv) string { return "call /api/v1/pages" },
		"a page id":            func(s *siteEnv) string { return "page " + s.home },
		"a user address":       func(*siteEnv) string { return "mail owner@example.com" },
	}
	for name, planted := range cases {
		t.Run(name, func(t *testing.T) {
			s := siteSetup(t)
			s.choose(s.home)
			s.publish()
			s.owner.publish(s.home, "Welcome", 1, docWith(planted(s)))
			s.owner.want(202, "POST", "/api/v1/sites/"+s.site+"/publish", map[string]any{})
			s.runJobs()
			if !strings.Contains(s.live("welcome.html"), "welcome words") {
				t.Fatalf("the old site is gone: %s", s.live("welcome.html"))
			}
			builds := s.owner.ok("GET", "/api/v1/sites/"+s.site+"/builds", nil)["builds"].([]any)
			if builds[0].(map[string]any)["state"] != "failed" {
				t.Fatalf("%+v", builds[0])
			}
		})
	}
}

func jpegWithMetadata(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	// An APP1 segment right after the start, as a camera writes it.
	comment := []byte("Exif\x00\x00GPS 52.09 5.12 secret-location")
	segment := append([]byte{0xFF, 0xE1, 0, byte(len(comment) + 2)}, comment...)
	return append(append(append([]byte{}, data[:2]...), segment...), data[2:]...)
}

func TestOnlyShownFilesGoAlongAndPhotosLoseTheirMetadata(t *testing.T) {
	s := siteSetup(t)
	page := s.owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": s.space, "title": "Photo"})["id"].(string)
	_, shown := s.owner.upload(page, "photo.jpg", string(jpegWithMetadata(t)))
	s.owner.upload(page, "private.txt", "not shown anywhere")
	id := regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(shown)[1]
	s.owner.publish(page, "Photo", 0, map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "image", "attrs": map[string]any{"attachment_id": id, "alt": "A view"}}}})
	s.choose(page)
	s.publish()
	if !strings.Contains(s.live("photo.html"), `alt="A view"`) {
		t.Fatalf("%s", s.live("photo.html"))
	}
	data := s.live("files/1.jpg")
	if data == "" {
		t.Fatal("the image did not go along")
	}
	if strings.Contains(data, "secret-location") {
		t.Fatal("the metadata went along")
	}
	if s.live("files/2.txt") != "" {
		t.Fatal("a file the page does not show went along")
	}
}

func TestSearchEnginesAreAllowedOrBlocked(t *testing.T) {
	s := siteSetup(t)
	s.choose(s.home)
	s.publish()
	if !strings.Contains(s.live("robots.txt"), "Disallow: /") || !strings.Contains(s.live("welcome.html"), "noindex") || s.live("sitemap.xml") != "" {
		t.Fatalf("blocked: %q", s.live("robots.txt"))
	}
	s.owner.ok("PATCH", "/api/v1/sites/"+s.site, map[string]any{"name": siteName, "address": "https://docs.example.org", "search_engines": true})
	s.publish()
	if strings.Contains(s.live("robots.txt"), "Disallow") || strings.Contains(s.live("welcome.html"), "noindex") {
		t.Fatalf("allowed: %q", s.live("robots.txt"))
	}
	if !strings.Contains(s.live("sitemap.xml"), "https://docs.example.org/welcome.html") {
		t.Fatalf("sitemap: %q", s.live("sitemap.xml"))
	}
}

func TestASiteLoadsNothingFromElsewhere(t *testing.T) {
	s := siteSetup(t)
	s.choose(s.home)
	s.publish()
	page := s.live("welcome.html")
	if !strings.Contains(page, "Content-Security-Policy") {
		t.Fatal("no policy")
	}
	if regexp.MustCompile(`(src|href)="https?:`).MatchString(page) {
		t.Fatalf("loads from elsewhere: %s", page)
	}
}

func TestRollbackGoesBackAndTheLastThreeStay(t *testing.T) {
	s := siteSetup(t)
	s.choose(s.home)
	for i, text := range []string{"one", "two", "three", "four"} {
		s.owner.publish(s.home, "Welcome", i+1, docWith(text))
		s.publish()
	}
	builds := s.owner.ok("GET", "/api/v1/sites/"+s.site+"/builds", nil)["builds"].([]any)
	if len(builds) != 3 {
		t.Fatalf("%d builds", len(builds))
	}
	s.owner.want(204, "POST", "/api/v1/sites/"+s.site+"/rollback", map[string]any{"number": 3})
	if !strings.Contains(s.live("welcome.html"), "three") {
		t.Fatalf("%s", s.live("welcome.html"))
	}
}

func TestARollbackCannotBringBackARestrictedPage(t *testing.T) {
	s := siteSetup(t)
	other := s.owner.page(s.space, nil, "Later closed", "x")
	s.choose(s.home, other)
	s.publish()
	s.owner.want(204, "PUT", "/api/v1/sites/"+s.site+"/pages", map[string]any{"page_ids": []string{s.home}})
	s.publish()
	s.owner.restrict(other, "view", s.owner.id)
	s.owner.want(409, "POST", "/api/v1/sites/"+s.site+"/rollback", map[string]any{"number": 1})
}

func TestWithdrawLeavesAPageThatSaysSo(t *testing.T) {
	s := siteSetup(t)
	s.choose(s.home)
	s.publish()
	s.owner.want(204, "POST", "/api/v1/sites/"+s.site+"/withdraw", map[string]any{})
	if s.live("welcome.html") != "" || !strings.Contains(s.live("index.html"), "withdrawn") {
		t.Fatalf("%q", s.live("index.html"))
	}
	if s.auditActions("site.withdrawn") != 1 {
		t.Fatal("a withdrawal must be audited")
	}
}

func TestChosenPagesShowAPublicMark(t *testing.T) {
	s := siteSetup(t)
	reader := s.user("reader@example.com", identity.PermUseProduct)
	s.owner.grant(s.space, "user", reader.id, permissions.TemplateViewer)
	if got := reader.ok("GET", "/api/v1/pages/"+s.home+"/public-state", nil); got["public"] != false {
		t.Fatalf("%+v", got)
	}
	s.choose(s.home)
	if got := reader.ok("GET", "/api/v1/pages/"+s.home+"/public-state", nil); got["public"] != true {
		t.Fatalf("%+v", got)
	}
}

func TestSitesAreOffUntilAnAdministratorSaysSo(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.PublishDir = t.TempDir() })
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("OFF")
	page := owner.page(space, nil, "Page", "x")
	owner.want(409, "POST", "/api/v1/spaces/"+space+"/sites", map[string]any{"name": "No"})
	owner.want(409, "PUT", "/api/v1/pages/"+page+"/public-link", map[string]any{})
}

func linkFile(s *siteEnv, path, name string) string {
	s.t.Helper()
	data, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(strings.Trim(path, "/")), name))
	if err != nil {
		return ""
	}
	return string(data)
}

func TestAPublicLinkShowsOneVersionUntilWithdrawn(t *testing.T) {
	s := siteSetup(t)
	link := s.owner.ok("PUT", "/api/v1/pages/"+s.home+"/public-link", map[string]any{})
	path := link["path"].(string)
	if !regexp.MustCompile(`^/links/[0-9a-f]{32}/$`).MatchString(path) {
		t.Fatalf("path %s", path)
	}
	if !strings.Contains(linkFile(s, path, "index.html"), "welcome words") {
		t.Fatal("no page")
	}
	s.owner.publish(s.home, "Welcome", 1, docWith("newer words"))
	s.deliver()
	if strings.Contains(linkFile(s, path, "index.html"), "newer words") {
		t.Fatal("a later publish reached the link")
	}
	s.owner.want(204, "DELETE", "/api/v1/pages/"+s.home+"/public-link", nil)
	if linkFile(s, path, "index.html") != "" {
		t.Fatal("still there")
	}
}

func TestARestrictionWithdrawsAPublicLinkAtOnce(t *testing.T) {
	s := siteSetup(t)
	link := s.owner.ok("PUT", "/api/v1/pages/"+s.home+"/public-link", map[string]any{})
	s.owner.restrict(s.home, "view", s.owner.id)
	s.deliver()
	if linkFile(s, link["path"].(string), "index.html") != "" {
		t.Fatal("the link survived a restriction")
	}
}

func TestAdministratorSeesAllPublicLinks(t *testing.T) {
	s := siteSetup(t)
	s.owner.ok("PUT", "/api/v1/pages/"+s.home+"/public-link", map[string]any{})
	if got := s.admin.ok("GET", "/api/v1/admin/public-links", nil)["links"].([]any); len(got) != 1 {
		t.Fatalf("%v", got)
	}
	s.owner.want(403, "GET", "/api/v1/admin/public-links", nil)
}
