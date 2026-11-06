// SPDX-License-Identifier: Apache-2.0

// Package example makes the example space: a first introduction that shows
// everything a page can hold (spec 016). It only uses the public services with a
// real caller, so permissions, audit and outbox apply as they do for a person.
//
// To add something, add a document to content/ and a line to content/space.json.
package example

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/spaces"
	"github.com/diffluence/diffluence/internal/templates"
)

//go:embed content
var content embed.FS

// offeredMark remembers that the decision was made, so a deleted example stays gone.
const offeredMark = "example_space_offered"

// firstVersion gives every page a second version, so History and Compare have
// something to show.
const (
	firstVersionText    = "This page is about to be filled in."
	secondVersionReason = "Filled in the example"
	filledVersion       = 2
)

// Placeholders in the documents, which become real ids once they exist.
var (
	filePlaceholder = regexp.MustCompile(`@file:([A-Za-z0-9._-]+)`)
	pagePlaceholder = regexp.MustCompile(`@page:([a-z0-9-]+)`)
)

// ownerPlaceholder is the person the example belongs to, for a mention and a task.
const ownerPlaceholder = "@person:owner"

// planComment is a comment under a page, or on words of it when it has a quote.
type planComment struct {
	Text  string `json:"text"`
	Quote string `json:"quote"`
}

// planStage is the status of a page: a word from the list of the space, and its colour.
type planStage struct {
	Text  string `json:"text"`
	Color string `json:"color"`
}

type planPage struct {
	Slug           string        `json:"slug"`
	Title          string        `json:"title"`
	Document       string        `json:"document"`
	Emoji          string        `json:"emoji"`
	FullWidth      bool          `json:"full_width"`
	TitleAlign     string        `json:"title_align"`
	Header         string        `json:"header"`
	HeaderPosition int           `json:"header_position"`
	Files          []string      `json:"files"`
	EditOnlyOwner  bool          `json:"edit_only_owner"`
	Comments       []planComment `json:"comments"`
	Reactions      []string      `json:"reactions"`
	Labels         []string      `json:"labels"`
	Stage          *planStage    `json:"stage"`
	Folder         bool          `json:"folder"`
	Database       bool          `json:"database"`
	Children       []planPage    `json:"children"`
}

// planPost is a blog post of the space.
type planPost struct {
	Title    string   `json:"title"`
	Document string   `json:"document"`
	Labels   []string `json:"labels"`
}

// planTemplate is a template of the space.
type planTemplate struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Category     string   `json:"category"`
	Document     string   `json:"document"`
	Labels       []string `json:"labels"`
	TitlePattern string   `json:"title_pattern"`
}

type planShortcut struct {
	Label  string `json:"label"`
	Target string `json:"target"`
}

type plan struct {
	Key         string         `json:"key"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Icon        string         `json:"icon"`
	Home        string         `json:"home"`
	HomeFiles   []string       `json:"home_files"`
	Pages       []planPage     `json:"pages"`
	Posts       []planPost     `json:"posts"`
	Templates   []planTemplate `json:"templates"`
	Shortcuts   []planShortcut `json:"shortcuts"`
}

func loadPlan() (plan, error) {
	var p plan
	raw, err := content.ReadFile("content/space.json")
	if err != nil {
		return p, fmt.Errorf("example: read the plan: %w", err)
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("example: read the plan: %w", err)
	}
	return p, nil
}

// walk visits every page, each parent before its children.
func (p plan) walk(visit func(page planPage, parent *planPage)) {
	var down func(list []planPage, parent *planPage)
	down = func(list []planPage, parent *planPage) {
		for i := range list {
			visit(list[i], parent)
			down(list[i].Children, &list[i])
		}
	}
	down(p.Pages, nil)
}

// assets lists every file the pages use.
func (p plan) assets() []string {
	names := append([]string{}, p.HomeFiles...)
	p.walk(func(page planPage, _ *planPage) {
		names = append(names, page.Files...)
		if page.Header != "" {
			names = append(names, page.Header)
		}
	})
	return names
}

// references turn the placeholders of a document into real ids.
type references struct {
	files map[string]uuid.UUID
	pages map[string]uuid.UUID
	owner uuid.UUID
}

func replaceAll(raw []byte, pattern *regexp.Regexp, ids map[string]uuid.UUID) ([]byte, error) {
	var missing string
	out := pattern.ReplaceAllFunc(raw, func(found []byte) []byte {
		name := string(pattern.FindSubmatch(found)[1])
		id, ok := ids[name]
		if !ok {
			missing = name
			return found
		}
		return []byte(id.String())
	})
	if missing != "" {
		return nil, fmt.Errorf("example: nothing is called %q", missing)
	}
	return out, nil
}

func (r references) fill(raw []byte) ([]byte, error) {
	out, err := replaceAll(raw, filePlaceholder, r.files)
	if err != nil {
		return nil, err
	}
	out, err = replaceAll(out, pagePlaceholder, r.pages)
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(out, []byte(ownerPlaceholder), []byte(r.owner.String())), nil
}

// Service makes the example space.
type Service struct {
	identity    *identity.Service
	settings    *settings.Service
	spaces      *spaces.Service
	pages       *pages.Service
	attachments *attachments.Service
	permissions *permissions.Service
	comments    *comments.Service
	templates   *templates.Service
}

// NewService creates the service.
func NewService(id *identity.Service, st *settings.Service, sp *spaces.Service, pg *pages.Service,
	att *attachments.Service, perms *permissions.Service, cm *comments.Service, tpl *templates.Service) *Service {
	return &Service{identity: id, settings: st, spaces: sp, pages: pg, attachments: att, permissions: perms, comments: cm, templates: tpl}
}

// OnNewInstance makes the example space on an instance that has never had a
// space, once there is an administrator to own it. The decision is made once, so
// an instance with spaces gets none, and a deleted example stays gone.
func (s *Service) OnNewInstance(ctx context.Context) error {
	offered, err := s.settings.HasMark(ctx, offeredMark)
	if err != nil || offered {
		return err
	}
	admin, found, err := s.identity.FirstAdministrator(ctx)
	if err != nil || !found {
		return err
	}
	empty, err := s.spaces.NoneExist(ctx)
	if err != nil {
		return err
	}
	if empty {
		// The system acts, and the administrator owns what it makes.
		c := identity.Caller{UserID: admin, System: "example-space"}
		if _, err := s.Create(ctx, c); err != nil {
			return err
		}
	}
	return s.settings.SetMark(ctx, offeredMark)
}

// Create makes the example space with the caller as its owner.
func (s *Service) Create(ctx context.Context, c identity.Caller) (spaces.Space, error) {
	p, err := loadPlan()
	if err != nil {
		return spaces.Space{}, err
	}
	sp, err := s.spaces.Create(ctx, c, spaces.Input{Key: p.Key, Name: p.Name, Description: p.Description, Icon: p.Icon})
	if err != nil {
		return sp, err
	}
	if err := s.permissions.ApplyPreset(ctx, c, sp.ID, permissions.PresetTeamSpace); err != nil {
		return sp, err
	}
	if sp.HomePageID == nil {
		return sp, fmt.Errorf("example: the space has no home page")
	}
	refs := references{files: map[string]uuid.UUID{}, pages: map[string]uuid.UUID{}, owner: c.UserID}
	if err := s.upload(ctx, c, *sp.HomePageID, p.HomeFiles, refs); err != nil {
		return sp, err
	}
	if err := s.makePages(ctx, c, p, sp, refs); err != nil {
		return sp, err
	}
	// The pages exist by now, so documents can link to each other.
	if err := s.fillIn(ctx, c, *sp.HomePageID, p.Name, p.Home, 1, refs); err != nil {
		return sp, err
	}
	var failure error
	p.walk(func(page planPage, _ *planPage) {
		if failure == nil {
			failure = s.finish(ctx, c, page, refs)
		}
	})
	if failure != nil {
		return sp, failure
	}
	if err := s.makePosts(ctx, c, p, sp.ID, refs); err != nil {
		return sp, err
	}
	if err := s.makeTemplates(ctx, c, p, sp.ID, refs); err != nil {
		return sp, err
	}
	if err := s.spaces.SetOwners(ctx, c, sp.ID, []uuid.UUID{c.UserID}); err != nil {
		return sp, err
	}
	return sp, s.setShortcuts(ctx, c, sp.ID, p, refs)
}

func (s *Service) makePosts(ctx context.Context, c identity.Caller, p plan, spaceID uuid.UUID, refs references) error {
	for _, post := range p.Posts {
		created, err := s.pages.Create(ctx, c, pages.NewPage{SpaceID: spaceID, Title: post.Title, Kind: pages.KindPost})
		if err != nil {
			return err
		}
		if err := s.fillIn(ctx, c, created.ID, post.Title, post.Document, 0, refs); err != nil {
			return err
		}
		if err := s.addLabels(ctx, c, created.ID, post.Labels); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) makeTemplates(ctx context.Context, c identity.Caller, p plan, spaceID uuid.UUID, refs references) error {
	for _, template := range p.Templates {
		raw, err := content.ReadFile("content/" + template.Document)
		if err != nil {
			return fmt.Errorf("example: read %s: %w", template.Document, err)
		}
		filled, err := refs.fill(raw)
		if err != nil {
			return err
		}
		_, err = s.templates.Create(ctx, c, &spaceID, templates.Input{Name: template.Name, Description: template.Description,
			Category: template.Category, Labels: template.Labels, TitlePattern: template.TitlePattern, Document: filled})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) addLabels(ctx context.Context, c identity.Caller, pageID uuid.UUID, labels []string) error {
	for _, label := range labels {
		if _, err := s.pages.AddLabel(ctx, c, pageID, label); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) upload(ctx context.Context, c identity.Caller, pageID uuid.UUID, names []string, refs references) error {
	for _, name := range names {
		raw, err := content.ReadFile("content/assets/" + name)
		if err != nil {
			return fmt.Errorf("example: read %s: %w", name, err)
		}
		a, err := s.attachments.Upload(ctx, c, pageID, name, bytes.NewReader(raw))
		if err != nil {
			return err
		}
		refs.files[name] = a.ID
	}
	return nil
}

func oneLine(text string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}}})
	return raw
}

// makePages creates every page with a short first version, parents first: a page
// can only go below a published page.
func (s *Service) makePages(ctx context.Context, c identity.Caller, p plan, sp spaces.Space, refs references) error {
	var failure error
	p.walk(func(page planPage, parent *planPage) {
		if failure != nil {
			return
		}
		under := sp.HomePageID
		if parent != nil {
			id := refs.pages[parent.Slug]
			under = &id
		}
		kind := pages.KindPage
		if page.Database {
			kind = pages.KindDatabase
		}
		created, err := s.pages.Create(ctx, c, pages.NewPage{SpaceID: sp.ID, ParentID: under, Title: page.Title, Kind: kind})
		if err != nil {
			failure = err
			return
		}
		refs.pages[page.Slug] = created.ID
		// A database is born with its first version: an empty sheet.
		if !page.Database {
			_, failure = s.pages.Publish(ctx, c, created.ID, pages.PublishInput{Title: page.Title, Document: oneLine(firstVersionText)})
		}
		if failure != nil {
			return
		}
		names := page.Files
		if page.Header != "" {
			names = append(append([]string{}, names...), page.Header)
		}
		failure = s.upload(ctx, c, created.ID, names, refs)
	})
	return failure
}

func (s *Service) fillIn(ctx context.Context, c identity.Caller, pageID uuid.UUID, title, file string, version int, refs references) error {
	raw, err := content.ReadFile("content/" + file)
	if err != nil {
		return fmt.Errorf("example: read %s: %w", file, err)
	}
	filled, err := refs.fill(raw)
	if err != nil {
		return err
	}
	_, err = s.pages.Publish(ctx, c, pageID, pages.PublishInput{Title: title, Document: filled,
		BaseVersion: version, Summary: secondVersionReason})
	return err
}

// finish publishes the real text of a page, and sets how it looks and who may edit it.
func (s *Service) finish(ctx context.Context, c identity.Caller, page planPage, refs references) error {
	id := refs.pages[page.Slug]
	if err := s.fillIn(ctx, c, id, page.Title, page.Document, 1, refs); err != nil {
		return err
	}
	look := pages.Look{Emoji: page.Emoji, FullWidth: page.FullWidth, TitleAlign: page.TitleAlign, HeaderPosition: page.HeaderPosition}
	if look.TitleAlign == "" {
		look.TitleAlign = pages.AlignStart
	}
	if page.Header != "" {
		header := refs.files[page.Header]
		look.HeaderAttachmentID = &header
	}
	if _, err := s.pages.SetLook(ctx, c, id, look); err != nil {
		return err
	}
	if err := s.respond(ctx, c, page, id); err != nil {
		return err
	}
	if err := s.organise(ctx, c, page, id); err != nil {
		return err
	}
	if !page.EditOnlyOwner {
		return nil
	}
	owner := permissions.Subject{Type: permissions.SubjectUser, ID: c.UserID}
	return s.permissions.SetRestrictions(ctx, c, id, "edit", []permissions.Subject{owner})
}

// organise gives a page its labels and status, and turns it into a folder last,
// because a folder keeps its text in its versions.
func (s *Service) organise(ctx context.Context, c identity.Caller, page planPage, id uuid.UUID) error {
	if err := s.addLabels(ctx, c, id, page.Labels); err != nil {
		return err
	}
	if page.Stage != nil {
		if _, err := s.pages.SetStage(ctx, c, id, page.Stage.Text, page.Stage.Color); err != nil {
			return err
		}
	}
	if !page.Folder {
		return nil
	}
	_, err := s.pages.SetKind(ctx, c, id, pages.KindFolder)
	return err
}

// respond adds the comments and reactions of a page, after its real text is there.
func (s *Service) respond(ctx context.Context, c identity.Caller, page planPage, id uuid.UUID) error {
	for _, comment := range page.Comments {
		in := comments.NewComment{Body: oneLine(comment.Text)}
		if comment.Quote != "" {
			in.Anchor = &comments.Anchor{Kind: comments.AnchorText, Exact: comment.Quote, Version: filledVersion}
		}
		if _, err := s.comments.Create(ctx, c, id, in); err != nil {
			return err
		}
	}
	for _, emoji := range page.Reactions {
		if err := s.comments.SetReaction(ctx, c, comments.TargetPage, id, emoji, true); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) setShortcuts(ctx context.Context, c identity.Caller, spaceID uuid.UUID, p plan, refs references) error {
	list := make([]spaces.ShortcutInput, len(p.Shortcuts))
	for i, shortcut := range p.Shortcuts {
		target, err := refs.fill([]byte(shortcut.Target))
		if err != nil {
			return err
		}
		list[i] = spaces.ShortcutInput{Label: shortcut.Label, Target: string(target)}
	}
	return s.spaces.SetShortcuts(ctx, c, spaceID, list)
}
