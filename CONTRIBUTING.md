# Contributing to Diffluence

Thank you for helping. Two rules are non-negotiable: the **DCO** and the **clean-room rules**.

## 1. Developer Certificate of Origin (DCO)

Every commit must be signed off:

```sh
git commit -s -m "pages: add move endpoint"
```

That adds `Signed-off-by: Your Name <you@example.com>`. With it you certify the [Developer Certificate of Origin 1.1](https://developercertificate.org/), which in short says you wrote the change or have the right to submit it under the project's licence (Apache 2.0). CI rejects pull requests with unsigned commits.

## 2. Clean-room rules

Diffluence offers behaviour similar to other wikis, but **all code, text and design must be our own**. See `docs/blueprint/02-clean-room-and-licensing.md`.

**Never:**
- read, copy or translate source code of Confluence, Docmost, Plane, OpenProject, Huly or any other copyleft or proprietary wiki or issue tracker
- use an Atlassian product, account or export from your employer to develop or test Diffluence
- copy documentation text, UI copy, icons, illustrations or pixel-exact layouts
- add a dependency with a copyleft licence (GPL, AGPL, LGPL) or an unknown licence

**Do:**
- base features on a spec in `docs/specs/` that is written in our own words and lists its public sources
- tell the maintainers if you have previously worked on one of the projects above, so we can decide which areas you can safely work on

## 3. Workflow

1. One issue → one branch → one pull request. Keep it small.
2. Write or update tests first. Every change must keep `make check` green.
3. Every new HTTP route declares its access rule. Every state change writes an audit event and an outbox event **in the same transaction**.
4. Migrations are append-only: never edit a migration that has been merged.
5. Fill in the pull request template, including the plain-language "How to test" checklist.

## 4. Dependencies

New dependencies need a reason in the pull request, a permissive licence (MIT, BSD, Apache 2.0, ISC, MPL-2.0 as a separate unmodified module) and an active maintainer. Fewer is better.
