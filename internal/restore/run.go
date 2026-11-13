// SPDX-License-Identifier: Apache-2.0

package restore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/audit"
)

// Steps are what the worker does to an installation. The real ones run gpg, pg_restore
// and tar; tests put in steps that fail where they choose.
type Steps interface {
	// Check proves a backup is whole and opens with the key, and changes nothing.
	Check(ctx context.Context, name string) error
	// NewestMigration is the newest migration the backup's database had.
	NewestMigration(ctx context.Context, name string) (int, error)
	// StopOthers ends every other session on the database, so a node that missed the
	// waiting page cannot write while the tables are made again.
	StopOthers(ctx context.Context) error
	RestoreDatabase(ctx context.Context, name string) error
	RestoreFiles(ctx context.Context, name string) error
	// Migrate brings the restored database up to this release.
	Migrate(ctx context.Context) error
	// Record writes the restore into the restored audit log, sealed, and notes the safety
	// backup for the checks before the next upgrade.
	Record(ctx context.Context, r Restore) error
	// Verify checks the audit chain and that every secret and file opens with the keys.
	Verify(ctx context.Context) error
}

// ErrNewerBackup means the backup came from a newer release than the worker's.
var ErrNewerBackup = errors.New("restore: the backup was made by a newer release")

// Runner moves a restore along, step by step, and writes each step down before it takes
// it, so the console and the waiting page say where it is and where it stopped.
type Runner struct {
	// Pool connects as the owner of the tables.
	Pool  *pgxpool.Pool
	Steps Steps
	// Gate checks that the backup service confirmed the restore it is asked to run.
	Gate Gate
	// NewestKnown is the newest migration of this release.
	NewestKnown int
	// StopWithin is how long the nodes get to stop serving; Poll is how often to look.
	StopWithin time.Duration
	Poll       time.Duration
	Log        *slog.Logger
}

func (r Runner) open(ctx context.Context, id uuid.UUID, want ...string) (Restore, error) {
	rs, err := Get(ctx, r.Pool, id)
	if err != nil {
		return Restore{}, err
	}
	if !rs.Open() {
		return Restore{}, fmt.Errorf("restore: %s is already finished (%s)", id, rs.State)
	}
	if !slices.Contains(want, rs.State) {
		return Restore{}, fmt.Errorf("restore: %s is %s, not %v", id, rs.State, want)
	}
	// What the application role wrote is not an order: only a restore this backup service
	// confirmed, of the backup it confirmed, goes on (spec 070).
	sealed, _, err := seals(ctx, r.Pool, id)
	if err != nil {
		return Restore{}, err
	}
	if !r.Gate.sealHolds("restore", id, rs.BackupName, sealed) {
		return Restore{}, r.reject(ctx, rs, "the backup service did not confirm this restore of this backup")
	}
	return rs, nil
}

// reject stops a restore the backup service never confirmed and records it. Nothing was
// changed by this worker, so a restore that had not changed anything ends refused; one marked
// changed keeps the nodes waiting for a person, as any failed restore does.
func (r Runner) reject(ctx context.Context, rs Restore, why string) error {
	write, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	err := r.fail(write, rs.ID, why+"; nothing was changed", ErrNotConfirmed)
	if recErr := audit.RecordStandalone(write, r.Pool, refusal(rs, "not_confirmed")); recErr != nil {
		return errors.Join(err, recErr)
	}
	return err
}

// fail writes down the step that failed, and returns the error.
func (r Runner) fail(ctx context.Context, id uuid.UUID, step string, cause error) error {
	r.Log.Error("the restore stopped", "restore", id, "step", step, "error", cause)
	// The context of the run may be gone; the step must still be written down.
	write, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := Fail(write, r.Pool, id, step); err != nil {
		return errors.Join(cause, err)
	}
	return fmt.Errorf("%s: %w", step, cause)
}

// Prepare checks the backup while the nodes still serve, then stops them. When it returns
// nil, every node waits and the worker makes the safety backup. Nothing is changed here:
// on any failure the restore ends refused and the nodes serve again.
func (r Runner) Prepare(ctx context.Context, id uuid.UUID) error {
	rs, err := r.open(ctx, id, StatePreparing)
	if err != nil {
		return err
	}
	if err := r.Steps.Check(ctx, rs.BackupName); err != nil {
		return r.fail(ctx, id, "the backup is not whole or does not open with the key; nothing was changed", err)
	}
	newest, err := r.Steps.NewestMigration(ctx, rs.BackupName)
	if err != nil {
		return r.fail(ctx, id, "the backup's database could not be read; nothing was changed", err)
	}
	if newest > r.NewestKnown {
		return r.fail(ctx, id, "the backup was made by a newer release of Diffluence; restore it with that release; nothing was changed",
			fmt.Errorf("%w: migration %d, this release knows %d", ErrNewerBackup, newest, r.NewestKnown))
	}
	if err := Move(ctx, r.Pool, id, StateStopping, "stopping every node"); err != nil {
		return err
	}
	if err := WaitForNodes(ctx, r.Pool, r.StopWithin, r.Poll); err != nil {
		return r.fail(ctx, id, "a node kept serving; nothing was changed", err)
	}
	return Move(ctx, r.Pool, id, StateBackingUp, "making a safety backup of the installation as it is")
}

// Abort ends a restore whose safety backup failed. It changed nothing.
func (r Runner) Abort(ctx context.Context, id uuid.UUID, step string) error {
	if _, err := r.open(ctx, id, StateBackingUp, StateStopping); err != nil {
		return err
	}
	return r.fail(ctx, id, "the safety backup failed, so nothing was changed: "+step, errors.New("no safety backup"))
}

// Run restores the chosen backup after the safety backup is made. From the first step that
// alters the installation on, a failure keeps every node waiting, and the way back is the
// safety backup.
func (r Runner) Run(ctx context.Context, id uuid.UUID, safety string) error {
	rs, err := r.open(ctx, id, StateBackingUp)
	if err != nil {
		return err
	}
	if err := setSafety(ctx, r.Pool, r.Gate, id, safety); err != nil {
		return err
	}
	rs.SafetyName = safety
	return r.restore(ctx, rs)
}

// WayBack restores the safety backup of a restore that failed after it changed something.
func (r Runner) WayBack(ctx context.Context) (Restore, error) {
	failed, err := FailedOpen(ctx, r.Pool)
	if err != nil {
		return Restore{}, err
	}
	if failed == nil {
		return Restore{}, errors.New("restore: no failed restore waits for the way back")
	}
	// Only a safety backup this backup service made. A failure written by hand keeps the
	// nodes waiting: a person decides what happened (spec 070).
	_, safetySeal, err := seals(ctx, r.Pool, failed.ID)
	if err != nil {
		return *failed, err
	}
	if !r.Gate.sealHolds("safety", failed.ID, failed.SafetyName, safetySeal) {
		r.Log.Error("the way back was refused: the backup service did not make this safety backup", "restore", failed.ID)
		if err := audit.RecordStandalone(ctx, r.Pool, refusal(*failed, "safety_not_sealed")); err != nil {
			return *failed, errors.Join(ErrNotConfirmed, err)
		}
		return *failed, fmt.Errorf("%w: the safety backup %s was not made by this backup service; restore by hand with scripts/restore.sh",
			ErrNotConfirmed, failed.SafetyName)
	}
	if err := r.Steps.Check(ctx, failed.SafetyName); err != nil {
		return *failed, fmt.Errorf("restore: the safety backup %s is not whole: %w", failed.SafetyName, err)
	}
	if err := takeOver(ctx, r.Pool, r.Gate, failed.ID, failed.SafetyName); err != nil {
		return *failed, err
	}
	rs, err := Get(ctx, r.Pool, failed.ID)
	if err != nil {
		return rs, err
	}
	return rs, r.restore(ctx, rs)
}

func (r Runner) restore(ctx context.Context, rs Restore) error {
	id := rs.ID
	// Nodes that started meanwhile have seen the waiting state; look once more before the
	// first change.
	if err := WaitForNodes(ctx, r.Pool, r.StopWithin, r.Poll); err != nil && !rs.Changed {
		return r.fail(ctx, id, "a node kept serving; nothing was changed", err)
	}
	steps := []struct {
		state, step string
		do          func(context.Context) error
	}{
		{StateRestoring, "ending the sessions of other nodes", r.Steps.StopOthers},
		{StateRestoring, "restoring the database", func(ctx context.Context) error { return r.Steps.RestoreDatabase(ctx, rs.BackupName) }},
		{StateRestoring, "restoring the stored files and keys", func(ctx context.Context) error { return r.Steps.RestoreFiles(ctx, rs.BackupName) }},
		{StateChecking, "bringing the database up to this release", r.Steps.Migrate},
		{StateChecking, "writing the restore into the audit log", func(ctx context.Context) error { return r.Steps.Record(ctx, rs) }},
		{StateChecking, "checking the audit log and the keys", r.Steps.Verify},
	}
	for _, s := range steps {
		if err := Move(ctx, r.Pool, id, s.state, s.step); err != nil {
			return err
		}
		if err := s.do(ctx); err != nil {
			return r.fail(ctx, id, "failed while "+s.step, err)
		}
	}
	r.Log.Info("the restore is done", "restore", id, "backup", rs.BackupName, "safety", rs.SafetyName)
	return Finish(ctx, r.Pool, id)
}
