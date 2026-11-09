# Handoff 2026-11-09: a faster mutation check

Branch `faster-mutations`. See spec 037 and `docs/reports/2026-11-09-faster-mutations.md`.

## Done

- The runner queues every mutation and runs `MUTATION_JOBS` at once (default 4), each in its own copy, and prints the same lines in the same order with the same exit code.
- The twelve mutations that ran all of `internal/app` now run every test that caught them in a whole-package run.
- `MUTATION_CHANGED` runs only what a change can affect, for builders. Main and CI run everything.
- A full run went from 99.7 to 34.9 minutes, with the same 361 mutations caught by the same tests.

## When adding a mutation

Name the tests with `-run` as before. If you let one run a whole package and later want it targeted, record every test that fails in a whole-package run and name all of them, not only the first.

## Next

Narrower `go vet` per mutation, only with proof that it still refuses every mutation that does not compile.
