// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Approval of templates (spec 074, TPL-510 to TPL-525): one decision by one person about one
// version. The approved version stays in use while a newer one waits.

const (
	// maxReasonRunes bounds a comment or a reason, as the table does.
	maxReasonRunes = 2000
	// maxApprovers is how many people one proposal tells (TPL-722).
	maxApprovers = 50
)

// The decisions recorded about a version (TPL-522, TPL-602).
const (
	DecisionProposed  = "proposed"
	DecisionApproved  = "approved"
	DecisionSentBack  = "sent_back"
	DecisionWithdrawn = "withdrawn"
)

// Decision is one proposal or decision about a version, with who and why.
type Decision struct {
	ID         uuid.UUID `json:"id"`
	Version    int       `json:"version"`
	Decision   string    `json:"decision"`
	PersonID   uuid.UUID `json:"person_id"`
	PersonName string    `json:"person_name"`
	Comment    string    `json:"comment"`
	At         time.Time `json:"at"`
}

// reviewsOf reads the proposals and decisions of a template, oldest first.
func reviewsOf(ctx context.Context, q db.Querier, templateID uuid.UUID) ([]Decision, error) {
	rows, err := q.Query(ctx, `SELECT id, version, decision, person_id, comment, created_at FROM template_reviews
		WHERE template_id = $1 ORDER BY created_at, id`, templateID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Decision, error) {
		var d Decision
		err := r.Scan(&d.ID, &d.Version, &d.Decision, &d.PersonID, &d.Comment, &d.At)
		return d, err
	})
}

var (
	errSelfApproval = &apperr.Error{Kind: apperr.KindForbidden, Code: "self_approval",
		Message: "Every change to templates here needs a second person, so the author of a version cannot decide on it."}
	errNotNewest = &apperr.Error{Kind: apperr.KindConflict, Code: "version_superseded",
		Message: "A newer version of this template was saved since, so this one cannot be proposed or decided on. Look at the newest."}
	errNotProposable = &apperr.Error{Kind: apperr.KindConflict, Code: "not_proposable",
		Message: "This version waits for a decision or is in use already, so it cannot be proposed."}
	errNothingApproved = &apperr.Error{Kind: apperr.KindConflict, Code: "nothing_approved",
		Message: "This template has no approved version to withdraw."}
)

// cleanReason checks the words of a comment or a reason. required says the words must be
// there, as a reason for sending back or withdrawing is (TPL-522, TPL-524).
func cleanReason(text string, required reasonNeed) (string, error) {
	text = strings.TrimSpace(text)
	if required == reasonRequired && text == "" {
		return "", apperr.Invalid("Say why, so the author knows what to change.")
	}
	if utf8.RuneCountInString(text) > maxReasonRunes {
		return "", apperr.Invalid("A comment or a reason can have at most 2000 characters.")
	}
	return text, nil
}

// reasonNeed says whether words are required with a decision.
type reasonNeed int

const (
	reasonOptional reasonNeed = iota
	reasonRequired
)

// lockVersion takes the row of one version for the rest of tx, so two decisions about it take
// turns and one wins (TPL-855).
func lockVersion(ctx context.Context, tx pgx.Tx, templateID uuid.UUID, number int) (Version, error) {
	v, err := scanVersion(tx.QueryRow(ctx, "SELECT "+versionColumns+` FROM template_versions
		WHERE template_id = $1 AND number = $2 FOR UPDATE`, templateID, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, apperr.NotFound()
	}
	return v, err
}

// requireNewest refuses a version that is no longer the newest (TPL-856).
func requireNewest(ctx context.Context, tx pgx.Tx, templateID uuid.UUID, v Version) error {
	latest, _, err := latestVersion(ctx, tx, templateID)
	if err != nil {
		return err
	}
	if latest.Number != v.Number {
		return errNotNewest
	}
	return nil
}

// mayDecideOwn says whether a person may decide on a version they wrote themselves: only
// where no second person is asked for, as a manager publishes their own work directly there
// anyway (TPL-511, TPL-520).
func mayDecideOwn(ctx context.Context, q db.Querier, st standing, v Version) (bool, error) {
	if v.AuthorID != st.userID {
		return true, nil
	}
	on, err := fourEyesOn(ctx, q, st.t.SpaceID)
	return !on, err
}

// Propose asks for approval of the newest version of a template: a draft, one sent back, or a
// withdrawn one proposed again (TPL-512, TPL-524). Whoever may edit the template proposes.
func (s *Service) Propose(ctx context.Context, c identity.Caller, id uuid.UUID, number int) (History, error) {
	if err := s.holdForOlderRelease(ctx); err != nil {
		return History{}, err
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		t, err := lockTemplate(ctx, tx, c.UserID, id)
		if err != nil {
			return err
		}
		st, err := s.requireTo(ctx, tx, c, t, standing.mayEdit)
		if err != nil {
			return err
		}
		v, err := lockVersion(ctx, tx, id, number)
		if err != nil {
			return err
		}
		if err := requireNewest(ctx, tx, id, v); err != nil {
			return err
		}
		if !slices.Contains([]string{StateDraft, StateSentBack, StateWithdrawn}, v.State) {
			return errNotProposable
		}
		if err := setStates(ctx, tx, id, number, StateWaiting); err != nil {
			return err
		}
		approvers, err := s.approversOf(ctx, tx, t, st.rights, v.AuthorID)
		if err != nil {
			return err
		}
		return s.recordDecision(ctx, tx, c, t, decisionRecord{version: number, decision: DecisionProposed,
			event: "template.proposed", tell: approvers})
	})
	if err != nil {
		return History{}, err
	}
	return s.Versions(ctx, c, id)
}

// Approve makes a waiting version the one the template offers, after checking it again
// against the world as it is now (TPL-520 to TPL-525). The version it replaces stays usable
// for a day for a form already open on it.
func (s *Service) Approve(ctx context.Context, c identity.Caller, id uuid.UUID, number int, comment string) (History, error) {
	comment, err := cleanReason(comment, reasonOptional)
	if err != nil {
		return History{}, err
	}
	err = s.decide(ctx, c, id, number, func(tx pgx.Tx, t Template, v Version) (decisionRecord, error) {
		if _, err := s.checkVersion(ctx, tx, t, v.Manifest, forUse); err != nil {
			return decisionRecord{}, err
		}
		if err := supersedeApproved(ctx, tx, id); err != nil {
			return decisionRecord{}, err
		}
		if err := setStates(ctx, tx, id, number, StateApproved); err != nil {
			return decisionRecord{}, err
		}
		if _, err := tx.Exec(ctx, "UPDATE templates SET approved_version = $2 WHERE id = $1", id, number); err != nil {
			return decisionRecord{}, err
		}
		return decisionRecord{version: number, decision: DecisionApproved, comment: comment, event: "template.approved",
			tell: []uuid.UUID{v.AuthorID}}, nil
	})
	if err != nil {
		return History{}, err
	}
	return s.Versions(ctx, c, id)
}

// SendBack returns a waiting version to its author with a reason they see (TPL-522).
func (s *Service) SendBack(ctx context.Context, c identity.Caller, id uuid.UUID, number int, reason string) (History, error) {
	reason, err := cleanReason(reason, reasonRequired)
	if err != nil {
		return History{}, err
	}
	err = s.decide(ctx, c, id, number, func(tx pgx.Tx, _ Template, v Version) (decisionRecord, error) {
		if err := setStates(ctx, tx, id, number, StateSentBack); err != nil {
			return decisionRecord{}, err
		}
		return decisionRecord{version: number, decision: DecisionSentBack, comment: reason, event: "template.sent_back",
			tell: []uuid.UUID{v.AuthorID}}, nil
	})
	if err != nil {
		return History{}, err
	}
	return s.Versions(ctx, c, id)
}

// decide runs one decision about a waiting version, by someone who manages the template and,
// where four eyes are asked for, did not write it. The rows of the template and the version
// are held, so two people deciding at once make one decision (TPL-855).
func (s *Service) decide(ctx context.Context, c identity.Caller, id uuid.UUID, number int,
	apply func(tx pgx.Tx, t Template, v Version) (decisionRecord, error)) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		t, err := lockTemplate(ctx, tx, c.UserID, id)
		if err != nil {
			return err
		}
		st, err := s.requireTo(ctx, tx, c, t, standing.manages)
		if err != nil {
			return err
		}
		v, err := lockVersion(ctx, tx, id, number)
		if err != nil {
			return err
		}
		// Only the newest version waits: a newer save ended the proposal (TPL-522, TPL-856).
		if v.State != StateWaiting {
			return s.alreadyDecided(ctx, tx, id, number)
		}
		ownOK, err := mayDecideOwn(ctx, tx, st, v)
		if err != nil {
			return err
		}
		if !ownOK {
			return errSelfApproval
		}
		record, err := apply(tx, t, v)
		if err != nil {
			return err
		}
		return s.recordDecision(ctx, tx, c, t, record)
	})
}

// alreadyDecided names who decided on a version that no longer waits (TPL-855).
func (s *Service) alreadyDecided(ctx context.Context, q db.Querier, templateID uuid.UUID, number int) error {
	var person uuid.UUID
	err := q.QueryRow(ctx, `SELECT person_id FROM template_reviews WHERE template_id = $1 AND version = $2
		AND decision <> 'proposed' ORDER BY created_at DESC, id DESC LIMIT 1`, templateID, number).Scan(&person)
	if errors.Is(err, pgx.ErrNoRows) {
		return &apperr.Error{Kind: apperr.KindConflict, Code: "not_waiting", Message: "This version does not wait for a decision."}
	}
	if err != nil {
		return err
	}
	names, err := identity.DisplayNames(ctx, q, []uuid.UUID{person})
	if err != nil {
		return err
	}
	return &apperr.Error{Kind: apperr.KindConflict, Code: "already_decided",
		Message: "This version was already decided on by " + names[person] + "."}
}

// Withdraw takes the approved version of a template out of use at once, with a reason. Forms
// already open are refused on submit, and pages made from it stay as they are (TPL-524). It
// may be proposed again.
func (s *Service) Withdraw(ctx context.Context, c identity.Caller, id uuid.UUID, reason string) (History, error) {
	reason, err := cleanReason(reason, reasonRequired)
	if err != nil {
		return History{}, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		t, err := lockTemplate(ctx, tx, c.UserID, id)
		if err != nil {
			return err
		}
		if _, err := s.requireTo(ctx, tx, c, t, standing.manages); err != nil {
			return err
		}
		if t.TemplateVersion == 0 {
			return errNothingApproved
		}
		// Every version that could still be used goes out of use, the one replaced within the
		// day included, held as a set holds them (lockVersionToUse).
		if _, err := tx.Exec(ctx, `UPDATE template_versions SET state = 'withdrawn'
			WHERE template_id = $1 AND state IN ('approved', 'superseded')`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE templates SET approved_version = NULL,
				state = (SELECT v.state FROM template_versions v WHERE v.template_id = $1 ORDER BY v.number DESC LIMIT 1)
			WHERE id = $1`, id); err != nil {
			return err
		}
		tell, err := s.managersToTell(ctx, tx, t)
		if err != nil {
			return err
		}
		return s.recordDecision(ctx, tx, c, t, decisionRecord{version: t.TemplateVersion, decision: DecisionWithdrawn,
			comment: reason, event: "template.withdrawn", tell: append(tell, t.ownerID)})
	})
	if err != nil {
		return History{}, err
	}
	return s.Versions(ctx, c, id)
}

// setStates gives one version a state, and the template the state of its newest version.
func setStates(ctx context.Context, tx pgx.Tx, templateID uuid.UUID, number int, state string) error {
	if _, err := tx.Exec(ctx, "UPDATE template_versions SET state = $3 WHERE template_id = $1 AND number = $2",
		templateID, number, state); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "UPDATE templates SET state = $2, updated_at = now() WHERE id = $1", templateID, state)
	return err
}

// decisionRecord is what one proposal or decision writes down and whom it tells.
type decisionRecord struct {
	version  int
	decision string
	comment  string
	event    string
	tell     []uuid.UUID
}

// recordDecision keeps a proposal or decision, writes its audit event and the outbox event
// that tells people, in the transaction of the change (TPL-711, TPL-722). The event carries
// ids only: its words are found again when it is shown, by those who may still see it.
func (s *Service) recordDecision(ctx context.Context, tx pgx.Tx, c identity.Caller, t Template, r decisionRecord) error {
	reviewID := ids.New()
	if _, err := tx.Exec(ctx, `INSERT INTO template_reviews (id, template_id, version, decision, person_id, comment)
		VALUES ($1, $2, $3, $4, $5, $6)`, reviewID, t.ID, r.version, r.decision, c.UserID, r.comment); err != nil {
		return err
	}
	spaceID, scope := "", "all_spaces"
	if t.SpaceID != nil {
		spaceID, scope = t.SpaceID.String(), "space"
	}
	details := map[string]any{"name": t.Name, "version": r.version, "scope": scope}
	if r.comment != "" {
		details["reason"] = r.comment
	}
	if r.decision == DecisionApproved {
		details["approver"] = c.UserID
	}
	if _, err := audit.Record(ctx, tx, c.Event(r.event, "template", t.ID.String(), spaceID, details)); err != nil {
		return err
	}
	home, err := s.homeOf(ctx, tx, t.SpaceID)
	if err != nil {
		return err
	}
	people := []uuid.UUID{}
	for _, p := range r.tell {
		if p != uuid.Nil && !slices.Contains(people, p) {
			people = append(people, p)
		}
	}
	_, err = outbox.Enqueue(ctx, tx, r.event, map[string]any{"template_id": t.ID, "page_id": t.pageID, "space_id": home,
		"actor_id": c.UserID, "version": r.version, "user_ids": people, "review_id": reviewID, "decision": r.decision})
	return err
}

// approversOf names who may decide on a version of a template: whoever manages its scope and
// is on its editors list when it has one, but never its author (TPL-520, TPL-521). It names at
// most maxApprovers, each once.
func (s *Service) approversOf(ctx context.Context, q db.Querier, t Template, rights Rights, author uuid.UUID) ([]uuid.UUID, error) {
	candidates, err := s.managersOf(ctx, q, t.SpaceID)
	if err != nil {
		return nil, err
	}
	out := []uuid.UUID{}
	for _, person := range candidates {
		if person == author || len(out) == maxApprovers {
			continue
		}
		if len(rights.Edit) > 0 && t.SpaceID != nil {
			facts, err := s.perms.LoadUser(ctx, q, person)
			if err != nil {
				return nil, err
			}
			if !matches(person, facts.Groups, rights.Edit) {
				continue
			}
		}
		out = append(out, person)
	}
	return out, nil
}

// managersToTell names the managers of a template's scope that a withdrawal tells (TPL-722).
func (s *Service) managersToTell(ctx context.Context, q db.Querier, t Template) ([]uuid.UUID, error) {
	list, err := s.managersOf(ctx, q, t.SpaceID)
	if len(list) > maxApprovers {
		list = list[:maxApprovers]
	}
	return list, err
}

// managersOf names the active people who manage the templates of a scope: holders of
// space.templates in a space, by themselves or through a group, or the administrators for
// all spaces. Each is checked by the rule itself, so a grant that no longer works is no one.
func (s *Service) managersOf(ctx context.Context, q db.Querier, spaceID *uuid.UUID) ([]uuid.UUID, error) {
	if spaceID == nil {
		admins, _, err := s.identity.ListUsers(ctx, identity.UserFilter{Role: identity.PermAdministrator, Status: "active", Limit: 200})
		out := make([]uuid.UUID, 0, len(admins))
		for _, a := range admins {
			out = append(out, a.ID)
		}
		return out, err
	}
	holders, err := s.perms.HoldersOf(ctx, q, *spaceID, []string{permissions.SpaceTemplates})
	if err != nil {
		return nil, err
	}
	people := []uuid.UUID{}
	for _, h := range holders {
		switch h.Type {
		case permissions.SubjectUser:
			people = append(people, h.ID)
		case permissions.SubjectGroup:
			members, err := s.identity.GroupMembers(ctx, h.ID)
			if err != nil && !apperr.IsNotFound(err) {
				return nil, err
			}
			for _, m := range members {
				people = append(people, m.ID)
			}
		}
	}
	out := []uuid.UUID{}
	for _, person := range people {
		if slices.Contains(out, person) {
			continue
		}
		d, err := s.perms.CheckSpace(ctx, q, person, *spaceID, permissions.SpaceTemplates)
		if err != nil {
			return nil, err
		}
		if d.Allowed {
			out = append(out, person)
		}
	}
	return out, nil
}

// FourEyes is the setting "every template change needs a second person" of a scope, with
// whether anyone could approve a change of the person asking (TPL-511, TPL-521).
type FourEyes struct {
	On bool `json:"on"`
	// OthersCanApprove says someone besides the person asking manages templates in the scope,
	// so a change of theirs can be approved; with four eyes on and nobody else, nothing they
	// write can be published (TPL-521).
	OthersCanApprove bool `json:"others_can_approve"`
}

// FourEyesOf reads the setting of a scope, for those who manage its templates.
func (s *Service) FourEyesOf(ctx context.Context, c identity.Caller, spaceID *uuid.UUID) (FourEyes, error) {
	if err := s.requireManager(ctx, s.pool, c, spaceID); err != nil {
		return FourEyes{}, err
	}
	on, err := fourEyesOn(ctx, s.pool, spaceID)
	if err != nil {
		return FourEyes{}, err
	}
	managers, err := s.managersOf(ctx, s.pool, spaceID)
	others := slices.ContainsFunc(managers, func(p uuid.UUID) bool { return p != c.UserID })
	return FourEyes{On: on, OthersCanApprove: others}, err
}

// SetFourEyes turns the setting of a scope on or off. Whoever runs the space sets it, and an
// administrator for all spaces: those who manage templates are the ones it binds, so they do
// not choose it (TPL-511). Turning it on withdraws nothing; it applies to the next change.
func (s *Service) SetFourEyes(ctx context.Context, c identity.Caller, spaceID *uuid.UUID, on bool) (FourEyes, error) {
	if err := s.holdForOlderRelease(ctx); err != nil {
		return FourEyes{}, err
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireScopeAdministrator(ctx, tx, c, spaceID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM template_four_eyes WHERE space_id IS NOT DISTINCT FROM $1", spaceID); err != nil {
			return err
		}
		if on {
			if _, err := tx.Exec(ctx, "INSERT INTO template_four_eyes (space_id, changed_by) VALUES ($1, $2)", spaceID, c.UserID); err != nil {
				return err
			}
		}
		objectID, scope := "", "all_spaces"
		if spaceID != nil {
			objectID, scope = spaceID.String(), "space"
		}
		if _, err := audit.Record(ctx, tx, c.Event("template.setting_changed", "space", objectID, objectID,
			map[string]any{"setting": "four_eyes", "scope": scope, "on": on})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "template.setting_changed", map[string]any{"space_id": spaceID, "on": on})
		return err
	})
	if err != nil {
		return FourEyes{}, err
	}
	return s.FourEyesOf(ctx, c, spaceID)
}

// requireScopeAdministrator refuses whoever does not run a space, or the instance for all
// spaces.
func (s *Service) requireScopeAdministrator(ctx context.Context, q db.Querier, c identity.Caller, spaceID *uuid.UUID) error {
	if spaceID != nil {
		return s.perms.RequireSpace(ctx, q, c.UserID, *spaceID, permissions.SpaceAdminister)
	}
	return s.requireManager(ctx, q, c, nil)
}
