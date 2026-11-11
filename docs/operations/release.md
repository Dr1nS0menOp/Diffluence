# Making a release

Written for: maintainers who cut a release.

Before 1.0 there are no releases with support: every build on main is upgraded to the next one with the same machinery, and CI proves that on every change. From 1.0 a release comes every month (`1.x.0`), and a security fix as a patch release (`1.x.y`).

## Support policy

From 1.0 (OPS-098, also in `SECURITY.md`):

- **Only the newest release line is supported.** A fix is never back-ported to an older line: the upgrade is the fix. That is what keeps a project without staff honest about what it can look after.
- **A security fix ships as a patch of the newest line within days** of its confirmation.
- **An installation up to six months old upgrades to the newest release in one step** (spec 048 D3). Older ones go through the release of six months before.

## What a release is made of

A pushed tag `vX.Y.Z` on `main` runs `.github/workflows/release.yml` (spec 057), and nothing else does. It makes:
- the archives for Linux amd64 and arm64;
- the `.deb` and `.rpm` packages;
- `checksums.txt`, signed with Sigstore;
- SBOMs in SPDX and CycloneDX;
- SLSA provenance for every file;
- the image on `ghcr.io` for both architectures, signed and attested.

It leaves a **draft** release with the notes of that version from `CHANGELOG.md`. CI's `release-dry-run` builds the same files on every push to main, and checks that two builds are the same byte for byte.

## Checklist

1. **Main is green.** The last CI run on main passed, including `upgrade`, and so did the last nightly run: fuzzing, the mutation check and the restore drill.
2. **Migrations follow the rules.** Each new file in `internal/db/migrations/` starts with `-- phase: expand` or `-- phase: contract`, and no merged file was changed (the migration test refuses that). A contract migration only removes what the **previous** release already stopped reading and writing (spec 048 D2): it runs a day after every node knows it, and from then on that previous release refuses to start, so it must not need what is gone. The rewrite check (`TestNoMigrationRewritesATable`) passes, with every `-- rewrite-ok:` exception explained. Every format that crosses versions still reads what the previous release wrote ([version-skew.md](version-skew.md)).
3. **The upgrade from the previous release works.** Run it from that release's tag, not from the commit before:

   ```sh
   bash scripts/dev bash scripts/upgrade-check.sh v1.4.0
   ```

   It must end with `upgrade test passed`. For a jump that is allowed but not promised without downtime, say so in the notes.
4. **The notes are written.** Move the `Unreleased` part of `CHANGELOG.md` under the new version and date. Every entry is plain language and has an `Upgrade impact` line: `none`, a migration, or what an operator must do before updating. Anything an operator must do comes first, in bold.
5. **The example configuration still loads.** `TestTheExampleFileLoads` passes, and every setting added since the last release is in `deploy/diffluence.example.yaml` or `.env.example` with a word on when to change it (spec 056).
6. **The release still builds.** The last `release-dry-run` job on main passed: the configuration is valid, two builds are the same byte for byte, and the `.deb` and `.rpm` install.
7. **Tag.** Tag the commit on `main` as `v1.x.y` and push the tag. The release workflow builds everything. It takes the version from the tag and the build date from the date of the commit, which the console's warning about an old release reads (spec 051).
8. **Check the draft.** In the release workflow's run, every step passed. Download `checksums.txt`, its `.sigstore.json` bundle and one package, and verify them as `install.md` says ("Checking what you downloaded"). Run `cosign verify` on the image. Install the `.deb` in a clean Debian machine or container and run `diffluence version`. Then read the notes and publish the draft.
9. **Restore from the new version.** Run `bash scripts/restore-drill.sh` on the tagged commit, so the backup of this release is known to restore.
10. **Announce it.** Point to the notes, and to [upgrade.md](upgrade.md). From 1.0, say that the release line before it is no longer supported.

## Code that ships dark

No feature ships switched off today, so there are no toggles (OPS-097). The first one follows this rule:
- **A named setting** `DIFFLUENCE_FEATURE_<NAME>`, read once at start through the configuration of spec 056, and off by default.
- **An owner and a removal issue**, named in its changelog entry.
- **Removed within two releases**, either by turning the feature on for good or by deleting it.
- **No toggle service and no dependency.**

## When a release goes wrong

Roll forward with a patch release when you can. If an installation must go back, the previous release runs on the migrated database until a contract migration has run; after that, it refuses to start and names the migration, and the way back is the backup that the `system.upgrade_started` marker in the audit log names ([upgrade.md](upgrade.md)).
