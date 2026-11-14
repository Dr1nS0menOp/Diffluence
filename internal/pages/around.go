// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/search"
)

// The content around a controlled page (spec 064, wave DC3): comments, files, search,
// exports and notifications take the version a person reads from the one rule of DOC-300,
// permissions.ReadsWorking.

// WorkingDiffers reports whether a controlled page has a working version that readers do not
// see yet. Comments ask it, inside their own transaction, to keep a remark about the working
// version for its authors (DOC-500).
func (s *Service) WorkingDiffers(ctx context.Context, q db.Querier, pageID uuid.UUID) (bool, error) {
	var differs bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM page_control pc JOIN pages p ON p.id = pc.page_id
		WHERE pc.page_id = $1 AND pc.approved_version IS NOT NULL AND pc.head_version > p.current_version)`, pageID).Scan(&differs)
	return differs, err
}

// Controlled reports whether a page follows a workflow, and the version its readers see, for
// a module that must treat its readers apart, such as a mail about it.
func Controlled(ctx context.Context, q db.Querier, pageID uuid.UUID) (bool, int, error) {
	var controlled bool
	var current int
	err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM page_control WHERE page_id = $1), COALESCE((SELECT current_version FROM pages WHERE id = $1), 0)",
		pageID).Scan(&controlled, &current)
	return controlled, current, err
}

// VersionSeen is the version of a page a person reads: the working version for its authors
// and reviewers, and the version on the pages row for everyone else (DOC-300). Analytics
// count reads by it, so a report shows who read which version (DOC-571).
func VersionSeen(ctx context.Context, q db.Querier, perms *permissions.Service, userID, pageID uuid.UUID) (int, error) {
	var current, head int
	if err := q.QueryRow(ctx, `SELECT p.current_version, COALESCE((SELECT pc.head_version FROM page_control pc WHERE pc.page_id = p.id), 0)
		FROM pages p WHERE p.id = $1`, pageID).Scan(&current, &head); err != nil {
		return 0, err
	}
	if head <= current {
		return current, nil
	}
	working, err := perms.ReadsWorking(ctx, q, userID, pageID)
	if err != nil || !working {
		return current, err
	}
	return head, nil
}

// FileChanged follows a change of a file: a new version, an older one back, or a new name.
// When the working version of a controlled page shows the file, the change is a change of
// the page, so it writes a new version with the same text, which starts review again; the
// readers keep the version of the file the approved version pinned (DOC-510, DOC-511). It
// runs inside the transaction of the change, and implements attachments.ContentChanges.
func (s *Service) FileChanged(ctx context.Context, tx pgx.Tx, c identity.Caller, pageID, attachmentID uuid.UUID) error {
	p, err := lockPage(ctx, tx, pageID)
	if err != nil {
		return err
	}
	var controlled bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM page_control WHERE page_id = $1)", pageID).Scan(&controlled); err != nil {
		return err
	}
	if !controlled || p.CurrentVersion == 0 {
		return nil
	}
	head, err := headVersion(ctx, tx, p)
	if err != nil {
		return err
	}
	shown, err := attachments.Shows(ctx, tx, pageID, head, attachmentID)
	if err != nil || !shown {
		return err
	}
	title, doc, err := headDocument(ctx, tx, p)
	if err != nil {
		return err
	}
	_, err = s.appendVersion(ctx, tx, c, p, newVersion{Title: title, Doc: doc, Author: c.UserID})
	return err
}

// mentionedIn reads the people and groups a version mentions. A version that does not exist
// mentions nobody.
func mentionedIn(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, version int) (people, groups []uuid.UUID, err error) {
	if version <= 0 {
		return nil, nil, nil
	}
	var raw []byte
	err = tx.QueryRow(ctx, "SELECT document FROM page_versions WHERE page_id = $1 AND number = $2", pageID, version).Scan(&raw)
	if err != nil {
		return nil, nil, err
	}
	doc, err := document.Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	return document.MentionedUsers(doc), document.MentionedGroups(doc), nil
}

// newIn lists who is in now and was not before.
func newIn(now, before []uuid.UUID) []uuid.UUID {
	out := []uuid.UUID{}
	for _, id := range now {
		if !slices.Contains(before, id) && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// workingEvent tells other modules of a working version: watchers and webhooks of those who
// may edit hear of it, and nobody else (DOC-552, DOC-603). Events hold ids, never text.
func workingEvent(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, version int, payload map[string]any) error {
	payload["page_id"], payload["space_id"], payload["version"], payload["kind"] = p.ID, p.SpaceID, version, p.Kind
	if _, set := payload["actor_id"]; !set {
		payload["actor_id"] = c.UserID
	}
	c.Stamp(payload)
	_, err := outbox.Enqueue(ctx, tx, "page.working_changed", payload)
	return err
}

// announceWorking follows a version that waits for review: the second entry of search holds
// it (DOC-520), and those who may edit hear of it, with the people it newly mentions; the
// others mentioned hear of it when it is approved (DOC-606).
func (s *Service) announceWorking(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, previousHead int, in newVersion, number int) error {
	if err := search.IndexWorking(ctx, tx, p.ID, in.Title, document.Text(in.Doc)); err != nil {
		return err
	}
	people, groups, err := mentionedIn(ctx, tx, p.ID, previousHead)
	if err != nil {
		return err
	}
	return workingEvent(ctx, tx, c, p, number, map[string]any{"actor_id": in.Author, "silent": in.Silent,
		"mentions": newIn(document.MentionedUsers(in.Doc), people), "mentioned_groups": newIn(document.MentionedGroups(in.Doc), groups)})
}

// announceApproval tells other modules that a new version became the one readers see. It
// names the version approved before, so a mail shows the change between two approved
// versions and never a version nobody approved, and the people the new version mentions
// that the version before did not: their mention was held until now (DOC-604, DOC-606).
func announceApproval(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, ctl control, doc document.Node) error {
	previous := 0
	if ctl.approvedVersion != nil {
		previous = *ctl.approvedVersion
	}
	people, groups, err := mentionedIn(ctx, tx, p.ID, previous)
	if err != nil {
		return err
	}
	payload := map[string]any{"page_id": p.ID, "space_id": p.SpaceID, "version": ctl.headVersion, "previous": previous, "kind": p.Kind,
		"actor_id": c.UserID, "mentions": newIn(document.MentionedUsers(doc), people), "mentioned_groups": newIn(document.MentionedGroups(doc), groups)}
	c.Stamp(payload)
	_, err = outbox.Enqueue(ctx, tx, "page.version_approved", payload)
	return err
}
