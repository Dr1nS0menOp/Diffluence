// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/jobs"
)

// jobSet makes a large set in the background (TPL-121).
const jobSet = "template_set"

// setJob is what a background set carries: the answers only until it ends (TPL-1409).
type setJob struct {
	TemplateID uuid.UUID `json:"template_id"`
	Input      SetInput  `json:"input"`
}

// queueSet hands a set that passed every check to the job queue.
func (s *Service) queueSet(ctx context.Context, c identity.Caller, id uuid.UUID, in SetInput) (uuid.UUID, error) {
	var job uuid.UUID
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		job, err = s.jobs.Enqueue(ctx, tx, jobSet, setJob{TemplateID: id, Input: in}, c.UserID)
		return err
	})
	return job, err
}

// runSetJob makes a queued set as the person who asked for it, with every right checked
// again. At most one set per person and two per space run at once (TPL-827); a set over
// that limit goes back to the queue and waits.
func (s *Service) runSetJob(ctx context.Context, j jobs.Job) (any, error) {
	var in setJob
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	c := identity.Caller{UserID: j.CreatedBy}
	out, err := s.runQueuedSet(ctx, c, in)
	if errors.Is(err, jobs.ErrNotYet) {
		return nil, err
	}
	if err != nil {
		if auditErr := s.recordFailedSet(ctx, c, in, err); auditErr != nil {
			err = errors.Join(err, auditErr)
		}
	}
	// A worker that stopped halfway, whose context ended, leaves the job for a retry, which
	// needs the answers. Any other ending removes them.
	if ctx.Err() == nil {
		if forgetErr := s.jobs.ForgetPayload(context.WithoutCancel(ctx), j.ID); forgetErr != nil {
			err = errors.Join(err, forgetErr)
		}
	}
	return out, err
}

func (s *Service) runQueuedSet(ctx context.Context, c identity.Caller, in setJob) (SetResult, error) {
	p, err := s.prepare(ctx, c, in.TemplateID, in.Input)
	if err != nil {
		return SetResult{}, err
	}
	release, err := s.takeTurn(ctx, c.UserID, p.spaceID)
	if err != nil {
		return SetResult{}, err
	}
	defer release()
	return s.makeFor(ctx, c, p)
}

// setSlotsPerSpace is how many background sets of one space run at once (TPL-827).
const setSlotsPerSpace = 2

// takeTurn holds one slot of the person and one of the space for as long as the set runs,
// on a connection of its own, so a slot frees itself when that connection ends.
func (s *Service) takeTurn(ctx context.Context, person, space uuid.UUID) (func(), error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	release := func() {
		// Session locks end with the session; the connection goes back clean.
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock_all()")
		conn.Release()
	}
	var mine bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended('template-set-person:' || $1::text, 0))", person).Scan(&mine); err != nil {
		release()
		return nil, err
	}
	if !mine {
		release()
		return nil, jobs.ErrNotYet
	}
	for slot := range setSlotsPerSpace {
		var taken bool
		if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended('template-set-space:' || $1::text || ':' || $2::int, 0))",
			space, slot).Scan(&taken); err != nil {
			release()
			return nil, err
		}
		if taken {
			return release, nil
		}
	}
	release()
	return nil, jobs.ErrNotYet
}

// recordFailedSet writes template.set_failed after the rollback, so the audit log says a
// background set failed although nothing of it exists (TPL-820).
func (s *Service) recordFailedSet(ctx context.Context, c identity.Caller, in setJob, cause error) error {
	e := c.Event("template.set_failed", "template", in.TemplateID.String(), in.Input.SpaceID.String(), map[string]any{"reason": describe(cause)})
	e.Result = audit.ResultFailure
	return audit.RecordStandalone(context.WithoutCancel(ctx), s.pool, e)
}
