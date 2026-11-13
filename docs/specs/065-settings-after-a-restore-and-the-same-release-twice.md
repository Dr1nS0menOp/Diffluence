# Spec 065: the database settings after a restore, and the same release twice

Two loose ends from waves HF (spec 063) and O5 (spec 057).

## 1. The defaults of the database after a restore

Migration 0050 stores `plan_cache_mode = force_custom_plan` and `jit = off` as defaults of the database with `ALTER DATABASE … SET`, so every session plans well however it connects, PgBouncer included (specs 061 and 063). A dump made with `pg_dump -Fc` and without `--create`, as `scripts/backup.sh` makes it, holds the contents of the database, not the defaults of the database itself. Restored into the same database, nothing is lost: the defaults belong to the database object, which `pg_restore --clean` keeps. Restored into a fresh database, a new installation, a new volume or the restore drill, both defaults are gone while `schema_migrations` says 0050 ran, so the migration never runs again.

**What is built.**

- `db.ApplySessionSettings` runs the statements of 0050 once more, from the constant `db.SessionSettingsSQL`. A test keeps the constant equal to the body of 0050, so the two cannot drift apart.
- Every run of migrations calls it after migrating, even when nothing was pending:
  - a node starting with `DIFFLUENCE_MIGRATE_ON_START` (the default), in `prepareSchema`;
  - `App.migrate`;
  - `diffluence migrate`, which the restore from the console (spec 059) uses for its step "bringing the database up to this release".
- So every restore path ends in a step that sets the defaults again: `scripts/restore.sh` and `restore-check.sh` start a node, and the console restore migrates.
- It is idempotent and costs two catalogue writes per start. A role that does not own the database gets the same notice as from 0050, and the pool still sends both settings at connect.

**Proof.**

- `TestTheSessionSettingsComeBackOnADatabaseThatLostThem`;
- `TestTheSettingsAppliedAgainAreThoseOfTheMigration`;
- `TestANodeStartingOnARestoredDatabaseSetsTheSessionSettingsAgain`, which failed before the change;
- `TestMigrateSetsTheSessionSettingsAgainWithNothingToMigrate`;
- three mutations;
- the restore drill, which after restoring onto a clean installation asks `SHOW plan_cache_mode` and `SHOW jit` in a plain `psql` session, one that sends nothing at connect.

**Decided on the product owner's behalf on 2026-11-13, may be overruled.**

- **D1:** apply the settings at every migration run instead of adding a step to each restore script or a separate `diffluence db settings` command. The restore paths are four and will grow, while every one of them already ends with migrations. One place cannot be forgotten by the next path.

## 2. Two builds of one release differed on CI

The `release-dry-run` job built a snapshot twice and compared the checksums. On CI every archive and package differed, while the archives listed the same files with the same sizes and dates. On a development machine the two builds matched.

**The cause.**

- The step "keep the first run" moved `dist` to `dist-first` inside the checkout. `dist/` is ignored by git; `dist-first/` is not.
- Go stamps a binary with the state of its git checkout (`vcs.modified`), and any untracked file makes it `true`. So the first build said `vcs.modified=false` and the second `true`, and the bytes differed.
- On a development machine the dev container sets `GOFLAGS=-buildvcs=false`, and `scripts/reproducible.sh` does the same, which is why neither showed it.
- Reproduced locally in a plain `golang:1.26` container, from a clean clone:
  - two builds of a clean tree have the same checksum;
  - a third build with an untracked `dist-first/` has another, and `go version -m` shows `vcs.modified=true` against `false`.

**What is built.**

- The first run is kept in `$RUNNER_TEMP`, outside the checkout.
- A new step fails, naming each file, when the checkout is not clean before the second build: the same rule a real release from a tag builds under.
- The comparison decides the job again, so `continue-on-error` is gone.
- On a difference it still names each file, and for an archive it shows how `go version -m` of the two binaries differs.

**Decided on the product owner's behalf on 2026-11-13, may be overruled.**

- **D2:** keep the VCS stamping in releases rather than build them with `-buildvcs=false`. It records which commit a binary came from and whether it was built from a clean tree, which a release should state. A release builds from a clean checkout of its tag, where the stamp is the same every time.

**Not proven here:** the CI job itself, which only runs on main after a push.
