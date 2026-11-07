# Handoff 2026-11-07: breaking the architecture

Branch `pentest`. Spec 025 holds the plan. `docs/operations/limits.md` holds every number. This is a self-test, not an independent penetration test.

## Found and fixed

Worst first. Each has a test, and a mutation in `scripts/mutation-check.sh`.

| What | Fix |
|---|---|
| The Markdown parser works in the square of one block, so a paste of a megabyte kept a core busy for minutes, for anyone logged in. It went on after the person left. Very deep nesting ran out of stack, which stops the whole process and cannot be caught. That was seen in a test with the guard removed, not repeated over HTTP | A block with too many marks stays plain text with a warning, and never reaches the parser. A paste is 256 KiB at most, two at a time |
| The helper for PDF files could reach the database and the internet in `compose.yaml`, although ARC-074 says it cannot | An internal network, for the service for editing together too. The image is pinned, as ARC-077 asks |
| Anyone could lock a person out with 10 wrong passwords, from any address, without an account | The limit of 10 counts per account and address. A wide limit of 100 per account stops guesses from many addresses |
| One event too large for the queue stopped all events for good, including what takes a page off a public site. No route makes one today, and the relay must survive a fault | It is set aside under "Events that could not be handled", and the rest go on |
| One person editing could send messages of 100 MiB to the editing service, and a shared state past what the API can store stayed in memory | 16 MiB per message and per state. Past that, everyone goes back to the last good state |

`TestOneChangedByteStopsABundle` could pass for the wrong reason, when the changed byte broke the JSON. It now checks the reason, so its mutation is caught every time.

## First real runs

- The real Gotenberg 8.40 turned Word, ODT and RTF files into previews, so ARC-090 and ARC-094 are proven.
- The real editing service was attacked over a real WebSocket.
- The run with 50,000 pages is done (ARC-199, ARC-200), and the wait on the audit lock is measured (ARC-202).

## For the product owner

- **Upgrade:** `compose.yaml` has a new network. Anyone who put their own helper next to it must add it there.
- **A paste above 256 KiB** stays as plain text, which is what the editor already shows when conversion fails.
- **Search is the first ceiling.** Common words cost about 300 ms at 50,000 pages. ARC-187 has the cheap answer, ranking only the newest matches, and it changes what people find, so that is a choice.
- **The tree is the second.** 19 MB per load at 50,000 pages. SPC-031 is open.

## Not proven

See "Not run" in `docs/operations/limits.md`. The helper network was tried with a stand-in network: the database and the internet were out of reach. The compose file itself was checked with `docker compose config`, not started.
