# Making a release

Written for: maintainers who cut a release.

Before 1.0 there are no releases with support: every build on main is upgraded to the next one with the same machinery, and CI proves that on every change. From 1.0 a release comes every month (`1.x.0`), and a security fix as a patch release (`1.x.y`). Only the latest release line is supported (blueprint 07).

## Checklist

1. **Main is green.** The last CI run on main passed, including `upgrade`, and so did the last nightly run: fuzzing, the mutation check and the restore drill.
2. **Migrations follow the rules.** Each new file in `internal/db/migrations/` starts with `-- phase: expand` or `-- phase: contract`, and no merged file was changed (the migration test refuses that). A contract migration only removes what the previous release no longer reads: it runs as soon as every node knows it, and from then on that previous release refuses to start.
3. **The upgrade from the previous release works.** Run it from that release's tag, not from the commit before:

   ```sh
   bash scripts/dev bash scripts/upgrade-check.sh v1.4.0
   ```

   It must end with `upgrade test passed`. For a jump that is allowed but not promised without downtime, say so in the notes.
4. **The notes are written.** Move the `Unreleased` part of `CHANGELOG.md` under the new version and date. Every entry is plain language and has an `Upgrade impact` line: `none`, a migration, or what an operator must do before updating. Anything an operator must do comes first, in bold.
5. **Tag and build.** Tag the commit `v1.x.y`. `make build` takes the version from the tag and the build date from the date of the commit; check them with `diffluence version` and `diffluence doctor`. For the image, pass both: `docker build --build-arg VERSION=v1.x.y --build-arg BUILD_DATE=$(git log -1 --format=%cs) .`. The console warns when a release is more than 60 days old, from this date, so a release without it never warns (spec 051). Let the scan in CI pass.
6. **Restore from the new version.** Run `bash scripts/restore-drill.sh` on the tagged commit, so the backup of this release is known to restore.
7. **Announce it.** Point to the notes, and to [upgrade.md](upgrade.md). From 1.0, say that the release line before it is no longer supported.

## When a release goes wrong

Roll forward with a patch release when you can. If an installation must go back, the previous release runs on the migrated database until a contract migration has run; after that, it refuses to start and names the migration, and the way back is the backup that the `system.upgrade_started` marker in the audit log names ([upgrade.md](upgrade.md)).
