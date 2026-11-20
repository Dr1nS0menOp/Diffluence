// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/audit"
	"github.com/kenwerk/kenwerk/internal/document"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/pages"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// Verifying a twin (PUB-732): "this published text is the approved text". The draft version a
// release took must still hash as it did, and the same rewrite, done again from the link map
// the release kept, must give exactly the document the twin holds. The twin's text differs from
// the draft's only by what the map says, so the comparison is exact.

// The outcomes of a verification.
const (
	VerifyOK           = "verified"
	VerifyDiffers      = "differs"
	VerifyDraftChanged = "draft_changed"
	VerifyNotReleased  = "not_released"
	// VerifyDraftPurged is a twin whose draft page or version was purged: there is nothing left to
	// compare it with (PUB-920, PUB-1012).
	VerifyDraftPurged = "draft_purged"
	// VerifyCannot is a twin released before releases kept what verification needs; its next
	// release can be verified.
	VerifyCannot = "cannot_verify"
)

// Verification is what verifying the twin of a draft page found.
type Verification struct {
	Outcome      string     `json:"outcome"`
	TwinPageID   *uuid.UUID `json:"twin_page_id,omitempty"`
	DraftVersion int        `json:"draft_version,omitempty"`
	TwinVersion  int        `json:"twin_version,omitempty"`
}

// releasedFacts is the release that wrote the version a twin shows now.
type releasedFacts struct {
	twin         uuid.UUID
	draftVersion int
	twinVersion  int
	hash         string
	links        map[string]string
}

// Verify checks the twin of a draft page against the draft version it came from, for those who
// see the audit of the draft space, and audits the check.
func (s *Service) Verify(ctx context.Context, c identity.Caller, pageID uuid.UUID) (Verification, error) {
	var out Verification
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
			return err
		}
		src, err := pages.SourceForRelease(ctx, tx, pageID)
		if err != nil {
			return err
		}
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, src.SpaceID, permissions.SpaceAudit); err != nil {
			return err
		}
		p, err := activePairOf(ctx, tx, src.SpaceID, false)
		if err != nil {
			return err
		}
		if p.Draft != src.SpaceID {
			return apperr.Conflict("not_a_draft_page", "Verification starts from the page in the draft space.")
		}
		if out, err = s.verify(ctx, tx, p, pageID); err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("release.verified", "page", pageID.String(), p.Draft.String(),
			map[string]any{"pair_id": p.ID, "outcome": out.Outcome, "twin_page_id": out.TwinPageID,
				"draft_version": out.DraftVersion, "twin_version": out.TwinVersion}))
		return err
	})
	return out, err
}

func (s *Service) verify(ctx context.Context, tx pgx.Tx, p pair, pageID uuid.UUID) (Verification, error) {
	var out Verification
	var twin *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT pp.twin_page_id FROM page_pairs pp WHERE pp.draft_page_id = $1 AND pp.pair_id = $2 AND pp.released_version > 0`,
		pageID, p.ID).Scan(&twin)
	if noRows(err) {
		out.Outcome = VerifyNotReleased
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.TwinPageID = twin
	t, err := pages.SourceForRelease(ctx, tx, *twin)
	if err != nil {
		return out, err
	}
	f, found, err := releasedNow(ctx, tx, p, pageID, *twin, t.CurrentVersion)
	if err != nil {
		return out, err
	}
	if !found {
		// The version readers have was written by no release.
		out.Outcome = VerifyDiffers
		return out, nil
	}
	out.DraftVersion, out.TwinVersion = f.draftVersion, f.twinVersion
	if f.links[mapDraftID] == "" && f.links[mapStart] == "" {
		out.Outcome = VerifyCannot
		return out, nil
	}
	draft, err := pages.VersionForRelease(ctx, tx, pageID, f.draftVersion)
	if apperr.IsNotFound(err) {
		out.Outcome = VerifyDraftPurged
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if draft.Hash != f.hash {
		out.Outcome = VerifyDraftChanged
		return out, nil
	}
	held, err := pages.VersionForRelease(ctx, tx, f.twin, f.twinVersion)
	if err != nil {
		return out, err
	}
	// A release made the twin from the draft; a start made the draft from the twin. Either change
	// is done again from its map, in its own direction, and compared with what the other side holds.
	from, want := draft.Doc, held.Hash
	redo := rewriteFromMap
	if f.links[mapStart] != "" {
		from, want, redo = held.Doc, draft.Hash, startFromMap
	}
	again, err := redo(from, f.links)
	if err != nil {
		return out, err
	}
	var hash string
	if err := tx.QueryRow(ctx, `SELECT page_pair_content_hash($1::jsonb)`, again).Scan(&hash); err != nil {
		return out, err
	}
	out.Outcome = VerifyDiffers
	if hash == want {
		out.Outcome = VerifyOK
	}
	return out, nil
}

// startFromMap does the change of a start again from the map it kept, and gives the document as
// the copy in the draft space would hold it.
func startFromMap(doc document.Node, links map[string]string) ([]byte, error) {
	m, err := startMapFrom(links)
	if err != nil {
		return nil, err
	}
	again, _, err := startDocument(doc, m)
	if err != nil {
		return nil, err
	}
	return document.Marshal(again)
}

// releasedNow reads the release that wrote the version a twin shows now.
func releasedNow(ctx context.Context, tx pgx.Tx, p pair, pageID, twin uuid.UUID, twinVersion int) (releasedFacts, bool, error) {
	var f releasedFacts
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT rp.twin_page_id, rp.draft_version, rp.twin_version, rp.content_hash, rp.link_map
		FROM release_run_pages rp JOIN release_runs r ON r.id = rp.run_id
		WHERE r.pair_id = $1 AND rp.draft_page_id = $2 AND rp.twin_page_id = $3 AND rp.outcome IN ('released', 'started') AND rp.twin_version = $4
		ORDER BY r.finished_at DESC NULLS LAST LIMIT 1`, p.ID, pageID, twin, twinVersion).Scan(&f.twin, &f.draftVersion, &f.twinVersion, &f.hash, &raw)
	if noRows(err) {
		return f, false, nil
	}
	if err != nil {
		return f, false, err
	}
	if err := json.Unmarshal(raw, &f.links); err != nil {
		return f, false, err
	}
	return f, true, nil
}

// rewriteFromMap does the rewrite of a release again from the link map it kept, and gives the
// document as the twin would hold it.
func rewriteFromMap(doc document.Node, links map[string]string) ([]byte, error) {
	in, err := inputFromMap(links)
	if err != nil {
		return nil, err
	}
	raw, err := document.Marshal(rewrite(doc, in).doc)
	if err != nil {
		return nil, err
	}
	normal, err := document.Normalize(raw)
	if err != nil {
		return nil, err
	}
	return document.Marshal(normal)
}

// inputFromMap is the input of the rewrite a release did, read back from its link map.
func inputFromMap(links map[string]string) (rewriteInput, error) {
	draftID, err := uuid.Parse(links[mapDraftID])
	if err != nil {
		return rewriteInput{}, err
	}
	in := rewriteInput{draft: spaceRef{ID: draftID, Key: links[mapDraftKey]}, published: spaceRef{Key: links[mapPublishedKey]},
		twins: map[uuid.UUID]uuid.UUID{}, targets: map[uuid.UUID]pages.LinkTarget{}, files: map[uuid.UUID]uuid.UUID{},
		foreign: map[uuid.UUID]bool{}}
	// Another space only needs to differ from the draft space for the rewrite to keep it.
	elsewhere := uuid.Max
	for key, value := range links {
		prefix, rest, cut := strings.Cut(key, ":")
		id, err := uuid.Parse(rest)
		if !cut {
			id, err = uuid.Parse(key)
			prefix = ""
		}
		if err != nil {
			continue // the names of the spaces
		}
		switch prefix {
		case "":
			to, err := uuid.Parse(value)
			if err != nil {
				return in, err
			}
			in.twins[id] = to
			in.targets[id] = pages.LinkTarget{SpaceID: draftID, Kind: pages.KindPage}
		case strings.TrimSuffix(mapText, ":"):
			kind, title, _ := strings.Cut(value, ":")
			in.targets[id] = pages.LinkTarget{SpaceID: draftID, Kind: kind, Title: title}
		case strings.TrimSuffix(mapKeep, ":"):
			in.targets[id] = pages.LinkTarget{SpaceID: elsewhere, Kind: pages.KindPage}
		case strings.TrimSuffix(mapFile, ":"):
			to, err := uuid.Parse(value)
			if err != nil {
				return in, err
			}
			in.files[id] = to
		case strings.TrimSuffix(mapForeign, ":"):
			in.foreign[id] = true
		}
	}
	return in, nil
}
