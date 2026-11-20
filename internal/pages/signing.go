// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/attachments"
	"github.com/kenwerk/kenwerk/internal/audit"
	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/ids"
	"github.com/kenwerk/kenwerk/internal/permissions"
	"github.com/kenwerk/kenwerk/internal/workflow"
)

// Electronic signatures (spec 064, wave DC4, blueprint 09.8). A step of a workflow can
// require a signature: an author's own step is signed by its author, and the step that
// follows an approved review by every reviewer who approves. The signature takes a fresh
// confirmation of identity made for it alone, and is written in the transaction of its step
// with an audit entry that holds the hash of the signed content (DOC-700 to DOC-733).

// Confirmations is the part of the identity module that signing needs: a confirmation of who
// signs, begun for one signature and taken by it once (DOC-730).
type Confirmations interface {
	BeginSigning(ctx context.Context, c identity.Caller, in identity.SigningRequest) (identity.SigningStart, error)
	TakeSigningConfirmation(ctx context.Context, tx pgx.Tx, c identity.Caller, sessionID uuid.UUID, token, purpose string) (string, error)
	// MaySign says whether a person may sign at all, for a delegation or a stand-in.
	MaySign(ctx context.Context, q db.Querier, id uuid.UUID) (bool, error)
}

// SetConfirmations connects the identity module. It is set once while the application is
// wired, before any request.
func (s *Service) SetConfirmations(cf Confirmations) { s.confirmations = cf }

// SignatureInput is what a step that requires a signature carries: the session the person
// acts in, which the handler takes from the login and never from the request, and the
// token of their confirmation.
type SignatureInput struct {
	SessionID    uuid.UUID
	Confirmation string
}

// transitionPurpose and reviewPurpose bind a confirmation to exactly one signature: the
// page, its version and the step, or the stage of the round a reviewer answers.
func transitionPurpose(pageID uuid.UUID, version int, transition string) string {
	return fmt.Sprintf("page %s version %d step %s", pageID, version, transition)
}

func reviewPurpose(pageID uuid.UUID, version int, round uuid.UUID, stage int) string {
	return fmt.Sprintf("page %s version %d review %s stage %d approve", pageID, version, round, stage)
}

// signTarget is the one signature a step makes.
type signTarget struct {
	page       Page
	ctl        control
	transition workflow.Transition
	round      *uuid.UUID
	stage      *int
	purpose    string
}

// sign writes the signature of the person for the head version of a page, inside the
// transaction of its step (DOC-712): a failure of the step leaves no signature, and the
// confirmation is used only when the step commits. It answers the signature's id.
func (s *Service) sign(ctx context.Context, tx pgx.Tx, c identity.Caller, in SignatureInput, t signTarget) (uuid.UUID, error) {
	if s.confirmations == nil {
		return uuid.Nil, errors.New("pages: signing is not connected to identity")
	}
	method, err := s.confirmations.TakeSigningConfirmation(ctx, tx, c, in.SessionID, in.Confirmation, t.purpose)
	if err != nil {
		return uuid.Nil, err
	}
	meaning, err := requireMeaning(ctx, tx, t.page.SpaceID, t.transition.SignatureMeaning)
	if err != nil {
		return uuid.Nil, err
	}
	hash, err := s.versionHash(ctx, tx, t.page.ID, t.ctl.headVersion)
	if err != nil {
		return uuid.Nil, err
	}
	names, err := identity.DisplayNames(ctx, tx, []uuid.UUID{c.UserID})
	if err != nil {
		return uuid.Nil, err
	}
	// The name of this moment is the printed name, kept if the person is renamed (DOC-1225).
	printed := names[c.UserID]
	id := ids.New()
	details := map[string]any{"signature": id, "version": t.ctl.headVersion, "content_hash": hexHash(hash), "content_form": ContentForm,
		"meaning": meaning.Key, "method": method, "transition": t.transition.ID, "workflow": t.ctl.workflowID,
		"workflow_version": t.ctl.workflowVersion}
	if t.round != nil {
		details["round"], details["stage"] = *t.round, *t.stage
	}
	ev, err := audit.Record(ctx, tx, c.Event("page.signed", "page", t.page.ID.String(), t.page.SpaceID.String(), details))
	if err != nil {
		return uuid.Nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO signatures (id, page_id, space_id, version_number, workflow_id, workflow_version, transition_id,
			round_id, stage, signer_id, printed_name, meaning, meaning_name, meaning_description, method, content_hash, content_form, audit_event_id, signed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		id, t.page.ID, t.page.SpaceID, t.ctl.headVersion, t.ctl.workflowID, t.ctl.workflowVersion, t.transition.ID,
		t.round, t.stage, c.UserID, printed, meaning.Key, meaning.Name, meaning.Description, method, hash, ContentForm, ev.ID, ev.OccurredAt)
	return id, err
}

// versionHash gives the content hash of a version, storing it at its first signature. A
// version whose content no longer gives the hash stored for it was changed behind
// Kenwerk's back, and nobody signs it (DOC-741).
func (s *Service) versionHash(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, number int) ([]byte, error) {
	// The files a version shows are pinned when it is written; a version from before pins
	// existed gets them now, which is what it shows.
	if err := attachments.Pin(ctx, tx, pageID, number); err != nil {
		return nil, err
	}
	content, err := signedContentOf(ctx, tx, pageID, number)
	if err != nil {
		return nil, err
	}
	hash := content.Hash()
	var stored []byte
	var form int16
	err = tx.QueryRow(ctx, "SELECT content_hash, content_form FROM version_hashes WHERE page_id = $1 AND number = $2",
		pageID, number).Scan(&stored, &form)
	if errors.Is(err, pgx.ErrNoRows) {
		// Two signatures of one version at once both wait for the lock of the page row, so
		// the second finds the first one's hash here.
		_, err := tx.Exec(ctx, "INSERT INTO version_hashes (page_id, number, content_hash, content_form) VALUES ($1, $2, $3, $4)",
			pageID, number, hash, ContentForm)
		return hash, err
	}
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(stored, hash) || form != ContentForm {
		return nil, apperr.Conflict("content_changed", "This version no longer matches the hash kept for it, so it cannot be signed. Ask whoever looks after the installation.")
	}
	return hash, nil
}

// SignatureStep names the step a person is about to sign: an author's own step, or their
// approval as a reviewer.
type SignatureStep struct {
	// Transition is the id of an author's step; empty for a reviewer's approval.
	Transition string
	// Version is the version the person looked at.
	Version   int
	SessionID uuid.UUID
}

// SignaturePrompt is what the signing dialog shows before anything else: the meaning in
// plain words, the version and its title (DOC-720), and how the person can confirm.
type SignaturePrompt struct {
	PageID   uuid.UUID             `json:"page_id"`
	Title    string                `json:"title"`
	Version  int                   `json:"version"`
	StepName string                `json:"step_name"`
	Meaning  workflow.Meaning      `json:"meaning"`
	Start    identity.SigningStart `json:"start"`
}

// BeginSignature opens the signing step for one signature. It checks what the step itself
// checks, so nobody is asked to confirm a signature they could not give (DOC-760).
func (s *Service) BeginSignature(ctx context.Context, c identity.Caller, id uuid.UUID, in SignatureStep) (SignaturePrompt, error) {
	if s.confirmations == nil {
		return SignaturePrompt{}, errors.New("pages: signing is not connected to identity")
	}
	var out SignaturePrompt
	var purpose string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var t signTarget
		var err error
		if in.Transition != "" {
			t, err = s.authorStepToSign(ctx, tx, c, id, in)
		} else {
			t, err = s.approvalToSign(ctx, tx, c, id, in)
		}
		if err != nil {
			return err
		}
		meaning, err := requireMeaning(ctx, tx, t.page.SpaceID, t.transition.SignatureMeaning)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "SELECT title FROM page_versions WHERE page_id = $1 AND number = $2", id, t.ctl.headVersion).Scan(&out.Title); err != nil {
			return err
		}
		out.PageID, out.Version, out.StepName, out.Meaning, purpose = id, t.ctl.headVersion, t.transition.Name, meaning, t.purpose
		return nil
	})
	if err != nil {
		return SignaturePrompt{}, err
	}
	out.Start, err = s.confirmations.BeginSigning(ctx, c, identity.SigningRequest{SessionID: in.SessionID, Purpose: purpose,
		ReturnPath: "/pages/" + id.String()})
	return out, err
}

// authorStepToSign is an author's step that requires a signature, checked as Transition
// checks it.
func (s *Service) authorStepToSign(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID, in SignatureStep) (signTarget, error) {
	if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
		return signTarget{}, err
	}
	p, ctl, def, err := readControlled(ctx, tx, id)
	if err != nil {
		return signTarget{}, err
	}
	tr, ok := def.Transition(in.Transition)
	switch {
	case !ok:
		return signTarget{}, apperr.Invalid("This workflow has no such step.")
	case tr.Outcome != "":
		return signTarget{}, apperr.Conflict("taken_by_review", "This step follows the review: the reviewers' answers take it.")
	case tr.From != ctl.stateID:
		return signTarget{}, apperr.Conflict("state_changed", "This page has moved on since you looked: it is in another state now.")
	case in.Version != ctl.headVersion:
		return signTarget{}, apperr.Conflict("superseded", "A newer version of this page was published. Look at it first.")
	case tr.SignatureMeaning == "":
		return signTarget{}, apperr.Conflict("no_signature", "This step needs no signature.")
	}
	return signTarget{page: p, ctl: ctl, transition: tr, purpose: transitionPurpose(id, ctl.headVersion, tr.ID)}, nil
}

// approvalToSign is a reviewer's approval in a stage whose passing step requires a
// signature, checked as AnswerReview checks it.
func (s *Service) approvalToSign(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID, in SignatureStep) (signTarget, error) {
	if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionApprovalRead, uuid.Nil); err != nil {
		return signTarget{}, err
	}
	rc, err := s.lockReview(ctx, tx, id)
	if err != nil {
		return signTarget{}, err
	}
	asked, _, err := myReview(ctx, tx, rc.stage, c.UserID)
	if err != nil {
		return signTarget{}, err
	}
	if !asked {
		return signTarget{}, apperr.Forbidden()
	}
	if in.Version != rc.ctl.headVersion {
		return signTarget{}, apperr.Conflict("superseded", "A newer version of this page was published. Look at it first.")
	}
	t, signed := approvalSignature(rc)
	if !signed {
		return signTarget{}, apperr.Conflict("no_signature", "This approval needs no signature.")
	}
	return t, nil
}

// approvalSignature is the signature an approval in the current stage requires, if any.
func approvalSignature(rc reviewContext) (signTarget, bool) {
	tr, ok := rc.def.Outcome(rc.state.ID, workflow.OutcomeApproved)
	if !ok || tr.SignatureMeaning == "" {
		return signTarget{}, false
	}
	round, stage := rc.stage.round, rc.stage.number
	return signTarget{page: rc.page, ctl: rc.ctl, transition: tr, round: &round, stage: &stage,
		purpose: reviewPurpose(rc.page.ID, rc.ctl.headVersion, round, stage)}, true
}

// readControlled reads a controlled page with its control and the workflow version it follows.
func readControlled(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Page, control, workflow.Definition, error) {
	p, err := lockPage(ctx, tx, id)
	if err != nil {
		return p, control{}, workflow.Definition{}, err
	}
	ctl, controlled, err := lockControl(ctx, tx, id)
	if err != nil {
		return p, ctl, workflow.Definition{}, err
	}
	if !controlled {
		return p, ctl, workflow.Definition{}, apperr.Conflict("not_controlled", "This page follows no workflow.")
	}
	def, err := definitionOf(ctx, tx, ctl.workflowID, ctl.workflowVersion)
	return p, ctl, def, err
}

// Meanings of signatures (DOC-721): the four every space has, and those a space adds.

const (
	maxMeaningNameRunes = 60
	maxMeaningDescRunes = 300
	maxMeaningsPerSpace = 30
)

// meaningOf finds a meaning a space may sign with.
func meaningOf(ctx context.Context, q db.Querier, spaceID uuid.UUID, key string) (workflow.Meaning, bool, error) {
	if m, ok := workflow.BuiltInMeaning(key); ok {
		return m, true, nil
	}
	m := workflow.Meaning{Key: key}
	err := q.QueryRow(ctx, "SELECT name, description FROM signature_meanings WHERE space_id = $1 AND key = $2", spaceID, key).
		Scan(&m.Name, &m.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, false, nil
	}
	return m, err == nil, err
}

func requireMeaning(ctx context.Context, q db.Querier, spaceID uuid.UUID, key string) (workflow.Meaning, error) {
	m, ok, err := meaningOf(ctx, q, spaceID, key)
	if err != nil {
		return m, err
	}
	if !ok {
		return m, apperr.Conflict("meaning_unknown", "This step names a meaning of a signature that this space does not have.")
	}
	return m, nil
}

// checkMeanings refuses a workflow that names a meaning its space does not have. spaceID nil
// means a template of the instance, which may name only the meanings every space has.
func checkMeanings(ctx context.Context, q db.Querier, spaceID uuid.UUID, d workflow.Definition) error {
	for _, key := range d.SignatureMeanings() {
		ok := false
		if _, builtIn := workflow.BuiltInMeaning(key); builtIn {
			ok = true
		} else if spaceID != uuid.Nil {
			var err error
			if _, ok, err = meaningOf(ctx, q, spaceID, key); err != nil {
				return err
			}
		}
		if !ok {
			return apperr.Conflict("workflow_bad_meaning", fmt.Sprintf("The meaning %q of a signature does not exist here.", key))
		}
	}
	return nil
}

// SignatureMeanings lists the meanings a space can sign with: whoever views it may see them.
func (s *Service) SignatureMeanings(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]workflow.Meaning, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return nil, err
	}
	out := workflow.BuiltInMeanings()
	rows, err := s.pool.Query(ctx, "SELECT key, name, description FROM signature_meanings WHERE space_id = $1 ORDER BY created_at, key LIMIT $2",
		spaceID, maxMeaningsPerSpace)
	if err != nil {
		return nil, err
	}
	own, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (workflow.Meaning, error) {
		var m workflow.Meaning
		err := r.Scan(&m.Key, &m.Name, &m.Description)
		return m, err
	})
	return append(out, own...), err
}

// MeaningInput is a meaning a space adds.
type MeaningInput struct {
	Name        string
	Description string
}

var notKeyChars = regexp.MustCompile(`[^a-z0-9]+`)

// meaningKey makes a key from a name; a name with no letters a key can hold gets a number.
func meaningKey(name string, n int) string {
	key := strings.Trim(notKeyChars.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(key) > 30 {
		key = strings.Trim(key[:30], "-")
	}
	if key == "" || !workflow.MeaningKeyPattern.MatchString(key) {
		key = "meaning"
	}
	if n > 1 {
		key = fmt.Sprintf("%s-%d", key, n)
	}
	return key
}

// AddSignatureMeaning adds a meaning to a space (DOC-721). Whoever looks after its workflows
// may. A meaning is never removed or changed: records keep its words, and workflows its key.
func (s *Service) AddSignatureMeaning(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in MeaningInput) (workflow.Meaning, error) {
	name, desc := strings.TrimSpace(in.Name), strings.TrimSpace(in.Description)
	var out workflow.Meaning
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceWorkflows); err != nil {
			return err
		}
		if name == "" || utf8.RuneCountInString(name) > maxMeaningNameRunes || strings.ContainsAny(name, "\r\n\x00") {
			return apperr.Invalid("A meaning needs a name of one line and at most 60 characters.")
		}
		if desc == "" || utf8.RuneCountInString(desc) > maxMeaningDescRunes || strings.ContainsRune(desc, 0) {
			return apperr.Invalid("A meaning needs a description of at most 300 characters, shown to whoever signs.")
		}
		var count int
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT count(*), COALESCE(bool_or(lower(name) = lower($2)), false) FROM signature_meanings WHERE space_id = $1`,
			spaceID, name).Scan(&count, &taken); err != nil {
			return err
		}
		for _, m := range workflow.BuiltInMeanings() {
			taken = taken || strings.EqualFold(m.Name, name)
		}
		if taken {
			return apperr.Conflict("meaning_exists", "This space already has a meaning with that name.")
		}
		if count >= maxMeaningsPerSpace {
			return apperr.Conflict("too_many_meanings", "A space has at most 30 meanings of its own.")
		}
		for n := 1; ; n++ {
			key := meaningKey(name, n)
			if _, ok := workflow.BuiltInMeaning(key); ok {
				continue
			}
			tag, err := tx.Exec(ctx, `INSERT INTO signature_meanings (space_id, key, name, description, created_by) VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (space_id, key) DO NOTHING`, spaceID, key, name, desc, c.UserID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 1 {
				out = workflow.Meaning{Key: key, Name: name, Description: desc}
				break
			}
		}
		return recordAudit(ctx, tx, c, "space.signature_meaning_added", "space", spaceID, spaceID, map[string]any{"meaning": out.Key})
	})
	return out, err
}
