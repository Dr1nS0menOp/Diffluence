// SPDX-License-Identifier: Apache-2.0

package db

import "context"

// SessionSettingsSQL sets the defaults of the database that migration 0050 sets: every
// session plans with its values and does not compile plans (specs 061 and 063). A test keeps
// it equal to the migration. A role that does not own the database only gets a notice.
const SessionSettingsSQL = `DO $$
BEGIN
  EXECUTE format('ALTER DATABASE %I SET plan_cache_mode = force_custom_plan', current_database());
  EXECUTE format('ALTER DATABASE %I SET jit = off', current_database());
EXCEPTION WHEN insufficient_privilege THEN
  RAISE NOTICE 'diffluence: not the owner of the database, so plan_cache_mode and jit are only sent at connect';
END
$$;`

// ApplySessionSettings sets the defaults of the database again. A dump made without
// --create does not carry them, so a database restored into a fresh one has lost them while
// its migrations table says 0050 ran; every migration run applies them again, which costs
// two catalogue writes and changes nothing when they are already there (spec 065).
func ApplySessionSettings(ctx context.Context, q Querier) error {
	_, err := q.Exec(ctx, SessionSettingsSQL)
	return err
}
