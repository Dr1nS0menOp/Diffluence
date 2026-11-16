// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
)

// addExcerpts gives each item about a comment the start of that comment, for the panel
// (COL-118). One query reads every comment at once, by the rule of the mail excerpt: a
// removed comment gives nothing, and a thread kept for editors gives nothing to someone
// who may not edit the page (spec 064, DOC-604). The rows themselves already passed the
// rule of the page, so this runs only on what the person may see now.
func (s *Service) addExcerpts(ctx context.Context, user uuid.UUID, items []Item) error {
	ids := []uuid.UUID{}
	for _, i := range items {
		if i.CommentID != nil {
			ids = append(ids, *i.CommentID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.pool.Query(ctx, `SELECT c.id, c.body FROM comments c
		WHERE c.id = ANY($2) AND c.trash_item_id IS NULL
		  AND (NOT c.editors_only OR c.page_id IN (SELECT e.page_id FROM perm_editable_pages($1) e WHERE e.page_id = c.page_id))`,
		user, ids)
	if err != nil {
		return err
	}
	bodies, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (commentBody, error) {
		var b commentBody
		return b, r.Scan(&b.id, &b.raw)
	})
	if err != nil {
		return err
	}
	texts, err := s.commentTexts(ctx, bodies)
	if err != nil {
		return err
	}
	for n := range items {
		if id := items[n].CommentID; id != nil {
			items[n].Excerpt = texts[*id]
		}
	}
	return nil
}

type commentBody struct {
	id  uuid.UUID
	raw []byte
}

// commentTexts turns comments into short plain text, with today's names of whom they
// mention, asked for all comments in one query.
func (s *Service) commentTexts(ctx context.Context, bodies []commentBody) (map[uuid.UUID]string, error) {
	docs := make([]document.Node, len(bodies))
	mentioned := []uuid.UUID{}
	for n, b := range bodies {
		doc, err := document.Parse(b.raw)
		if err != nil {
			return nil, fmt.Errorf("notify: read comment: %v", err)
		}
		docs[n] = doc
		mentioned = append(mentioned, document.MentionedUsers(doc)...)
		mentioned = append(mentioned, document.MentionedGroups(doc)...)
	}
	names, err := identity.DisplayNames(ctx, s.pool, mentioned)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]string, len(bodies))
	for n, b := range bodies {
		document.FillMentionNames(&docs[n], names)
		words := []rune(strings.TrimSpace(document.BlockText(docs[n])))
		out[b.id] = string(words[:min(len(words), excerptRunes)])
	}
	return out, nil
}
