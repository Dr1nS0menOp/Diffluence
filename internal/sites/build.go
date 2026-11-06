// SPDX-License-Identifier: Apache-2.0

package sites

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed" // the search of a built site
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
)

//go:embed search.js
var searchScript string

const (
	liveLink        = "live"
	buildsFolder    = "builds"
	keptBuilds      = 3
	maxSitePages    = 50_000 // what one sitemap file can hold
	maxImagePixels  = 64 << 20
	maxShownBytes   = 100 << 20
	folderMode      = 0o755
	fileMode        = 0o644
	jpegQuality     = 90
	neutralMention  = "a colleague"
	securityHeaders = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'">`
	noIndex         = `<meta name="robots" content="noindex">`
)

// Report says what a build holds, and what it left out.
type Report struct {
	Versions  map[uuid.UUID]int `json:"versions"`
	Added     []string          `json:"added"`
	Changed   []string          `json:"changed"`
	Removed   []string          `json:"removed"`
	Left      []string          `json:"left_out"`
	Dead      []string          `json:"dead_links"`
	NoAlt     []string          `json:"no_alt"`
	Metadata  []string          `json:"metadata"`
	Stopped   string            `json:"stopped,omitempty"`
	Pages     int               `json:"pages"`
	Withdrawn bool              `json:"withdrawn,omitempty"`
}

func newReport() *Report {
	return &Report{Versions: map[uuid.UUID]int{}, Added: []string{}, Changed: []string{}, Removed: []string{}, Left: []string{},
		Dead: []string{}, NoAlt: []string{}, Metadata: []string{}}
}

func (s *Service) liveReport(ctx context.Context, v Site) (Report, error) {
	out := *newReport()
	if v.LiveBuild == 0 {
		return out, nil
	}
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT report FROM public_site_builds WHERE site_id = $1 AND number = $2`, v.ID, v.LiveBuild).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(raw, &out)
}

// sitePage is a page on its way to a site.
type sitePage struct {
	page    pages.PublicPage
	doc     document.Node
	file    string
	files   map[uuid.UUID]attachments.Attachment
	updated string
	author  string
}

var notInFile = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func fileFor(title string, used map[string]bool) string {
	base := strings.Trim(notInFile.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if r := []rune(base); len(r) > 60 {
		base = strings.Trim(string(r[:60]), "-")
	}
	if base == "" || base == "index" || base == "search" {
		base = "page-" + base
	}
	name := base
	for i := 2; used[name]; i++ {
		name = base + "-" + strconv.Itoa(i)
	}
	used[name] = true
	return name + ".html"
}

// gather reads what the rule lets through, as a reader who may see only those pages.
func (s *Service) gather(ctx context.Context, v Site, report *Report) ([]sitePage, error) {
	rows, err := s.pool.Query(ctx, `SELECT page_id FROM public_site_pages WHERE site_id = $1`, v.ID)
	if err != nil {
		return nil, err
	}
	chosen, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	if len(chosen) > maxSitePages {
		return nil, apperr.TooLarge("A site can hold at most 50,000 pages.")
	}
	allowed, err := s.mayBePublic(ctx, s.pool, v.SpaceID, chosen)
	if err != nil {
		return nil, err
	}
	pass := make([]uuid.UUID, 0, len(allowed))
	for id := range allowed {
		pass = append(pass, id)
	}
	for range len(chosen) - len(pass) {
		report.Left = append(report.Left, "A chosen page has a restriction, or is no longer published.")
	}
	list, err := s.pages.ForPublic(ctx, pass, v.ShowNames)
	if err != nil {
		return nil, err
	}
	return s.prepare(ctx, list, report)
}

func (s *Service) prepare(ctx context.Context, list []pages.PublicPage, report *Report) ([]sitePage, error) {
	used := map[string]bool{}
	out := make([]sitePage, 0, len(list))
	for _, p := range list {
		doc, err := document.Parse(p.Document)
		if err != nil {
			report.Left = append(report.Left, p.Title+": this page cannot be read.")
			continue
		}
		stored, err := s.attachments.ListForPage(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		files := map[uuid.UUID]attachments.Attachment{}
		for _, a := range stored {
			files[a.ID] = a
		}
		out = append(out, sitePage{page: p, doc: doc, file: fileFor(p.Title, used), files: files,
			updated: p.UpdatedAt.Format(time.DateOnly), author: p.Author})
		report.Versions[p.ID] = p.Version
	}
	report.Pages = len(out)
	return out, nil
}

// builder writes one site into a folder.
type builder struct {
	s       *Service
	v       Site
	dir     string
	list    []sitePage
	links   map[uuid.UUID]string
	report  *Report
	nextPic int
	search  []searchEntry
	single  bool // one page on its own, as a public link: no site around it
}

type searchEntry struct {
	Title string `json:"t"`
	URL   string `json:"u"`
	Text  string `json:"x"`
}

func (b *builder) write(name, body string) error {
	file := filepath.Join(b.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), folderMode); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(body), fileMode)
}

func (b *builder) nav(current uuid.UUID) string {
	nodes := map[uuid.UUID]*document.NavItem{}
	children := map[uuid.UUID][]uuid.UUID{}
	var top []uuid.UUID
	for _, p := range b.list {
		nodes[p.page.ID] = &document.NavItem{Title: p.page.Title, Href: url.PathEscape(p.file), Current: p.page.ID == current}
	}
	for _, p := range b.list {
		if parent := p.page.ParentID; parent != nil && nodes[*parent] != nil {
			children[*parent] = append(children[*parent], p.page.ID)
			continue
		}
		top = append(top, p.page.ID)
	}
	var tree func(ids []uuid.UUID) []document.NavItem
	tree = func(ids []uuid.UUID) []document.NavItem {
		out := make([]document.NavItem, 0, len(ids))
		for _, id := range ids {
			item := *nodes[id]
			item.Children = tree(children[id])
			out = append(out, item)
		}
		return out
	}
	return document.NavHTML(tree(top))
}

func (b *builder) head() string {
	head := securityHeaders + `<link rel="stylesheet" href="theme.css">`
	if !b.v.SearchEngines {
		head += noIndex
	}
	if b.single {
		return head
	}
	return head + `<script src="search.js" defer></script>`
}

const searchBox = `<form class="site-search" role="search" action="index.html"><label for="q">Search this site</label>` +
	`<input id="q" type="search" autocomplete="off"><div id="results" role="status"></div></form>`

func (b *builder) shell(title, body, nav string) string {
	if !b.single {
		nav = searchBox + nav
	}
	return document.Shell{Title: title, Site: b.v.Name, Language: b.v.Language, StyleHref: "style.css", NavLabel: "Pages",
		SkipLabel: "Skip to the content", Nav: nav, Head: b.head(), Body: body}.HTML()
}

func (b *builder) page(ctx context.Context, p sitePage) error {
	shown := map[uuid.UUID]string{}
	for _, id := range document.AttachmentIDs(p.doc) {
		a, ok := p.files[id]
		if !ok {
			continue
		}
		name, err := b.file(ctx, p, a)
		if err != nil {
			return err
		}
		if name != "" {
			shown[id] = name
		}
	}
	document.Walk(p.doc, func(n document.Node) {
		if alt, _ := n.Attrs["alt"].(string); n.Type == document.TypeImage && alt == "" {
			b.report.NoAlt = append(b.report.NoAlt, p.page.Title)
		}
	})
	local := func(id uuid.UUID) (string, bool) {
		href, ok := shown[id]
		return href, ok
	}
	options := document.HTMLOptions{ImageSrc: local, FileHref: local,
		PageHref: func(id uuid.UUID) (string, bool) {
			href, ok := b.links[id]
			if !ok {
				b.report.Dead = append(b.report.Dead, p.page.Title)
			}
			return href, ok
		}}
	if !b.v.ShowNames {
		options.Mention = func(document.Node) string { return neutralMention }
	}
	p.doc = document.ResolveLinks(p.doc, b.titleOnSite)
	body := document.ToHTML(p.doc, options)
	footer := "Updated " + p.updated
	if b.v.ShowNames && p.author != "" {
		footer += " by " + p.author
	}
	body += `<p class="updated">` + html.EscapeString(footer) + `</p>`
	b.search = append(b.search, searchEntry{Title: p.page.Title, URL: url.PathEscape(p.file), Text: searchText(p.doc, b.v.ShowNames)})
	nav := b.nav(p.page.ID)
	if b.single {
		nav = ""
	}
	return b.write(p.file, b.shell(p.page.Title, body, nav))
}

// titleOnSite names a page on this site. Anything else stays out of a public file (spec 023, D4).
func (b *builder) titleOnSite(id uuid.UUID) (string, bool) {
	if _, ok := b.links[id]; !ok {
		return "", false
	}
	for _, p := range b.list {
		if p.page.ID == id {
			return p.page.Title, true
		}
	}
	return "", false
}

// searchText is what the index may hold: only the words that are on the page itself,
// with people's names left out the way the page leaves them out (SEC-383).
func searchText(doc document.Node, names bool) string {
	if !names {
		doc = withoutMentions(doc)
	}
	return document.Text(doc)
}

func withoutMentions(n document.Node) document.Node {
	out := n
	out.Content = make([]document.Node, 0, len(n.Content))
	for _, c := range n.Content {
		if c.Type != document.TypeMention {
			out.Content = append(out.Content, withoutMentions(c))
		}
	}
	return out
}

// file brings along one file a page shows (D16). JPEG and PNG are written anew, which
// leaves their metadata behind. What cannot be cleaned is named in the report.
func (b *builder) file(ctx context.Context, p sitePage, a attachments.Attachment) (string, error) {
	src, err := b.s.attachments.OpenUnchecked(ctx, a)
	if apperr.IsNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = src.Close() }()
	data, err := io.ReadAll(io.LimitReader(src, maxShownBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxShownBytes {
		b.report.Left = append(b.report.Left, p.page.Title+": a file is too large for a site.")
		return "", nil
	}
	b.nextPic++
	ext := strings.ToLower(path.Ext(a.Filename))
	cleaned, ok := withoutMetadata(data, ext)
	if !ok {
		b.report.Left = append(b.report.Left, p.page.Title+": an image could not be cleaned, so it stayed out.")
		return "", nil
	}
	if ext == ".gif" || ext == ".webp" {
		b.report.Metadata = append(b.report.Metadata, p.page.Title)
	}
	name := fmt.Sprintf("files/%d%s", b.nextPic, safeExtension(ext))
	if err := b.write(name, string(cleaned)); err != nil {
		return "", err
	}
	return name, nil
}

var plainExtension = regexp.MustCompile(`^\.[a-z0-9]{1,8}$`)

func safeExtension(ext string) string {
	if plainExtension.MatchString(ext) {
		return ext
	}
	return ""
}

// withoutMetadata re-encodes JPEG and PNG. A picture of a size that could exhaust
// memory is refused from its header, before it is decoded.
func withoutMetadata(data []byte, ext string) ([]byte, bool) {
	switch ext {
	case ".jpg", ".jpeg", ".png":
	default:
		return data, true
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width*config.Height > maxImagePixels {
		return nil, false
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	var out bytes.Buffer
	if ext == ".png" {
		err = png.Encode(&out, img)
	} else {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality})
	}
	return out.Bytes(), err == nil
}

func (b *builder) index() error {
	var body strings.Builder
	body.WriteString(`<p>All pages of this site.</p>`)
	body.WriteString(b.nav(uuid.Nil))
	return b.write("index.html", b.shell(b.v.Name, body.String(), b.nav(uuid.Nil)))
}

func (b *builder) robots() error {
	if !b.v.SearchEngines {
		return b.write("robots.txt", "User-agent: *\nDisallow: /\n")
	}
	text := "User-agent: *\nAllow: /\n"
	if b.v.Address != "" {
		text += "Sitemap: " + b.v.Address + "/sitemap.xml\n"
	}
	if err := b.write("robots.txt", text); err != nil {
		return err
	}
	if b.v.Address == "" {
		return nil
	}
	var m strings.Builder
	m.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
	for _, p := range b.list {
		m.WriteString(`<url><loc>` + html.EscapeString(b.v.Address+"/"+url.PathEscape(p.file)) + `</loc><lastmod>` + p.updated + `</lastmod></url>`)
	}
	m.WriteString(`</urlset>`)
	return b.write("sitemap.xml", m.String())
}

func (b *builder) theme() string {
	accent := Accents[b.v.Accent]
	return fmt.Sprintf(":root { --accent: light-dark(%s, %s); }\n", accent[0], accent[1])
}

func (b *builder) all(ctx context.Context) error {
	b.links = map[uuid.UUID]string{}
	for _, p := range b.list {
		b.links[p.page.ID] = url.PathEscape(p.file)
	}
	for name, body := range map[string]string{"style.css": document.ExportStyle, "theme.css": b.theme(), "search.js": searchScript} {
		if err := b.write(name, body); err != nil {
			return err
		}
	}
	for _, p := range b.list {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := b.page(ctx, p); err != nil {
			return err
		}
	}
	if err := b.index(); err != nil {
		return err
	}
	raw, err := json.Marshal(b.search)
	if err != nil {
		return err
	}
	if err := b.write("search.json", string(raw)); err != nil {
		return err
	}
	return b.robots()
}

// folder gives the folder of a site. Its name came from targetFor, and this is the
// only place a path is made from it.
func (s *Service) folder(v Site) string { return filepath.Join(s.options.PublishDir, v.Target) }

func (s *Service) removeFolder(v Site) error {
	if s.options.PublishDir == "" {
		return nil
	}
	return os.RemoveAll(s.folder(v))
}

// switchLive points the address of a site at a build, in one atomic rename.
func switchLive(root string, number int) error {
	temp := filepath.Join(root, liveLink+".next")
	_ = os.Remove(temp)
	if err := os.Symlink(filepath.Join(buildsFolder, strconv.Itoa(number)), temp); err != nil {
		return err
	}
	return os.Rename(temp, filepath.Join(root, liveLink))
}

// build makes a site now and switches to it. A failed build keeps the old site.
func (s *Service) build(ctx context.Context, c identity.Caller, v Site) (Report, error) {
	report := newReport()
	if s.options.PublishDir == "" {
		return *report, apperr.Conflict("sites_off", "Public sites are off on this instance.")
	}
	number, err := s.startBuild(ctx, v)
	if err != nil {
		return *report, err
	}
	list, err := s.gather(ctx, v, report)
	if err != nil {
		return *report, s.failBuild(ctx, v, number, report, err)
	}
	root := s.folder(v)
	dir := filepath.Join(root, buildsFolder, strconv.Itoa(number))
	if err := os.MkdirAll(dir, folderMode); err != nil {
		return *report, s.failBuild(ctx, v, number, report, fmt.Errorf("sites: the target cannot be written: %w", err))
	}
	b := &builder{s: s, v: v, dir: dir, list: list, report: report}
	if err := b.all(ctx); err != nil {
		_ = os.RemoveAll(dir)
		return *report, s.failBuild(ctx, v, number, report, err)
	}
	if err := s.scrub(ctx, dir); err != nil {
		_ = os.RemoveAll(dir)
		return *report, s.failBuild(ctx, v, number, report, err)
	}
	if err := switchLive(root, number); err != nil {
		_ = os.RemoveAll(dir)
		return *report, s.failBuild(ctx, v, number, report, err)
	}
	if err := s.finishBuild(ctx, c, v, number, report); err != nil {
		return *report, err
	}
	return *report, s.pruneBuilds(ctx, v)
}

func (s *Service) startBuild(ctx context.Context, v Site) (int, error) {
	var number int
	err := s.pool.QueryRow(ctx, `INSERT INTO public_site_builds (site_id, number, state)
		SELECT $1, COALESCE(max(number), 0) + 1, 'building' FROM public_site_builds WHERE site_id = $1 RETURNING number`, v.ID).Scan(&number)
	return number, err
}

func (s *Service) failBuild(ctx context.Context, v Site, number int, report *Report, cause error) error {
	report.Stopped = "This build could not be finished."
	if a, ok := apperr.As(cause); ok {
		report.Stopped = a.Message
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if _, err := s.pool.Exec(finish, `UPDATE public_site_builds SET state = 'failed', report = $3, finished_at = now() WHERE site_id = $1 AND number = $2`,
		v.ID, number, raw); err != nil {
		return err
	}
	return cause
}

func (s *Service) finishBuild(ctx context.Context, c identity.Caller, v Site, number int, report *Report) error {
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE public_site_builds SET state = 'old' WHERE site_id = $1 AND state = 'live'`, v.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE public_site_builds SET state = 'live', report = $3, finished_at = now() WHERE site_id = $1 AND number = $2`,
			v.ID, number, raw); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE public_sites SET live_build = $2, withdrawn_at = NULL, updated_at = now() WHERE id = $1`, v.ID, number); err != nil {
			return err
		}
		return record(ctx, tx, c, "site.published", v, map[string]any{"build": number, "pages": report.Pages, "left_out": len(report.Left)})
	})
}

// pruneBuilds keeps the last few, so a build can be rolled back.
func (s *Service) pruneBuilds(ctx context.Context, v Site) error {
	rows, err := s.pool.Query(ctx, `SELECT number FROM public_site_builds WHERE site_id = $1 AND state = 'old' ORDER BY number DESC OFFSET $2`, v.ID, keptBuilds-1)
	if err != nil {
		return err
	}
	old, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return err
	}
	for _, number := range old {
		if err := os.RemoveAll(filepath.Join(s.folder(v), buildsFolder, strconv.Itoa(number))); err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, `DELETE FROM public_site_builds WHERE site_id = $1 AND number = $2`, v.ID, number); err != nil {
			return err
		}
	}
	return nil
}

// Preview says what the next build would add, change and remove, and what to watch.
func (s *Service) Preview(ctx context.Context, c identity.Caller, id uuid.UUID) (Report, error) {
	v, err := s.owned(ctx, c, id)
	if err != nil {
		return Report{}, err
	}
	report := newReport()
	list, err := s.gather(ctx, v, report)
	if err != nil {
		return *report, err
	}
	live, err := s.liveReport(ctx, v)
	if err != nil {
		return *report, err
	}
	for _, p := range list {
		before, there := live.Versions[p.page.ID]
		switch {
		case !there:
			report.Added = append(report.Added, p.page.Title)
		case before != p.page.Version:
			report.Changed = append(report.Changed, p.page.Title)
		}
		s.warnAbout(p, report)
	}
	gone, err := s.titlesLeaving(ctx, live, report)
	if err != nil {
		return *report, err
	}
	report.Removed = gone
	return *report, nil
}

func (s *Service) titlesLeaving(ctx context.Context, live Report, next *Report) ([]string, error) {
	leaving := []uuid.UUID{}
	for id := range live.Versions {
		if _, stays := next.Versions[id]; !stays {
			leaving = append(leaving, id)
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT title FROM pages WHERE id = ANY($1) ORDER BY title`, leaving)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (s *Service) warnAbout(p sitePage, report *Report) {
	chosen := report.Versions
	for _, id := range document.LinkedPages(p.doc) {
		if _, ok := chosen[id]; !ok {
			report.Dead = append(report.Dead, p.page.Title)
		}
	}
	document.Walk(p.doc, func(n document.Node) {
		if n.Type != document.TypeImage {
			return
		}
		if alt, _ := n.Attrs["alt"].(string); alt == "" {
			report.NoAlt = append(report.NoAlt, p.page.Title)
		}
	})
	for _, id := range document.AttachmentIDs(p.doc) {
		if a, ok := p.files[id]; ok {
			if ext := strings.ToLower(path.Ext(a.Filename)); ext == ".gif" || ext == ".webp" {
				report.Metadata = append(report.Metadata, p.page.Title)
			}
		}
	}
}

// Publish asks for a build. The first one needs the name of the site typed in.
func (s *Service) Publish(ctx context.Context, c identity.Caller, id uuid.UUID, confirm string) (uuid.UUID, error) {
	v, err := s.owned(ctx, c, id)
	if err != nil {
		return uuid.Nil, err
	}
	if err := s.requireEnabled(ctx); err != nil {
		return uuid.Nil, err
	}
	if v.LiveBuild == 0 && strings.TrimSpace(confirm) != v.Name {
		return uuid.Nil, apperr.Invalid("Type the name of the site to publish it for the first time.")
	}
	var job uuid.UUID
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		job, err = s.jobs.Enqueue(ctx, tx, JobKindBuild, buildPayload{SiteID: id}, c.UserID)
		return err
	})
	return job, err
}

type buildPayload struct {
	SiteID uuid.UUID `json:"site_id"`
}

func (s *Service) runBuild(ctx context.Context, j jobs.Job) (any, error) {
	var in buildPayload
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	v, err := load(ctx, s.pool, in.SiteID)
	if err != nil {
		return nil, err
	}
	c := identity.Caller{UserID: j.CreatedBy, Meta: identity.RequestMeta{RequestID: "job-" + j.ID.String()}}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, v.SpaceID, permissions.SpaceAdminister); err != nil {
		return nil, err
	}
	report, err := s.build(ctx, c, v)
	return report, err
}

// Build is one entry of a site's list of builds.
type Build struct {
	Number     int        `json:"number"`
	State      string     `json:"state"`
	Report     Report     `json:"report"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// Builds lists the builds of a site, newest first.
func (s *Service) Builds(ctx context.Context, c identity.Caller, id uuid.UUID) ([]Build, error) {
	if _, err := s.owned(ctx, c, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT number, state, report, started_at, finished_at FROM public_site_builds WHERE site_id = $1 ORDER BY number DESC`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Build, error) {
		var out Build
		var raw []byte
		if err := r.Scan(&out.Number, &out.State, &raw, &out.StartedAt, &out.FinishedAt); err != nil {
			return out, err
		}
		return out, json.Unmarshal(raw, &out.Report)
	})
}

// Rollback goes back to an earlier build. The current rules run over it first: a
// build that holds a page which may no longer be public cannot come back, because
// its search index and links would still name that page.
func (s *Service) Rollback(ctx context.Context, c identity.Caller, id uuid.UUID, number int) error {
	v, err := s.owned(ctx, c, id)
	if err != nil {
		return err
	}
	var raw []byte
	var state string
	err = s.pool.QueryRow(ctx, `SELECT state, report FROM public_site_builds WHERE site_id = $1 AND number = $2`, id, number).Scan(&state, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound()
	}
	if err != nil {
		return err
	}
	if state != "old" {
		return apperr.Conflict("build_not_old", "Choose an earlier build.")
	}
	report := *newReport()
	if err := json.Unmarshal(raw, &report); err != nil {
		return err
	}
	if err := s.buildStillAllowed(ctx, v, report); err != nil {
		return err
	}
	if err := switchLive(s.folder(v), number); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE public_site_builds SET state = 'old' WHERE site_id = $1 AND state = 'live'`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE public_site_builds SET state = 'live' WHERE site_id = $1 AND number = $2`, id, number); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE public_sites SET live_build = $2, updated_at = now() WHERE id = $1`, id, number); err != nil {
			return err
		}
		return record(ctx, tx, c, "site.changed", v, map[string]any{"rolled_back_to": number})
	})
}

func (s *Service) buildStillAllowed(ctx context.Context, v Site, report Report) error {
	shown := make([]uuid.UUID, 0, len(report.Versions))
	for page := range report.Versions {
		shown = append(shown, page)
	}
	allowed, err := s.mayBePublic(ctx, s.pool, v.SpaceID, shown)
	if err != nil {
		return err
	}
	gone, err := s.versionsGone(ctx, report.Versions)
	if err != nil {
		return err
	}
	if len(allowed) != len(shown) || gone {
		return apperr.Conflict("build_outdated", "A page in that build may no longer be public. Publish a new build instead.")
	}
	return nil
}

const withdrawnPage = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="robots" content="noindex"><title>Withdrawn</title></head>` +
	`<body><main><h1>This site was withdrawn</h1><p>It is no longer available.</p></main></body></html>`

// Withdraw empties the target and leaves a page that says so.
func (s *Service) Withdraw(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	v, err := s.owned(ctx, c, id)
	if err != nil {
		return err
	}
	return s.withdraw(ctx, c, v)
}

func (s *Service) withdraw(ctx context.Context, c identity.Caller, v Site) error {
	number, err := s.startBuild(ctx, v)
	if err != nil {
		return err
	}
	root := s.folder(v)
	dir := filepath.Join(root, buildsFolder, strconv.Itoa(number))
	if err := os.MkdirAll(dir, folderMode); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(withdrawnPage), fileMode); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "robots.txt"), []byte("User-agent: *\nDisallow: /\n"), fileMode); err != nil {
		return err
	}
	if err := switchLive(root, number); err != nil {
		return err
	}
	if err := removeOtherBuilds(root, number); err != nil {
		return err
	}
	raw, err := json.Marshal(Report{Versions: map[uuid.UUID]int{}, Withdrawn: true})
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM public_site_builds WHERE site_id = $1 AND number <> $2`, v.ID, number); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE public_site_builds SET state = 'live', report = $3, finished_at = now() WHERE site_id = $1 AND number = $2`,
			v.ID, number, raw); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE public_sites SET live_build = 0, withdrawn_at = now(), build_requested = NULL, updated_at = now() WHERE id = $1`, v.ID); err != nil {
			return err
		}
		return record(ctx, tx, c, "site.withdrawn", v, nil)
	})
}

func removeOtherBuilds(root string, keep int) error {
	folders, err := os.ReadDir(filepath.Join(root, buildsFolder))
	if err != nil {
		return err
	}
	for _, f := range folders {
		if f.Name() != strconv.Itoa(keep) {
			if err := os.RemoveAll(filepath.Join(root, buildsFolder, f.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// WriteZip writes the live build of a site as a zip.
func (s *Service) WriteZip(ctx context.Context, c identity.Caller, id uuid.UUID, w io.Writer) error {
	v, err := s.owned(ctx, c, id)
	if err != nil {
		return err
	}
	if v.LiveBuild == 0 {
		return apperr.Conflict("site_not_live", "This site has no build yet.")
	}
	dir := filepath.Join(s.folder(v), buildsFolder, strconv.Itoa(v.LiveBuild))
	zw := zip.NewWriter(w)
	if err := zw.AddFS(os.DirFS(dir)); err != nil {
		return err
	}
	return zw.Close()
}

// textFiles lists what a scrub reads. A file a page shows is a copy of what it was
// given, so only what we wrote is read.
func textFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".html", ".json", ".xml", ".txt":
			if !d.IsDir() {
				out = append(out, p)
			}
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}
