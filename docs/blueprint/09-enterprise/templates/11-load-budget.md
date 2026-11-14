# 09.T11: Load budget

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

Every feature states what it costs (ARC-238), and the costs are measured on wave PF's real-data dump, never only on generated data. Guided templates are not on the reading path: a page made from a set is an ordinary page. They cost in three places:
- the **gallery**, opened whenever someone creates a page;
- **making a set**, a burst of writes under the space's tree lock;
- the **audit sealer**, which receives many events at once.

The claim of ADR-043 stands: "one node, a few hundred people, measured on your own hardware". Nothing here claims more. The installation keeps room for four times the database work it measures (the headroom rule of the load budget), so each budget below is a ceiling for one use, and the expected rate is stated beside it.

- **TPL-900** · new · **The gallery** stays one query, as today. The audience lists (TPL-500) and the spaces offered (TPL-502) join as hash joins on small tables indexed by template, not a query per template. *Budget:* the gallery of a space with 100 templates, 10 of them with audience lists, within **30 ms** on the real-data dump, and at most one query more than today. *Proof:* a query-count test (ARC-239), and EXPLAIN on the dump showing hash joins.
- **TPL-901** · new · **Making a set** of *n* pages costs, inside one transaction:
  - *n* page rows, *n* drafts or versions, *n* origin rows;
  - the labels and restriction rows;
  - one file copy per file;
  - about *n* + 1 audit rows in `audit_pending`.

  Writes are batched per table, so the number of statements grows with the number of tables, not of pages. *Budget,* measured on the real-data dump, without files:
  - a set of 10 pages within **300 ms** at p95;
  - a set of 100 pages within **3 s** at p95, as background work;
  - a single page as today, within **10%** of today's time.

  *Proof:* a benchmark in GT1, run again in each wave.
- **TPL-902** · new · **The tree lock** of the space is held for the whole transaction of TPL-901, which blocks other creates and moves **in that space only**. *Budget:* at most 3 s for the largest set, and a hard stop at 60 s (TPL-826). The benchmark records how long other writers in the space wait. *Proof:* a test that a page created in another space during a 100-page set does not wait.
- **TPL-903** · new · **Audit:** a 100-page set puts about 101 events in `audit_pending` at once. The sealer seals pending events every second (ADR-041), so the set adds at most one second of sealing. Whether the sealer keeps up with a burst is measured, not assumed. *Budget:* after a 100-page set, the pending queue is empty again within **5 s**. *Proof:* a test on the benchmark.
- **TPL-904** · new · **The dry run** (TPL-341) does the work of making the set without writing. It reads the template's version and checks rights, then rolls back. *Budget:* within **150 ms** for 10 pages and **1 s** for 100 pages. It runs on Next to the check step, never on each keystroke. *Proof:* the benchmark.
- **TPL-905** · new · **Conditions** cost nothing measurable: at most 40 fields and 10 tests per condition, a pure function (TPL-430). *Budget:* evaluating every condition of a 100-entry template within **1 ms**. *Proof:* a Go benchmark.
- **TPL-906** · new · **Storage:** a manifest is at most **64 KiB** (TPL-600), and a template with a hundred versions costs a few MiB of manifests besides its texts. The text of each entry is versioned as a page already is. *Proof:* a size check in the validator.
- **TPL-907** · new · **The expected rate**, used to judge the budgets, is an assumption to check, not a measurement:
  - an instance of a few hundred people makes at most a few dozen sets a day;
  - most sets are under 10 pages;
  - large sets are rare: onboarding, project starts, planning rounds.

  At that rate guided templates add well under 1% to the database's write load. If the analytics of a real installation show otherwise, the budgets are revisited. *Proof:* the stress test of the server measures a burst of 20 sets of 10 pages within one minute beside the normal mix.
- **TPL-908** · new · **Background sets** are limited to two per space and one per person (TPL-827), so a burst cannot take every worker. *Proof:* see TPL-827.
