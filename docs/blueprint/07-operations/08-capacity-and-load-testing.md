# 07.8: Capacity planning and load testing

Blueprint 01 aims at thousands of people per instance, with pages under 300 ms (p95) at 5,000 simulated people. ADR-036 asks for room for four times the database work. `docs/operations/limits.md` holds what was measured on a development machine. This chapter sets out how to measure capacity so that the numbers can be trusted, and what a test on one virtual machine can and cannot prove.

## 8.1 What the research says about measuring

- **Open or closed: the model decides the answer.** Schroeder, Wierman and Harchol-Balter (NSDI 2006) showed that a load tool where each simulated user waits for an answer before sending the next request (a *closed* model) behaves very differently from real traffic where new people arrive whatever the server does (an *open* model). In a closed test, a slow server slows the test down and hides its own slowness. People on a wiki arrive like an open model. In k6 the open model is the `constant-arrival-rate` and `ramping-arrival-rate` executors; `constant-vus` is closed.
- **Coordinated omission.** A load tool that sends the next request only after the previous answer misses the requests that would have been sent during a stall, so its percentiles look far better than what people felt. Gil Tene named this and showed how large the error can be. The arrival-rate executors avoid it, provided the tool has enough free virtual users; k6 reports when it had to drop iterations, and that number must be read.
- **Little's Law.** The number of requests in the system equals the arrival rate times the time each spends there. It turns a measured latency into the number of connections, workers and memory needed, and shows why a queue in front of a slow part grows without limit once arrivals exceed what it can serve.
- **Scaling is not linear.** Gunther's Universal Scalability Law describes how contention (waiting for a shared lock) and coherence (keeping copies in step) make each extra node or CPU add less, and past a point less than nothing. Diffluence's audit lock is a contention point of exactly this kind (blueprint 04, ARC-201 to ARC-203).
- **The tail decides.** Dean and Barroso (CACM 2013): with several requests per page, the slowest decides what a person waits for. Targets are p95 and p99.
- **Method.** Jain's *The Art of Computer Systems Performance Analysis* remains the reference for planning experiments: one change at a time, repeated runs, and stated confidence instead of single numbers.

## 8.2 The kinds of test

| Test | Question | How |
|---|---|---|
| **Load** | Does the model of 5,000 people meet the targets? | Open model at the planned rate, 30 minutes or more |
| **Stress** | Where does it give way, and how? | Raise the arrival rate step by step until errors or p95 cross the target; note what gave way first |
| **Spike** | What happens when everyone arrives at 9:00? | From quiet to several times the rate in seconds |
| **Soak** | Does anything grow over time? | The model for 24 hours or more; watch memory, connections, disk, tables and queues |
| **Failure under load** | What do people see when a part dies? | Kill or pause the app, the database or the queue during a load test |
| **Recovery** | How long until normal after an overload? | Stop the overload and time the return to target |

## 8.3 The numbers to aim at

From blueprint 04.8 and `limits.md`: 5,000 people, 10% active at once, about 40% of the time in search. The model is about **130 page reads, 2 saves, 40 searches while typing and 10 results pages a second**, roughly 180 requests a second. ADR-036 asks for **four times the database work** without failing: about 720 requests a second of the same mix, with reading and saving still under target and search allowed to answer "busy" with `Retry-After` rather than slow everything down. That is what the load-protection of spec 031 was built for.

## 8.4 What one virtual machine can prove, and what it cannot

**It can prove:**

- the ceilings of one node and one database on real server hardware, for the model and for four times the model;
- what gives way first, and that it fails the designed way (503 with `Retry-After`, never a crash or lost data);
- memory, connection and disk behaviour over a long soak;
- restore times on a real disk for 10 GB and 100 GB (chapter 2);
- what happens after a hard reset of the machine, the honest form of a power cut (chapter 3);
- the cost in CPU of PostgreSQL for each person, which turns into the sizing table of chapter 7.

**It cannot prove:**

- fail-over and network partitions between machines, unless several virtual machines with their own network are used (chapter 3);
- that the HA topology scales across hosts, since all virtual machines share one host's CPU, memory bus and disks;
- real browsers at scale: k6 sends requests, it does not draw pages;
- results for other hardware: an older server CPU has fewer instructions per cycle than a new one, and a virtual disk on thin provisioning has different `fsync` times than a dedicated SSD.

**What to record with every result:** the CPU model and count given to each VM, memory, disk kind and the `fsync` rate measured with `pg_test_fsync`, the host's other load, CPU steal time from inside the VM, versions of everything, the data set, and the k6 script. A number without these is not a result.

**Separate the load tool.** The tool that makes the load must run on its own VM with its own CPUs, so it never competes with what it measures; k6 reports its own CPU, and a busy load tool gives false numbers.

## 8.5 Where Diffluence stands

| Practice | State | Where |
|---|---|---|
| A k6 script with the frontend's real pattern, 50,000 pages | Built | `scripts/load/reading.js`, ARC-199 |
| Measured ceilings and failure modes on a development machine | Built | `docs/operations/limits.md` |
| Load protection: an order of work, a limit on what waits, an honest `/readyz` | Built | spec 031 |
| A load test in CI | Not built | OPS-015 |
| Tests on server hardware, soak, spike, 4x the model | Not run | |

## 8.6 Recommendations

- **OPS-109** · new · **High.** Run the test series of 8.2 on two virtual machines (one for Diffluence and PostgreSQL, one for k6) with the open model, at the model, at two and at four times the model, and past that until it gives way. Record everything listed in 8.4, and put the results in `limits.md` next to the development numbers, marked with the machine.
- **OPS-110** · new · **High.** A soak of at least 24 hours at the model, with a sample every minute of memory, goroutines, database connections, table and index sizes, queue length and disk. Anything that only grows is a finding.
- **OPS-111** · new · **Medium.** k6 scripts in the repository for each test of 8.2, using arrival-rate executors only, with thresholds that fail the run, and a check that the tool dropped no iterations. The read mix, the search mix and the writers are separate scenarios so their share can be changed.
- **OPS-112** · new · **Medium.** A small load test in CI (OPS-015): the model at a tenth of its size for two minutes against the CI database, failing when p95 of reading grows more than a set share against the last main. It finds a slow query early; it does not measure capacity.
- **OPS-113** · new · **Medium.** Measure the CPU of PostgreSQL per active person at the model, and turn it into the sizing table of chapter 7 (OPS-108).
- **OPS-114** · new · **Low.** When the HA topology runs on several VMs (chapter 3), repeat the load test with a rolling upgrade in the middle, and record that no request failed (OPS-033).

## Sources

- Schroeder, Wierman, Harchol-Balter: [Open Versus Closed: A Cautionary Tale](https://www.usenix.org/conference/nsdi-06/open-versus-closed-cautionary-tale), NSDI 2006.
- Tene: [How NOT to Measure Latency](https://www.infoq.com/presentations/latency-response-time/), talk, QCon, 2015.
- Little: [A Proof for the Queuing Formula: L = λW](https://doi.org/10.1287/opre.9.3.383), Operations Research 9(3), 1961.
- Gunther: [Guerrilla Capacity Planning](https://doi.org/10.1007/978-3-540-31010-5), Springer 2007 (the Universal Scalability Law).
- Dean, Barroso: [The Tail at Scale](https://doi.org/10.1145/2408776.2408794), CACM 56(2), 2013.
- Jain: The Art of Computer Systems Performance Analysis, Wiley 1991.
- Grafana k6: [Scenarios and executors](https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/), [Open and closed models](https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/open-vs-closed/), [Dropped iterations](https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/dropped-iterations/).
- PostgreSQL 16: [pg_test_fsync](https://www.postgresql.org/docs/16/pgtestfsync.html), [pgbench](https://www.postgresql.org/docs/16/pgbench.html).

All sources of this study are also in [sources.md](sources.md).
