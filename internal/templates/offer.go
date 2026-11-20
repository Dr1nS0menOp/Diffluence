// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/identity"
)

// formLifetime is how long a form opened on a version keeps working after a newer version was
// approved: after that the person starts again, as the check step may no longer be what would
// happen (TPL-603).
const formLifetime = 24 * time.Hour

var (
	// errWithdrawn answers a form whose template was withdrawn while it was open (TPL-524).
	errWithdrawn = &apperr.Error{Kind: apperr.KindConflict, Code: "template_withdrawn",
		Message: "This template was withdrawn, so nothing was made. Pages made from it before stay as they are."}
	// errNotOffered answers a form bound to a version nobody may use: one that waits for a
	// decision, was sent back, or was replaced too long ago (TPL-510, TPL-523, TPL-603).
	errNotOffered = &apperr.Error{Kind: apperr.KindConflict, Code: "version_not_offered",
		Message: "This version of the template is not in use, so nothing was made. Open the template again to use the version in use now."}
)

// newestVersion is the newest version of a template, making the one a plain template saved by
// the previous release is owed first (TPL-1103). Only that case takes the lock of its row.
func (s *Service) newestVersion(ctx context.Context, c identity.Caller, t Template) (Version, error) {
	latest, found, err := latestVersion(ctx, s.pool, t.ID)
	if err != nil {
		return latest, err
	}
	if found {
		if t.Guided {
			return latest, nil
		}
		same, err := sameManifest(withRow(latest.Manifest, t, t.Version), latest.Manifest)
		if err != nil || same {
			return latest, err
		}
	}
	var v Version
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		locked, err := lockTemplate(ctx, tx, c.UserID, t.ID)
		if err != nil {
			return err
		}
		v, err = s.currentVersion(ctx, tx, c, locked)
		return err
	})
	return v, err
}

// offeredVersion is the version a form opened now by this person is bound to: the approved
// one, while a newer one waits (TPL-523). Without one, the owner tries their draft and those
// who look after the template see the newest; nobody else is offered anything (TPL-510).
func (s *Service) offeredVersion(ctx context.Context, c identity.Caller, st standing) (Version, error) {
	newest, err := s.newestVersion(ctx, c, st.t)
	if err != nil {
		return newest, err
	}
	switch {
	case newest.State == StateApproved:
		return newest, nil
	case st.t.TemplateVersion > 0:
		return versionAt(ctx, s.pool, st.t.ID, st.t.TemplateVersion)
	case st.mayLook():
		return newest, nil
	}
	return Version{}, apperr.NotFound()
}

// versionFor is the version a form is bound to, or the one offered to this person now.
func (s *Service) versionFor(ctx context.Context, c identity.Caller, st standing, number int) (Version, error) {
	if number > 0 {
		return versionAt(ctx, s.pool, st.t.ID, number)
	}
	return s.offeredVersion(ctx, c, st)
}

// usableFor decides whether pages may be made from a version, for real: the approved one, one
// replaced less than a day ago for a form opened on it, or a draft its owner tries. A version
// that waits, was sent back to someone else, or was withdrawn makes nothing, for anyone, its
// owner too: what an approver decides on is what the owner tried (TPL-510, TPL-523, TPL-524).
func usableFor(st standing, v Version, now time.Time) error {
	switch v.State {
	case StateApproved:
		return nil
	case StateSuperseded:
		if v.SupersededAt != nil && now.Sub(*v.SupersededAt) < formLifetime {
			return nil
		}
	case StateDraft, StateSentBack:
		if st.owner {
			return nil
		}
	case StateWithdrawn:
		return errWithdrawn
	}
	return errNotOffered
}

// tried says pages made from a version are a test of its owner, not yet for anyone else: they
// are drafts of the owner, whatever the template says about publishing (TPL-150, TPL-510).
func tried(v Version) bool {
	return v.State == StateDraft || v.State == StateSentBack
}

// lockVersionToUse reads a version again inside the transaction of a set, holding it, so that
// a withdrawal or an approval commits before or after the set, never during it (TPL-503,
// TPL-524).
func lockVersionToUse(ctx context.Context, q db.Querier, st standing, number int) error {
	v, err := scanVersion(q.QueryRow(ctx, "SELECT "+versionColumns+` FROM template_versions
		WHERE template_id = $1 AND number = $2 FOR SHARE`, st.t.ID, number))
	if err != nil {
		return err
	}
	var now time.Time
	if err := q.QueryRow(ctx, "SELECT now()").Scan(&now); err != nil {
		return err
	}
	return usableFor(st, v, now)
}
