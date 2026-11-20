// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/document"
	"github.com/kenwerk/kenwerk/internal/pages"
)

// withoutUnproposedDrafts leaves out of a managers' list the drafts that were never proposed,
// whose owners are still active and are someone else: such a draft is its owner's own until
// they propose it or leave (TPL-510, TPL-853).
func (s *Service) withoutUnproposedDrafts(ctx context.Context, list []Template) ([]Template, error) {
	var ids []uuid.UUID
	for _, t := range list {
		if t.TemplateVersion == 0 && t.State == StateDraft && !t.Mine {
			ids = append(ids, t.ID)
		}
	}
	if len(ids) == 0 {
		return list, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT template_id FROM template_reviews
		WHERE template_id = ANY ($1) AND decision = 'proposed'`, ids)
	if err != nil {
		return nil, err
	}
	proposed, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	out := make([]Template, 0, len(list))
	for _, t := range list {
		if slices.Contains(ids, t.ID) && !slices.Contains(proposed, t.ID) {
			owner, err := s.perms.LoadUser(ctx, s.pool, t.ownerID)
			if err != nil {
				return nil, err
			}
			if owner.Active {
				continue
			}
		}
		out = append(out, t)
	}
	return out, nil
}

// markGroupsGone marks each template of a list whose newest version has a condition testing a
// group that was deleted, for the list's warning (TPL-402, TPL-1468). The whole list costs
// three queries: the newest manifests, their texts, and the groups.
func (s *Service) markGroupsGone(ctx context.Context, list []Template) error {
	if len(list) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(list))
	for _, t := range list {
		ids = append(ids, t.ID)
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (template_id) template_id, manifest FROM template_versions
		WHERE template_id = ANY ($1) ORDER BY template_id, number DESC`, ids)
	if err != nil {
		return err
	}
	manifests := map[uuid.UUID]Manifest{}
	var wanted []pages.TemplateText
	for rows.Next() {
		var id uuid.UUID
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return err
		}
		var m Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			rows.Close()
			return err
		}
		m.normalise()
		manifests[id] = m
		for _, e := range m.Entries {
			if e.PageID != nil {
				wanted = append(wanted, pages.TemplateText{PageID: *e.PageID, Number: e.PageVersion})
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	docs, _, err := s.pages.TemplateDocumentsAt(ctx, s.pool, wanted)
	if err != nil {
		return err
	}
	tested := map[uuid.UUID][]string{}
	all := map[string]bool{}
	for i, t := range list {
		m, ok := manifests[t.ID]
		if !ok {
			continue
		}
		texts := map[string]document.Node{}
		for _, e := range m.Entries {
			if e.PageID != nil {
				texts[e.Key] = docs[pages.TemplateText{PageID: *e.PageID, Number: e.PageVersion}]
			}
		}
		groups := groupsTested(list[i], m, texts)
		tested[t.ID] = groups
		for _, g := range groups {
			all[g] = true
		}
	}
	gone, err := s.missingGroups(ctx, s.pool, groupIDs(all))
	if err != nil || len(gone) == 0 {
		return err
	}
	for i, t := range list {
		list[i].GroupsGone = slices.ContainsFunc(tested[t.ID], func(g string) bool { return slices.Contains(gone, g) })
	}
	return nil
}
