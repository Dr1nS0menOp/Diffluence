# Spec 037: a faster mutation check that is not weaker

Status: built on branch `faster-mutations`.

## Why

`scripts/mutation-check.sh` breaks a security rule on purpose, in a throw-away copy of the code, and fails when the tests stay green. It is the proof that the tests guard the rules. A full run took over an hour, every mutation ran after the previous one, and builders queued for it. The product owner approved making it faster on one condition: the security it proves must not change, and no bug may slip through. Slow and right beats fast and wrong.

## What was measured before

See `docs/reports/2026-11-09-faster-mutations.md` for the times, measured on the same commit with the same machine. The old runner took 99.7 minutes for 361 mutations; the new one takes 34.9. In short: one Go mutation copies the tree, runs `go vet ./internal/...` and then its tests. Most mutations run one named test. Twelve ran the whole `internal/app` package, which takes minutes each, and all ran one after the other.

## What changes

1. **Several at once.** Each mutation still runs in its own throw-away copy. `MUTATION_JOBS` sets how many run at the same time (default 4). Integration tests already get their own database each (`internal/testutil`), and the roles they make have random names, so runs beside each other cannot meet. Results print in the order of the file, with the same lines as before, and the exit code is the same: 1 when any mutation is missed or broken, 0 otherwise.
2. **A longer test limit.** Go stops a package after ten minutes. Under parallel load the whole `internal/app` package could pass that and count as broken, so a mutation run uses 30 minutes. A real hang still ends.
3. **Targeted tests.** The twelve mutations that ran the whole `internal/app` package get, with `-run`, every test that failed when the whole package ran with that mutation, not only the first. So exactly the same tests still catch them. The full new run proved each one is still caught. Mutations that run a small package whole, which takes seconds, stay as they are. No mutation is removed, and the list keeps its length.
4. **A mode for builders.** `MUTATION_CHANGED` names a file with changed paths. Then a mutation runs when a changed path is in the folder of the mutated file or of its tests, and every mutation runs when this script or a shared test helper (`internal/testutil/`, `web/src/test/`) changed. This mode is only for a builder's own branch.

## The rule that does not change

**Main and CI always run every mutation.** The builder mode never replaces the full run when a branch lands on main. `CLAUDE.md` says so.

## Proof

- The old and the new runner ran in full on the same commit. The set of caught mutations is the same, line by line. The logs are kept in `docs/reports/`.
- A temporary mutation that no test catches was added, and the new runner reported it as `MISSED` and exited with 1. Then it was removed.
- `shellcheck -S warning` is clean on the runner.

## Load budget

Not a feature people use. It runs on a developer machine and in CI, and uses up to `MUTATION_JOBS` copies of the tree and their test databases at once.
