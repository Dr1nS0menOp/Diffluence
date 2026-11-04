# Specs

Every feature starts as a spec here, **written in our own words** (clean-room, see `docs/blueprint/02`).

File name: `NNN-short-name.md`. Template:

```markdown
# NNN: Title

Status: draft | agreed | built
Phase: (from docs/blueprint/08)

## Problem
What the user needs, in one paragraph.

## Behaviour
What it does, step by step, from the user's point of view. No implementation details.

## Permissions and audit
Who may do it (05). Which audit events and outbox events it produces.

## Acceptance criteria
- [ ] Testable statements. Claude turns each into an automated test.

## How the product owner tests it
Plain-language steps for the review session.

## Provenance
Public sources consulted (URL + date), or "own design".
```
