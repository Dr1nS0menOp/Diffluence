# Report: a faster mutation check (spec 037)

Written for later build sessions. Measured on 8 and 9 November 2026, on one development machine (16 threads, about 12 GB free in WSL), with other waves running their own work beside it. Both full runs used the same code: main at `7ad53c9`, with only the runner changed.

## Times

| Run | Mutations | Caught | Time |
|---|---|---|---|
| Old runner, one at a time | 361 | 361 | 5,981 s (99.7 min) |
| New runner, `MUTATION_JOBS=4` | 361 | 361 | 2,092 s (34.9 min) |
| New runner, builder mode, one changed file in `internal/document/` | 21 | 21 | 231 s |

The new full run is 2.9 times faster.

## What made it slow

- Every mutation ran after the previous one.
- Twelve mutations ran the whole `internal/app` package, minutes each.
- Each Go mutation also copies the tree and runs `go vet ./internal/...`. That stays, because a mutation that does not compile must never count as caught.

## Proof that it is not weaker

1. **Same result.** `2026-11-09-faster-mutations/old-full.log` and `new-full.log` list the same 361 mutations in the same order, each caught by the same first failing test. Only the durations in brackets differ.
2. **Targeted tests keep every catching test.** For each of the twelve, the whole package ran with the mutation in place, and every test that failed was recorded in `2026-11-09-faster-mutations/catching-tests.tsv`. The mutation now runs exactly those tests, not only the first. That matters: "any member can import a font" was first caught by a test of the API description, but a behaviour test also catches it, and both stay.
3. **A survivor still fails the run.** A temporary mutation that changes only a comment was added. The new runner printed `MISSED  PROBE: …`, then "1 mutation(s) were not caught", and exited with 1, also through `scripts/dev`. The mutation was removed afterwards.
4. **Parallel runs cannot meet.** Every integration test makes its own database (`internal/testutil`), and the roles tests make have random names. The migration lock is per database. Each mutation has its own copy of the tree.
5. **The clock cannot fake a result.** A mutation run allows 30 minutes per package instead of Go's 10, so a slow package fails on a test, not on the clock.
6. `shellcheck -S warning` is clean on old and new. The `info` notes are about the expressions in the list, which use single quotes on purpose.

## Builder mode

`MUTATION_CHANGED=.mutation-changed` with `git diff --name-only main > .mutation-changed`. A mutation runs when a changed path sits in the folder of the mutated file or of its test package. Every mutation runs when the runner, `internal/testutil/` or `web/src/test/` changed. It never replaces the full run on main or in CI. `.mutation-changed` is in `.gitignore`.

## Left

- `go vet ./internal/...` per mutation is now the largest fixed cost. Vetting only the changed package would be faster, but a change can break another package, so it stays until there is proof that a narrower vet catches every non-compiling mutation.
- The default of 4 at once was not tuned further. More may be faster on this machine when nothing else runs; CI runners are smaller.
- Other packages that run whole (`internal/permissions`, `internal/document`, and the rest) were left as they are, because they take seconds.
- Builder mode was tried on one changed file and on the probe, not on a long branch.
