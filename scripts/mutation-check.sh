#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Do the tests catch real bugs? This script breaks a security rule on purpose,
# in a throw-away copy of the code, and fails when the tests stay green.
# Run it in the development toolbox: scripts/dev bash scripts/mutation-check.sh
#
# MUTATION_JOBS sets how many mutations run at once, each in its own copy (default 4).
# MUTATION_CHANGED names a file that lists changed paths, one per line; then only the
# mutations those paths can affect run. That mode is for builders: main and CI always
# run every mutation (spec 037).
set -uo pipefail

src=$(pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
survived=0
jobs_max=${MUTATION_JOBS:-4}
changed_list=${MUTATION_CHANGED:-}
# A list that cannot be read would select nothing, and nothing caught reads like a pass.
if [ -n "$changed_list" ] && [ ! -r "$changed_list" ]; then
  echo "MUTATION_CHANGED names $changed_list, which cannot be read" >&2
  exit 2
fi
count=0
ran=0
declare -a kinds names files exprs argv

# apply <file> <expression on s>: changes a file, and fails when nothing changed.
apply() {
  node -e '
    const fs = require("fs");
    const [file, expr] = process.argv.slice(1);
    const s = fs.readFileSync(file, "utf8");
    const out = eval(expr);
    if (out === s) { console.error("mutation did not change the file"); process.exit(3); }
    fs.writeFileSync(file, out);
  ' "$1" "$2"
}

# mutate_web <name> <file below web> <expression on s> <test file>: queues a frontend mutation.
mutate_web() {
  queue web "$1" "$2" "$3" "$4"
}

# mutate_collab <name> <file below collab> <expression on s> <test file>: queues a mutation of
# the service for editing together.
mutate_collab() {
  queue collab "$1" "$2" "$3" "$4"
}

# mutate <name> <file> <expression on s> <go test args...>: queues a Go mutation.
mutate() {
  local name=$1 file=$2 expr=$3
  shift 3
  queue go "$name" "$file" "$expr" "$@"
}

# queue <kind> <name> <file> <expression> <args...>: keeps a mutation for the run at the end.
queue() {
  kinds[count]=$1
  names[count]=$2
  files[count]=$3
  exprs[count]=$4
  shift 4
  argv[count]=$(printf '%q ' "$@")
  count=$((count + 1))
}

# needed <index>: whether a mutation must run. Every mutation runs unless MUTATION_CHANGED is
# set. With a list, a mutation runs when a changed path sits in the folder of the mutated file
# or of its tests, and every mutation runs when this script or a shared test helper changed.
needed() {
  local i=$1 path dir
  [ -z "$changed_list" ] && return 0
  dir=$(dirname "${files[i]}")
  [ "${kinds[i]}" = web ] && dir="web/$dir"
  [ "${kinds[i]}" = collab ] && dir="collab/$dir"
  local -a args
  eval "args=(${argv[i]})"
  while IFS= read -r path; do
    [ -z "$path" ] && continue
    case "$path" in
      scripts/mutation-check.sh | internal/testutil/* | web/src/test/*) return 0 ;;
    esac
    [ "$(dirname "$path")" = "$dir" ] && return 0
    local a
    for a in "${args[@]}"; do
      case "$a" in
        ./*/) [ "$(dirname "$path")/" = "${a#./}" ] && return 0 ;;
        *.test.ts | *.test.tsx) [ "$path" = "web/$a" ] && return 0 ;;
        *.test.mjs) [ "$path" = "collab/$a" ] && return 0 ;;
      esac
    done
  done <"$changed_list"
  return 1
}

# take_snapshot: copies the code once, before any mutation starts, so every mutation tests the
# same tree even when the working tree changes during a run. The installed packages of the web
# app are linked, not copied: they are large and do not change within a run.
snapshot="$work/snapshot"
take_snapshot() {
  mkdir -p "$snapshot/repo" "$snapshot/web"
  tar -C "$src" --exclude=./web --exclude=./collab --exclude=./bin --exclude=./.git -cf - . | tar -C "$snapshot/repo" -xf -
  tar -C "$src/web" --exclude=./node_modules --exclude=./e2e --exclude=./test-results -cf - . | tar -C "$snapshot/web" -xf -
  mkdir -p "$snapshot/collab"
  tar -C "$src/collab" --exclude=./node_modules -cf - . | tar -C "$snapshot/collab" -xf -
}

# run_collab <index> <folder>: one mutation of the collab service in its own copy.
run_collab() {
  local i=$1 dir=$2
  mkdir -p "$dir/collab"
  tar -C "$snapshot/collab" -cf - . | tar -C "$dir/collab" -xf -
  ln -s "$src/collab/node_modules" "$dir/collab/node_modules"
  if ! apply "$dir/collab/${files[i]}" "${exprs[i]}" 2>/dev/null; then
    echo "BROKEN  ${names[i]} (the mutation no longer applies; update this script)"
    return
  fi
  local -a args
  eval "args=(${argv[i]})"
  if (cd "$dir/collab" && timeout 300 node --test "${args[@]}" >"$dir/out.txt" 2>&1); then
    echo "MISSED  ${names[i]}"
  elif grep -a -q -E '^# fail [1-9]' "$dir/out.txt"; then
    echo "caught  ${names[i]}"
  else
    echo "BROKEN  ${names[i]} (no test failed: $(grep -a -m1 . "$dir/out.txt"))"
  fi
}

# run_web <index> <folder>: one frontend mutation in its own copy; prints one result line.
run_web() {
  local i=$1 dir=$2
  mkdir -p "$dir/web"
  tar -C "$snapshot/web" -cf - . | tar -C "$dir/web" -xf -
  ln -s "$src/web/node_modules" "$dir/web/node_modules"
  if ! apply "$dir/web/${files[i]}" "${exprs[i]}" 2>/dev/null; then
    echo "BROKEN  ${names[i]} (the mutation no longer applies; update this script)"
    return
  fi
  local -a args
  eval "args=(${argv[i]})"
  if (cd "$dir/web" && npx vitest run "${args[@]}" >"$dir/out.txt" 2>&1); then
    echo "MISSED  ${names[i]}"
  elif grep -a -q -E 'Tests .*failed' "$dir/out.txt"; then
    echo "caught  ${names[i]}"
  else
    echo "BROKEN  ${names[i]} (no test failed: $(grep -a -m1 . "$dir/out.txt"))"
  fi
}

# run_go <index> <folder>: one Go mutation in its own copy; prints one result line.
run_go() {
  local i=$1 dir=$2
  mkdir -p "$dir/repo"
  tar -C "$snapshot/repo" -cf - . | tar -C "$dir/repo" -xf -
  if ! apply "$dir/repo/${files[i]}" "${exprs[i]}" 2>/dev/null; then
    echo "BROKEN  ${names[i]} (the mutation no longer applies; update this script)"
    return
  fi
  # A mutation that does not compile proves nothing, so it must not count as caught.
  if ! (cd "$dir/repo" && go vet ./internal/... >"$dir/out.txt" 2>&1); then
    echo "BROKEN  ${names[i]} (the mutated code does not compile: $(grep -a -m1 -E '\.go:' "$dir/out.txt"))"
    return
  fi
  local -a args
  eval "args=(${argv[i]})"
  # A long limit, so a package slowed by runs beside it fails on a test, not on the clock.
  if (cd "$dir/repo" && go test -count=1 -timeout=30m "${args[@]}" >"$dir/out.txt" 2>&1); then
    echo "MISSED  ${names[i]}"
  elif failed=$(grep -a -m1 -E '^--- FAIL' "$dir/out.txt"); then
    echo "caught  ${names[i]}: $failed"
  else
    echo "BROKEN  ${names[i]} (no test failed: $(grep -a -m1 . "$dir/out.txt"))"
  fi
}

# run_one <index>: runs a mutation and leaves its result line in a file, then cleans its copy.
run_one() {
  local i=$1 dir="$work/job-$1"
  mkdir -p "$dir"
  if [ "${kinds[i]}" = web ]; then
    run_web "$i" "$dir" >"$work/result-$i.tmp"
  elif [ "${kinds[i]}" = collab ]; then
    run_collab "$i" "$dir" >"$work/result-$i.tmp"
  else
    run_go "$i" "$dir" >"$work/result-$i.tmp"
  fi
  rm -rf "$dir"
  mv "$work/result-$i.tmp" "$work/result-$i"
}

# finish_mutations: runs the queued mutations, at most MUTATION_JOBS at once, and prints the
# results in the order of this file as soon as they are known. Sets survived.
finish_mutations() {
  local next=0 printed=0 running=0 line
  local -a picked=()
  local i
  for ((i = 0; i < count; i++)); do
    if needed "$i"; then picked+=("$i"); fi
  done
  ran=${#picked[@]}
  if [ -n "$changed_list" ]; then
    echo "running $ran of $count mutations for the changed paths in $changed_list"
  fi
  take_snapshot
  while [ "$printed" -lt "${#picked[@]}" ]; do
    while [ "$next" -lt "${#picked[@]}" ] && [ "$running" -lt "$jobs_max" ]; do
      run_one "${picked[next]}" &
      next=$((next + 1))
      running=$((running + 1))
    done
    while [ "$printed" -lt "${#picked[@]}" ] && [ -f "$work/result-${picked[printed]}" ]; do
      line=$(cat "$work/result-${picked[printed]}")
      echo "$line"
      case "$line" in
        caught*) ;;
        *) survived=$((survived + 1)) ;;
      esac
      printed=$((printed + 1))
    done
    if [ "$running" -gt 0 ] && [ "$printed" -lt "${#picked[@]}" ]; then
      wait -n
      running=$((running - 1))
    fi
  done
  wait
}

perms=./internal/permissions/
app=./internal/app/

mutate "Evaluate ignores view restrictions" internal/permissions/evaluate.go \
  's.replace("in.User.matchesAny(level.Subjects), true)", "true, true)")' $perms
mutate "Evaluate ignores edit restrictions" internal/permissions/evaluate.go \
  's.replace("in.User.matchesAny(p.EditSubjects), false)", "true, false)")' $perms
mutate "guests count as all members" internal/permissions/evaluate.go \
  's.replace(`return u.Type == "member"`, "return true")' $perms
mutate "archived spaces stay writable" internal/permissions/evaluate.go \
  's.replace(`in.Space.Status != "archived", false)`, "true, false)")' $perms
mutate "others can read unpublished pages" internal/permissions/evaluate.go \
  's.replace(`p.Status != "draft" || p.CreatedBy == in.User.ID, true)`, "true, true)")' $perms
mutate "SQL: restrictions hide nothing" internal/db/migrations/0003_permission_functions.sql \
  's.replace("HAVING NOT bool_or(", "HAVING false AND NOT bool_or(")' $perms
mutate "SQL: guests count as all members" internal/db/migrations/0010_people.sql \
  's.replace(`(g.subject_type = '"'"'all_members'"'"' AND x.type = '"'"'member'"'"')`, `(g.subject_type = '"'"'all_members'"'"')`)' $perms
mutate "reading a page skips the check" internal/pages/pages.go \
  's.replace("if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {\n\t\treturn Detail{}, err\n\t}", "")' $app -run '^(TestADatabaseOnlyOpensForThoseWhoMaySeeIt|TestANamedApproverCanReadAnUnpublishedPage|TestAPageTheAccountCannotSeeStopsTheRun|TestARestoreBelowAnArchivedPageCannotDropARestriction|TestAdministratorsHaveNoContentAccessUntilRecovery|TestAllowingAccessNamesEveryLevelAndNeedsThemAll|TestAnApprovedInvitationGoesThroughTheQueueWithoutTheContent|TestAskingForAccessReachesSomeoneWhoMayGrantIt|TestDeleteRestoreAndPurge|TestDeletingAPageTakesArchivedPagesBelowItAlong|TestDeletingAParentAloneIsRefusedSoNothingFallsOpen|TestEveryObjectRouteIsProtected|TestGroups|TestMoveToAnotherSpaceTakesTheBranchAlong|TestRestrictionsAcrossTreeSearchFavouritesAndExport|TestSeveralWordFilesWaitUnderARestrictedPage|TestSharingOnlyNotifies|TestSpaceKeepsAnAdministratorAndGuestsAreNotMembers|TestStarterTemplatesAreThereAndCanBeHidden|TestUnpublishedPageIsPrivate)$'
mutate "downloads skip the check" internal/attachments/attachments.go \
  's.replace("if err := s.perms.RequirePage(ctx, s.pool, c.UserID, a.PageID, permissions.ActionPageView, uuid.Nil); err != nil {\n\t\treturn a, nil, err\n\t}\n\tf, err :=", "f, err :=")' $app -run '^(TestAFileAnswersNotModifiedButStillChecksAccess|TestAFileMovesAcrossSpacesAndFollowsTheRulesOfItsNewPage|TestAnOfficeFileOpensAsPDFThroughTheHelper|TestAttachmentsNeverTrustAFile|TestEveryObjectRouteIsProtected|TestTemplateFilesFollowTheRuleOfTheTemplate)$'
mutate "any member can import a font" internal/httpapi/handlers_appearance.go \
  's.replace("admin := SessionOnly(Permission(identity.PermAdministrator))", "admin := SessionOnly(Permission(identity.PermUseProduct))")' $app -run '^(TestAdministratorImportsReplacesAndRemovesAFont|TestRouteTableIsDocumented|TestTheAPIDescriptionMatchesTheRoutes)$'
mutate "any file passes as a font" internal/settings/font.go \
  's.replace(`return "", false`, `return "woff2", true`)' $app -run '^(TestAdministratorImportsReplacesAndRemovesAFont)$'
mutate "font changes are not audited" internal/settings/font.go \
  's.replace(`c.Event("settings.font_changed"`, `c.Event("settings.updated"`)' $app -run '^(TestAdministratorImportsReplacesAndRemovesAFont)$'
mutate "a replaced font file stays on disk" internal/settings/font.go \
  's.replace("if found {", "if false && found {")' $app -run '^(TestAdministratorImportsReplacesAndRemovesAFont)$'
mutate "publishing skips the check" internal/pages/versions.go \
  's.replace("if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {\n\t\t\treturn err\n\t\t}\n\t\tp, err := lockPage(ctx, tx, id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\thead, err := headVersion(ctx, tx, p)", "p, err := lockPage(ctx, tx, id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\thead, err := headVersion(ctx, tx, p)")' $app -run '^(TestADatabaseOnlyOpensForThoseWhoMaySeeIt|TestArchivedPagesAreReadOnlyAndLeaveEveryList|TestEditRestrictionDoesNotInherit|TestEveryObjectRouteIsProtected|TestScheduledPublishingStopsWhenTheRightsAreGone)$'
mutate "search ignores permissions" internal/search/words.go \
  's.replace("JOIN (SELECT v.page_id FROM perm_visible_pages(@user) v WHERE v.page_id = ANY(@ids)) v ON v.page_id = p.id", "")' $app -run TestAWrongRowOfTheSearchTableShowsNothing
mutate "a hidden page counts in a search" internal/search/words.go \
  's.replace("const notHidden = ` AND w.page_id NOT IN (SELECT unnest(@hidden::uuid[]))`", "const notHidden = ``")' $app -run TestARestrictionCountsAtOnceInSearch
mutate "a space nobody gave counts in a search" internal/search/words.go \
  's.replace("FROM perm_spaces(@user, \x27space.view\x27) g JOIN spaces s", "FROM (SELECT id AS space_id FROM spaces) g JOIN spaces s")' $app -run TestLosingASpaceCountsAtOnceInSearch
mutate "the search table misses the archive" internal/db/migrations/0028_search_pages.sql \
  's.replace("published = EXCLUDED.published, archived = EXCLUDED.archived,", "published = EXCLUDED.published,")' $app -run TestTheSearchTableFollowsThePage
mutate "the search table misses the trash" internal/db/migrations/0028_search_pages.sql \
  's.replace("published = EXCLUDED.published, archived = EXCLUDED.archived,", "archived = EXCLUDED.archived,")' $app -run TestTheSearchTableFollowsThePage
mutate "a hidden child shows as a branch" internal/pages/branches.go \
  's.replace("EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = p.id AND ` + listed(\"c\") + `)", "EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = p.id)")' $app -run TestAHiddenChildLeavesNoTraceInTheTree
mutate "the tree picks hidden rows and the rule has to stop them" internal/pages/branches.go \
  's.replace("AND ` + alias + `.id NOT IN (SELECT unnest(@hidden::uuid[]))`", "AND @hidden::uuid[] IS NOT NULL`")' $app -run TestAHiddenChildLeavesNoTraceInTheTree
mutate "the rows of a tree skip the rule" internal/pages/branches.go \
  's.replace("JOIN (SELECT v.page_id FROM perm_listed_pages(@user) v WHERE v.page_id = ANY(@ids)) v ON v.page_id = p.id", "JOIN (SELECT unnest(@ids::uuid[]) AS page_id) v ON v.page_id = p.id")' $app -run TestAHiddenChildLeavesNoTraceInTheTree
mutate "export ignores permissions" internal/pages/export.go \
  's.replace("JOIN perm_visible_pages($1) vis ON vis.page_id = p.id\n", "JOIN (SELECT id AS page_id FROM pages, (SELECT $1::uuid) x) vis ON vis.page_id = p.id\n")' $app -run '^(TestExportLeavesOutWhatThePersonCannotSee|TestRestrictionsAcrossTreeSearchFavouritesAndExport)$'
mutate "moves can drop restrictions" internal/pages/tree.go \
  's.replace("if !slices.Contains(after, r) {", "if !slices.Contains(after, r) && len(before) < 0 {")' $app -run '^(TestMoveCannotQuietlyDropARestriction)$'
mutate "moves can create loops" internal/pages/tree.go \
  's.replace("if slices.Contains(tree, *parent) {", "if slices.Contains(tree, *parent) && len(tree) < 0 {")' $app -run '^(TestTreeMoveAndStableURL)$'
mutate "a page returns while its parent is in the trash" internal/pages/tree.go \
  's.replace("if parent.Status == StatusTrashed {", "if parent.Status == StatusTrashed && len(parent.Title) < 0 {")' $app -run '^(TestDeleteRestoreAndPurge|TestTrashCanBeSearchedAndWorkedOffTogether)$'
mutate "SVG shown inline" internal/attachments/attachments.go \
  's.replace(`"image/webp": true}`, `"image/webp": true, "text/xml; charset=utf-8": true, "text/plain": true, "text/xml": true}`)' $app -run '^(TestAttachmentsNeverTrustAFile|TestPageLookIsCheckedAndAudited)$'
mutate "publish loses its audit event" internal/pages/pages.go \
  's.replace("if err := record(ctx, tx, c, \"page.created\", p, map[string]any{\"parent_id\": in.parent, \"kind\": in.kind}); err != nil {", "if err := error(nil); err != nil {")' ./internal/app/ -run TestAuditAndOutboxPerStateChange
mutate "javascript links accepted" internal/document/document.go \
  's.replace("default:\n\t\treturn \"\", bad\n\t}\n\treturn href, nil", "default:\n\t}\n\treturn href, nil")' ./internal/document/
mutate "a file may point anywhere" internal/document/attrs.go \
  's.replace("return errors.New(\"files must point to an attachment\")", "id = \"\"")' ./internal/document/
mutate "free colours accepted" internal/document/attrs.go \
  's.replace("if !allowed[color] {", "if color == \"\" {")' ./internal/document/
mutate "any date accepted" internal/document/attrs.go \
  's.replace("if _, err := time.Parse(time.DateOnly, value); err != nil {", "if _, err := time.Parse(time.DateOnly, \"2026-01-01\"); err != nil {")' ./internal/document/
mutate "any number of columns accepted" internal/document/document.go \
  's.replace("if n.Type == TypeColumns && (len(n.Content) < minColumns || len(n.Content) > maxColumns) {", "if n.Type == TypeColumns && len(n.Content) < 0 {")' ./internal/document/
mutate "a table takes any width" internal/document/attrs.go \
  's.replace("if stretch, ok := intAttr(in[\"stretch\"]); ok && stretch >= 1 && stretch <= maxTableStretch {", "if stretch, ok := intAttr(in[\"stretch\"]); ok && stretch != maxTableStretch-99 {")' ./internal/document/
mutate "unknown panels accepted" internal/document/attrs.go \
  's.replace("panelKinds.keep(in, out, \"kind\")\n\tif out[\"kind\"] == nil {", "out[\"kind\"] = in[\"kind\"]\n\tif out[\"kind\"] == nil {")' ./internal/document/

mutate "a TOTP code can be replayed" internal/identity/factors.go \
  's.replace("WHERE user_id = $1 AND last_step < $2", "WHERE user_id = $1 AND last_step <= $2 + 100")' $app -run TestTOTP
mutate "password alone opens a session with TOTP on" internal/identity/identity.go \
  's.replace("mfa, err := s.hasTOTP(ctx, s.pool, u.ID)", "mfa, err := s.hasTOTP(ctx, s.pool, u.ID)\n\tmfa = false")' $app -run TestTOTP
mutate "OIDC takes over a local account by e-mail" internal/sso/sso.go \
  's.replace("if source != identity.SourceSCIM || hasPassword || !verified {", "if len(source) < 0 && hasPassword && verified {")' $app -run TestOIDCRefusals
mutate "OIDC ignores the nonce" internal/sso/sso.go \
  's.replace("if idToken.Nonce != f.Nonce {", "if len(idToken.Nonce) < 0 {")' $app -run TestOIDCRefusals
mutate "OIDC login is not tied to the browser" internal/sso/sso.go \
  's.replace("if binding == \"\" || binding != f.Binding {", "if len(binding) < 0 {")' $app -run TestOIDCRefusals
mutate "SCIM accepts a revoked token" internal/scim/scim.go \
  's.replace("WHERE token_hash = $1 AND revoked_at IS NULL", "WHERE token_hash = $1")' $app -run TestSCIMTokens
mutate "people can edit a group the provider owns" internal/identity/groups.go \
  's.replace("if c.System == \"\" && g.Source != SourceLocal {", "if len(g.Source) < 0 {")' $app -run TestSCIMEntra
mutate "deleting a group lifts restrictions" internal/identity/groups.go \
  's.replace("if _, err := tx.Exec(ctx, \"DELETE FROM groups WHERE id = $1\", id); err != nil {", "if _, err := tx.Exec(ctx, \"DELETE FROM page_restrictions WHERE subject_id = $1\", id); err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif _, err := tx.Exec(ctx, \"DELETE FROM groups WHERE id = $1\", id); err != nil {")' $app -run TestGroups
mutate "the app role can change audit events" internal/db/db.go \
  's.replace("\"REVOKE UPDATE, DELETE, TRUNCATE ON audit_events FROM \" + ident,", "")' $app -run TestApplicationRole
mutate "contract migrations ignore older nodes" internal/app/ops.go \
  's.replace("if oldest < pending[len(pending)-1].Version {", "if oldest < 0 {")' $app -run TestContract
mutate "disabled users keep their sessions" internal/identity/users.go \
  's.replace("if _, err := tx.Exec(ctx, \"DELETE FROM sessions WHERE user_id = $1\", id); err != nil {\n\t\t\t\treturn err\n\t\t\t}\n\t\t\tif err := s.keepsAnAdministrator", "if err := s.keepsAnAdministrator")' $app -run TestAdminUserManagement

mutate "links reveal hidden pages" internal/pages/details.go \
  's.replace("refJoins   = `JOIN perm_listed_pages($1) v ON v.page_id = p.id", "refJoins   = `JOIN (SELECT $1::uuid AS u) v ON true")' $app -run TestDetailsNeverRevealHiddenPages
mutate "a header can be a file from another page" internal/attachments/attachments.go \
  's.replace("WHERE a.id = $1 AND a.page_id = $2 AND a.status = \x27active\x27`, id, pageID)", "WHERE a.id = $1 AND $2::uuid IS NOT NULL AND a.status = \x27active\x27`, id, pageID)")' $app -run TestPageLook
mutate "anyone can undo a delete" internal/trash/trash.go \
  's.replace("AND deleted_by = $2", "AND $2::uuid IS NOT NULL")' $app -run TestUndoOfADelete
mutate "undo never runs out" internal/trash/trash.go \
  's.replace("AND deleted_at > now() - make_interval(secs => $3) FOR UPDATE", "AND $3::float8 > 0 FOR UPDATE")' $app -run TestUndoOfADelete
mutate "any theme is accepted" internal/identity/preferences.go \
  's.replace("case !slices.Contains(themes, p.Theme):", "case len(p.Theme) < 0:")' $app -run TestPreferences
mutate "a shortcut may go anywhere" internal/spaces/shortcuts.go \
  's.replace("if err != nil || strings.HasPrefix(lower", "if (err != nil && len(lower) < 0) || strings.HasPrefix(lower")' $app -run TestShortcuts
mutate "the directory counts hidden pages" internal/spaces/directory.go \
  's.replace("FROM pages p JOIN perm_listed_pages($1) v ON v.page_id = p.id", "FROM pages p")' $app -run TestDirectory
mutate "quick search shows people to everyone" internal/httpapi/handlers_reading.go \
  's.replace("if err != nil || !allowed {", "if err != nil || (!allowed && len(text) < 0) {")' $app -run TestQuickSearch
mutate "quick search shows hidden files" internal/attachments/attachments.go \
  's.replace("SELECT v.page_id FROM perm_listed_pages($1) v", "SELECT v.id AS page_id FROM pages v")' $app -run TestQuickSearch
mutate "a row menu counts as a visit" internal/pages/details.go \
  's.replace("out.Can, err = s.capabilities(ctx, c, p)\n\treturn out, err", "s.touchRecent(ctx, c.UserID, p)\n\tout.Can, err = s.capabilities(ctx, c, p)\n\treturn out, err")' $app -run TestOpeningARowMenu

mutate "archived pages stay writable" internal/permissions/evaluate.go \
  's.replace("if p.Archived && !worksOnArchivedPage(in.Action) {", "if p.Archived && len(in.Action) < 0 {")' $perms
mutate "templates open as pages" internal/permissions/evaluate.go \
  's.replace("p.Kind != KindTemplate, true)", "true, true)")' $perms
mutate "SQL: lists show archived pages" internal/db/migrations/0008_organising.sql \
  's.replace("WHERE p.archived_at IS NULL\n$$;", "WHERE true\n$$;")' $perms
mutate "SQL: templates show in lists" internal/db/migrations/0051_document_control.sql \
  's.replace("AND p.kind <> \x27template\x27\n", "\n")' $perms
mutate "a label reveals hidden pages" internal/pages/labels.go \
  's.replace("JOIN perm_listed_pages($1) v ON v.page_id = l.page_id", "JOIN (SELECT $1::uuid AS u) v ON true")' $app -run TestLabelsAreCleanedAndNeverRevealHiddenPages
mutate "a spelling suggestion uses hidden titles" internal/search/search.go \
  's.replace("AND w.page_id NOT IN (SELECT h.page_id FROM perm_hidden_pages($1) h)", "")' $app -run TestSearchFiltersNeverRevealHiddenPages
mutate "a move skips hidden pages" internal/pages/move.go \
  's.replace("if hidden {\n", "if hidden && len(tree) < 0 {\n")' $app -run TestMoveBetweenSpacesIsRefusedWhenItWouldTouchTooMuch
mutate "a restore drops a restriction" internal/pages/archive.go \
  's.replace("if len(parents) > 0 && !withParents {", "if len(parents) < 0 && !withParents {")' $app -run TestARestoreBelowAnArchivedPageCannotDropARestriction
mutate "publishing ignores required approval" internal/pages/control.go \
  's.replace("\treturn s.requireApproved(ctx, tx, p, title, doc)\n", "\treturn nil\n")' $app -run TestASpaceCanRequireApprovalOfTheExactText
mutate "a scheduled publish ignores lost rights" internal/pages/schedule.go \
  's.replace("_, err = s.Publish(ctx, identity.Caller{UserID: due.user}, due.page,", "var author uuid.UUID\n\t\t_ = s.pool.QueryRow(ctx, \"SELECT created_by FROM pages WHERE id = $1\", due.page).Scan(&author)\n\t\t_, err = s.Publish(ctx, identity.Caller{UserID: author}, due.page,")' $app -run TestScheduledPublishingStopsWhenTheRightsAreGone
mutate "handing over gives delete own" internal/permissions/service.go \
  's.replace("SELECT p.space_id, p.status, p.created_by, p.kind,", "SELECT p.space_id, p.status, COALESCE(p.owner_id, p.created_by), p.kind,")' $app -run TestHandingOverAPageGivesNoRightToDeleteIt
mutate "a bulk job skips the check" internal/pages/content.go \
  's.replace("if err := s.applyContent(ctx, c, id, in.Action); err != nil {", "var author uuid.UUID\n\t\t_ = s.pool.QueryRow(ctx, \"SELECT created_by FROM pages WHERE id = $1\", id).Scan(&author)\n\t\t_ = c\n\t\tif err := s.applyContent(ctx, identity.Caller{UserID: author}, id, in.Action); err != nil {")' $app -run TestContentScreenFiltersAndReportsWhatWorked

mutate "the trash names a hidden page" internal/trash/trash.go \
  's.replace("CASE WHEN h.page_id IS NULL THEN t.title ELSE \x27\x27 END, t.deleted_by", "t.title, t.deleted_by")' $app -run TestTrashNeverNamesWhatIsHiddenFromThePerson
mutate "the trash search finds a hidden title" internal/trash/trash.go \
  's.replace("($2 = \x27\x27 OR (h.page_id IS NULL AND position(", "($2 = \x27\x27 OR (position(")' $app -run TestTrashNeverNamesWhatIsHiddenFromThePerson
mutate "a hidden item can be purged by hand" internal/trash/trash.go \
  's.replace("if hidden {\n", "if hidden && len(it.Title) < 0 {\n")' $app -run TestTrashNeverNamesWhatIsHiddenFromThePerson
mutate "a purge takes hidden pages along" internal/trash/trash.go \
  's.replace("OR h.page_id IN (SELECT t.descendant_id FROM page_tree t WHERE t.ancestor_id = $2))", "OR h.page_id = $2)")' $app -run TestPurgingAPageIsRefusedWhenAPageBelowItIsHidden
mutate "a copy can lose a restriction from above" internal/pages/tree.go \
  's.replace("lost, _, err := inheritedChange(ctx, tx, p, in.ParentID)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif len(lost) > 0 {", "lost, _, err := inheritedChange(ctx, tx, p, in.ParentID)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif len(lost) < 0 {")' $app -run TestACopyCannotQuietlyLoseAnInheritedRestriction
mutate "any file comes through a template" internal/attachments/attachments.go \
  's.replace("if a.PageID != pageID {", "if a.PageID != pageID && len(a.Filename) < 0 {")' $app -run TestTemplateFilesFollowTheRuleOfTheTemplate
mutate "template files ignore who may read the template" internal/templates/templates.go \
  's.replace("if err := s.mayRead(ctx, s.pool, c, t); err != nil {\n\t\treturn attachments.Attachment{}, nil, err\n\t}\n\treturn s.files.OpenOfPage", "return s.files.OpenOfPage")' $app -run TestTemplateFilesFollowTheRuleOfTheTemplate
mutate "the link filter tells hidden from gone" internal/pages/content.go \
  's.replace("AND k.target_id NOT IN (SELECT g.page_id FROM perm_visible_pages($1) g)))", "AND k.target_id NOT IN (SELECT g.id FROM pages g WHERE g.status <> \x27trashed\x27 AND $1::uuid IS NOT NULL)))")' $app -run TestABrokenLinkFilterTreatsHiddenPagesAsGone
mutate "reading a round ignores restrictions" internal/permissions/evaluate.go \
  's.replace("for _, level := range p.ViewChain {", "for _, level := range p.ViewChain {\n\t\tif in.Action == ActionApprovalRead {\n\t\t\tbreak\n\t\t}")' $perms -run TestApprovalReadIsViewWithoutTheDraftRule

mutate "a manager reaches administer" internal/permissions/manage.go \
  's.replace("if !d.Allowed && touchesAdminister {", "if !d.Allowed && touchesAdminister && len(after) < 0 {")' $app -run TestManager
mutate "administer does not imply managing access" internal/permissions/evaluate.go \
  's.replace("holds(in, SpaceManageAccess) || holds(in, SpaceAdminister)", "holds(in, SpaceManageAccess)")' $perms
mutate "SQL: an expired account stays active" internal/db/migrations/0010_people.sql \
  's.replaceAll("AND (x.expires_at IS NULL OR x.expires_at > now())", "")' $perms -run TestMatrixSQL
mutate "LoadUser: an expired account stays active" internal/permissions/service.go \
  's.replace("u.Active = activeAt(status, expires, time.Now())", "u.Active = status == \"active\"")' $perms -run TestMatrixSQL
mutate "a guest sees everyone" internal/permissions/evaluate.go \
  's.replace("return target.ID == viewer.ID || slices.Contains(viewer.Global, globalViewProfiles) || target.SharedSpace", "return true")' $perms -run TestMatrixPeople
mutate "SQL: a guest sees everyone" internal/db/migrations/0010_people.sql \
  's.replace("AND (t.id = u\n        OR perm_has_global(u, \x27view_profiles\x27)", "AND (true\n        OR perm_has_global(u, \x27view_profiles\x27)")' $perms -run TestMatrixPeople
mutate "a reset link works twice" internal/identity/factors.go \
  's.replace("\"DELETE FROM auth_challenges WHERE token_hash = $1 AND kind = $2 RETURNING user_id, data, expires_at\"", "\"SELECT user_id, data, expires_at FROM auth_challenges WHERE token_hash = $1 AND kind = $2\"")' $app -run TestAResetLink
mutate "a reset says whether the address is known" internal/identity/lifecycle.go \
  's.replace("if errors.Is(err, pgx.ErrNoRows) {\n\t\treturn nil\n\t}\n\tif err != nil {\n\t\treturn err\n\t}\n\tif _, err := s.pool.Exec(ctx, \"DELETE FROM auth_challenges WHERE kind = $1 AND user_id = $2\"", "if errors.Is(err, pgx.ErrNoRows) {\n\t\treturn s.send(ctx, s.pool, AccountMail{To: email, Kind: MailReset})\n\t}\n\tif err != nil {\n\t\treturn err\n\t}\n\tif _, err := s.pool.Exec(ctx, \"DELETE FROM auth_challenges WHERE kind = $1 AND user_id = $2\"")' $app -run TestAResetLink
mutate "a read token writes" internal/httpapi/server.go \
  's.replace("return identity.ScopeWrite\n", "return identity.ScopeRead\n")' $app -run TestTokens
mutate "a token reaches account security" internal/httpapi/server.go \
  's.replace("case p.ViaToken && a.sessionOnly:", "case p.ViaToken && len(r.Method) < 0:")' $app -run TestTokens
mutate "sensitive lets anything through" internal/httpapi/server.go \
  's.replace("case a.sensitive && time.Since(p.ConfirmedAt) > identity.ReauthWindow:", "case a.sensitive && time.Since(p.ConfirmedAt) < 0:")' $app -run TestEverySensitive
mutate "a write passes during maintenance" internal/httpapi/middleware.go \
  's.replace("if state.On {", "if state.On && len(r.Method) < 0 {")' $app -run TestMaintenance
mutate "a dangerous CSV cell passes" internal/csvout/csvout.go \
  's.replace("return \"\x27\" + s", "return s")' $app -run TestWhoHasAccess
mutate "a failing accent is accepted" internal/settings/sections.go \
  's.replace("if contrast(a, b) < normalTextRatio {", "if contrast(a, b) < 0 {")' $app -run TestAnAccent
mutate "SVG passes as a logo" internal/settings/logo.go \
  's.replace("if !slices.Contains(logoTypes, contentType) {", "if len(contentType) < 0 && !slices.Contains(logoTypes, contentType) {")' $app -run TestALogo
mutate "a space audit shows a hidden title" internal/pages/lists.go \
  's.replace("row.ObjectID, row.Details = \"\", nil", "row.ObjectType = e.ObjectType")' $app -run TestASpaceAudit
mutate "polling keeps a session alive" internal/httpapi/middleware.go \
  's.replace("if r.Header.Get(backgroundHeader) != \"\" {", "if len(r.Method) < 0 {")' $app -run TestPolling
mutate "a leaked password is accepted" internal/identity/lifecycle.go \
  's.replace("if s.blocklist[lower] || filter.Contains(lower) {", "if s.blocklist[lower] || (len(lower) < 0 && filter.Contains(lower)) {")' $app -run TestANewPassword
mutate "the sign-in question can be put off for ever" internal/identity/lifecycle.go \
  's.replace("out.Prompt = out.Weak && (after == nil || !s.now().Before(*after))", "out.Prompt = out.Weak && after == nil")' $app -run TestPeopleAreAsked
mutate "picture metadata survives" internal/people/picture.go \
  's.replace("if err := png.Encode(&out, scaled(img, crop)); err != nil {\n\t\treturn nil, err\n\t}\n\treturn out.Bytes(), nil", "_ = png.Encode\n\t_ = scaled\n\t_ = img\n\t_ = &out\n\treturn data, nil")' $app -run TestAPicture
mutate "anonymising leaves the profile" internal/httpapi/handlers_console.go \
  's.replace("if err := s.deps.People.Forget(r.Context(), id); err != nil {", "if err := error(nil); err != nil {")' $app -run TestAnonymising

mutate "a changed anchor passes" internal/audit/prune.go \
  's.replace("ed25519.Verify(a.publicKey, anchorMessage(a.seq, a.prevHash), a.signature)", "len(a.signature) >= 0")' $app -run TestPruningLeaves
mutate "a damaged chain is pruned" internal/audit/prune.go \
  's.replace("if !check.OK {", "if !check.OK && len(check.Reason) < 0 {")' $app -run TestPruningADamaged
mutate "jobs run during maintenance" internal/jobs/jobs.go \
  's.replace("if held {", "if held && s == nil {")' $app -run TestMaintenanceHolds
mutate "a passkey login needs no local check" internal/identity/passkeys.go \
  's.replace("webauthn.WithUserVerification(protocol.VerificationRequired)", "webauthn.WithUserVerification(protocol.VerificationDiscouraged)")' $app -run TestPasskeysRequire
mutate "a passkey is registered without a local check" internal/identity/passkeys.go \
  's.replace("UserVerification:   protocol.VerificationRequired,", "UserVerification:   protocol.VerificationDiscouraged,")' $app -run TestPasskeysRequire
mutate "adding a passkey needs no proof" internal/httpapi/handlers_identity.go \
  's.replace("\"POST /api/v1/me/passkeys/begin\", Sensitive(me)", "\"POST /api/v1/me/passkeys/begin\", me")' $app -run TestFactorChanges
mutate "removing a passkey needs no proof" internal/httpapi/handlers_identity.go \
  's.replace("\"DELETE /api/v1/me/passkeys/{id}\", Sensitive(me)", "\"DELETE /api/v1/me/passkeys/{id}\", me")' $app -run TestFactorChanges
mutate "turning off a factor needs no proof" internal/httpapi/handlers_identity.go \
  's.replace("\"POST /api/v1/me/totp/disable\", Sensitive(me)", "\"POST /api/v1/me/totp/disable\", me")' $app -run TestFactorChanges
mutate "a typed address reaches the audit log" internal/identity/identity.go \
  's.replace("details[\"address_hash\"] = hex.EncodeToString(sum)", "details[\"address_hash\"] = hex.EncodeToString(sum) + f.email")' $app -run TestAFailedLogin
mutate "checkpoints are not checked" internal/audit/checkpoint.go \
  's.replace("res.Checkpoints, err = VerifyCheckpoints(ctx, q, public)", "res.Checkpoints, err = 1, nil")' $app -run TestABadCheckpoint
mutate "a damaged log stays out of the console" internal/console/console.go \
  's.replace("\tif damaged {", "\tif damaged && len(out) < 0 {")' $app -run TestABadCheckpoint
mutate "recovery codes hold 80 bits" internal/identity/factors.go \
  's.replace("recoveryCodeBytes = 16", "recoveryCodeBytes = 10")' ./internal/identity/ -run TestARecoveryCode
mutate "an administrator's reset leaves sessions" internal/identity/factors.go \
  's.replace("_, err := tx.Exec(ctx, \"DELETE FROM sessions WHERE user_id = $1\", id)\n\t\treturn err\n\t})\n}\n\nfunc (s *Service) removeTOTP", "return nil\n\t})\n}\n\nfunc (s *Service) removeTOTP")' $app -run TestAnAdministratorsReset
mutate "an expired SCIM token works" internal/scim/scim.go \
  's.replace(" AND expires_at > now()`, hash(token))", "`, hash(token))")' $app -run TestSCIMTokensEnd
mutate "an administrator reads a hidden title on the screen" internal/pages/lists.go \
  's.replace("if id := auditedContent(e); id != \"\" && !visible[id] {", "if id := auditedContent(e); id != \"\" && len(id) < 0 {")' $app -run TestAnAdministratorSeesACensored
mutate "an export shows a hidden title" internal/audit/audit.go \
  's.replace("\t\te, err := f.read(row.payload, row.part)", "\t\tuncensored := f\n\t\tuncensored.Censor = nil\n\t\te, err := uncensored.read(row.payload, row.part)")' $app -run TestAnAdministratorSeesACensored
mutate "the note on a page shows its title" internal/pages/lists.go \
  's.replace("if !note.Readable {", "if !note.Readable && len(note.Title) < 0 {")' $app -run TestAnAdministratorSeesACensored
mutate "the grace period never ends" internal/app/app.go \
  's.replace("if time.Since(created) < settings.SecondFactorGrace {", "if time.Since(created) > 0 {")' $app -run TestAdministratorsNeedASecondFactor
mutate "a name goes into the chain" internal/audit/audit.go \
  's.replace("if !slices.Contains(personalDetails, key) {", "if len(key) > 0 || !slices.Contains(personalDetails, key) {")' $app -run TestPersonalDataInTheAuditLog
mutate "erasing leaves the personal part" internal/identity/lifecycle.go \
  's.replace("if err := audit.ErasePersonal(ctx, tx, id.String()); err != nil {", "if err := error(nil); err != nil {")' $app -run TestPersonalDataInTheAuditLog
mutate "a changed personal part is not noticed" internal/audit/audit.go \
  's.replace("if part != nil && personalDigest(salt, *part) != e.PersonalDigest {", "if part != nil && len(salt) < 0 {")' $app -run TestChangingAPersonalPart
mutate "the queue reads the link of an account mail" internal/app/account_mail.go \
  's.replace("sealed, err := m.keyring.Seal(body, accountMailContext)", "sealed, err := body, error(nil)")' $app -run TestAResetMail
mutate "a sent account mail stays sealed in the queue" internal/mail/queue.go \
  's.replace("last_error = \x27\x27, sealed = NULL,", "last_error = \x27\x27,")' $app -run TestAResetMail
mutate "mail goes out during maintenance" internal/mail/queue.go \
  's.replace("if err != nil || held {", "if err != nil || (held && s == nil) {")' $app -run TestMailWaits
mutate "the audit log stays for ever without a reason" internal/settings/sections.go \
  's.replace("strings.TrimSpace(r.LongerBecause) == \"\"", "len(r.LongerBecause) < 0")' $app -run TestTheAuditLogStays
mutate "anonymising leaves the watches" internal/identity/lifecycle.go \
  's.replace("\"group_members\", \"follows\", \"watches\", \"comment_seen\",", "\"group_members\", \"follows\", \"comment_seen\",")' $app -run TestAnonymisingLeavesTheId
mutate "a rotated token keeps its full life" internal/scim/scim.go \
  's.replace("SET expires_at = LEAST(expires_at, $2)", "SET expires_at = GREATEST(expires_at, $2)")' $app -run TestSCIMTokensEnd
mutate "a notification reaches someone who cannot see the page" internal/notify/notify.go \
  's.replace("if err != nil || !allowed {\n\t\treturn err\n\t}\n\tdata, err :=", "if err != nil || (!allowed && len(r.Type) < 0) {\n\t\treturn err\n\t}\n\tdata, err :=")' $app -run TestWatchersHearOnceAndLosingAccessEndsIt
mutate "the bell shows a page you lost" internal/notify/list.go \
  's.replaceAll("JOIN perm_visible_pages($1) v ON v.page_id = n.page_id", "JOIN (SELECT id AS page_id FROM pages, (SELECT $1::uuid) x) v ON v.page_id = n.page_id")' $app -run TestWatchersHearOnceAndLosingAccessEndsIt
mutate "mail about a page you lost still goes out" internal/notify/render.go \
  's.replace("s.pool.Query(ctx, visibleRows+", "s.pool.Query(ctx, strings.Replace(visibleRows, \"JOIN perm_visible_pages($1) v ON v.page_id = n.page_id\", \"JOIN (SELECT id AS page_id FROM pages, (SELECT $1::uuid) x) v ON v.page_id = n.page_id\", 1)+")' $app -run TestMailIsWrittenWhenSentAndChecksAccessAgain
mutate "people hear about their own actions" internal/notify/notify.go \
  's.replace("(r.UserID == r.ActorID && !choices.NotifyOwn)", "(len(r.Type) < 0 && !choices.NotifyOwn)")' $app -run TestOwnActionsOnlyWhenAskedFor
mutate "a forged unsubscribe token works" internal/notify/unsubscribe.go \
  's.replace("raw, err := s.options.Keyring.Open(sealed, tokenContext)\n\tif err != nil {\n\t\treturn apperr.NotFound()\n\t}", "raw, err := s.options.Keyring.Open(sealed, tokenContext)\n\tif err != nil {\n\t\traw = sealed\n\t}")' $app -run TestUnsubscribeNeedsNoSessionAndCannotBeForged
mutate "mail goes out without TLS" internal/mail/smtp.go \
  's.replace("return errors.New(\"mail: the server does not offer STARTTLS\")", "return errors.Join()")' ./internal/mail/
mutate "sharing gives access" internal/sharing/sharing.go \
  's.replace("missing = append(missing, person)\n\t\t\t\tcontinue\n", "if err := s.perms.GrantEditor(ctx, tx, c, space.ID, person); err != nil {\n\t\t\t\t\treturn err\n\t\t\t\t}\n\t\t\t\tcontinue\n")' $app -run TestSharingOnlyNotifies
mutate "a page manager can widen a whole space" internal/sharing/sharing.go \
  's.replace("if effect == EffectSpaceGrant {\n\t\treturn permissions.SpaceAdminister", "if len(effect) < 0 {\n\t\treturn permissions.SpaceAdminister")' $app -run TestAllowingOpensTheSpaceAndEveryRestriction
mutate "a closed page still takes comments" internal/pages/look.go \
  's.replace("return closed, err\n}", "return false, err\n}")' $app -run TestClosingCommentsStopsNewOnesAtOnce
mutate "comments ignore what pages says about closing" internal/comments/comments.go \
  's.replace("if !on || closed {", "if !on || closed && false {")' $app -run TestClosingCommentsStopsNewOnesAtOnce
mutate "a comment for editors reaches everyone" internal/comments/list.go \
  's.replace("if v.EditorsOnly && !a.canEdit {", "if v.EditorsOnly && !a.canEdit && len(v.Status) < 0 {")' $app -run TestInlineThreadsResolveAndStayWithEditorsUntilPublishing
mutate "a mention reaches a group of any size" internal/notify/fanout.go \
  's.replace("if len(members) <= maxGroupMembers {", "if len(members) >= 0 {")' $app -run TestGroupMentionsAreOffAndHaveALimit
mutate "a silent update reaches watchers" internal/notify/fanout.go \
  's.replace("if p.Silent {\n\t\treturn nil\n\t}\n\twatchers", "if p.Silent && len(p.Mentions) < 0 {\n\t\treturn nil\n\t}\n\twatchers")' $app -run TestWatchersHearOnceAndLosingAccessEndsIt

mutate "a stored mention keeps the name" internal/document/attrs.go \
  's.replace("out[\"kind\"] = MentionUser\n\t}\n\treturn nil", "out[\"kind\"] = MentionUser\n\t}\n\tout[\"label\"] = in[\"label\"]\n\treturn nil")' $app -run TestAMentionStoresTheIdAndNeverTheName
mutate "search does not find a mentioned person" internal/search/words.go \
  's.replace("WHERE (x.tsv || n.tsv) @@ to_tsquery", "WHERE x.tsv @@ to_tsquery")' $app -run TestAMentionStoresTheIdAndNeverTheName
mutate "an unpublished sentence reaches readers" internal/comments/comments.go \
  's.replace("if reachedReaders(t.anchor, doc) {", "if t.id != uuid.Nil || reachedReaders(t.anchor, doc) {")' $app -run TestInlineThreadsResolveAndStayWithEditorsUntilPublishing
mutate "a reaction changes an archived space" internal/permissions/evaluate.go \
  's.replace("case SpaceView, ActionPageView, ActionApprovalRead, SpaceExport, SpaceAdminister, SpaceManageAccess, AnalyticsView,", "case SpaceView, ActionPageView, ActionApprovalRead, SpaceExport, SpaceAdminister, SpaceManageAccess, AnalyticsView, ActionReact,")' $app -run TestAReactionChangesNothingInAnArchivedSpace
mutate "following tells who exists" internal/watches/watches.go \
  's.replace("if !known || !slices.Contains(facts.Global, identity.PermViewProfiles) {", "if !known || (len(facts.Global) < 0 && !slices.Contains(facts.Global, identity.PermViewProfiles)) {")' $app -run TestFollowingGoesByTheRuleForSeeingPeople
mutate "allowing skips a level out of reach" internal/sharing/sharing.go \
  's.replace("if !canOpenAll(levels) {", "if len(levels) < 0 {")' $app -run TestAllowingAccessNamesEveryLevelAndNeedsThemAll
mutate "a level out of reach shows its title" internal/sharing/sharing.go \
  's.replace("level := Level{Kind: b.Kind, CanManage: d.Allowed}\n\t\tif d.Allowed {", "level := Level{Kind: b.Kind, CanManage: d.Allowed}\n\t\tif len(b.Kind) > 0 {")' $app -run TestAllowingAccessNamesEveryLevelAndNeedsThemAll

mutate "mail ignores the certificate of the server" internal/mail/smtp.go \
  's.replace("return &tls.Config{ServerName: s.Config.Host, MinVersion: tls.VersionTLS12}", "return &tls.Config{ServerName: s.Config.Host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}")' ./internal/mail/
mutate "an unsubscribe token reaches the request log" internal/httpapi/middleware.go \
  's.replace("\"path\", loggedPath(r.URL.Path),", "\"path\", r.URL.Path,")' ./internal/httpapi/
mutate "the words of a removed version live on in a quote" internal/comments/comments.go \
  's.replace("anchor = anchor - \x27exact\x27 - \x27prefix\x27 - \x27suffix\x27 - \x27offset\x27", "anchor = anchor - \x27offset\x27")' $app -run TestRemovingAVersionBlanksTheQuotesOfItsWords
mutate_web "people share what the browser keeps" src/personal.ts \
  's.replace("`${PERSONAL_PREFIX}${person}.${name}`", "`${PERSONAL_PREFIX}${name}`")' src/personal.test.ts
mutate_web "logging out leaves personal data" src/personal.ts \
  's.replace(".filter((key) => key.startsWith(PERSONAL_PREFIX) && !key.includes(UNSAVED_PART))\n    .forEach((key) => localStorage.removeItem(key));\n  sessionStorage.clear();", ".filter((key) => key.startsWith(\"nothing\") && !key.includes(UNSAVED_PART))\n    .forEach((key) => localStorage.removeItem(key));")' src/personal.test.ts
mutate_web "a comment draft stays for ever" src/personal.ts \
  's.replace("draft !== null && now - draft.saved < DRAFT_KEEP_MS", "draft !== null")' src/personal.test.ts

mutate "the example space ignores its setting" internal/app/app.go \
  's.replace("if a.cfg.ExampleSpace {", "if a.example != nil {")' $app -run TestTheSettingTurnsTheExampleOff
mutate "a deleted example comes back" internal/example/example.go \
  's.replace("if err != nil || offered {", "if err != nil || (offered && len(offeredMark) < 0) {")' $app -run TestADeletedExampleStaysGone
mutate "an instance with spaces gets an example" internal/example/example.go \
  's.replace("if empty {", "if empty || len(offeredMark) > 0 {")' $app -run TestAnInstanceWithSpaces
mutate "any member can add the example space" internal/httpapi/handlers_reading.go \
  's.replace("Permission(identity.PermAdministrator), s.handleExampleSpace", "Permission(identity.PermUseProduct), s.handleExampleSpace")' $app -run TestAnAdministratorCanAddTheExampleLater

mutate "changes in a mail are not escaped" internal/document/changes.go \
  's.replace("text := html.EscapeString(w.Text)", "text := w.Text")' ./internal/document/
mutate "added text in a mail is not escaped" internal/document/changes.go \
  's.replace("html.EscapeString(text) + `</`", "text + `</`")' ./internal/document/

mutate "a mail shows changes without the setting" internal/notify/render.go \
  's.replace("if !allowed || i.Type != TypePageChanged || i.firstVersion <= 1 {", "if i.Type != TypePageChanged || i.firstVersion <= 1 {")' $app -run TestAMailShowsNoChanges
mutate "a mail of changes reads through access" internal/notify/list.go \
  's.replace("JOIN perm_visible_pages($1) v ON v.page_id = n.page_id\n\tJOIN pages p", "JOIN (SELECT $1::uuid AS u) v ON true\n\tJOIN pages p")' $app -run TestAMailAboutAChangeGoesNowhere

mutate "a move leaves comments behind" internal/pages/move.go \
  's.replace("var tablesWithSpace = []string{\"comments\", ", "var tablesWithSpace = []string{")' $app -run TestAMoveTakesEveryRowOfTheBranchAlong
mutate "a move leaves the trash behind" internal/pages/move.go \
  's.replace("OR object_id IN (SELECT id FROM comments WHERE page_id = ANY($1))", "")' $app -run TestAMoveTakesEveryRowOfTheBranchAlong
mutate "search shows a thread for editors" internal/db/migrations/0008_organising.sql \
  's.replace("AND (NOT c.editors_only OR c.page_id IN (SELECT e.page_id FROM perm_editable_pages(u) e))", "")' $app -run TestCommentSearchFollowsTheRuleOfAComment
mutate "search shows a comment in the trash" internal/db/migrations/0008_organising.sql \
  's.replace("WHERE c.trash_item_id IS NULL AND NOT c.purged", "WHERE NOT c.purged")' $app -run TestCommentSearchFollowsTheRuleOfAComment
# Migration 0044 holds the latest perm_editable_pages.
mutate "an edit restriction does not count for a thread" internal/db/migrations/0044_database_edit_and_role_names.sql \
  's.replace("WHERE r.kind = \x27edit\x27\n        GROUP BY r.page_id", "WHERE r.kind = \x27none\x27\n        GROUP BY r.page_id")' $app -run TestCommentVisibilityAgreesWithTheCheckOnOneComment
mutate "a reaction works on an archived page" internal/permissions/evaluate.go \
  's.replace("case ActionPageView, ActionApprovalRead, SpaceExport, SpaceAdminister, ActionPageDelete, PageArchive, RestrictionManage, AnalyticsView:", "case ActionPageView, ActionApprovalRead, ActionReact, SpaceExport, SpaceAdminister, ActionPageDelete, PageArchive, RestrictionManage, AnalyticsView:")' $app -run TestAReactionChangesNothingOnAnArchivedPageOrInAnArchivedSpace
mutate "a removed version lives on in a quote" internal/pages/versions.go \
  's.replace("if err := comments.BlankQuotes(ctx, tx, c, id, removed, current); err != nil {", "_ = removed\n\t\tif err := comments.BlankQuotes(ctx, tx, c, id, current, current); err != nil {")' $app -run TestRemovingAVersionBlanksTheQuotesThatHoldIt
mutate "nodes hand out rights at the same time" internal/db/db.go \
  's.replace("\"SELECT pg_advisory_xact_lock($1)\", migrationLockKey", "\"SELECT $1::bigint\", migrationLockKey")' ./internal/db/ -count=5 -run TestGrantApp

mutate "an export holds a hidden page" internal/pages/export.go \
  's.replace("JOIN perm_visible_pages($1) vis ON vis.page_id = p.id", "JOIN (SELECT id AS page_id FROM pages WHERE $1::uuid IS NOT NULL) vis ON vis.page_id = p.id")' $app -run TestExportLeavesOutWhatThePersonCannotSee
mutate "anyone can fetch an export" internal/exchange/export.go \
  's.replace("FROM export_runs WHERE id = $1 AND created_by = $2`, id, c.UserID", "FROM export_runs WHERE id = $1 AND $2::uuid IS NOT NULL`, id, c.UserID")' $app -run TestOnlyTheOwnerDownloadsAnExport
mutate "a file stays open after a page is hidden" internal/exchange/export.go \
  's.replace("if hidden > 0 {", "if hidden < 0 {")' $app -run TestAFileIsRefusedOnceAPageInItIsHidden
mutate "restricted pages go along unasked" internal/exchange/export.go \
  's.replace("if restricted[p.ID] {", "if restricted[p.ID] && false {")' $app -run TestRestrictedPagesStayOutUntilChosen
mutate "a format that is off still works" internal/exchange/policy.go \
  's.replace("if err != nil || !r.Enabled {", "if err != nil {")' $app -run TestAFormatCanBeTurnedOffOrLimitedToAGroup
mutate "a bundle with a wrong signature is accepted" internal/exchange/bundlein.go \
  's.replace("|| !ed25519.Verify(public, raw, sig)", "|| len(sig) == 0")' $app -run TestOneChangedByteStopsABundle
mutate "a changed file in a bundle is accepted" internal/exchange/bundlein.go \
  's.replace("hex.EncodeToString(sum.Sum(nil)) != e.SHA256", "hex.EncodeToString(sum.Sum(nil)) == \"\"")' $app -run TestOneChangedByteStopsABundle
mutate "an unknown signer is trusted" internal/exchange/bundlein.go \
  's.replace("return apperr.Conflict(\"unknown_signer\", \"Enter the fingerprint of the instance this bundle comes from.\")", "return nil")' $app -run TestAnUnknownSignerNeedsAFingerprint
mutate "an unmatched restriction opens a page" internal/exchange/bundlein.go \
  's.replace("subjectType, subject = permissions.SubjectUser, in.c.UserID\n", "if !ok {\n\t\t\t\tcontinue\n\t\t\t}\n")' $app -run TestAnUnmatchedRestrictionStaysClosed
mutate "a bundle skips a page it cannot see" internal/exchange/bundle.go \
  's.replace("if hidden > 0 {", "if hidden < 0 {")' $app -run TestABundleStopsOnAPageThePersonCannotSee
mutate "undo removes an edited page" internal/exchange/import.go \
  's.replace("p.current_version <> i.version\n", "false\n")' $app -run TestUndoNeverRemovesSomeonesEdit
mutate "anyone can see an import" internal/exchange/import.go \
  's.replace("FROM import_runs WHERE id = $1 AND created_by = $2`, id, c.UserID", "FROM import_runs WHERE id = $1 AND $2::uuid IS NOT NULL`, id, c.UserID")' $app -run TestOnlyTheOwnerSeesAnImport
mutate "a script in imported HTML survives" internal/exchange/htmlimport.go \
  's.replace("atom.Script: true, ", "")' $app -run TestHTMLThroughTheAllowListKeepsNoScript
mutate "an external relationship is followed" internal/office/read.go \
  's.replace("if !ok || rel.external {", "if !ok {")' ./internal/office/ -run TestReadDOCXNeverFollowsAnExternalImage
mutate "a restricted page can be public" internal/sites/rule.go \
  's.replace("AND NOT EXISTS (SELECT 1 FROM page_tree t JOIN page_restrictions r ON r.page_id = t.ancestor_id AND r.kind = '"'"'view'"'"'\n\t\t\tWHERE t.descendant_id = p.id)", "")' $app -run TestARestrictedPageCannotBeChosen
mutate "an event does not reach the public rule" internal/sites/rule.go \
  's.replace("if slices.Contains(eventsThatCanHide, m.Type) {", "if slices.Contains(eventsThatCanHide, m.Type+\"x\") {")' $app -run TestEveryEventThatCanHideAPageTakesItOffTheSite
mutate "a removed version stays public" internal/sites/rule.go \
  's.replace("if len(allowed) == len(shown) && !gone {", "if len(allowed) == len(shown) && (!gone || true) {")' $app -run TestRemovingTheShownVersionTakesItOffTheSite
mutate "a photo keeps its metadata" internal/sites/build.go \
  's.replace("cleaned, ok := withoutMetadata(data, ext)", "cleaned, ok := data, true")' $app -run TestOnlyShownFilesGoAlongAndPhotosLoseTheirMetadata
mutate "the scrub is off" internal/sites/build.go \
  's.replace("if err := s.scrub(ctx, dir); err != nil {\n\t\t_ = os.RemoveAll(dir)", "if err := error(nil); err != nil {\n\t\t_ = os.RemoveAll(dir)")' $app -run TestTheScrubStopsABuildAndKeepsTheOldSite
mutate "a mention prints a name on a site" internal/sites/build.go \
  's.replace("if !b.v.ShowNames {\n\t\toptions.Mention", "if false {\n\t\toptions.Mention")' $app -run TestAMentionIsNeutralUnlessTheSiteShowsNames
mutate "a public link survives a restriction" internal/sites/links.go \
  's.replace("if allowed[l.page] && !gone {", "if true || (allowed[l.page] && !gone) {")' $app -run TestARestrictionWithdrawsAPublicLinkAtOnce
mutate "a block shows a hidden page" internal/blocks/lists.go \
  's.replace("JOIN perm_visible_pages($1) v ON v.page_id = p.id\n\tJOIN spaces s ON s.id = p.space_id\n\tLEFT JOIN users u ON u.id = p.updated_by\n\tLEFT JOIN page_tree t", "JOIN (SELECT $1::uuid AS u) v ON true\n\tJOIN spaces s ON s.id = p.space_id\n\tLEFT JOIN users u ON u.id = p.updated_by\n\tLEFT JOIN page_tree t")' $app -run TestNoBlockShowsAHiddenPage
mutate "an include skips the check at a level" internal/blocks/reuse.go \
  's.replace("if ok, err := r.visible(ctx, in.Page); !ok {\n\t\treturn nil, nil, err\n\t}\n", "")' $app -run TestIncludesCheckEveryLevelAndStopOnALoop
mutate "an include has no depth limit" internal/blocks/reuse.go \
  's.replace("if depth > maxDepth {", "if false {")' $app -run TestIncludesCheckEveryLevelAndStopOnALoop
mutate "an include runs in a loop" internal/blocks/reuse.go \
  's.replace("if seen[in.Page] {", "if false {")' $app -run TestIncludesCheckEveryLevelAndStopOnALoop
mutate "HTML passes as a video" internal/httpapi/handlers_content.go \
  's.replace("attachments.IsMedia(a.ContentType)", "true")' $app -run TestAnHTMLFileNamedMp4StaysADownload
mutate "anyone reads the storage of a space" internal/attachments/versions.go \
  's.replace("if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {\n\t\treturn Usage{}, err\n\t}\n", "")' $app -run TestFilesNeverLeakAcrossARestriction
mutate "an editor removes an old version" internal/attachments/versions.go \
  's.replace("permissions.AttachmentDeleteAny", "permissions.AttachmentAdd")' $app -run TestFileVersionsCanBeRestoredAndRemovedByThoseWhoMay
mutate "the drawing service shares our origin" internal/config/config.go \
  's.replace("if public != nil && strings.EqualFold(u.Scheme, public.Scheme)", "if false && strings.EqualFold(u.Scheme, public.Scheme)")' ./internal/config/ -run TestTheDrawingServiceIsOptionalAndNeverOurOwnOrigin
mutate "any address may be framed" internal/webui/webui.go \
  's.replace("frames += \" \" + drawingOrigin", "frames += \" *\"")' ./internal/webui/ -run TestOnlyOurViewersAndTheDrawingServiceMayBeFramed
mutate "a viewer may reach our origin" internal/webui/webui.go \
  's.replace("sandbox allow-scripts allow-downloads", "sandbox allow-scripts allow-downloads allow-same-origin")' ./internal/webui/ -run TestAViewerRunsInASandboxThatCannotReachTheSession
mutate "the whole app may run WebAssembly" internal/webui/webui.go \
  's.replace("return \"default-src \x27none\x27; script-src \x27self\x27; style-src", "return \"default-src \x27none\x27; script-src \x27self\x27 \x27wasm-unsafe-eval\x27; style-src")' ./internal/webui/ -run TestOnlyTheViewersGetTheLoosePolicy
mutate "a normal page holds a sheet" internal/pages/kinds.go \
  's.replace("if kind != KindDatabase && document.HasSheet(doc) {", "if false {")' $app -run TestADocumentMustFitItsKind
mutate "a database holds anything" internal/pages/kinds.go \
  's.replace("if kind == KindDatabase && !document.IsSheetDocument(doc) {", "if false {")' $app -run TestADocumentMustFitItsKind
mutate "a link cell skips the link gate" internal/document/sheet.go \
  's.replace("href, err := SafeHref(s)", "href, err := s, error(nil)")' ./internal/document/ -run TestASheetThatBreaksARuleIsRefused
mutate_web "a CSV cell may start a formula" src/components/sheet/csv.ts \
  's.replace("DANGEROUS_START.test(value) && !PLAIN_NUMBER.test(value)", "false")' src/components/sheet/sheet.test.ts
mutate_web "a formula leaves in a CSV" src/components/sheet/csv.ts \
  's.replace("csvField(read(r, c))", "csvField(sheet.rows[r]?.cells[sheet.columns[c]?.id ?? \"\"] ?? \"\")")' src/components/sheet/sheet.test.ts
mutate "the policy blocks video and audio" internal/webui/webui.go \
  's.replace("media-src \x27self\x27; ", "")' ./internal/webui/ -run TestVideoAndAudioFromOurOwnOriginMayPlay

mutate "names survive a stricter mode" internal/analytics/analytics.go \
  's.replace("if v.Mode != ModeNames {", "if len(v.Mode) < 0 {")' $app -run TestLeavingNameModeDeletesNamesForGoodAndIsAudited
mutate "a report shows a hidden page" internal/analytics/reports.go \
  's.replace("FROM pages pg JOIN perm_visible_pages($1) v ON v.page_id = pg.id\n\t\tWHERE pg.space_id = $2 AND pg.status = \x27published\x27\n\t\tORDER BY 8", "FROM pages pg, (SELECT $1::uuid) v\n\t\tWHERE pg.space_id = $2 AND pg.status = \x27published\x27\n\t\tORDER BY 8")' $app -run TestReportsLeaveOutHiddenPages
mutate "own views are counted" internal/analytics/analytics.go \
  's.replace("|| f.owner == c.UserID {", "{")' $app -run TestOwnPageIsNotCountedAndAPersonCountsOncePerHalfHour
mutate "a rule skips the check of its account" internal/automation/run.go \
  's.replace("identity.Caller{UserID: r.rule.AccountID,", "identity.Caller{UserID: r.rule.OwnerID,")' $app -run TestARuleCannotDoMoreThanItsAccount
mutate "a rule triggers itself" internal/automation/match.go \
  's.replace("case slices.Contains(rc.Chain, r.ID):", "case len(rc.Chain) < 0:")' $app -run TestARuleCannotTriggerItself
mutate "a chain runs on" internal/automation/match.go \
  's.replace("case len(rc.Chain) >= maxChain:", "case len(rc.Chain) < 0:")' $app -run TestAChainOfRulesStopsAtFive
mutate "a redirect is followed" internal/outward/outward.go \
  's.replace("CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },", "")' ./internal/outward/ -run TestARedirectIsNotFollowed
mutate "a private address is allowed" internal/outward/outward.go \
  's.replace("if err != nil || (!o.AllowPrivate && !isPublic(a)) {", "if err != nil || (!o.AllowPrivate && len(a.String()) < 0) {")' ./internal/outward/ -run TestAConnectionToAPrivateAddressIsRefusedAtConnectTime
mutate "a delivery goes unsigned" internal/webhooks/webhooks.go \
  's.replace("\"webhook-signature\": signature", "\"webhook-signature\": \"\"+signature[:0]")' $app -run TestAWebhookIsSignedAndHoldsNoText
mutate "a hook token is not checked" internal/automation/match.go \
  's.replace("WHERE token_hash = $1\", hashToken(token)", "WHERE token_hash <> $1\", hashToken(token)")' $app -run TestAnIncomingHookNeedsItsToken
mutate "a scope is not checked" internal/httpapi/server.go \
  's.replace("if slices.Contains(p.Scopes, scope) {", "if len(p.Scopes) >= 0 {")' $app -run TestAScopeLimitsATokenAndATokenNeverExceedsItsOwner
mutate "a CSV cell goes unguarded" internal/csvout/csvout.go \
  's.replace("safe[i] = Cell(c)", "safe[i] = c")' ./internal/csvout/ -run TestRowsAreGuardedAndQuoted
mutate "a message reaches someone who cannot see the page" internal/automation/actions.go \
  's.replace("if !allowed {\n\t\t\tcontinue", "if !allowed && len(people) < 0 {\n\t\t\tcontinue")' $app -run TestAMessageReachesNobodyWhoCannotSeeEveryPageItRefersTo
mutate "a call skips the list of hosts" internal/outward/outward.go \
  's.replace("if !allowed {", "if !allowed && len(address) < 0 {")' $app -run TestARuleCallsOutThroughTheAllowedList
mutate "an event about a lost page is sent" internal/webhooks/webhooks.go \
  's.replace("if !allowed.Allowed {", "if !allowed.Allowed && len(data) < 0 {")' $app -run TestAPageTheOwnerLostIsNotSent
mutate "a reader joins the shared document" internal/collab/collab.go \
  's.replace("pageID, permissions.PageEdit, uuid.Nil)", "pageID, permissions.ActionPageView, uuid.Nil)")' $app -run TestOnlySomeoneWhoMayEditCanConnect
mutate "a stored document skips the gate" internal/collab/collab.go \
  's.replace("if _, err := document.Normalize(in.Document); err != nil {", "if _, err := document.Normalize(in.Document); err != nil && len(in.State) < 0 {")' $app -run TestAHostileDocumentIsNotStored
mutate "the service token is not checked" internal/httpapi/handlers_collab.go \
  's.replace("if !found || !s.deps.Collab.TrustsToken(token) {", "if !found && !s.deps.Collab.TrustsToken(token) {")' $app -run TestTheServiceRoutesNeedTheToken
mutate "a socket comes from another site" internal/httpapi/handlers_collab.go \
  's.replace("if r.Header.Get(\"Origin\") != origin {", "if r.Header.Get(\"Origin\") != origin && len(origin) < 0 {")' $app -run TestASocketFromAnotherSiteIsRefused
mutate "the service is not told of a change of rights" internal/collab/collab.go \
  's.replace("|| strings.HasPrefix(eventType, \"restriction.\")", "")' $app -run TestAChangeOfRightsMakesTheServiceCheckAgain
mutate "a failed event comes back for ever" internal/queue/queue.go \
  's.replace("if tries <= len(q.retryWaits) {", "if tries > 0 {").replace("q.retryWaits[tries-1]", "q.retryWaits[0]")' ./internal/queue/ -run TestABadEventStopsComingBackAndIsHandedOver
mutate "a replay needs no administrator" internal/httpapi/handlers_console.go \
  's.replace("\"POST /api/v1/admin/event-failures/{id}/replay\", SessionOnly(admin)", "\"POST /api/v1/admin/event-failures/{id}/replay\", Authenticated()")' $app -run TestAnEventGivenUpOnIsShownAndCanBeReplayed
mutate "metrics take any token" internal/httpapi/handlers_metrics.go \
  's.replace("if !found || !sameSecret(token, configured) {", "if !found && !sameSecret(token, configured) {")' $app -run TestMetricsNeedTheRightToken
mutate "metrics exist without a token" internal/httpapi/handlers_metrics.go \
  's.replace("if configured == \"\" {", "if configured == \"never\" {")' $app -run TestMetricsDoNotExistWithoutAToken
mutate "a file is shared with other caches" internal/httpapi/handlers_content.go \
  's.replace("\"private, no-cache\"", "\"public, max-age=3600\"")' $app -run TestAFileAnswersNotModifiedButStillChecksAccess
mutate_web "offline text stays after logout" src/personal.ts \
  's.replace("localStorage.getItem(unsavedMarkOf(name)) === null &&", "localStorage.getItem(unsavedMarkOf(name)) === \"never\" &&")' src/personal.test.ts
mutate_web "unsaved work goes at logout" src/personal.ts \
  's.replace(" && !key.includes(UNSAVED_PART))\n    .forEach", ")\n    .forEach")' src/personal.test.ts
mutate "a move skips add on the target" internal/attachments/moves.go \
  's.replace("c.UserID, target, permissions.AttachmentAdd, uuid.Nil); err != nil {\n\t\t\treturn err", "c.UserID, target, permissions.ActionPageView, uuid.Nil); err != nil {\n\t\t\treturn err")' $app -run TestMovingAFileNeedsDeleteOnTheSourceAndAddOnTheTarget
mutate "a copy skips add on the target" internal/attachments/moves.go \
  's.replace("c.UserID, target, permissions.AttachmentAdd, uuid.Nil); err != nil {\n\t\treturn Attachment{}, err", "c.UserID, target, permissions.ActionPageView, uuid.Nil); err != nil {\n\t\treturn Attachment{}, err")' $app -run TestACopyDoesNotTellWhatAPageItMayNotChangeHolds
mutate "a move overwrites a file with the same name" internal/attachments/moves.go \
  's.replace("if taken {\n\t\t\treturn errNameTaken()", "if taken && len(a.Filename) < 0 {\n\t\t\treturn errNameTaken()")' $app -run TestAMoveDoesNotOverwriteAFileWithTheSameName
mutate "a blocked type looks at the name only" internal/attachments/kinds.go \
  's.replace("found := []string{extension, detectedExtensions[contentType]}", "found := []string{extension}")' $app -run TestABlockedFileTypeIsRefusedByNameAndByContent
mutate "a program named as a picture gets through" internal/attachments/kinds.go \
  's.replace("if bytes.HasPrefix(head, windowsProgramMark) {", "if bytes.HasPrefix(head, windowsProgramMark) && len(name) < 0 {")' $app -run TestABlockedFileTypeIsRefusedByNameAndByContent
mutate "a rename gets around a blocked type" internal/attachments/versions.go \
  's.replace("if isBlocked(blocked, name, \"\", nil) {", "if isBlocked(blocked, name, \"\", nil) && len(name) < 0 {")' $app -run TestRenamingCannotGetAroundABlockedType
mutate "a hidden page counts as reachable" internal/pages/details.go \
  's.replace("SELECT v.page_id FROM perm_visible_pages($1) v WHERE v.page_id = ANY($2)", "SELECT p.id FROM pages p WHERE $1::uuid IS NOT NULL AND p.id = ANY($2) AND p.status = \x27published\x27")' $app -run TestAHiddenPageAndADeletedPageLookTheSameToAReader
mutate "pasted Markdown has no limit" internal/httpapi/handlers_organising.go \
  's.replace("if len(in.Markdown) > maxPastedMarkdown {", "if len(in.Markdown) > maxPastedMarkdown && len(in.Markdown) < 0 {")' $app -run TestPastedMarkdownHasALimit
mutate "a text file goes to the office helper" internal/httpapi/handlers_files.go \
  's.replace("if !attachments.IsOffice(a.Filename) || !s.deps.Exchange.OfficeAvailable() {", "if !s.deps.Exchange.OfficeAvailable() {")' $app -run TestAnOfficeFileOpensAsPDFThroughTheHelper
mutate "the office helper hears the file name" internal/exchange/office.go \
  's.replace("officeDocumentName+path.Ext(filename)", "filename+path.Ext(filename)")' $app -run TestAnOfficeFileOpensAsPDFThroughTheHelper
mutate "a converted file loses its locked headers" internal/httpapi/handlers_files.go \
  's.replace("h.Set(\"Content-Security-Policy\", \"default-src \x27none\x27; sandbox\")\n\t\th.Set(\"X-Content-Type-Options\", \"nosniff\")\n\t\th.Set(\"Cache-Control\", \"private, no-store\")\n\t\th.Set(\"Content-Type\", \"application/octet-stream\")\n\t\th.Set(\"Content-Disposition\", attachmentHeader(a.Filename+\".pdf\"))", "h.Set(\"Content-Disposition\", attachmentHeader(a.Filename+\".pdf\"))")' $app -run TestAnOfficeFileOpensAsPDFThroughTheHelper
mutate "a member without the role asks for an invitation" internal/sharing/invitations.go \
  's.replace("if !mayInvite {\n\t\t\treturn errMayNotInvite", "if !mayInvite \x26\x26 len(in.Email) < 0 {\n\t\t\treturn errMayNotInvite")' $app -run TestInvitationMatrix
mutate "a space manager without the role asks for a guest" internal/sharing/invitations.go \
  's.replace("if err := s.requireGlobal(ctx, c, identity.PermInvitePeople); err != nil {\n\t\treturn uuid.Nil, err", "if err := s.requireGlobal(ctx, c, identity.PermInvitePeople); err != nil \x26\x26 len(in.Email) < 0 {\n\t\treturn uuid.Nil, err")' $app -run TestASpaceManagerOnlyAsksForAGuest
mutate "a space manager invites a guest without approval" internal/httpapi/handlers_people.go \
  's.replace("if !administrator {", "if !administrator \x26\x26 len(in.Email) < 0 {")' $app -run TestASpaceManagerOnlyAsksForAGuest
mutate "asking sends the invitation at once" internal/sharing/invitations.go \
  's.replace("return recordInvitation(ctx, tx, c, \"invitation.requested\", r)", "if _, err := s.identity.InviteWithin(ctx, tx, c, identity.NewInvitation{Email: address, Type: identity.TypeGuest, SpaceID: \x26r.spaceID}); err != nil {\n\t\treturn err\n\t}\n\treturn recordInvitation(ctx, tx, c, \"invitation.requested\", r)")' $app -run TestAskingForAnInvitationSendsNothing
mutate "someone who is no administrator answers" internal/sharing/invitations.go \
  's.replace("if err := s.requireGlobal(ctx, c, identity.PermAdministrator); err != nil {\n\t\treturn \"\", err", "if err := s.requireGlobal(ctx, c, identity.PermAdministrator); err != nil \x26\x26 len(reason) < 0 {\n\t\treturn \"\", err")' $app -run TestInvitationMatrix
mutate "someone who is no administrator lists requests" internal/sharing/invitations.go \
  's.replace("if err := s.requireGlobal(ctx, c, identity.PermAdministrator); err != nil {\n\t\treturn nil, err", "if err := s.requireGlobal(ctx, c, identity.PermAdministrator); err != nil \x26\x26 c.System != \"\" {\n\t\treturn nil, err")' $app -run TestInvitationMatrix
mutate "a refused invitation is approved after all" internal/sharing/invitations.go \
  's.replace("if status != invitationWaiting {", "if status != invitationWaiting \x26\x26 status == \"\" {")' $app -run TestARefusalSendsNothingAndStays
mutate "an answer needs no reason" internal/sharing/invitations.go \
  's.replace("if reason == \"\" || !utf8.ValidString(reason)", "if !utf8.ValidString(reason)")' $app -run TestAnAnswerNeedsAReason
mutate "an approval goes through after the role is lost" internal/sharing/invitations.go \
  's.replace("if !mayInvite {\n\t\t\treturn apperr.Conflict(\"requester_lost_role\"", "if !mayInvite \x26\x26 r.email == \"\" {\n\t\t\treturn apperr.Conflict(\"requester_lost_role\"")' $app -run TestApprovalFailsWhenTheRequesterLostTheRole
mutate "an approval goes through while invitations are off" internal/sharing/invitations.go \
  's.replace("r, err := lockWaitingInvitation(ctx, tx, id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif err := s.requireInvitationsOn(ctx, tx); err != nil {\n\t\t\treturn err\n\t\t}", "r, err := lockWaitingInvitation(ctx, tx, id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}")' $app -run TestRequestsWaitOnlyWhileInvitationsAreOn
mutate "the invitation makes a member, not a guest" internal/sharing/invitations.go \
  's.replace("identity.NewInvitation{Email: r.email, Type: identity.TypeGuest, SpaceID: \x26r.spaceID})\n\t\treturn err", "identity.NewInvitation{Email: r.email, Type: identity.TypeMember, SpaceID: \x26r.spaceID})\n\t\treturn err")' $app -run TestAnApprovedInvitationGoesThroughTheQueueWithoutTheContent
mutate "the requester never hears the answer" internal/sharing/invitations.go \
  's.replace("return s.notify.Deliver(ctx, tx, notify.Row{UserID: r.requester,", "return s.notify.Deliver(ctx, tx, notify.Row{UserID: c.UserID,")' $app -run TestTheRequesterHearsTheAnswerAndTheReason
mutate "the console shows the page title" internal/sharing/invitations.go \
  's.replace("SELECT r.id, r.email, u.display_name, r.space_id, sp.name, r.note, r.created_at\n\t\tFROM invitation_requests r\n\t\tJOIN users u ON u.id = r.requester_id", "SELECT r.id, r.email, u.display_name, r.space_id, sp.name, r.note || (SELECT title FROM pages WHERE id = r.page_id), r.created_at\n\t\tFROM invitation_requests r\n\t\tJOIN users u ON u.id = r.requester_id")' $app -run TestTheConsoleShowsTheSpaceAndNeverThePage
mutate "two answers on a page share a line" internal/notify/notify.go \
  's.replace("if r.Type == TypeInvitationAnswer {", "if r.Type == \"\" {")' $app -run TestTheRequesterHearsTheAnswerAndTheReason
mutate "link facts show a hidden page" internal/pages/links.go \
  's.replace("JOIN perm_visible_pages($1) v ON v.page_id = p.id\n\t\tJOIN spaces sp", "JOIN perm_visible_pages($1) v ON true\n\t\tJOIN spaces sp")' $app -run TestLinkFactsLeaveOutWhatTheReaderMayNotSee
mutate "a placeholder leads to a page the reader may not see" internal/pages/links.go \
  's.replace("JOIN perm_visible_pages($1) v ON v.page_id = h.target_id", "JOIN perm_visible_pages($1) v ON true")' $app -run TestAPlaceholderOnlyLeadsToAPageTheReaderMaySee
mutate "following a placeholder needs no right to create" internal/pages/links.go \
  's.replace("if err := s.checkParent(ctx, tx, c, placement{spaceID: p.SpaceID, parent: source, kind: KindPage}); err != nil {", "if err := s.checkParent(ctx, tx, c, placement{spaceID: p.SpaceID, parent: source, kind: KindPage}); err != nil \x26\x26 title == \"\" {")' $app -run TestFollowingAPlaceholderNeedsTheRightToCreate
mutate "following gives the id of a page someone may not see" internal/pages/links.go \
  's.replace("return s.requireStarted(ctx, tx, c, *target)", "return nil")' $app -run TestSomeoneWhoCannotSeeTheStartedPageGetsAConflict
mutate "any title on a page can be followed" internal/pages/links.go \
  's.replace("if errors.Is(err, pgx.ErrNoRows) {\n\t\t\treturn apperr.NotFound()\n\t\t}\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif target != nil {", "if err != nil \x26\x26 !errors.Is(err, pgx.ErrNoRows) {\n\t\t\treturn err\n\t\t}\n\t\tif target != nil {")' $app -run TestOnlyAPlaceholderOnThePageCanBeFollowed
mutate "the placeholder list shows a hidden page" internal/pages/links.go \
  's.replace("JOIN pages p ON p.id = h.source_id `+refJoins+`", "JOIN pages p ON p.id = h.source_id JOIN spaces s ON s.id = p.space_id CROSS JOIN (SELECT $1::uuid) AS caller")' $app -run TestThePlaceholderListSkipsAPageTheAdministratorMayNotSee
mutate "an export names a hidden page" internal/pages/links.go \
  's.replace("facts, err := s.LinkFacts(ctx, c, document.LinkedPages(doc))", "facts, err := s.LinkFacts(ctx, identity.Caller{UserID: s.anyOwner(ctx, doc)}, document.LinkedPages(doc))\n\t_ = c").concat("\nfunc (s *Service) anyOwner(ctx context.Context, doc document.Node) uuid.UUID {\n\tvar id uuid.UUID\n\tfor _, p := range document.LinkedPages(doc) {\n\t\t_ = s.pool.QueryRow(ctx, \"SELECT created_by FROM pages WHERE id = $1\", p).Scan(\x26id)\n\t}\n\treturn id\n}\n")' $app -run TestAnExportNamesOnlyWhatTheExporterMaySee
mutate "resolving keeps the id of a hidden page" internal/document/links.go \
  's.replace("return Node{Type: TypeText, Text: LinkedPageWords}", "return Node{Type: TypeText, Text: id.String()}")' ./internal/document/
mutate "a site names a page that is not on it" internal/sites/build.go \
  's.replace("if _, ok := b.links[id]; !ok {\n\t\treturn \"\", false\n\t}\n\tfor _, p := range b.list {\n\t\tif p.page.ID == id {\n\t\t\treturn p.page.Title, true\n\t\t}\n\t}\n\treturn \"\", false", "var title string\n\t_ = b.s.pool.QueryRow(context.Background(), \"SELECT title FROM pages WHERE id = $1\", id).Scan(\x26title)\n\treturn title, true")' $app -run TestALinkThatFollowsItsPageNamesOnlyPagesOnTheSite
mutate "a linked title is stored in the search index" internal/document/document.go \
  's.replace("TypePlaceholderLink: \"title\"}[n.Type]", "TypePlaceholderLink: \"title\", TypePageLink: \"page_id\"}[n.Type]")' ./internal/document/
mutate "a fragment may hold markup" internal/document/links.go \
  's.replace("len(fragment) <= maxFragmentBytes \x26\x26 fragmentPattern.MatchString(fragment)", "fragment != \"\"")' ./internal/document/
mutate "a table may stay inside a table" internal/document/document.go \
  's.replace("\tliftNestedTables(\x26n, false)\n", "")' ./internal/document/
mutate "one version of a file skips the check" internal/attachments/details.go \
  's.replace("a, err := s.viewable(ctx, c, id)\n\tif err != nil {\n\t\treturn a, nil, err\n\t}\n\tvar key string", "a, err := get(ctx, s.pool, id)\n\tif err != nil {\n\t\treturn a, nil, err\n\t}\n\tvar key string")' $app -run TestOneVersionOfAFileHasItsOwnAddressThatGivesNoAccess
mutate "a reader changes the labels of a file" internal/attachments/details.go \
  's.replace("return a, s.perms.RequirePage(ctx, tx, c.UserID, a.PageID, permissions.AttachmentAdd, uuid.Nil)", "return a, nil")' $app -run TestFileLabelsFollowTheRulesOfLabels
mutate "a file label gets around the fixed list" internal/attachments/details.go \
  's.replace("if refused {", "if refused \x26\x26 raw == \"\" {")' $app -run TestFileLabelsFollowTheRulesOfLabels
mutate "search finds a page by the label of a trashed file" internal/search/words.go \
  's.replace("fa.status = \x27active\x27 AND ", "")' $app -run TestSearchFindsAPageByTheLabelOfItsFileAndOnlyWhatMayBeSeen
mutate "a filter alone finds a page by the label of a trashed file" internal/search/search.go \
  's.replace("fa.status = \x27active\x27 AND ", "")' $app -run TestSearchFindsAPageByTheLabelOfItsFileAndOnlyWhatMayBeSeen
mutate "the Files screen shows what is hidden" internal/attachments/versions.go \
  's.replace("JOIN perm_visible_pages($1) vp ON vp.page_id = a.page_id", "JOIN (SELECT $1::uuid AS u) vp ON true")' $app -run TestTheFilesScreenFiltersAndOffersOnlyWhatMayBeSeen
mutate "anyone deletes files in bulk" internal/attachments/details.go \
  's.replace("permissions.SpaceAdminister); err != nil {\n\t\treturn Outcome{}, err", "permissions.SpaceAdminister); err != nil \x26\x26 len(list) < 0 {\n\t\treturn Outcome{}, err")' $app -run TestBulkDeleteTakesOnlyWhatTheAdministratorMayReachInThatSpace
mutate "bulk delete ignores the page" internal/attachments/details.go \
  's.replace("err = s.perms.RequirePage(ctx, tx, c.UserID, a.PageID, permissions.ActionAttachmentDelete, a.CreatedBy)", "err = nil")' $app -run TestBulkDeleteTakesOnlyWhatTheAdministratorMayReachInThatSpace
mutate "bulk delete reaches another space" internal/attachments/details.go \
  's.replace("return a.SpaceID == spaceID", "return a.SpaceID != uuid.Nil")' $app -run TestBulkDeleteTakesOnlyWhatTheAdministratorMayReachInThatSpace
mutate "a space administrator raises their own limit" internal/httpapi/handlers_files.go \
  's.replace("SessionOnly(Permission(identity.PermAdministrator)), s.handleSpaceStorageLimit", "Object(\"space.administer\"), s.handleSpaceStorageLimit")' $app -run TestOnlyAnAdministratorSetsTheLimitOfASpace
mutate "a full space takes a moved file" internal/attachments/limits.go \
  's.replace("if limited \x26\x26 size > left {", "if limited \x26\x26 size > left \x26\x26 size < 0 {")' $app -run TestAFullSpaceRefusesEveryWayInAndLeavesNothingBehind
mutate "a blocked name comes in on a tile" internal/attachments/attachments.go \
  's.replace("[]string{current.Filename, name}", "[]string{current.Filename, name[:0]}")' $app -run TestADroppedFileCannotComeInUnderTheNameOfAnAllowedOne
mutate "clearing up takes a file of an old version" internal/attachments/details.go \
  's.replace("FROM attachment_refs r WHERE r.attachment_id = a.id)\n", "FROM attachment_refs r WHERE false AND r.attachment_id = a.id)\n")' $app -run TestOnlyFilesNothingHoldsAreOfferedForClearingUp
mutate "clearing up takes a file of a draft" internal/attachments/details.go \
  's.replace("FROM drafts d WHERE d.page_id", "FROM drafts d WHERE false AND d.page_id")' $app -run TestOnlyFilesNothingHoldsAreOfferedForClearingUp
mutate "clearing up takes a file of a comment" internal/attachments/details.go \
  's.replace("FROM comments m WHERE m.page_id", "FROM comments m WHERE false AND m.page_id")' $app -run TestOnlyFilesNothingHoldsAreOfferedForClearingUp
mutate "clearing up takes the header of a page" internal/attachments/details.go \
  's.replace("FROM pages h WHERE h.header_attachment_id", "FROM pages h WHERE false AND h.header_attachment_id")' $app -run TestOnlyFilesNothingHoldsAreOfferedForClearingUp
mutate "clearing up takes an upload on its way" internal/attachments/details.go \
  's.replace("a.created_at < now() - interval \x271 day\x27 AND NOT", "NOT")' $app -run TestOnlyFilesNothingHoldsAreOfferedForClearingUp
mutate "clearing up looks into a hidden page" internal/attachments/details.go \
  's.replace("func (s *Service) ClearUp(ctx context.Context, c identity.Caller, pageID uuid.UUID) (Outcome, error) {\n\tif err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {", "func (s *Service) ClearUp(ctx context.Context, c identity.Caller, pageID uuid.UUID) (Outcome, error) {\n\tif err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil \x26\x26 pageID == uuid.Nil {")' $app -run TestClearingUpOnlyTakesWhatThePersonMayDelete
mutate "the history of files needs no access" internal/attachments/details.go \
  's.replace("func (s *Service) Events(ctx context.Context, c identity.Caller, pageID uuid.UUID) ([]Event, error) {\n\tif err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {", "func (s *Service) Events(ctx context.Context, c identity.Caller, pageID uuid.UUID) ([]Event, error) {\n\tif err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil \x26\x26 pageID == uuid.Nil {")' $app -run TestThePageHistoryTellsWhatHappenedToItsFiles
mutate "the history names the page a file went to" internal/attachments/moves.go \
  's.replace("addEvent(ctx, tx, c, a.PageID, id, a.Filename, eventMovedAway)", "addEvent(ctx, tx, c, a.PageID, id, a.Filename+\" \"+target.String(), eventMovedAway)")' $app -run TestThePageHistoryTellsWhatHappenedToItsFiles
mutate "a label counts through a hidden page" internal/blocks/labels.go \
  's.replace("JOIN perm_visible_pages($1) v ON v.page_id = l.page_id", "JOIN perm_visible_pages($1) v ON true")' $app -run 'TestLabelBlocksCountOnlyWhatAReaderMaySee|TestNoNewBlockShowsAHiddenPage'
mutate "a related label reads a hidden page" internal/blocks/labels.go \
  's.replace("func relatedLabels(ctx context.Context, r reader) (Answer, error) {\n\tid := r.target()\n\tif ok, err := r.visible(ctx, id); !ok {\n\t\treturn Answer{}, err\n\t}", "func relatedLabels(ctx context.Context, r reader) (Answer, error) {\n\tid := r.target()")' $app -run TestLabelBlocksCountOnlyWhatAReaderMaySee
mutate "a file of a hidden page shows in a block" internal/blocks/files.go \
  's.replace("JOIN perm_visible_pages($1) v ON v.page_id = p.id", "JOIN perm_visible_pages($1) v ON true")' $app -run TestSpaceFilesWalkPastARestriction
mutate "everyone sees who watches a page" internal/blocks/people.go \
  's.replace("\tif d.Allowed {\n\t\treturn kinds, nil\n\t}", "\tif d.Allowed || len(kinds) > 0 {\n\t\treturn kinds, nil\n\t}")' $app -run TestWatchersShowOnlyToThoseWhoMaySeeThem
mutate "a comment for editors makes a contributor" internal/blocks/people.go \
  's.replace("WHERE page_id = $1 AND NOT editors_only AND NOT purged", "WHERE page_id = $1 AND NOT purged")' $app -run TestCommentsForEditorsDoNotCountForReaders
mutate "a comment for editors counts in the table" internal/blocks/people.go \
  's.replace("WHERE NOT c.editors_only AND NOT c.purged", "WHERE NOT c.purged")' $app -run TestCommentsForEditorsDoNotCountForReaders
mutate "the contribution table shows a hidden page" internal/blocks/people.go \
  's.replace("JOIN perm_visible_pages($1) v ON v.page_id = t.descendant_id", "JOIN perm_visible_pages($1) v ON true")' $app -run TestNoNewBlockShowsAHiddenPage
mutate "a person card goes around the directory rule" internal/blocks/people.go \
  's.replace("if errors.As(err, &refused) && refused.Kind == apperr.KindNotFound {\n\t\treturn Person{}, false, nil\n\t}", "if errors.As(err, &refused) && refused.Kind == apperr.KindNotFound {\n\t\treturn Person{ID: id, Name: \"stranger\"}, true, nil\n\t}")' $app -run TestAPersonCardShowsWhatTheDirectoryShows
mutate "popular pages show while analytics is off" internal/analytics/reports.go \
  's.replace("\tif err != nil || !st.Enabled {\n\t\treturn []LinkCount{}, err\n\t}", "\tif err != nil {\n\t\treturn []LinkCount{}, err\n\t}\n\t_ = st")' $app -run TestPopularPagesStayQuietWhileAnalyticsIsOff
mutate "popular pages count a hidden page" internal/analytics/reports.go \
  's.replace("sum(d.views)::int, pg.space_id FROM page_view_days d\n\t\tJOIN pages pg ON pg.id = d.page_id JOIN perm_visible_pages($1) v ON v.page_id = pg.id", "sum(d.views)::int, pg.space_id FROM page_view_days d\n\t\tJOIN pages pg ON pg.id = d.page_id JOIN perm_visible_pages($1) v ON true")' $app -run TestPopularPagesStayQuietWhileAnalyticsIsOff
mutate "a synced block offers everyone its source" internal/blocks/reuse.go \
  's.replace("source.CanEdit = d.Allowed", "source.CanEdit = d.Allowed || true")' $app -run TestASyncedBlockSaysWhetherTheReaderMayEditItsSource
mutate "a filter by text finds a hidden page" internal/blocks/lists.go \
  's.replace("JOIN perm_visible_pages($1) v ON v.page_id = p.id\n\tJOIN spaces s ON s.id = p.space_id\n\tLEFT JOIN users u ON u.id = p.updated_by\n\tLEFT JOIN page_tree t", "JOIN perm_visible_pages($1) v ON true\n\tJOIN spaces s ON s.id = p.space_id\n\tLEFT JOIN users u ON u.id = p.updated_by\n\tLEFT JOIN page_tree t")' $app -run TestFilterListFindsByTextPeopleAndDate
mutate "wrong passwords lock the person out everywhere" internal/httpapi/handlers.go \
  's.replace("accKey := allAddressesKey + \"|\" + meta.IP", "accKey := allAddressesKey")' $app -run TestWrongPasswordsFromOneAddressDoNotLockOutThePerson
mutate "guesses from many addresses have no limit" internal/httpapi/handlers.go \
  's.replace(" || !s.loginByAllIPs.allow(allAddressesKey)", "")' $app -run TestGuessesFromManyAddressesStillStopAtALimit
mutate "any Markdown goes to the parser" internal/document/markdown.go \
  's.replace("if tooHardToParse(source) {", "if false {")' ./internal/document -run TestTheLimitOfOneBlockIsExact
mutate "pastes have no slots" internal/httpapi/handlers_organising.go \
  's.replace("case s.markdownBusy <- struct{}{}:\n\t\tdefer func() { <-s.markdownBusy }()\n\tdefault:\n\t\twriteError(w, http.StatusTooManyRequests, \"rate_limited\", \"Too much text is being converted right now. Try again shortly.\")\n\t\treturn\n\t}", "default:\n\t}")' ./internal/httpapi -run TestPastesWaitForAFreeSlot
mutate "an event too large stops the relay" internal/outbox/outbox.go \
  's.replace("if len(body) > MaxEventBytes {", "if false {")' ./internal/outbox -run TestAnEventTooLargeForTheQueueDoesNotStopTheOthers
mutate "a helper gets the network of the database" compose.yaml \
  's.replace("    networks: [helpers]\n", "")' $app -run TestHelpersHaveNoWayOut
mutate "a private saved block reaches everyone" internal/savedblocks/savedblocks.go \
  's.replace("WHERE b.owner_id = $1\n", "WHERE b.owner_id = $1 OR true\n")' $app -run TestASavedBlockStaysPrivateUntilShared
mutate "a guest sees what members shared" internal/savedblocks/savedblocks.go \
  's.replace("u.type = '"'"'member'"'"'", "u.type IS NOT NULL")' $app -run TestAGuestDoesNotSeeSharedBlocks
mutate "anyone deletes a saved block" internal/savedblocks/savedblocks.go \
  's.replace("DELETE FROM saved_blocks WHERE id = $1 AND owner_id = $2", "DELETE FROM saved_blocks WHERE id = $1 AND $2::uuid IS NOT NULL")' $app -run TestOnlyTheOwnerChangesASavedBlock
mutate "anyone shares a saved block" internal/savedblocks/savedblocks.go \
  's.replace("UPDATE saved_blocks SET shared = $3 WHERE id = $1 AND owner_id = $2", "UPDATE saved_blocks SET shared = $3 WHERE id = $1 AND $2::uuid IS NOT NULL")' $app -run TestOnlyTheOwnerChangesASavedBlock
mutate "a saved block skips the document gate" internal/savedblocks/savedblocks.go \
  's.replace("doc, err := document.Normalize(raw)\n\tif err != nil {\n\t\treturn Block{}, err\n\t}\n\tstored, err := document.Marshal(doc)", "doc, _ := document.Normalize(raw)\n\tstored, err := document.Marshal(doc)\n\tstored = raw")' $app -run TestASavedBlockPassesTheDocumentGate
mutate "a spotlight button takes any address" internal/document/attrs.go \
  's.replace("safe, err := SafeHref(href)\n\tif err != nil {\n\t\treturn err\n\t}\n\tkeepText(in, out, \"button_text\", maxButtonBytes)", "safe := href\n\tkeepText(in, out, \"button_text\", maxButtonBytes)")' ./internal/document/
mutate "an image link takes any address" internal/document/attrs.go \
  's.replace("safe, err := SafeHref(href)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tout[\"href\"] = safe", "out[\"href\"] = href")' ./internal/document/
mutate "an inline image points anywhere" internal/document/attrs.go \
  's.replace("func cleanInlineImage(in, out map[string]any) error {\n\tid, ok := attachmentID(in)\n\tif !ok {\n\t\treturn errors.New(\"images must point to an attachment; external images are not allowed\")\n\t}", "func cleanInlineImage(in, out map[string]any) error {\n\tid, _ := attachmentID(in)")' ./internal/document/
mutate "a panel emoji takes markup" internal/document/attrs.go \
  's.replace("if !isEmoji(emoji) {\n\t\treturn errors.New(\"a panel takes one emoji\")\n\t}\n", "")' ./internal/document/
mutate "an unknown block reaches an export" internal/document/html.go \
  's.replace("w.b.WriteString(`<p class=\"unknown-block\">Block not shown: ` + esc(name) + `</p>`)", "orig, _ := n.Attrs[\"original\"].(string)\n\t\tw.b.WriteString(name + orig)")' ./internal/document/
# Wave U (spec 030).
mutate "the user list names spaces a guest cannot open" internal/identity/users.go \
  's.replace("FROM perm_spaces(u.id, '"'"'space.view'"'"') ps JOIN spaces s ON s.id = ps.space_id", "FROM spaces s")' $app -run TestTheUserListNamesTheSpacesOfAGuestOnly
mutate "the content screen ignores the last day of a change" internal/pages/content.go \
  's.replace("p.updated_at < $20::timestamptz + interval '"'"'1 day'"'"'", "p.updated_at < $20::timestamptz")' $app -run TestContentFiltersByTheDayOfChangeAndOfCreation
mutate "the reach of a person counts pages of others" internal/identity/lifecycle.go \
  's.replace("FROM pages p WHERE COALESCE(p.owner_id, p.created_by) = $1 AND p.status", "FROM pages p WHERE p.status")' $app -run TestAnAdministratorSeesTheReachOfAPersonButNoTitles
mutate "a good login gives back more than its own try" internal/httpapi/handlers.go \
  's.replace("\ts.loginByIP.giveBack(\"ip:\" + meta.IP)\n\ts.loginByAllIPs.giveBack(allAddressesKey)", "\ts.loginByIP.giveBack(\"ip:\" + meta.IP)\n\ts.loginByIP.giveBack(\"ip:\" + meta.IP)\n\ts.loginByAllIPs.giveBack(allAddressesKey)")' $app -run TestOwnGoodLoginsDoNotBuyGuessesOnOtherAccounts
mutate "a word asks for no other forms" internal/search/forms.go \
  's.replace("\tall := forms(strings.ToLower(word))", "\tall := []string{strings.ToLower(word)}")' $app -run TestSearchFindsWordFormsAndKeepsTheExactTitleFirst
mutate "quotes ask for other forms too" internal/search/language.go \
  's.replace("parts = append(parts, p.exactTerm(word))", "parts = append(parts, p.term(word))")' $app -run TestSearchFindsWordFormsAndKeepsTheExactTitleFirst
mutate "the Files screen ignores the name" internal/attachments/versions.go \
  's.replace("`+fileNameWords+`", "true")' $app -run TestTheFilesScreenFiltersAndOffersOnlyWhatMayBeSeen

# Spec 031: the order of work, readiness, hashing and reuse of permission facts.
mutate "a save waits behind reading" internal/httpapi/admission.go \
  's.replace("\tif !isRead(r) {\n\t\treturn classSave\n\t}", "\tif r.Method == \"\" {\n\t\treturn classSave\n\t}")' ./internal/httpapi/ -run TestEachRequestHasTheRightClass
mutate "reading takes the reserve of saving" internal/httpapi/admission.go \
  's.replace("read := max(poolSize-quarter, 1)", "read := max(poolSize, 1)")' ./internal/httpapi/ -run TestReadingLeavesAQuarterOfThePoolForSaving
mutate "search does not count against reading" internal/httpapi/admission.go \
  's.replace("\t\t\ttaken = append(taken, a.gates[classRead])\n", "")' ./internal/httpapi/ -run TestSearchNeverTakesTheReserveOfSaving
mutate "the queue has no end" internal/httpapi/admission.go \
  's.replace("if g.waiting.Add(1) > g.maxWaiting {", "if g.waiting.Add(1) < 0 {")' ./internal/httpapi/ -run TestPastTheWaitingLimitTheAnswerComesAtOnce
mutate "probes wait in line" internal/httpapi/admission.go \
  's.replace("\t\t\tif r.URL.Path == path {\n\t\t\t\tnext.ServeHTTP(w, r)\n\t\t\t\treturn\n\t\t\t}", "\t\t\t_ = path")' ./internal/httpapi/ -run TestProbesAndMetricsNeverWait
mutate "a long queue is not reported" internal/httpapi/admission.go \
  's.replace("if g.waiting.Load() > g.maxWaiting/2 {", "if g.waiting.Load() < 0 {")' ./internal/httpapi/ -run TestReadinessSaysBusyWhenAQueueIsLong
mutate "a write reuses old permission facts" internal/httpapi/admission.go \
  's.replace("\t\tif isRead(r) {\n\t\t\tr = r.WithContext(permissions.WithMemo", "\t\tif true {\n\t\t\tr = r.WithContext(permissions.WithMemo")' ./internal/httpapi/ -run TestOnlyAReadReusesPermissionFacts
mutate "a full pool looks ready" internal/app/app.go \
  's.replace("if stat := a.pool.Stat(); stat.AcquiredConns() >= stat.MaxConns() {", "if stat := a.pool.Stat(); stat.AcquiredConns() > stat.MaxConns() {")' $app -run TestAFullPoolIsBusyNotAGoneDatabase
mutate "hashes run without a limit" internal/identity/password.go \
  's.replace("\thashSlots <- struct{}{}\n\tdefer func() { <-hashSlots }()\n", "")' ./internal/identity/ -run TestNoMoreHashesRunAtOnceThanTheLimit
mutate "a memo mixes up people" internal/permissions/memo.go \
  's.replace("\tu, ok := m.users[id]\n\treturn u, ok", "\tfor _, u := range m.users {\n\t\treturn u, true\n\t}\n\treturn UserFacts{}, false")' ./internal/permissions/ -run TestTheMemoKeepsEachPersonsOwnFactsAndLivesOneContext
# Files at rest (SEC-725, spec 032).
mutate "a file is stored plain" internal/storage/encrypted.go \
  's.replace("if _, err := e.inner.Put(ctx, key, enc, MaxCiphertext(maxBytes)); err != nil {", "if _, err := e.inner.Put(ctx, key, io.MultiReader(bytes.NewReader(magic), r), MaxCiphertext(maxBytes)); err != nil {\n\t\t_ = enc")' $app -run TestAFileIsStoredEncryptedAndComesBackWhole
mutate "a plain file is served after the job" internal/storage/encrypted.go \
  's.replace("\tif done {\n\t\t_ = f.Close()\n\t\treturn nil, ErrNotEncrypted", "\tif false && done {\n\t\t_ = f.Close()\n\t\treturn nil, ErrNotEncrypted")' ./internal/storage/ -run TestPlainFilesAreReadUntilEveryFileIsEncrypted
mutate "a file cut at a chunk border opens" internal/storage/encrypted.go \
  's.replace("\tif last {\n\t\taad[8] = 1\n\t}", "")' ./internal/storage/ -run TestAFileCutShortOrChangedNeverOpensOrReads
mutate "a file key is not bound to its name" internal/storage/encrypted.go \
  's.replace("func keyContext(key string) string { return \"storage-file:\" + key }", "func keyContext(key string) string { return \"storage-file\" }")' ./internal/storage/ -run TestAFileMovedUnderAnotherNameDoesNotOpen
mutate "the job never refuses plain files" internal/app/encryption.go \
  's.replace("\ta.encrypted.MarkEveryFileEncrypted()\n", "").replace("UPDATE storage_state SET encrypted_all_at = now()", "UPDATE storage_state SET encrypted_all_at = NULL")' $app -run TestOldPlainFilesAreEncryptedThenPlainIsRefused
mutate "a preview is converted on every request" internal/exchange/previews.go \
  's.replace("if kept, err := s.store.Open(ctx, key); err == nil {", "if kept, err := s.store.Open(ctx, key+\"-never\"); err == nil {")' $app -run TestAnOfficePreviewIsMadeOncePerVersionAndDroppedWhenUnused
mutate "the purge drops previews in use" internal/exchange/previews.go \
  's.replace("\t\tdelete(kept, hex.EncodeToString(raw))\n", "\t\t_ = raw\n")' $app -run TestAnOfficePreviewIsMadeOncePerVersionAndDroppedWhenUnused
mutate "the notification cursor skips what shares a time" internal/notify/list.go \
  's.replace(" OR (n.created_at = $5 AND $7::uuid IS NOT NULL AND n.id > $7)", "")' $app -run TestPagingNotificationsWithACursorSkipsNothingThatSharesATime
mutate "the old master key is retired while it is still needed" internal/app/keyrotation.go \
  's.replace("\tif !rep.AllCurrent() {\n\t\treturn rep, ErrOldKeyStillNeeded", "\tif false {\n\t\treturn rep, ErrOldKeyStillNeeded")' $app -run TestTheOldKeyCannotBeRetiredBeforeEverythingIsSealedAgain
mutate "a check counts an older key as current" internal/app/keyrotation.go \
  's.replaceAll("if id != r.CurrentKey {", "if false && id != r.CurrentKey {")' $app -run TestTheOldKeyCannotBeRetiredBeforeEverythingIsSealedAgain
mutate "resealing overwrites a value changed meanwhile" internal/app/keyrotation.go \
  's.replace("WHERE %s::text = $1 AND %s = $3\", table, column, idCol, column)", "WHERE %s::text = $1 AND $3::bytea IS NOT NULL\", table, column, idCol)")' $app -run TestAValueChangedWhileItIsSealedAgainKeepsTheNewValue
mutate "a check passes a value that does not open" internal/app/keyrotation.go \
  's.replace("if _, err := k.Open(sealed, purpose); err != nil {", "if _, err := k.Open(sealed, purpose); err != nil && false {")' $app -run TestAWrongKeyIsNamedForEveryValueAndFileWithoutShowingThem
mutate "a deep check reads only the end of a file" internal/app/keyrotation.go \
  's.replace("\tif !deep {\n\t\treturn nil\n\t}", "\tif true {\n\t\treturn nil\n\t}")' $app -run TestADeepCheckFindsDamageInsideAFile
mutate "the encryption job starts at the first file every run" internal/app/encryption.go \
  's.replace("\t\tif name <= cursor {\n\t\t\tcontinue\n\t\t}", "\t\tif false && name <= cursor {\n\t\t\tcontinue\n\t\t}")' $app -run TestTheEncryptionJobGoesOnWhereTheLastRunStopped
mutate "a value of an unknown key is tried with the current one" internal/keys/keys.go \
  's.replace("\t\te, ok := k.byID(sealed[1 : 1+idSize])\n\t\tif !ok {\n\t\t\treturn nil, ErrNoKey\n\t\t}", "\t\te := k.current")' ./internal/keys/ -run TestAValueNamingAKeyThatIsNotLoadedFailsClearly
mutate "promote drops the old key" internal/keys/rotate.go \
  's.replace("\t\t\tif err := writeFile(path+PreviousSuffix, kek); err != nil {\n\t\t\t\treturn \"\", err\n\t\t\t}", "\t\t\t_ = kek")' ./internal/keys/ -run TestTheKeyCanBeReplacedWhileEveryValueStaysReadable
mutate "a second key change starts while the first is unfinished" internal/keys/rotate.go \
  's.replace("\tif _, err := os.Stat(path + PreviousSuffix); err == nil {\n\t\treturn \"\", ErrPreviousPresent\n\t}", "")' ./internal/keys/ -run TestANewChangeWaitsUntilTheLastOneIsFinished
# Spec 035: blueprints 05 and 06.
mutate "the PDF helper gets no password" internal/exchange/pdf.go \
  's.replace("\t\treq.SetBasicAuth(helperUser, s.pdfPassword)\n", "")' $app -run TestThePDFHelperGetsItsPasswordAndRefusesWithoutIt
mutate "a password is hashed as typed, not as NFC" internal/identity/password.go \
  's.replace("func HashPassword(pw string, p Params) (string, error) {\n\tpw = norm.NFC.String(pw)\n", "func HashPassword(pw string, p Params) (string, error) {\n")' ./internal/identity/ -run TestAPasswordIsTheSameComposedOrDecomposed
mutate "a password may hold the product name" internal/identity/lifecycle.go \
  's.replace(", local, productWord)", ", local)")' ./internal/identity/ -run TestANewPasswordCannotHoldTheNameOfTheProduct
mutate "a title keeps the marks that reverse reading" internal/pages/pages.go \
  's.replace("strings.TrimSpace(plaintext.WithoutDirectionMarks(title))", "strings.TrimSpace(plaintext.WithoutDirectionMarks(\"\") + title)")' $app -run TestNamesLoseTheMarksThatReverseReading
mutate "a parent can be deleted alone and its children fall open" internal/db/migrations/0035_no_fall_open.sql \
  's.replace("ON DELETE NO ACTION", "ON DELETE SET NULL")' $app -run TestDeletingAParentAloneIsRefusedSoNothingFallsOpen
mutate "ending one person's sessions ends everyone's" internal/identity/factors.go \
  's.replace("query, args = \"DELETE FROM sessions WHERE user_id = $1\", []any{id}", "query, args = \"DELETE FROM sessions\", []any{}")' $app -run TestAnOperatorEndsTheSessionsOfOnePersonOrOfEveryone
mutate "a step-up through the provider takes an old login" internal/sso/sso.go \
  's.replace("if f.FreshAfter != 0 && c.AuthTime < f.FreshAfter {", "if false && c.AuthTime < f.FreshAfter {")' $app -run TestAStepUpThroughTheProviderNeedsALoginAfterItStarted
# Spec 033: a rule of the instance acts only in its chosen spaces, the user list filters, and guests show.
mutate "a chosen space does not narrow a rule" internal/automation/definition.go \
  's.replace("return len(d.Spaces) == 0 || space == nil || slices.Contains(d.Spaces, *space)", "return true")' $app -run TestAnInstanceRuleActsOnlyInTheSpacesItWasGiven
mutate "a rule of a space chooses spaces" internal/automation/service.go \
  's.replace("\tif spaceID != nil {\n\t\treturn apperr.Invalid(\"Only a rule of the whole instance chooses spaces.\")", "\tif spaceID != nil && false {\n\t\treturn apperr.Invalid(\"Only a rule of the whole instance chooses spaces.\")")' $app -run TestOnlyAnInstanceRuleChoosesSpacesAndOnlyRealOnes
mutate "a lookup looks outside the chosen spaces" internal/automation/run.go \
  's.replace("AND (cardinality($7::uuid[]) = 0 OR p.space_id = ANY($7))", "AND (cardinality($7::uuid[]) >= 0)")' $app -run TestAFindStepOfAnInstanceRuleLooksOnlyInItsSpaces
mutate "the user list ignores the space" internal/identity/users.go \
  's.replace("AND ($11::uuid IS NULL OR $11 IN (SELECT ps.space_id FROM perm_spaces(u.id, '"'"'space.view'"'"') ps))", "AND ($11::uuid IS NULL OR true)")' $app -run TestTheUserListFiltersByRoleAndBySpace
mutate "a guest is not marked" internal/permissions/access.go \
  's.replace("holders[i].Guest = holders[i].Subject.Type == SubjectUser && slices.Contains(guests, holders[i].Subject.ID)", "holders[i].Guest = len(guests) < 0")' $app -run TestWhoHasAccessShowsHowAndExportsSafely
mutate "anyone makes a role" internal/permissions/roles.go \
  's.replace("if !u.Active || !slices.Contains(u.Global, identity.PermAdministrator) {", "if false && !u.Active {")' $perms -run TestRolesRefuseWhatWouldChangeAccessQuietly
mutate "a built-in role changes its meaning" internal/permissions/roles.go \
  's.replace("if r.BuiltIn != \"\" {\n\t\t\treturn apperr.Conflict(\"built_in\", \"A built-in role keeps", "if false {\n\t\t\treturn apperr.Conflict(\"built_in\", \"A built-in role keeps")' $perms -run TestRolesRefuseWhatWouldChangeAccessQuietly
mutate "a built-in role can be deleted" internal/permissions/roles.go \
  's.replace("if r.BuiltIn != \"\" {\n\t\t\treturn apperr.Conflict(\"built_in\", \"A built-in role cannot be deleted.\")", "if false {\n\t\t\treturn apperr.Conflict(\"built_in\", \"A built-in role cannot be deleted.\")")' $perms -run TestRolesRefuseWhatWouldChangeAccessQuietly
mutate "a role in use is deleted from under the people who hold it" internal/permissions/roles.go \
  's.replace("if r.InUse > 0 {\n\t\t\treturn apperr.Conflict(\"role_in_use\"", "if false {\n\t\t\treturn apperr.Conflict(\"role_in_use\"")' $perms -run TestRolesRefuseWhatWouldChangeAccessQuietly
mutate "a role in use gains space administration" internal/permissions/roles.go \
  's.replace("slices.Contains(difference(in.Permissions, r.Permissions), SpaceAdminister)", "false")' $perms -run TestRolesRefuseWhatWouldChangeAccessQuietly
mutate "a role in use changes without a confirmation" internal/permissions/roles.go \
  's.replace("if !confirm {", "if false {")' $perms -run TestRolesRefuseWhatWouldChangeAccessQuietly
mutate "a role with system administration needs no system administrator" internal/permissions/roles.go \
  's.replace("if !ok {\n\t\treturn apperr.Conflict(\"needs_system_administrator\"", "if false && !ok {\n\t\treturn apperr.Conflict(\"needs_system_administrator\"")' $perms -run TestRolesRefuseWhatWouldChangeAccessQuietly
mutate "giving roles skips the limits of managing access" internal/permissions/roles.go \
  's.replace("if err := s.mayChangeSubject(ctx, tx, c, spaceID, sub, before, after); err != nil {", "if err := error(nil); err != nil {")' $perms -run TestAssigningRolesKeepsTheLimitsOfManagingAccess
mutate "instance roles can remove the last administrator" internal/permissions/roles.go \
  's.replace("if err := keepsAnAdministrator(ctx, tx, c); err != nil {\n\t\t\treturn err\n\t\t}\n\t\treturn recordChange", "if err := error(nil); err != nil {\n\t\t\treturn err\n\t\t}\n\t\treturn recordChange")' $perms -run TestAssigningRolesKeepsTheLimitsOfManagingAccess
mutate "anyone previews the roles of a space" internal/permissions/roles.go \
  's.replace("learns nothing from the answer.\n\t\tif err := s.RequireSpace(ctx, tx, c.UserID, spaceID, SpaceManageAccess); err != nil {", "learns nothing from the answer.\n\t\tif err := error(nil); err != nil {")' $perms -run TestPreviewingRolesSaysWhatChangesAndChangesNothing
mutate "a role preview ignores the roles chosen" internal/permissions/roles.go \
  's.replace("out, err = previewAgainst(ctx, tx, sub, before, after)", "out, err = previewAgainst(ctx, tx, sub, before, nil)")' $perms -run TestPreviewingRolesSaysWhatChangesAndChangesNothing
mutate "a role preview promises what the change refuses" internal/permissions/roles.go \
  's.replace("cannot happen.\n\t\tif err := s.mayChangeSubject(ctx, tx, c, spaceID, sub, before, after); err != nil {", "cannot happen.\n\t\tif err := error(nil); err != nil {")' $perms -run TestPreviewingRolesSaysWhatChangesAndChangesNothing
mutate "a preview hides that someone loses the space" internal/permissions/access.go \
  's.replace("out.LosesView = slices.Contains(out.Loses, SpaceView)", "out.LosesView = false")' $perms -run TestPreviewingRolesSaysWhatChangesAndChangesNothing
mutate "adopting grants as roles drops a permission" internal/db/migrations/0040_finer_permissions.sql \
  's.replace("PERFORM rbac_set_exact(g.space_id, g.subject_type, g.subject_id, g.perms);", "PERFORM rbac_set_exact(g.space_id, g.subject_type, g.subject_id, g.perms[2:]);")' $perms -run TestAdoptingGrantsChangesNoDecision
mutate "a changed role does not reach the people who hold it" internal/db/migrations/0036_roles.sql \
  's.replace("FOR EACH ROW EXECUTE FUNCTION rbac_role_changed();", "FOR EACH ROW WHEN (false) EXECUTE FUNCTION rbac_role_changed();")' $perms -run TestRolesRefuseWhatWouldChangeAccessQuietly
mutate "taking a role away leaves its grants behind" internal/db/migrations/0036_roles.sql \
  's.replace("DELETE FROM space_grants g WHERE g.space_id = sp", "DELETE FROM space_grants g WHERE false AND g.space_id = sp")' $perms -run TestAssigningRolesKeepsTheLimitsOfManagingAccess
mutate "the application role writes grants itself" internal/db/db.go \
  's.replace("\"REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON space_grants FROM \" + ident,", "")' ./internal/db/ -run TestApplicationRoleGrantsOnlyThroughRoles
mutate "publishing a site needs space administration again" internal/sites/sites.go \
  's.replace("RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SitePublish); err != nil {\n\t\treturn Site{}, err", "RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {\n\t\treturn Site{}, err")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "a public link needs space administration again" internal/sites/links.go \
  's.replace("permissions.SitePublish)", "permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "automation rules need space administration again" internal/automation/service.go \
  's.replace("permissions.SpaceAutomation)", "permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "webhooks need space administration again" internal/webhooks/webhooks.go \
  's.replace("permissions.SpaceAutomation)", "permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "space templates need space administration again" internal/templates/templates.go \
  's.replace("permissions.SpaceTemplates)", "permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "the audit of a space needs space administration again" internal/pages/lists.go \
  's.replace("permissions.SpaceAudit)", "permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "a space template is made by anyone who sees the space" internal/templates/templates.go \
  's.replace("permissions.SpaceTemplates)", "permissions.SpaceView)")' $app -run TestAdministeringASpaceNoLongerHoldsWhatWasSplitOff
mutate "administering a space still holds what was split off it" internal/permissions/evaluate.go \
  's.replace("return in.Action, holds(in, in.Action)", "return in.Action, holds(in, in.Action) || holds(in, SpaceAdminister)")' $perms -run TestMatrixEvaluate
mutate "what was split off administering stops on an archived page" internal/permissions/evaluate.go \
  's.replace("\treturn splitOffAdminister(action)\n}\n\n// worksWhenArchived", "\treturn false\n}\n\n// worksWhenArchived")' $perms -run TestMatrixEvaluate
mutate "adoption leaves out what was split off" internal/db/migrations/0040_finer_permissions.sql \
  's.replace("AS f(p)\n        WHERE NOT EXISTS", "AS f(p)\n        WHERE false AND NOT EXISTS")' $perms -run TestAdoptingGrantsChangesNoDecision
mutate "a permission set with administering becomes a role without what was split off" internal/db/migrations/0040_finer_permissions.sql \
  's.replace("rbac_with_split_off(s.permissions));", "s.permissions);")' $perms -run TestAdoptingGrantsChangesNoDecision
mutate "an old bundle loses what the broader permission gave" internal/exchange/bundlein.go \
  's.replace("version < heldSince[finer]", "version < 0")' ./internal/exchange/ -run TestABundleFromBeforeTheSplitKeepsWhatTheBroaderPermissionGave
# Spec 040: eight more permissions split off broader ones, and facts kept between requests.
mutate "a version 2 bundle gains what it already held" internal/exchange/bundlein.go \
  's.replace("version < heldSince[finer]", "version <= heldSince[finer]")' ./internal/exchange/ -run TestABundleOfVersionTwoGainsOnlyTheLaterSplit
mutate "labels need space administration again" internal/pages/labels.go \
  's.replaceAll("permissions.SpaceLabels)", "permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "statuses need space administration again" internal/pages/ownership.go \
  's.replace("permissions.SpaceStatuses)", "permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "the review period needs space administration again" internal/pages/ownership.go \
  's.replace(/permissions.SpaceStatuses\)([\s\S]*)permissions.SpaceStatuses\)/, "permissions.SpaceStatuses)$1permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "the approval rule needs space administration again" internal/pages/approvals.go \
  's.replaceAll("permissions.SpaceApprovals)", "permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "the look of a space needs space administration again" internal/spaces/settings.go \
  's.replace("return s.requirePermission(ctx, tx, c, id, permissions.SpaceLook)", "return s.requirePermission(ctx, tx, c, id, permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "shortcuts need space administration again" internal/spaces/shortcuts.go \
  's.replace("permissions.SpaceLook)", "permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "watchers need space administration again" internal/watches/watches.go \
  's.replaceAll("permissions.PageWatchers,", "permissions.SpaceAdminister,")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "a bundle needs space administration again" internal/exchange/bundle.go \
  's.replace("spaceID, permissions.SpaceBundle)", "spaceID, permissions.SpaceAdminister)")' $app -run TestEachSplitPermissionWorksWithoutAdministering
mutate "a bundle no longer needs export" internal/exchange/bundle.go \
  's.replace("if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceExport); err != nil {\n\t\treturn ExportRun{}, err\n\t}\n\tallowed, err := s.mayUseFormat(ctx, c.UserID, FormatBundle)", "allowed, err := s.mayUseFormat(ctx, c.UserID, FormatBundle)")' $app -run TestABundleStillNeedsExport
mutate "asking for guests needs only manage access again" internal/httpapi/handlers_people.go \
  's.replace("id, permissions.SpaceInviteGuests); err != nil {", "id, permissions.SpaceManageAccess); err != nil {")' $app -run TestAskingForGuestsIsItsOwnPermission
mutate "administering no longer asks for guests" internal/permissions/evaluate.go \
  's.replace("holds(in, SpaceInviteGuests) || holds(in, SpaceAdminister)", "holds(in, SpaceInviteGuests)")' $perms -run TestMatrixEvaluate
mutate "moving out needs only edit again" internal/pages/move.go \
  's.replace("[]string{permissions.PageMoveOut, permissions.CreatePermission(p.Kind)}", "[]string{permissions.PageEdit, permissions.CreatePermission(p.Kind)}")' $app -run TestMovingAPageToAnotherSpaceIsItsOwnPermission
mutate "the impact of a move out needs only edit" internal/pages/move.go \
  's.replace("action = permissions.PageMoveOut", "action = permissions.PageEdit")' $app -run TestMovingAPageToAnotherSpaceIsItsOwnPermission
mutate "an edit restriction does not stop a move out" internal/permissions/evaluate.go \
  's.replace("AttachmentAdd, ActionAttachmentDelete, PageMoveOut, DatabaseEdit:", "AttachmentAdd, ActionAttachmentDelete, DatabaseEdit:")' $perms -run TestMatrixEvaluate
mutate "asking for guests stops in an archived space" internal/permissions/evaluate.go \
  's.replace("AnalyticsView,\n\t\tSpaceInviteGuests:", "AnalyticsView:")' $perms -run TestMatrixEvaluate
# Migration 0051 holds the latest rbac_split_off, so the mutations of the split aim there.
mutate "the migration gives editors no move out" internal/db/migrations/0051_document_control.sql \
  's.replace("WHEN '"'"'page.edit'"'"' THEN ARRAY['"'"'page.move_out'"'"', '"'"'database.edit'"'"'", "WHEN '"'"'page.edit'"'"' THEN ARRAY['"'"'database.edit'"'"'")' $perms -run TestTheSplitIsTheSameInGoAndSQL
mutate "the migration leaves existing roles as they were" internal/db/migrations/0040_finer_permissions.sql \
  's.replace("WHERE scope = '"'"'space'"'"' AND permissions IS DISTINCT FROM rbac_with_split_off(permissions);", "WHERE scope = '"'"'space'"'"' AND permissions IS DISTINCT FROM rbac_with_split_off(permissions) AND false;")' $perms -run TestBuiltInRolesMatchTheTemplates
mutate "kept facts ignore the permission generation" internal/permissions/factcache.go \
  's.replace("\treturn gen == c.gen\n", "\treturn true\n").replace("\tif gen > c.gen {", "\tif false {")' $perms -run TestAChangeEndsKeptFactsAtOnce
mutate "a revoked space grant does not count" internal/db/migrations/0040_finer_permissions.sql \
  's.replace("ON space_grants\n    FOR EACH STATEMENT", "ON space_grants\n    FOR EACH STATEMENT WHEN (false)")' $perms -run TestAChangeEndsKeptFactsAtOnce
mutate "leaving a group does not count" internal/db/migrations/0040_finer_permissions.sql \
  's.replace("ON group_members\n    FOR EACH STATEMENT", "ON group_members\n    FOR EACH STATEMENT WHEN (false)")' $perms -run TestAChangeEndsKeptFactsAtOnce
mutate "archiving a space does not count" internal/db/migrations/0040_finer_permissions.sql \
  's.replace("UPDATE OF status ON spaces\n    FOR EACH STATEMENT", "UPDATE OF status ON spaces\n    FOR EACH STATEMENT WHEN (false)")' $perms -run TestAChangeEndsKeptFactsAtOnce
mutate "disabling an account does not count" internal/db/migrations/0040_finer_permissions.sql \
  's.replace("ON users\n    FOR EACH STATEMENT", "ON users\n    FOR EACH STATEMENT WHEN (false)")' $perms -run TestAChangeEndsKeptFactsAtOnce
mutate "losing the product does not count" internal/db/migrations/0040_finer_permissions.sql \
  's.replace("ON global_grants\n    FOR EACH STATEMENT", "ON global_grants\n    FOR EACH STATEMENT WHEN (false)")' $perms -run TestAChangeEndsKeptFactsAtOnce
mutate "an account past its end stays active while kept" internal/permissions/factcache.go \
  's.replace("\tu.Active = activeAt(e.status, e.expires, c.now())\n", "")' $perms -run TestAKeptAccountEndsAtItsEndDate
mutate "kept facts never grow old" internal/permissions/factcache.go \
  's.replace("return c.now().Sub(at) < FactMaxAge", "return true")' $perms -run TestKeptFactsEndWithTheirGenerationAndTheirAge
mutate "a request from before a change uses kept facts" internal/permissions/factcache.go \
  's.replace("\treturn gen == c.gen\n", "\treturn gen <= c.gen\n")' $perms -run TestKeptFactsEndWithTheirGenerationAndTheirAge
mutate "a purged page leaves its reactions" internal/db/migrations/0040_finer_permissions.sql \
  's.replace("CREATE TRIGGER pages_take_reactions AFTER DELETE ON pages\n    FOR EACH ROW", "CREATE TRIGGER pages_take_reactions AFTER DELETE ON pages\n    FOR EACH ROW WHEN (false)")' $app -run TestAPurgedPageLeavesNothingBehind
mutate "a list shows a hidden page" internal/db/migrations/0051_document_control.sql \
  's.replace("      AND p.archived_at IS NULL\n      AND p.id NOT IN (SELECT h.page_id FROM perm_hidden_pages(u) h)", "      AND p.archived_at IS NULL")' $app -run TestNoListShowsAHiddenPageOrWhatHangsOnIt
mutate "a move out of a restricted branch needs no right to restrict" internal/pages/move.go \
  's.replace("\tif len(lost) \x3e 0 {\n\t\tif err := s.perms.RequirePage(ctx, tx, c.UserID, p.ID, permissions.RestrictionManage, uuid.Nil); err != nil {", "\tif len(lost) \x3c 0 {\n\t\tif err := s.perms.RequirePage(ctx, tx, c.UserID, p.ID, permissions.RestrictionManage, uuid.Nil); err != nil {")' $app -run TestNoPlaceChangeOpensAPage
mutate "the faster list forgets the archive" internal/db/migrations/0051_document_control.sql \
  's.replace("      AND p.archived_at IS NULL\n", "")' $perms -run TestMatrixSQLAgreesWithEvaluate
mutate "the faster hidden pages miss a group" internal/db/migrations/0040_finer_permissions.sql \
  's.replace("OR (r.subject_type = '"'"'group'"'"' AND r.subject_id IN\n                   (SELECT gm.group_id FROM group_members gm WHERE gm.user_id = u)))))", "OR false)))")' $perms -run TestMatrixSQLAgreesWithEvaluate
mutate "the old grants route gives administration without what was split off it" internal/permissions/manage.go \
  's.replace("\tperms = WithSplitOff(perms)\n", "")' $perms -run TestEveryWayToAdministerASpaceGivesTheFinerPermissions
mutate "a list from between the splits loses the later one" internal/permissions/split.go \
  's.replace("g := group{broad, splitInSpec[f]}", "g := group{broad, 0}")' $perms -run TestAListFromBeforeASplitGetsWhatWasSplitOff
mutate "a write reads kept facts" internal/permissions/service.go \
  's.replace("\tif m == nil {\n\t\tu, _, _, err := s.loadUser(ctx, q, id)\n\t\treturn u, err\n\t}\n", "")' $perms -run TestAWriteNeverReadsKeptFacts
mutate "anyone reads who holds a role" internal/permissions/roles.go \
  's.replace("([]RoleHolder, int, error) {\n\tif err := s.requireAdministrator(ctx, s.pool, c); err != nil {", "([]RoleHolder, int, error) {\n\tif err := error(nil); err != nil {")' $perms -run TestHoldersOfARoleShowOnlyToAdministrators
mutate "a write may read from a copy that lags" internal/httpapi/admission.go \
  's.replace("if !isRead(r) {\n\t\treturn false\n\t}\n\tif classOf(r) == classHeavy {", "if classOf(r) == classHeavy {")' ./internal/httpapi/ -run TestOnlyListsAndHeavyReadsMayLag
mutate "a helper that does not answer is still offered" internal/httpapi/handlers.go \
  's.replace("s.deps.Exchange.PDFAvailable() && !down[\"pdf\"]", "s.deps.Exchange.PDFAvailable()")' $app -run TestAHelperThatDoesNotAnswerIsShownAndNoLongerOffered
mutate "a viewer is offered to save a changed PDF" internal/attachments/versions.go \
  's.replace("out[i].MayAddVersion = allowed", "out[i].MayAddVersion = true")' $app -run TestOnlySomeoneWhoMayAddFilesIsOfferedToSaveAChangedPDF
mutate_web "any window may pose as a frame" src/components/frame-messages.ts \
  's.replace("!!frame && event.source === frame && event.origin === origin", "!!frame && event.origin === origin")' src/components/frame-messages.test.ts
mutate_web "the viewer may give back something that is not a PDF" src/components/craft-frame.tsx \
  's.replace("return start === PDF_START ? bytes : null;", "return bytes;")' src/components/craft-frame.test.ts
mutate "any setup code works" internal/setup/setup.go \
  's.replace("matches := subtle.ConstantTimeCompare(given, hash) == 1", "matches := len(given) > 0 || subtle.ConstantTimeCompare(given, hash) == 1")' $app -run TestTheSetupScreenRefusesAnyoneWithoutTheCode
mutate "a setup code never expires" internal/setup/setup.go \
  's.replace("if wrong >= MaxWrongTries || !s.now().Before(expiresAt) {", "if wrong >= MaxWrongTries {")' $app -run TestAnExpiredSetupCodeIsRefused
mutate "a setup code can be guessed for ever" internal/setup/setup.go \
  's.replace("if wrong >= MaxWrongTries || !s.now().Before(expiresAt) {", "if !s.now().Before(expiresAt) {")' $app -run TestASetupCodeEndsAfterTooManyWrongTries
mutate "a used setup code stays unused" internal/setup/setup.go \
  's.replace("\tif use {\n", "\tif use && false {\n")' $app -run TestTheSetupStaysClosedEvenIfNoAdministratorIsLeft
mutate "the setup stays open with an administrator" internal/setup/setup.go \
  's.replace("SELECT EXISTS (SELECT 1 FROM global_grants WHERE permission = '"'"'administrator'"'"')\n\t\tOR ", "SELECT false AND EXISTS (SELECT 1 FROM global_grants WHERE permission = '"'"'administrator'"'"')\n\t\tOR ")' $app -run TestTheSetupIsGoneOnceAnAdministratorExists
mutate "the setup form is read before the code" internal/httpapi/handlers_setup.go \
  's.replace("\tif err := s.deps.Setup.Check(r.Context(), in.Code, from); err != nil {", "\tif err := s.deps.Setup.Check(r.Context(), in.Code, from); err != nil && false {")' $app -run TestTheSetupScreenRefusesAnyoneWithoutTheCode
mutate "one address may guess setup codes for ever" internal/httpapi/handlers_setup.go \
  's.replace("\tif !s.setupTries.allow(\"ip:\" + meta.IP) {", "\tif !s.setupTries.allow(\"ip:\" + meta.IP) && false {")' $app -run TestOneAddressCannotKeepGuessingSetupCodes
mutate "turning off TOTP lets a session guess the password" internal/httpapi/handlers_identity.go \
  's.replace("\tif !s.loginByAcc.allow(\"pw:\" + PrincipalFrom(r.Context()).User.ID.String()) {", "\tif false {")' $app -run TestTurningOffTOTPCannotBeUsedToGuessThePassword
mutate "a plain address on the network counts as safe" internal/config/config.go \
  's.replace("return !strings.EqualFold(c.PublicURL.Hostname(), \"localhost\")", "return false && !strings.EqualFold(c.PublicURL.Hostname(), \"localhost\")")' ./internal/config/ -run TestAPlainAddressIsWeakOnlyWhenOthersReachIt
mutate "the console keeps quiet about a plain address" internal/httpapi/handlers_console.go \
  's.replace("if s.deps.Config.PlainToOthers() {", "if false {")' $app -run TestTheConsoleWarnsOfAPlainPublicAddressThatOthersReach
mutate "the app listens on every address" compose.yaml \
  's.replace("${DIFFLUENCE_BIND:-127.0.0.1}:${DIFFLUENCE_PORT:-8080}:8080", "${DIFFLUENCE_PORT:-8080}:8080")' $app -run TestEveryPublishedPortListensOnThisMachineByDefault
mutate "a SCIM filter on an attribute nobody reads is ignored" internal/scim/scim.go \
  's.replace("if !slices.Contains(known, attr) {", "if !slices.Contains(known, attr) && false {")' ./internal/scim/ -run TestAFilterIsUnderstoodWholeOrRefused
mutate "many addresses together flood one inbox" internal/httpapi/handlers_people.go \
  's.replace("\tif s.mayMail(in.Email) {\n\t\tif err := s.deps.Identity.RequestReset(", "\tif true {\n\t\tif err := s.deps.Identity.RequestReset(")' $app -run TestResetMailToOnePersonStopsAtALimitFromAnyAddress
mutate "a new second factor goes unmentioned" internal/identity/factors.go \
  's.replace("return s.tellOfChange(ctx, tx, c.UserID)", "return nil")' $app -run TestAPersonIsToldOfEveryChangeToTheirSignIn
mutate "a new password goes unmentioned" internal/identity/users.go \
  's.replace("return s.tellOfChange(ctx, tx, c.UserID)", "return nil")' $app -run TestAPersonIsToldOfEveryChangeToTheirSignIn
mutate "a password an administrator set goes unmentioned" internal/identity/users.go \
  's.replace("return s.tellOfChange(ctx, tx, id)", "return nil")' $app -run TestAPersonIsToldOfEveryChangeToTheirSignIn
mutate "a reset by link goes unmentioned" internal/identity/lifecycle.go \
  's.replace("return s.tellOfChange(ctx, tx, u.ID)", "return nil")' $app -run TestAResetMailGoesThroughTheQueueWithItsLinkSealed
mutate "the login flow cookie has no secure prefix" internal/httpapi/server.go \
  's.replace("s.flowCookie = \"__Secure-\" + oidcCookie", "s.flowCookie = oidcCookie")' $app -run TestTheLoginFlowCookieIsSecureByNameOnHTTPS
mutate_collab "a browser names itself in its cursor" presence.mjs \
  's.replace("states.set(clientID, { ...state, user: { id: user.id, name: user.name } });", "")' presence.test.mjs
mutate_collab "a connection without a person keeps its cursor" presence.mjs \
  's.replace("states.delete(clientID);", "")' presence.test.mjs

mutate_web "a shared address may lead anywhere" src/components/editor/shared-clean.ts \
  's.replace("return SAFE_SCHEMES.has(", "return true || SAFE_SCHEMES.has(")' src/components/editor/shared-clean.test.ts
mutate_web "a shared node keeps an attribute it does not have" src/components/editor/shared-clean.ts \
  's.replace("if (!allowed.has(name) || (", "if ((")' src/components/editor/shared-clean.test.ts
mutate_web "a shared link to an unsafe place stays" src/components/editor/shared-clean.ts \
  's.replace("isAddressAttribute(key) && !isSafeAddress(value))) return null;", "isAddressAttribute(key) && !isSafeAddress(value) && false)) return null;")' src/components/editor/shared-clean.test.ts
mutate_web "a shared node the schema does not know stays" src/components/editor/shared-clean.ts \
  's.replace("parent.delete(i, 1);", "")' src/components/editor/shared-clean.test.ts
mutate_web "changes from another client are not cleaned" src/components/editor/shared-clean.ts \
  's.replace("doc.on(\x27afterTransaction\x27, onTransaction);", "")' src/components/editor/shared-clean.test.ts

# Wave O7 (spec 044): the checks before an upgrade, and a refusal to run on a database
# a newer release contracted.
mutate "an older release runs after a contract migration" internal/db/compat.go \
  's.replace("WHERE phase = $1 AND version > $2", "WHERE phase = $1 AND version > $2 AND false")' ./internal/db/ -run TestOlderNodeRefusesADatabaseAfterAContractMigration
mutate "migrate skips the check for a newer contract" internal/db/migrate.go \
  's.replace("if err := CheckCompatible(ctx, conn, migs); err != nil {", "if err := error(nil); err != nil {")' ./internal/db/ -run TestOlderNodeRefusesADatabaseAfterAContractMigration
mutate "a node that does not migrate skips the check" internal/app/upgrade.go \
  's.replace("return db.CheckCompatible(ctx, a.pool, a.migrations)", "return nil")' $app -run TestANodeRefusesADatabaseANewerReleaseContracted
mutate "an old PostgreSQL is accepted" internal/db/compat.go \
  's.replace("if number < minPostgresVersion {", "if number < 0 {")' ./internal/db/ -run TestPostgresBelowTheMinimumIsRefused
mutate "a full data folder does not stop an upgrade" internal/app/upgrade.go \
  's.replace("f.freeBytes < minFreeForUpgrade", "f.freeBytes < 0")' $app -run TestTheChecksBeforeAnUpgradeSayWhyTheyRefuse
mutate "a queue that does not answer does not stop an upgrade" internal/app/upgrade.go \
  's.replace("if !f.queueHealthy {", "if false {")' $app -run TestTheChecksBeforeAnUpgradeSayWhyTheyRefuse
mutate "an old backup does not stop an upgrade" internal/app/upgrade.go \
  's.replace("case now.Sub(f.lastBackup) > maxAge:", "case false:")' $app -run TestTheChecksBeforeAnUpgradeSayWhyTheyRefuse
mutate "the checks are skipped before migrating" internal/app/upgrade.go \
  's.replace("if err := a.checkBeforeUpgrade(ctx, owner); err != nil {", "if err := error(nil); err != nil {")' $app -run TestAnUpgradeWithoutTheBackupTheOperatorAskedForIsRefusedBeforeMigrating
mutate "the backup the operator asked for is never read" internal/app/upgrade.go \
  's.replace("} else if err := readBackupFacts(ctx, owner, &facts); err != nil {", "} else if false {")' $app -run TestAnUpgradeLeavesAMarkerNamingTheLastBackup
mutate "an upgrade leaves no marker" internal/app/upgrade.go \
  's.replace("return markUpgrade(ctx, owner, st, facts.lastBackup, a.maxMigration())", "return nil")' $app -run TestAnUpgradeLeavesAMarkerNamingTheLastBackup
mutate "a restart counts as an upgrade" internal/app/upgrade.go \
  's.replace("if st.PendingExpand == 0 {", "if st.PendingExpand < 0 {")' $app -run TestARestartIsNoUpgrade
# Spec 045: blog posts and databases have a create permission of their own.
mutate "a post is made with page creation" internal/permissions/evaluate.go \
  's.replace("\tcase kindPost:\n\t\treturn PostCreate\n", "\tcase kindPost:\n\t\treturn PageCreate\n")' $app -run TestEachKindIsMadeWithItsOwnPermissionAlone
mutate "a database is made with page creation" internal/permissions/evaluate.go \
  's.replace("\tcase kindDatabase:\n\t\treturn DatabaseCreate\n", "\tcase kindDatabase:\n\t\treturn PageCreate\n")' $app -run TestPagesWithoutPostsOrDatabasesCannotMakeEither
mutate "a new item asks for page creation whatever its kind" internal/pages/pages.go \
  's.replace("in.SpaceID, permissions.CreatePermission(in.Kind)); err != nil {", "in.SpaceID, permissions.PageCreate); err != nil {")' $app -run TestPagesWithoutPostsOrDatabasesCannotMakeEither
mutate "below a page a database asks for page creation" internal/pages/pages.go \
  's.replace("at.parent, permissions.CreatePermission(at.kind), uuid.Nil)", "at.parent, permissions.PageCreate, uuid.Nil)")' $app -run TestEachKindIsMadeWithItsOwnPermissionAlone
mutate "turning a page into a post needs only edit" internal/pages/kinds.go \
  's.replace("if made := permissions.CreatePermission(kind); made != permissions.CreatePermission(p.Kind) {", "if made := permissions.CreatePermission(kind); false \x26\x26 made != \"\" {")' $app -run TestTurningAPageIntoAPostAsksForPosts
mutate "a copied item asks for nothing of its kind" internal/pages/tree.go \
  's.replace("\tif err := s.perms.RequireSpace(ctx, tx, c.UserID, plan.spaceID, permissions.CreatePermission(p.Kind)); err != nil {\n\t\treturn Page{}, err\n\t}\n", "")' $app -run TestCopyingADatabaseAsksForDatabasesWhereTheCopyGoes
mutate "a branch moves out without the kinds in it" internal/pages/move.go \
  's.replace("append([]string{permissions.SpaceView}, needed...)", "append([]string{permissions.SpaceView, permissions.PageCreate}, needed[:0]...)")' $app -run TestMovingADatabaseToAnotherSpaceAsksForDatabasesThere
mutate "the migration gives page creators no posts" internal/db/migrations/0051_document_control.sql \
  's.replace("WHEN '"'"'page.create'"'"' THEN ARRAY['"'"'post.create'"'"', '"'"'database.create'"'"']", "WHEN '"'"'page.create'"'"' THEN ARRAY['"'"'database.create'"'"']")' $perms -run TestTheSplitIsTheSameInGoAndSQL
mutate "the migration leaves roles without posts and databases" internal/db/migrations/0041_posts_and_databases.sql \
  's.replace("\nSELECT rbac_split_roles();", "\nSELECT 1;")' $perms -run TestBuiltInRolesMatchTheTemplates
mutate "an older bundle gives page creators no databases" internal/exchange/bundlein.go \
  's.replace("\tpermissions.DatabaseCreate:    4,\n", "\tpermissions.DatabaseCreate:    1,\n")' ./internal/exchange/ -run TestABundleOfVersionThreeGivesWhoeverCreatesPagesPostsAndDatabases
mutate "the page created event forgets its kind" internal/pages/pages.go \
  's.replace("\"parent_id\": in.parent, \"kind\": in.kind}", "\"parent_id\": in.parent}")' $app -run TestThePageCreatedEventSaysWhichKindWasMade
mutate_web "the create menu offers a database to anyone who makes pages" src/create-permission.ts \
  's.replace("if (kind === \x27database\x27) return \x27database.create\x27;", "")' src/create-permission.test.ts

# Wave SX (spec 046): the witness of the audit log outside the database, and the stop
# on password login after 100 wrong passwords in a row (ADR-038, ADR-039).
mutate "a quieter log level drops the witness" internal/app/app.go \
  's.replace("witness: slog.New(alwaysEnabled{log.Handler()})", "witness: log")' $app -run TestTheWitnessLogShowsATailRemovedFromTheDatabase
mutate "no witness line is written" internal/app/ops.go \
  's.replace("a.witness.Info(audit.WitnessMessage, cp.WitnessAttrs()...)", "_ = cp")' $app -run TestTheWitnessLogShowsATailRemovedFromTheDatabase
mutate "checkpoints are daily again" internal/app/ops.go \
  's.replace("checkpointEvery = 5 * time.Minute", "checkpointEvery = 24 * time.Hour")' $app -run TestTheHeadOfTheChainIsSignedEveryFiveMinutes
mutate "a quiet instance signs the same head again" internal/audit/checkpoint.go \
  's.replace("if action == checkpointSignedAction {", "if action == \"\" {")' $app -run TestNothingNewMeansNoNewCheckpoint
mutate "a removed tail passes the witness" internal/audit/witness.go \
  's.replace("if c.Seq > head {", "if c.Seq > head && head < 0 {")' $app -run TestTheWitnessLogShowsATailRemovedFromTheDatabase
mutate "a forged witness line passes" internal/audit/witness.go \
  's.replace("if !ed25519.Verify(public, checkpointMessage(c.Seq, c.Hash), c.Signature) {", "if len(c.Signature) < 0 {")' $app -run TestAForgedWitnessLineIsRefused
mutate "a chain that differs from the witness passes" internal/audit/witness.go \
  's.replace("if !bytes.Equal(chain, c.Hash) {", "if len(chain) < 0 {")' ./internal/audit/ -run TestAChainThatDiffersFromTheWitnessIsNamed
mutate "a broken witness line is skipped" internal/audit/witness.go \
  's.replace("return nil, fmt.Errorf(\"audit: witness line %d cannot be read\", number)", "continue")' ./internal/audit/ -run TestABrokenWitnessLineIsAnError
mutate "checkpoints from before a prune count as damage" internal/audit/witness.go \
  's.replace("if c.Seq <= prunedUpTo {", "if c.Seq <= prunedUpTo && prunedUpTo < 0 {")' ./internal/audit/ -run TestWitnessCheckpointsBeforeAPruneAreSkipped
mutate "audit verify ignores the witness" cmd/diffluence/main.go \
  's.replace("if res.OK && witness != nil {", "if res.OK && witness == nil {")' ./cmd/diffluence/ -run TestAuditVerifyComparesTheChainWithTheWitnessLog
mutate "an empty witness log passes" cmd/diffluence/main.go \
  's.replace("if len(witness) == 0 {", "if len(witness) < 0 {")' ./cmd/diffluence/ -run TestAuditVerifyRefusesAWitnessLogWithoutCheckpoints
mutate "a hundred wrong passwords stop nothing" internal/identity/identity.go \
  's.replace("THEN now() END", "THEN NULL END")' ./internal/identity/ -run TestAHundredWrongPasswordsStopPasswordLoginAcrossARestart
mutate "the right password opens a stopped account" internal/identity/identity.go \
  's.replace("\tcase locked:\n", "\tcase locked && len(password) < 0:\n")' $app -run TestTheHundredthWrongPasswordStopsPasswordLogin
mutate "the right password leaves the run of wrong ones" internal/identity/identity.go \
  's.replace("if failedInRow > 0 {", "if failedInRow < 0 {")' $app -run TestTheRightPasswordEndsARunOfWrongOnes
mutate "a disabled account counts wrong passwords" internal/identity/identity.go \
  's.replace("if f.known && f.reason == \"bad_password\" {", "if f.known {")' $app -run TestOnlyWrongPasswordsOnALiveAccountCount
mutate "the person is not told of the stop" internal/identity/identity.go \
  's.replace("return s.send(ctx, s.pool, AccountMail{To: f.account.Email, Kind: MailPasswordLocked})", "return nil")' $app -run TestTheHundredthWrongPasswordStopsPasswordLogin
mutate "a new password leaves the stop" internal/identity/users.go \
  's.replace("updated_at = now(),\n\t\t\tfailed_logins_in_row = 0, password_locked_at = NULL`", "updated_at = now()`")' $app -run TestAnAdministratorLiftsTheStopAndANewPasswordDoesToo
mutate "lifting the stop changes nothing" internal/identity/users.go \
  's.replace("UPDATE local_credentials SET failed_logins_in_row = 0, password_locked_at = NULL\n\t\t\tWHERE", "UPDATE local_credentials SET failed_logins_in_row = 0\n\t\t\tWHERE")' $app -run TestAnAdministratorLiftsTheStopAndANewPasswordDoesToo
mutate "anyone can lift the stop" internal/httpapi/handlers_identity.go \
  's.replace("\"POST /api/v1/admin/users/{id}/unlock-password\", admin,", "\"POST /api/v1/admin/users/{id}/unlock-password\", SessionOnly(Authenticated()),")' $app -run TestAnAdministratorLiftsTheStopAndANewPasswordDoesToo
mutate_web "a Word file shows as text" src/components/file-preview.tsx \
  's.replace("const TEXT_FILE = /^(?:text\\/[^;]+|[^/;]+\\/(?:[^;]*\\+)?(?:json|xml|yaml|x-yaml))(?:\\s*;.*)?$/;", "const TEXT_FILE = /^text\\/|json|xml|yaml/;")' src/components/file-preview.test.ts
mutate "the console does not show the stop" internal/identity/users.go \
  's.replace("WHERE c.user_id = u.id AND c.password_locked_at IS NOT NULL),", "WHERE c.user_id = u.id AND c.password_locked_at IS NULL AND false),")' $app -run TestAnAdministratorLiftsTheStopAndANewPasswordDoesToo
# Spec 047: the storage limit everywhere, near matches for names, and the floating bars.
mutate "a file in the trash takes no room" internal/attachments/limits.go \
  's.replace("WHERE a.space_id = $1) stored)`", "WHERE a.space_id = $1 AND a.status = \x27active\x27) stored)`")' $app -run TestAFileInTheTrashStillTakesRoomUntilItIsPurged
mutate "a version made current again counts twice" internal/attachments/limits.go \
  's.replace("`(SELECT COALESCE(sum(stored.size), 0)::bigint FROM (SELECT DISTINCT", "`(SELECT COALESCE(sum(stored.size), 0)::bigint FROM (SELECT ALL")' $app -run TestAVersionMadeCurrentAgainCountsItsStoredFileOnce
mutate "a copy stores files without room" internal/attachments/attachments.go \
  's.replace("\tif err := requireRoomForCopy(ctx, tx, from, spaceID); err != nil {\n\t\treturn nil, err\n\t}\n", "")' $app -run 'TestEveryWayThatCopiesFilesIntoAFullSpaceIsRefusedWhole|TestALargeCopyReportsThePagesWhoseFilesNoLongerFit'
mutate "a branch moves into a full space" internal/attachments/attachments.go \
  's.replace("\tif err := requireRoom(ctx, tx, spaceID, size); err != nil {\n\t\treturn err\n\t}\n\t_, err := tx.Exec", "\t_ = size\n\t_, err := tx.Exec")' $app -run TestABranchMovesIntoAFullSpaceOnlyWithRoomForItsFiles
mutate "saving as a template makes the template before the refusal" internal/templates/templates.go \
  's.replace("s.pages.RequireRoomForFiles(ctx, pageID, detail.SpaceID)", "error(nil)")' $app -run TestEveryWayThatCopiesFilesIntoAFullSpaceIsRefusedWhole
mutate "the Files screen ignores a name long enough for near matches" internal/attachments/versions.go \
  's.replace("` AND (` + fileNameWords + ` OR ` + pastWords(f.Name, \"$9\", \"$10\") + `)` + filters", "filters")' $app -run TestTheFilesScreenFiltersAndOffersOnlyWhatMayBeSeen
mutate "a dialog is told the files fit when they do not" internal/attachments/limits.go \
  's.replace("return !limited || size <= left, nil", "return !limited || size <= left || true, nil")' $app -run 'TestTheCopyDialogIsWarnedWhenTheFilesDoNotFit|TestABranchMovesIntoAFullSpaceOnlyWithRoomForItsFiles'
mutate "an upgrade starts without pg_trgm" internal/app/upgrade.go \
  's.replace("\tcase extensionMissing:\n", "\tcase -1:\n")' ./internal/app/ -run TestTheChecksBeforeAnUpgradeSayWhyTheyRefuse
mutate "a typed underscore matches any letter" internal/search/words.go \
  's.replace(", `_`, `\\_`)", ")")' ./internal/search/ -run TestTypedSignsBecomeLettersInALikePattern
mutate "two letters look for near titles" internal/search/words.go \
  's.replace("return n >= minNearRunes && n <= maxNearRunes", "return n >= 0 && n <= maxNearRunes")' $app -run TestTheNearestTitleComesFirstAndShortTypingFindsNoNearMatch
mutate "four letters look for typos" internal/search/words.go \
  's.replace("minTypoRunes = 5", "minTypoRunes = 3")' $app -run TestTheNearestTitleComesFirstAndShortTypingFindsNoNearMatch
mutate "near titles are sorted by date alone" internal/search/words.go \
  's.replace("ORDER BY word_similarity(lower(@text), lower(w.title)) DESC, w.updated_at DESC", "ORDER BY w.updated_at DESC")' $app -run TestTheNearestTitleComesFirstAndShortTypingFindsNoNearMatch
mutate "a title that holds the typed text is not found" internal/search/words.go \
  's.replace("if err := p.addAll(ctx, p.sql.titles, containsTier); err != nil {", "if err := error(nil); err != nil {")' $app -run TestTitlesThatHoldTheTypedTextOrANearWordComeAfterTheWords
mutate "a typo in a title stands before a word in the text" internal/search/words.go \
  's.replace("if typos && q.Quick {", "if typos {")' $app -run TestTitlesThatHoldTheTypedTextOrANearWordComeAfterTheWords
mutate "the typo threshold lets other words through" internal/search/words.go \
  's.replace("const NearThreshold = \"0.4\"", "const NearThreshold = \"0.1\"")' $app -run TestTheThresholdFindsTheTyposAndLeavesTheDecoys
mutate "the Files screen orders names by date alone" internal/attachments/versions.go \
  's.replace("ORDER BY ` + fileNameWords + ` DESC, ` + nearNameOrder", "ORDER BY a.updated_at DESC, ` + nearNameOrder")' $app -run TestAFileNameIsFoundTheSameWayInTheBoxAndOnTheFilesScreen
mutate "the box finds no near file names" internal/attachments/attachments.go \
  's.replace("len(found) >= limit || !search.NearLength(text)", "len(found) >= 0")' $app -run TestAFileNameIsFoundTheSameWayInTheBoxAndOnTheFilesScreen
mutate_web "a floating bar shows in a code block" src/components/editor/floating-bars.tsx \
  's.replace("if (facts.empty || facts.inCode) return null;", "if (facts.empty) return null;")' src/components/editor/floating-bars.test.ts
mutate_web "a floating bar shows without a selection" src/components/editor/floating-bars.tsx \
  's.replace("if (facts.empty || facts.inCode) return null;", "if (facts.inCode) return null;")' src/components/editor/floating-bars.test.ts
mutate_web "a touch screen gets the bar over its own menu" src/components/editor/floating-bars.tsx \
  's.replace("placement: coarse ? (\x27bottom\x27 as const) : (\x27top\x27 as const),", "placement: \x27top\x27 as const,")' src/components/editor/floating-bars.test.ts
mutate_web "the bar may cover the sticky row" src/components/editor/floating-bars.tsx \
  's.replace("options.flip.padding.top = keepOut + EDGE;", "")' src/components/editor/floating-bars.test.ts
mutate_web "the arrow keys ignore a right-to-left page" src/components/editor/roving-focus.ts \
  's.replace("return rightToLeft ? previous : next;", "return next;")' src/components/editor/floating-bars.test.ts

# Spec 049: editing a database has its own permission, people hold several roles, combinations
# nobody named have short names, and an upgrade asks for a recent backup.
mutate "a database is edited with page.edit" internal/permissions/evaluate.go \
  's.replace("if p != nil && p.Kind == kindDatabase {", "if false {")' $perms -run TestDatabasesAndPagesAreEditedApart
mutate "page.edit ignores the kind of the item" internal/permissions/evaluate.go \
  's.replace("perm := spaceEditPermission(in.Page)", "perm := PageEdit")' $perms -run TestDatabasesAndPagesAreEditedApart
mutate "an edit restriction does not stop editing a database" internal/permissions/evaluate.go \
  's.replace("ActionAttachmentDelete, PageMoveOut, DatabaseEdit:", "ActionAttachmentDelete, PageMoveOut:")' $perms -run TestMatrixEvaluate
mutate "database.edit is not split off page.edit in Go" internal/permissions/evaluate.go \
  's.replace("\tDatabaseEdit:      PageEdit,\n", "")' $perms -run TestTheSplitIsTheSameInGoAndSQL
mutate "the built-in editor cannot edit databases" internal/permissions/manage.go \
  's.replace("PostCreate, DatabaseCreate,\n\t\tDatabaseEdit},", "PostCreate, DatabaseCreate},")' $perms -run TestBuiltInRolesMatchTheTemplates
mutate "the editable list ignores the kind" internal/db/migrations/0044_database_edit_and_role_names.sql \
  's.replace("AND CASE WHEN p.kind = '"'"'database'"'"'", "AND CASE WHEN false")' $perms -run TestDatabasesAndPagesAreEditedApart
mutate "database.edit is not split off page.edit in SQL" internal/db/migrations/0051_document_control.sql \
  's.replace("WHEN '"'"'page.edit'"'"' THEN ARRAY['"'"'page.move_out'"'"', '"'"'database.edit'"'"', ", "WHEN '"'"'page.edit'"'"' THEN ARRAY['"'"'page.move_out'"'"', ")' $perms -run TestTheSplitIsTheSameInGoAndSQL
mutate "the migration gives editors no database editing" internal/db/migrations/0044_database_edit_and_role_names.sql \
  's.replace("\nSELECT rbac_split_roles_from(", "\nSELECT 1, (")' $perms -run TestBuiltInRolesMatchTheTemplates
mutate "the migration runs every earlier split again" internal/db/migrations/0044_database_edit_and_role_names.sql \
  's.replace("UPDATE roles SET permissions = ARRAY(SELECT DISTINCT p FROM unnest(permissions || finer) p ORDER BY 1),", "UPDATE roles SET permissions = rbac_with_split_off(permissions || finer),")' $perms -run TestTheMigrationAddsOnlyDatabaseEditing
mutate "a bundle from before the split gives no database editing" internal/exchange/bundlein.go \
  's.replace("permissions.DatabaseEdit:      5,", "permissions.DatabaseEdit:      1,")' ./internal/exchange/ -run TestEverySplitPermissionKnowsTheBundleVersionThatHoldsIt
mutate "one permission is called permissions" internal/permissions/roles.go \
  's.replace("if permissionCount == 1 {", "if permissionCount < 1 {")' $perms -run TestUnnamedRoleNamesAreShortAndTheSameInGoAndSQL
mutate "the database names one permission permissions" internal/db/migrations/0044_database_edit_and_role_names.sql \
  's.replace("CASE WHEN perms = 1 THEN '"'"' permission)'"'"' ELSE '"'"' permissions)'"'"' END", "'"'"' permissions)'"'"'")' $perms -run TestUnnamedRoleNamesAreShortAndTheSameInGoAndSQL
mutate "a combination is named after its cut list again" internal/db/migrations/0044_database_edit_and_role_names.sql \
  's.replace("VALUES (found, sc, rbac_free_unnamed_name(sc, cardinality(wanted)),", "VALUES (found, sc, left('"'"'Custom: '"'"' || rbac_words(wanted), 90),")' $perms -run TestUnnamedRoleNamesAreShortAndTheSameInGoAndSQL
mutate "every combination gets number zero" internal/db/migrations/0044_database_edit_and_role_names.sql \
  's.replace("0) + 1", "0) + 0")' $perms -run TestUnnamedRoleNamesAreShortAndTheSameInGoAndSQL
mutate "combinations named before keep the cut list" internal/db/migrations/0044_database_edit_and_role_names.sql \
  's.replace("WHERE builtin IS NULL AND name LIKE '"'"'Custom: %'"'"'", "WHERE false AND name LIKE '"'"'Custom: %'"'"'")' $perms -run TestCombinationsNamedBeforeGetTheShortName
mutate "a name an administrator gave is renamed" internal/db/migrations/0044_database_edit_and_role_names.sql \
  's.replace("          AND description = '"'"'Made from permissions that were given one by one.'"'"'\n", "\n")' $perms -run TestCombinationsNamedBeforeGetTheShortName
mutate "no role is marked unnamed" internal/permissions/roles.go \
  's.replace("\t_, v.Unnamed = unnamedNumber(v.Name)\n", "")' $perms -run TestUnnamedRoleNamesAreShortAndTheSameInGoAndSQL
mutate "the count in an unnamed name goes stale" internal/permissions/roles.go \
  's.replace("unnamed && changed && in.Name == r.Name {", "false && unnamed && changed && in.Name == r.Name {")' $perms -run TestAnUnnamedRoleKeepsItsCountTrue
mutate "an upgrade asks for no backup unless told" internal/config/config.go \
  's.replace("l.note(key, defaultUpgradeBackupMaxAge.String())\n\t\treturn defaultUpgradeBackupMaxAge", "l.note(key, defaultUpgradeBackupMaxAge.String())\n\t\treturn 0")' ./internal/config/ -run TestTheBackupAgeForAnUpgradeIsADayByDefaultAndOffOnlyOnPurpose
mutate "off does not switch the backup check off" internal/config/config.go \
  's.replace("if strings.EqualFold(strings.TrimSpace(v), backupOff) {", "if false {")' ./internal/config/ -run TestTheBackupAgeForAnUpgradeIsADayByDefaultAndOffOnlyOnPurpose
mutate "an age of zero switches the backup check off" internal/config/config.go \
  's.replace("if err != nil || d <= 0 {", "if err != nil || d < 0 {")' ./internal/config/ -run TestTheBackupAgeForAnUpgradeIsADayByDefaultAndOffOnlyOnPurpose
mutate "the refusal does not say how to go on" internal/app/upgrade.go \
  's.replace("\"or set DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off to upgrade without one\"", "\"or switch the check off\"")' $app -run TestTheChecksBeforeAnUpgradeSayWhyTheyRefuse
mutate_web "adding a role takes the others away" src/views/access-roles.tsx \
  's.replace("return held.includes(added) ? held : [...held, added];", "return [added];")' src/views/access-roles.test.ts
mutate_web "taking one role away takes them all" src/views/access-roles.tsx \
  's.replace("return held.filter((id) => id !== removed);", "return [];")' src/views/access-roles.test.ts
# Spec 050: work that holds a connection never waits for a second one, every wait of a
# request for a connection has a limit, and the feed, the bell and file search ask the
# permission rule about few pages.
mutate "a new space reads the instance through a second connection" internal/features/features.go \
  's.replace("v, err := s.instanceIn(ctx, tx)", "v, err := s.Instance(ctx)")' $app -run TestManySpacesCreatedAtOnceAllFinish
mutate "a wait for a connection has no limit" internal/db/db.go \
  's.replace("bounded, cancel := context.WithTimeoutCause(ctx, watch.limit, errAcquireLimit)", "bounded, cancel := context.WithTimeoutCause(ctx, watch.limit*100000, errAcquireLimit)")' ./internal/db/ -run TestAWaitForAConnectionGivesUpAtTheLimitOfItsRequest
mutate "a wait that ran out is not noted" internal/db/db.go \
  's.replace("acquireLimitKey{}).(*limitWatch); ok {\n\t\t\twatch.ranOut.Store(true)", "acquireLimitKey{}).(*limitWatch); ok {\n\t\t\twatch.ranOut.Store(false)")' ./internal/db/ -run TestAWaitForAConnectionGivesUpAtTheLimitOfItsRequest
mutate "requests wait for a connection without a limit" internal/httpapi/server.go \
  's.replace("\t\twithAcquireLimit,\n", "")' $app -run TestARequestOnAFullPoolIsToldBusyInTime
mutate "a request on a full pool is not told busy" internal/httpapi/server.go \
  's.replace("case db.AcquireRanOut(r.Context()):", "case false:")' $app -run TestARequestOnAFullPoolIsToldBusyInTime
mutate "the feed shows an older version of a page" internal/feed/feed.go \
  's.replace("AND NOT EXISTS (SELECT 1 FROM page_versions n WHERE n.page_id = v.page_id AND n.number > v.number AND NOT n.silent", "AND NOT EXISTS (SELECT 1 FROM page_versions n WHERE false")' $app -run TestTheFeedGivesWhatTheFormerQueryGave
mutate "the feed shows pages the reader may not see" internal/feed/feed.go \
  's.replace("AND ` + visible(\"v.page_id\") + `", "AND true")' $app -run TestTheFeedGivesWhatTheFormerQueryGave
mutate "popular leaves out pages with only reactions" internal/feed/feed.go \
  's.replace("WHERE r.target_type = \x27page\x27 AND r.created_at", "WHERE r.target_type = \x27none\x27 AND r.created_at")' $app -run TestTheFeedGivesWhatTheFormerQueryGave
mutate "popular shows pages the reader may not see" internal/feed/feed.go \
  's.replace("FROM counted v\n\t\tWHERE ` + visible(\"v.page_id\") + `", "FROM counted v\n\t\tWHERE true")' $app -run TestTheFeedGivesWhatTheFormerQueryGave
mutate "the bell counts rows about hidden pages" internal/notify/list.go \
  's.replace("SELECT v.page_id FROM perm_visible_pages($1) v\n", "SELECT v.id AS page_id FROM pages v\n")' $app -run TestWatchersHearOnceAndLosingAccessEndsIt
mutate "an IP address is taken as the public address" internal/config/config.go \
  's.replace("case isAddress(u.Hostname()):", "case false && isAddress(u.Hostname()):")' ./internal/config/ -run TestAnIPAddressIsRefusedAsThePublicAddress

# Spec 051: the installation watches itself. Every threshold of the health checks, the guard
# that refuses files on a nearly full disk, the cleaner, and doctor.
health=./internal/health/
mutate "files are refused at exactly 2% free" internal/health/health.go \
  's.replace("percentFree(free, total) < DiskRefusePercent", "percentFree(free, total) <= DiskRefusePercent")' $health -run TestADiskAtTwoPercentWarnsButStillTakesFiles
mutate "an unknown disk size refuses files" internal/health/health.go \
  's.replace("return total > 0 && percentFree", "return total >= 0 \x26\x26 percentFree")' $health -run TestADiskOfUnknownSizeNeverWarnsAndNeverRefuses
mutate "a disk at 10% free warns" internal/health/health.go \
  's.replace("case percent < DiskWarnPercent:", "case percent <= DiskWarnPercent:")' $health -run TestADiskAtTenPercentIsFine
mutate "growth equal to the free room warns" internal/health/health.go \
  's.replace("uint64(f.Growth30d) > f.DiskFree:", "uint64(f.Growth30d) >= f.DiskFree:")' $health -run TestADiskThatAMonthOfGrowthWouldFillWarns
mutate "growth that is not known warns" internal/health/health.go \
  's.replace("case f.GrowthKnown \x26\x26 f.Growth30d > 0", "case f.Growth30d > 0")' $health -run TestGrowthThatIsNotKnownYetNeverWarns
mutate "no backup noted is fine" internal/health/health.go \
  's.replace("if f.LastBackup.IsZero() {\n\t\treturn check(\"backup\", Warn", "if false {\n\t\treturn check(\"backup\", Warn")' $health -run TestNoBackupNotedWarns
mutate "a backup of exactly 26 hours warns" internal/health/health.go \
  's.replace("if age > BackupWarnAfter {", "if age >= BackupWarnAfter {")' $health -run TestABackupOfTwentySixHoursIsStillFine
mutate "a contract migration of exactly a day warns" internal/health/health.go \
  's.replace("f.Now.Sub(f.LastMigrationAt) > f.ContractGrace+ContractWaitWarnAfter", "f.Now.Sub(f.LastMigrationAt) >= f.ContractGrace+ContractWaitWarnAfter")' $health -run TestAContractMigrationWaitingMoreThanADayWarns
mutate "an event stuck in the outbox is only a warning" internal/health/health.go \
  's.replace("case f.OutboxStuck > 0:", "case false:")' $health -run TestAnEventStuckInTheOutboxIsAProblem
mutate "failed jobs are fine" internal/health/health.go \
  's.replace("case f.FailedJobs7d > 0 || f.EventFailures > 0:", "case f.EventFailures > 0:")' $health -run TestFailedJobsOrEventsWarn
mutate "no mail server is fine" internal/health/health.go \
  's.replace("case !f.MailConfigured:", "case false:")' $health -run TestMailThatIsOffOrFailingWarns
mutate "a stopped password login is fine" internal/health/health.go \
  's.replace("if f.PasswordStops > 0 {", "if false {")' $health -run TestAnAccountWhosePasswordLoginStoppedWarns
mutate "a release of exactly 60 days warns" internal/health/health.go \
  's.replace("if age > ReleaseWarnAfter {", "if age >= ReleaseWarnAfter {")' $health -run TestAReleaseOlderThanSixtyDaysWarns
mutate "the guard lets every file through" internal/storage/guard.go \
  's.replace("\tif g.Refusing(ctx) {\n\t\treturn Object{}, errNoRoom", "\tif false {\n\t\treturn Object{}, errNoRoom")' ./internal/storage/ -run TestAStoreBelowTwoPercentRefusesFilesWithNoRoom
mutate "the guard reads the disk on every file" internal/storage/guard.go \
  's.replace("g.now().Sub(g.readAt) < roomFreshFor", "g.now().Sub(g.readAt) < 0")' ./internal/storage/ -run TestTheRoomIsReadAtMostEveryThirtySeconds
mutate "a refused file is a bad request" internal/httpapi/handlers_content.go \
  's.replace("\tcase apperr.KindNoRoom:\n\t\tstatus = http.StatusInsufficientStorage\n", "")' $app -run TestANearlyFullDiskRefusesFilesButTextStillSaves
mutate "a nearly full disk leaves no note on readyz" internal/app/health.go \
  's.replace("if a.guard != nil \x26\x26 a.guard.Refusing(ctx) {", "if false {")' $app -run TestANearlyFullDiskRefusesFilesButTextStillSaves
mutate "readyz drops its notes" internal/httpapi/server.go \
  's.replace("len(list) > 0 {", "len(list) > 99 {")' $app -run TestANearlyFullDiskRefusesFilesButTextStillSaves
mutate "the health page takes any event for a backup" internal/app/health.go \
  's.replace("FROM audit_events WHERE action = $1 ORDER BY seq DESC LIMIT 1", "FROM audit_events WHERE action <> $1 ORDER BY seq DESC LIMIT 1")' $app -run TestTheHealthPageShowsTheBackupAndTheDisk
mutate "the growth is measured from the newest sample" internal/app/health.go \
  's.replace("WHERE taken_at <= now() - make_interval(secs => $2)", "WHERE taken_at >= now() - make_interval(secs => $2)")' $app -run TestTheDatabaseGrowthComesFromTheHourlySamples
mutate "the cleaner keeps finished jobs" internal/app/health.go \
  's.replace("status = \x27done\x27 AND finished_at < now()", "status = \x27done\x27 AND finished_at > now()")' $app -run TestTheCleanerRemovesOnlyOldFinishedWork
mutate "failed mail goes as soon as sent mail" internal/app/health.go \
  's.replace("status = \x27failed\x27 AND created_at < now() - make_interval(secs => $1)\", keepFailedWork}", "status = \x27failed\x27 AND created_at < now() - make_interval(secs => $1)\", keepFinishedWork}")' $app -run TestTheCleanerRemovesOnlyOldFinishedWork
mutate "the cleaner keeps nodes that are gone" internal/app/health.go \
  's.replace("DELETE FROM nodes WHERE last_seen_at < now()", "DELETE FROM nodes WHERE last_seen_at > now()")' $app -run TestTheCleanerRemovesOnlyOldFinishedWork
mutate "doctor passes with a problem" cmd/diffluence/doctor.go \
  's.replace("\tif problems > 0 {\n\t\treturn errDoctorFound", "\tif false {\n\t\treturn errDoctorFound")' ./cmd/diffluence/ -run TestDoctorFailsWhenTheKeyIsMissing
mutate "doctor forgets a key it cannot read" cmd/diffluence/doctor.go \
  's.replace("if !doctorKeys(ctx, cfg, stdout) {", "if false \x26\x26 !doctorKeys(ctx, cfg, stdout) {")' ./cmd/diffluence/ -run TestDoctorFailsWhenTheKeyIsMissing

# Spec 053: upgrades made stronger.
db=./internal/db/
mutate "a migration runs without a time limit" internal/db/migrate.go \
  's.replace("SET LOCAL statement_timeout = %d", "SET LOCAL statement_timeout = 0 -- %d")' $db -run TestAMigrationThatRunsTooLongIsStoppedAndRolledBack
mutate "the header of a migration gives it no more time" internal/db/migrate.go \
  's.replace("\tif m.StatementTimeout > 0 {\n\t\treturn m.StatementTimeout", "\tif false {\n\t\treturn m.StatementTimeout")' $db -run TestTheHeaderGivesAMigrationMoreTime
mutate "a file without a transaction runs in one" internal/db/migrate.go \
  's.replace("\t\tif m.NoTransaction {\n\t\t\terr = applyAlone", "\t\tif false {\n\t\t\terr = applyAlone")' $db -run TestAConcurrentIndexIsBuiltOutsideATransaction
mutate "an index left invalid passes" internal/db/migrate.go \
  's.replace("\tif len(invalid) > 0 {", "\tif false {")' $db -run TestAnIndexLeftInvalidIsNamed
mutate "a file without a transaction may hold two statements" internal/db/migrate.go \
  's.replace("statementCount(m.SQL) != 1", "statementCount(m.SQL) < 1")' $db -run TestHeadersThatMakeNoSenseAreRefused
mutate "any word after transaction means none" internal/db/migrate.go \
  's.replace("; value != \"none\" {", "; value == \"\" \x26\x26 false {")' $db -run TestHeadersThatMakeNoSenseAreRefused
mutate "a new column type passes the rewrite check" internal/db/lint.go \
  's.replace("\t\tcase newType.MatchString(code):", "\t\tcase false:")' $db -run TestStatementsThatRewriteOrLongLockATableAreRefused
mutate "SET NOT NULL passes the rewrite check" internal/db/lint.go \
  's.replace("\t\tcase setNotNull.MatchString(code):", "\t\tcase false:")' $db -run TestStatementsThatRewriteOrLongLockATableAreRefused
mutate "a volatile default passes the rewrite check" internal/db/lint.go \
  's.replace("\t\tcase volatileDflt.MatchString(code):", "\t\tcase false:")' $db -run TestStatementsThatRewriteOrLongLockATableAreRefused
mutate "a stored column passes the rewrite check" internal/db/lint.go \
  's.replace("\t\tcase storedColumn.MatchString(code):", "\t\tcase false:")' $db -run TestStatementsThatRewriteOrLongLockATableAreRefused
mutate "an index that blocks writes passes the rewrite check" internal/db/lint.go \
  's.replace(" \x26\x26 !concurrently.MatchString(match[1]) {", " \x26\x26 false {")' $db -run TestStatementsThatRewriteOrLongLockATableAreRefused
mutate "a constraint checked at once passes the rewrite check" internal/db/lint.go \
  's.replace("\t\tcase checkedAtOnce.MatchString(code) \x26\x26 !notValid.MatchString(code):", "\t\tcase false:")' $db -run TestStatementsThatRewriteOrLongLockATableAreRefused
mutate "a constraint added NOT VALID is refused" internal/db/lint.go \
  's.replace(" \x26\x26 !notValid.MatchString(code):", ":")' $db -run TestSafeChangesPass
mutate "VACUUM FULL passes the rewrite check" internal/db/lint.go \
  's.replace("\tif vacuumFull.MatchString(code) {", "\tif false {")' $db -run TestStatementsThatRewriteOrLongLockATableAreRefused
mutate "CLUSTER passes the rewrite check" internal/db/lint.go \
  's.replace("} else if cluster.MatchString(code) {", "} else if false {")' $db -run TestStatementsThatRewriteOrLongLockATableAreRefused
mutate "a table made in the same file counts as busy" internal/db/lint.go \
  's.replace("\t\tcreated[tableKey(match[1])] = true", "\t\t_ = match")' $db -run TestSafeChangesPass
mutate "an exception without a reason passes" internal/db/lint.go \
  's.replace("rewrite-ok:[ \\t]*\\S", "rewrite-ok:")' $db -run TestAnExceptionNeedsAReason
mutate "an exception covers the whole file" internal/db/lint.go \
  's.replace("\t\tif rewriteOK.MatchString(statement) {", "\t\tif rewriteOK.MatchString(m.SQL) {")' $db -run TestAnExceptionNeedsAReason
mutate "migrations from before the rule are checked" internal/db/lint.go \
  's.replace("\tif m.Version < LintFrom {", "\tif false {")' $db -run TestEarlierMigrationsAreNotChecked
mutate "contract migrations ignore the grace" internal/app/ops.go \
  's.replace("time.Since(*youngestStart) < a.cfg.ContractGrace", "time.Since(*youngestStart) < 10*time.Minute")' $app -run TestContractMigrationsWaitForOlderNodes
mutate "the health check forgets the grace" internal/health/health.go \
  's.replace("> f.ContractGrace+ContractWaitWarnAfter", "> ContractWaitWarnAfter")' $health -run TestAContractMigrationWarnsADayAfterItsGrace
mutate "a negative grace is taken" internal/config/config.go \
  's.replace("\tif d < 0 {\n\t\tl.fail(key", "\tif false {\n\t\tl.fail(key")' ./internal/config/ -run TestContractMigrationsWaitADayByDefault
mutate "the grace is not a day by default" internal/config/config.go \
  's.replace("const defaultContractGrace = 24 * time.Hour", "const defaultContractGrace = 10 * time.Minute")' ./internal/config/ -run TestContractMigrationsWaitADayByDefault
mutate "a worker takes a job of any kind" internal/jobs/jobs.go \
  's.replace("AND kind = ANY($2) AND NOT kind", "AND $2::text[] IS NOT NULL AND NOT kind")' $app -run TestAJobOfANewerReleaseWaitsForANodeThatKnowsIt
mutate "saving settings drops the fields of a newer release" internal/settings/sections.go \
  's.replace("if _, ok := fresh[name]; !ok \x26\x26 !known[name] {", "if known[name] \x26\x26 false {")' $app -run TestSavingSettingsKeepsFieldsOfANewerRelease
mutate "an embedded section field counts as unknown" internal/settings/sections.go \
  's.replace("\t\tif f.Anonymous \x26\x26 !hasTag {", "\t\tif hasTag \x26\x26 false {")' ./internal/settings/ -run TestTheFieldsASectionKnowsIncludeOmittedAndEmbeddedOnes
mutate "a field left out by omitempty counts as unknown" internal/settings/sections.go \
  's.replace("\t\tname, _, _ := strings.Cut(tag, \",\")", "\t\tname := tag")' ./internal/settings/ -run TestTheFieldsASectionKnowsIncludeOmittedAndEmbeddedOnes

# Spec 054: a node waits instead of looping, the first upgrade without any backup goes on
# once on the record, and migrate asks for the backup too.
mutate "the first upgrade without any backup is refused" internal/app/upgrade.go \
  's.replace("case f.lastBackup.IsZero() \x26\x26 f.firstWithoutBackup:", "case false:")' $app -run TestTheChecksBeforeAnUpgradeSayWhyTheyRefuse
mutate "every upgrade without a backup is waived" internal/app/upgrade.go \
  's.replace("f.firstWithoutBackup = f.lastBackup.IsZero() \x26\x26 !waived", "f.firstWithoutBackup = f.lastBackup.IsZero()")' $app -run TestAnUpgradeWithoutTheBackupTheOperatorAskedForIsRefusedBeforeMigrating
mutate "the first upgrade without a backup leaves no record" internal/app/upgrade.go \
  's.replace("return maxAge > 0 \x26\x26 f.lastBackup.IsZero() \x26\x26 f.firstWithoutBackup", "return false")' $app -run TestTheFirstUpgradeWithoutAnyBackupGoesOnOnceAndIsRecorded
mutate "a refusal is a plain error a node cannot wait on" internal/app/upgrade.go \
  's.replace("return \x26UpgradeRefused{Problems: problems}", "return errors.New(\"upgrade refused before migrating: no backup was noted\")")' $app -run TestAnUpgradeWithoutTheBackupTheOperatorAskedForIsRefusedBeforeMigrating
mutate "migrate asks for no backup" internal/app/upgrade.go \
  's.replace("if p, refused := backupProblem(facts, time.Now(), maxAge); refused {\n\t\t\treturn", "if p, refused := backupProblem(facts, time.Now(), maxAge); false \x26\x26 refused {\n\t\t\treturn")' $app -run TestMigrateAsksForTheBackupAndGoesOnOnceWhenToldTo
mutate "migrate --without-backup leaves no record" internal/app/upgrade.go \
  's.replace("if err := recordWaiver(ctx, owner, log, \"cli\", waiverOperator); err != nil {", "if err := error(nil); err != nil {")' $app -run TestMigrateAsksForTheBackupAndGoesOnOnceWhenToldTo
mutate "the waiver of an upgrade waits for the sealer" internal/app/upgrade.go \
  's.replace("return audit.RecordSealed(ctx, owner, audit.Event{ActorType: audit.ActorSystem, ActorID: actor,", "return audit.RecordStandalone(ctx, owner, audit.Event{ActorType: audit.ActorSystem, ActorID: actor,")' $app -run TestTheFirstUpgradeWithoutAnyBackupGoesOnOnceAndIsRecorded
mutate "a refusal names a kind twice" internal/app/upgrade.go \
  's.replace("if !slices.Contains(kinds, p.Kind) {", "if !slices.Contains(kinds[:0], p.Kind) {")' $app -run TestARefusalNamesEachKindOnce
mutate "a waiting node is not alive" internal/app/waiting.go \
  's.replace("case r.URL.Path == \"/livez\":", "case false:")' $app -run TestAWaitingNodeIsAliveButNotReady
mutate "a waiting node gives no reasons to a load balancer" internal/app/waiting.go \
  's.replace("case r.URL.Path == \"/readyz\":", "case false:")' $app -run TestAWaitingNodeIsAliveButNotReady
mutate "the API of a waiting node says nothing of when to try again" internal/app/waiting.go \
  's.replace("h.Set(\"Retry-After\", waitingRetryAfter)\n\t\t\twriteWaitingJSON", "writeWaitingJSON")' $app -run TestTheAPIOfAWaitingNodeSaysToTryAgain
mutate "the waiting page shows the reasons in full" internal/app/waiting.go \
  's.replace("_ = waitingPage.Execute(w, waitingFixes(kinds))", "_ = waitingPage.Execute(w, []waitingFix{{Title: refused.Error()}})")' $app -run TestTheWaitingPageNamesTheFixButNoDetails
mutate "a retry keeps the waiting node" internal/app/waiting.go \
  's.replace("\tcase <-retry:\n", "\tcase <-make(chan struct{}):\n")' $app -run TestAWaitingNodeFreesItsAddressWhenItTriesAgain
mutate "SIGHUP does not end the wait" cmd/diffluence/main.go \
  's.replace("\t\tcase <-hup:\n", "\t\tcase <-make(chan os.Signal):\n")' ./cmd/diffluence/ -run TestAWaitingNodeTriesAgainOnSIGHUP
mutate "a stopped waiting node reports a failure" cmd/diffluence/main.go \
  's.replace("if errors.Is(err, context.Canceled) {\n\t\treturn nil", "if false {\n\t\treturn nil")' ./cmd/diffluence/ -run TestAWaitingNodeStoppedBySIGTERMExitsCleanly
# Spec 052, ADR-041: the audit chain is sealed a moment after saving.
audit=./internal/audit/
mutate "a save takes the chain lock again" internal/audit/audit.go \
  's.replace("\t_, err = tx.Exec(ctx, `INSERT INTO audit_pending", "\tif _, err := tx.Exec(ctx, \"SELECT pg_advisory_xact_lock($1)\", chainLockKey); err != nil {\n\t\treturn p.event, err\n\t}\n\t_, err = tx.Exec(ctx, `INSERT INTO audit_pending")' $audit -run TestASaveDoesNotWaitForTheChain
mutate "the sealer does not wait for the chain" internal/audit/seal.go \
  's.replace("tx.Exec(ctx, \"SELECT pg_advisory_xact_lock($1)\", chainLockKey); err != nil {\n\t\treturn 0,", "tx.Exec(ctx, \"SELECT $1::bigint\", chainLockKey); err != nil {\n\t\treturn 0,")' $audit -run TestTheSealerWaitsWhileTheChainIsHeld
mutate "a sealed event stays waiting" internal/audit/seal.go \
  's.replace("batch.Queue(\"DELETE FROM audit_pending WHERE pend = ANY($1)\", sealed)", "_ = sealed")' $audit -run TestVerifyCountsWaitingEvents
mutate "a long backlog stops after one step" internal/audit/seal.go \
  's.replace("if n < SealBatch {", "if n <= SealBatch {")' $audit -run TestABacklogLongerThanOneStepIsSealedWhole
mutate "waiting events are sealed newest first" internal/audit/seal.go \
  's.replace("payload, salt, data FROM audit_pending ORDER BY pend`", "payload, salt, data FROM audit_pending ORDER BY pend DESC`")' $audit -run TestEventsAreSealedInTheOrderTheyBecameVisible
mutate "after a prune of everything the chain starts again" internal/audit/seal.go \
  's.replace("\treturn start.prevHash, nil", "\treturn genesis, nil")' $audit -run TestTheChainGoesOnFromTheAnchorAfterEverythingWasPruned
mutate "a prune leaves events waiting" internal/audit/prune.go \
  's.replace("\t\t_, err = SealIn(ctx, tx, 0)", "\t\t_ = SealIn")' $audit -run TestTheChainGoesOnFromTheAnchorAfterEverythingWasPruned
mutate "erasing leaves the part of a waiting event" internal/audit/audit.go \
  's.replace("OR object_id = $1) AND salt IS NOT NULL`", "OR object_id = $1) AND salt IS NULL`")' $audit -run TestErasingAPersonReachesAWaitingEvent
mutate "a waiting event can be removed" internal/db/migrations/0047_audit_pending.sql \
  's.replace("IF EXISTS (SELECT 1 FROM audit_events WHERE id = OLD.id) THEN", "IF true THEN")' $audit -run TestAWaitingEventCannotBeRemovedOrChanged
mutate "a waiting event can be rewritten" internal/db/migrations/0047_audit_pending.sql \
  's.replace("IF NEW.salt IS NULL AND NEW.data IS NULL", "IF true OR NEW.salt IS NULL AND NEW.data IS NULL")' $audit -run TestAWaitingEventCannotBeRemovedOrChanged
mutate "a list leaves out waiting events" internal/audit/audit.go \
  's.replace("listWaitingSQL, false, f.Limit, keep)", "listWaitingSQL, false, 0, keep)")' $audit -run TestReadersSeeWaitingEventsAtOnce
mutate "an export leaves out waiting events" internal/audit/audit.go \
  's.replace("exportWaitingSQL, false, maxRows-sealed, write)", "exportWaitingSQL, false, 0, write)")' $audit -run TestReadersSeeWaitingEventsAtOnce
mutate "a check does not count waiting events" internal/audit/audit.go \
  's.replace("\"SELECT count(*) FROM audit_pending\").Scan(&res.Unsealed)", "\"SELECT 0::bigint\").Scan(&res.Unsealed)")' $audit -run TestVerifyCountsWaitingEvents
mutate "events waiting a minute do not warn" internal/health/health.go \
  's.replace("case lag > AuditSealWarnAfter:", "case lag > AuditSealProblemAfter:")' $health -run TestAuditEventsWaitingLongerThanAMinuteWarn
mutate "events waiting fifteen minutes need nobody" internal/health/health.go \
  's.replace("case lag > AuditSealProblemAfter:", "case lag > 2*AuditSealProblemAfter:")' $health -run TestAuditEventsWaitingLongerThanFifteenMinutesAreAProblem
mutate "a worker seals once a minute" internal/app/ops.go \
  's.replace("sealEvery = time.Second", "sealEvery = time.Minute")' $app -run TestAWorkerSealsEverySecond
mutate "a stopping worker leaves events behind" internal/app/ops.go \
  's.replace("if err := a.SealAudit(last); err != nil {", "if err := last.Err(); err != nil {")' $app -run TestAStoppingWorkerSealsWhatIsLeft
mutate "a checkpoint leaves out what waits" internal/app/ops.go \
  's.replace("// sealer already reached.\n\tif err := a.SealAudit(ctx); err != nil {\n\t\treturn err\n\t}", "// sealer already reached.")' $app -run TestTheWitnessLogShowsATailRemovedFromTheDatabase
mutate "a check leaves out what waits" internal/app/ops.go \
  's.replace("func (a *App) CheckAudit(ctx context.Context) (audit.VerifyResult, error) {\n\tif err := a.SealAudit(ctx); err != nil {\n\t\treturn audit.VerifyResult{}, err\n\t}", "func (a *App) CheckAudit(ctx context.Context) (audit.VerifyResult, error) {")' $app -run TestACheckSealsWhatWaitsFirst
mutate "the health page does not see what waits" internal/app/health.go \
  's.replace("(SELECT count(*) FROM audit_pending),\n\t(SELECT min(occurred_at) FROM audit_pending)", "(SELECT 0::bigint),\n\t(SELECT NULL::timestamptz)")' $app -run TestEventsLeftUnsealedShowOnTheHealthPage
mutate "the metrics say nothing waits long" internal/app/gauges.go \
  's.replace("Value: auditLag(f)}", "Value: 0}")' $app -run TestEventsLeftUnsealedShowOnTheHealthPage
mutate "a backup note waits for the sealer" internal/app/upgrade.go \
  's.replace("return audit.RecordSealed(ctx, pool, audit.Event{ActorType: audit.ActorSystem, ActorID: \"cli\",", "return audit.RecordStandalone(ctx, pool, audit.Event{ActorType: audit.ActorSystem, ActorID: \"cli\",")' $app -run TestTheHealthPageShowsTheBackupAndTheDisk

# Spec 056: the configuration file and diffluence config check.
cfgpkg=./internal/config/
mutate "an unknown key in the file is taken" internal/config/report.go \
  's.replace("\t\tif !l.asked[name] {", "\t\tif false {")' $cfgpkg -run TestAnUnknownKeyInTheFileIsRefusedWithItsLine
mutate "a setting written twice keeps the first" internal/config/file.go \
  's.replace("if earlier, ok := f.entries[name]; ok {", "if earlier, ok := f.entries[name]; ok \x26\x26 false {")' $cfgpkg -run TestASettingWrittenTwiceIsRefused
mutate "a group written twice is merged" internal/config/file.go \
  's.replace("if line, ok := seen[strings.ToUpper(k.Value)]; ok {", "if line, ok := seen[strings.ToUpper(k.Value)]; ok \x26\x26 false {")' $cfgpkg -run TestASettingWrittenTwiceIsRefused
mutate "a list is taken as one value" internal/config/file.go \
  's.replace("\t\tcase yaml.ScalarNode:\n", "\t\tcase yaml.ScalarNode, yaml.SequenceNode:\n")' $cfgpkg -run TestAListWhereOneValueBelongsIsRefused
mutate "the file wins over the environment" internal/config/config.go \
  's.replace("if (ok \x26\x26 v != \"\") || fok {", "if fok {")' $cfgpkg -run TestTheEnvironmentWinsOverTheFile
mutate "an empty variable hides the file" internal/config/config.go \
  's.replace("if (ok \x26\x26 v != \"\") || fok {", "if ok || fok {")' $cfgpkg -run TestAnEmptyVariableLeavesTheFileInEffect
mutate "a key and its _file both in the file are taken" internal/config/config.go \
  's.replace("\tcase has \x26\x26 hasFile:\n", "\tcase false:\n")' $cfgpkg -run TestAKeyAndItsFileVariantBothInTheFileAreRefused
mutate "a secret in a file others read is taken" internal/config/report.go \
  's.replace("if secretKeys[name] \x26\x26 e.text != \"\" \x26\x26 !l.file.private {", "if false {")' $cfgpkg -run TestASecretInAFileOthersCanReadIsRefused
mutate "every file counts as private" internal/config/file.go \
  's.replace("f.private = info.Mode().Perm()\x260o077 == 0", "f.private = info.Mode().Perm()\x260o077 == 0 || true")' $cfgpkg -run TestASecretInAFileOthersCanReadIsRefused
mutate "an error does not name the line of the file" internal/config/config.go \
  's.replace("if o := l.origins[key]; o.source == sourceFile {", "if o := l.origins[key]; false {")' $cfgpkg -run TestYAMLGuessesAreNotTaken
mutate "the report shows secrets" internal/config/report.go \
  's.replace("\tif !secretKeys[key] {\n\t\treturn value", "\tif true {\n\t\treturn value")' $cfgpkg -run TestTheReportNamesEverySourceAndHidesSecrets
mutate "a database address shows its password" internal/config/report.go \
  's.replace("return u.Redacted()", "return u.String()")' $cfgpkg -run TestTheReportNamesEverySourceAndHidesSecrets
mutate "the report hides that the environment overrides the file" internal/config/report.go \
  's.replace("\t\tif o.overruns {", "\t\tif false {")' $cfgpkg -run TestTheReportNamesEverySourceAndHidesSecrets
mutate "the mail server is not tried" internal/config/probe.go \
  's.replace("\tif c.SMTP.Host != \"\" {\n\t\trun(", "\tif false {\n\t\trun(")' $cfgpkg -run TestAMailServerThatDoesNotAnswerWarns
mutate "a discovery document of another issuer passes" internal/config/probe.go \
  's.replace("if strings.TrimSuffix(doc.Issuer, \"/\") != strings.TrimSuffix(issuer, \"/\") {", "if false {")' $cfgpkg -run TestADiscoveryDocumentOfAnotherIssuerWarns
mutate "a failed status of the provider is not named" internal/config/probe.go \
  's.replace("if resp.StatusCode != http.StatusOK {", "if false {")' $cfgpkg -run TestAnIdentityProviderWithoutADiscoveryDocumentWarns
mutate "the publish folder is not tried" internal/config/probe.go \
  's.replace("\tif c.PublishDir != \"\" {\n\t\trun(", "\tif false {\n\t\trun(")' $cfgpkg -run TestAPublishFolderThatCannotBeWrittenWarns
mutate "the test file stays in the publish folder" internal/config/probe.go \
  's.replace("if err := os.Remove(name); err != nil {\n\t\treturn errors.New(\"a test file", "if err := error(nil); err != nil {\n\t\treturn errors.New(\"a test file")' $cfgpkg -run TestTryingThePublishFolderLeavesNothingBehind
mutate "config check passes a configuration a start refuses" cmd/diffluence/config.go \
  's.replace("\t\treturn errConfigRefused\n", "\t\treturn nil\n")' ./cmd/diffluence/ -run TestConfigCheckExitsOneOnABadFile

# Spec 058: the console runs backups through a worker, shows the configuration and logs
# people out.
mutate "back up now can wait twice" internal/db/migrations/0048_backup_runs.sql \
  's.replace("CREATE UNIQUE INDEX backup_requests_one_open", "CREATE INDEX backup_requests_one_open")' $app -run TestBackUpNowCanWaitOnceAndIsLimitedPerHour
mutate "back up now has no hourly limit" internal/backups/backups.go \
  's.replace("if lastHour >= RequestsPerHour {", "if false {")' $app -run TestBackUpNowCanWaitOnceAndIsLimitedPerHour
mutate "a taken request is taken again" internal/backups/backups.go \
  's.replace("case err == nil \x26\x26 (claimed == nil || time.Since(*claimed) > staleClaim):", "case err == nil:")' $app -run TestTheWorkerFollowsTheConsole
mutate "the console keep is ignored" internal/backups/backups.go \
  's.replace("\tif b.Keep > 0 {", "\tif false {")' $app -run TestTheWorkerFollowsTheConsole
mutate "the worker waits the whole interval" internal/backups/backups.go \
  's.replace("out.Wait = min(due, pollAtMost)", "out.Wait = due")' $app -run TestTheWorkerFollowsTheConsole
mutate "a report of any name is taken" internal/backups/backups.go \
  's.replace("\tif !backupName.MatchString(r.Name) {\n\t\treturn apperr.Invalid", "\tif false {\n\t\treturn apperr.Invalid")' $app -run TestAWorkerReportIsCheckedAndKeptPlain
mutate "a message keeps its line breaks" internal/backups/backups.go \
  's.replace("\t\tif unicode.IsControl(r) {", "\t\tif unicode.IsControl(r) \x26\x26 false {")' $app -run TestAWorkerReportIsCheckedAndKeptPlain
mutate "a failed backup does not warn" internal/health/health.go \
  's.replace("\tcase f.BackupFailed:", "\tcase false:")' $app -run TestFailedBackupsAndRestoresShowOnTheHealthPage
mutate "a failed test restore does not warn" internal/health/health.go \
  's.replace("\tcase f.RestoreCheckFailed:", "\tcase false:")' $app -run TestFailedBackupsAndRestoresShowOnTheHealthPage
mutate "every failure mails at once" internal/backups/backups.go \
  's.replace("if sentAt == nil || time.Since(*sentAt) >= mailAtMostEvery {", "if true {")' $app -run TestAFailedBackupMailsTheAdministratorsAtMostTwiceADay
mutate "a failure mails people who are not administrators" internal/backups/backups.go \
  's.replace("AND u.anonymised_at IS NULL AND perm_has_global(u.id, \x27administrator\x27)", "AND u.anonymised_at IS NULL")' $app -run TestAFailedBackupMailsTheAdministratorsAtMostTwiceADay
mutate "no address logs everyone out" internal/httpapi/handlers_running.go \
  's.replace("if (strings.TrimSpace(in.Email) == \"\") == !in.All {", "if false {")' $app -run TestAnAdministratorLogsPeopleOutFromTheConsole
# Spec 059: a restore over the live installation from the console.
mutate "a restore takes any name typed" internal/restore/restore.go \
  's.replace("if strings.TrimSpace(a.Confirm) != a.Installation {", "if false {")' $app -run TestOneRestoreAtATimeAndOneADay
mutate "a restore takes a backup that never completed" internal/restore/restore.go \
  's.replace("if !complete {", "if false {")' $app -run TestOnlyAnAdministratorRestoresACompleteBackup
mutate "a restore runs beside a backup asked for" internal/restore/restore.go \
  's.replace("if backingUp {", "if false {")' $app -run TestOneRestoreAtATimeAndOneADay
mutate "two restores a day" internal/restore/restore.go \
  's.replace("if today >= PerDay {", "if today > PerDay {")' $app -run TestOneRestoreAtATimeAndOneADay
mutate "a failed restore lets the nodes serve" internal/restore/restore.go \
  's.replace("StateRestoring, StateChecking, StateFailed))", "StateRestoring, StateChecking, StateChecking))")' $app -run TestARestoreThatFailsHalfwayKeepsTheNodesWaitingUntilTheWayBack
mutate "the nodes wait for a worker that died" internal/restore/restore.go \
  's.replace("(state IN ($1, $2) AND updated_at > now() - make_interval(secs => $3))", "(state IN ($1, $2) AND $3::float8 > 0)")' $app -run TestARestoreThatStoodStillLongBeforeAnyChangeIsNotWaitedFor
mutate "a restore that changed something ends as refused" internal/restore/restore.go \
  's.replace("state = CASE WHEN changed THEN $3 ELSE $4 END,", "state = $4,")' $app -run TestARestoreThatFailsHalfwayKeepsTheNodesWaitingUntilTheWayBack
mutate "restoring is not marked as a change" internal/restore/restore.go \
  's.replace("changed = changed OR $2 = $4", "changed = changed OR $2 = $3")' $app -run TestARestoreThatFailsHalfwayKeepsTheNodesWaitingUntilTheWayBack
mutate "a backup of a newer release is restored" internal/restore/run.go \
  's.replace("if newest > r.NewestKnown {", "if newest > r.NewestKnown+1 {")' $app -run TestABackupOfANewerReleaseIsRefused
mutate "the worker restores under a serving node" internal/restore/run.go \
  's.replace("if err := WaitForNodes(ctx, r.Pool, r.StopWithin, r.Poll); err != nil {", "if err := error(nil); err != nil {")' $app -run TestANodeThatKeepsServingStopsTheRestore
mutate "a password alone confirms a restore" internal/identity/tokens.go \
  's.replace("if method != nil && *method == ConfirmedByPasskey && at != nil && s.now().Sub(*at) <= ReauthWindow {", "if true {")' $app -run TestARestoreAsksForASecondFactor
mutate "a wrong code confirms a restore" internal/identity/tokens.go \
  's.replace("\t\t\tif method == \"\" {\n\t\t\t\treturn secondFactorRequired(", "\t\t\tif false {\n\t\t\t\treturn secondFactorRequired(")' $app -run TestARestoreAsksForASecondFactor
mutate "a typing error costs the code" internal/httpapi/handlers_running.go \
  's.replace("if strings.TrimSpace(in.Confirm) != installation {", "if false {")' $app -run TestARestoreAsksForTheNameOfTheInstallation
mutate "a node starts to serve during a restore" internal/app/app.go \
  's.replace("if a.restoreBegan(ctx) {", "if false {")' $app -run TestServeRefusesToServeDuringARestore
mutate "a serving node ignores a restore" internal/app/restoring.go \
  's.replace("\t\t\tclose(started)\n", "\t\t\t_ = started\n")' $app -run TestAServingNodeStopsWhenARestoreBegins
mutate "a waiting node never serves again" internal/app/restoring.go \
  's.replace("\t\tif now == nil {\n", "\t\tif now == nil {\n\t\t\tcontinue\n\t\t}\n\t\tif false {\n")' $app -run TestANodeWaitsOnThePageWhileARestoreRunsAndGoesOnAfter
mutate "a restore makes the schema ops again" cmd/diffluence/restore.go \
  's.replace("slices.Contains(fields, \"ops\") || makesPublic", "(false \x26\x26 slices.Contains(fields, \"ops\")) || makesPublic")' ./cmd/diffluence/ -run TestARestoreLeavesTheSchemaOpsAlone
mutate "the last migration in a dump counts, not the newest" cmd/diffluence/restore.go \
  's.replace("newest = max(newest, v)", "newest = v")' ./cmd/diffluence/ -run TestTheNewestMigrationIsReadFromTheDump
mutate "the files step empties any folder" cmd/diffluence/restore.go \
  's.replace("if keyErr != nil \x26\x26 markErr != nil {", "if keyErr != nil \x26\x26 markErr != nil \x26\x26 false {")' ./cmd/diffluence/ -run TestTheFilesStepEmptiesOnlyADataFolder
mutate "a backup name may hold a backslash" cmd/diffluence/restore.go \
  's.replace("strings.ContainsAny(name, `/\\`)", "false")' ./cmd/diffluence/ -run TestABackupNameStaysInTheBackupFolder

# Spec 061: reads fast on real data, and no read can hold the database for long.
mutate "a query of a request has no limit" internal/db/db.go \
  's.replace("bounded, cancel := context.WithTimeoutCause(ctx, watch.limit, errQueryLimit)", "bounded, cancel := context.WithTimeoutCause(ctx, watch.limit*1000, errQueryLimit)")' ./internal/db/ -run TestAQueryOfARequestStopsAtItsLimit
mutate "a query that ran out is not noticed" internal/db/db.go \
  's.replace("errors.Is(context.Cause(ctx), errQueryLimit)", "false")' ./internal/db/ -run TestAQueryOfARequestStopsAtItsLimit
mutate "a query that failed on its own counts as running out" internal/db/db.go \
  's.replace("data.Err != nil \x26\x26 errors.Is(context.Cause(ctx), errQueryLimit)", "data.Err != nil")' ./internal/db/ -run TestAQueryThatFailsForAnotherReasonDoesNotCountAsRunningOut
mutate "the pool reuses generic plans" internal/db/db.go \
  's.replace("cfg.ConnConfig.RuntimeParams[\"plan_cache_mode\"] = \"force_custom_plan\"", "")' ./internal/db/ -run TestThePoolStillAsksForBothSettings
mutate "the pool compiles plans" internal/db/db.go \
  's.replace("cfg.ConnConfig.RuntimeParams[\"jit\"] = \"off\"", "")' ./internal/db/ -run TestThePoolStillAsksForBothSettings
mutate "for you leaves out watched spaces" internal/feed/feed.go \
  's.replace("WHERE w.user_id = $1 AND w.target_type = \x27space\x27", "WHERE w.user_id = $1 AND w.target_type = \x27none\x27")' $app -run TestTheFeedGivesWhatTheFormerQueryGave
mutate "for you leaves out watched branches" internal/feed/feed.go \
  's.replace("WHERE w.user_id = $1 AND w.target_type = \x27branch\x27", "WHERE w.user_id = $1 AND w.target_type = \x27none\x27")' $app -run TestTheFeedGivesWhatTheFormerQueryGave
mutate "for you leaves out followed people" internal/feed/feed.go \
  's.replace("WHERE f.user_id = $1\n", "WHERE false\n")' $app -run TestTheFeedGivesWhatTheFormerQueryGave
mutate "for you shows an older version" internal/feed/feed.go \
  's.replace("ORDER BY n.number DESC LIMIT 1) v", "ORDER BY n.number LIMIT 1) v")' $app -run TestTheFeedGivesWhatTheFormerQueryGave
mutate "the feed shows what one may not see" internal/feed/feed.go \
  's.replace("THEN true END)", "THEN true ELSE true END)")' $app -run TestTheFeedGivesWhatTheFormerQueryGave
mutate "popular counts no comments" internal/feed/feed.go \
  's.replace("SELECT m.page_id, 0, 1 FROM comments m", "SELECT m.page_id, 0, 0 FROM comments m")' $app -run TestTheFeedGivesWhatTheFormerQueryGave
# Spec 060: traces with OpenTelemetry, off by default, never with content.
tracing=./internal/tracing/
mutate "a share of NaN is taken" internal/config/config.go \
  's.replace("if err != nil || !(parsed >= 0 \x26\x26 parsed <= 1) {", "if err != nil || parsed > 1 {")' ./internal/config/ -run TestTracingIsOffByDefaultAndItsShareIsFromZeroToOne
mutate "every new trace is kept by default" internal/config/config.go \
  's.replace("const defaultTracingSample = 0.1", "const defaultTracingSample = 1")' ./internal/config/ -run TestTracingIsOffByDefaultAndItsShareIsFromZeroToOne
mutate "a span runs while tracing is off" internal/tracing/tracing.go \
  's.replace("func Span(ctx context.Context, what, which string) (context.Context, End) {\n\tif !On() {", "func Span(ctx context.Context, what, which string) (context.Context, End) {\n\tif false {")' $tracing -run TestOffDoesNothingAndChangesNothing
mutate "a failed span is not marked" internal/tracing/tracing.go \
  's.replace("span.SetStatus(codes.Error, \"error\")", "span.SetStatus(codes.Unset, \"error\")")' $tracing -run TestSpanRecordsAFailureWithoutItsMessageAndACount
mutate "a count is not recorded" internal/tracing/tracing.go \
  's.replace("trace.SpanFromContext(ctx).SetAttributes(attribute.Int(key, n))", "_ = n")' $tracing -run TestSpanRecordsAFailureWithoutItsMessageAndACount
mutate "a sampled trace that arrives is cut" internal/tracing/tracing.go \
  's.replace("sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(o.Sample))))", "sdktrace.WithSampler(sdktrace.TraceIDRatioBased(o.Sample)))")' $tracing -run TestATraceThatArrivesSampledStaysSampled
mutate "export failures fill the log" internal/tracing/tracing.go \
  's.replace("if now := time.Now(); now.Sub(q.last) >= q.every {", "if now := time.Now(); now.After(q.last) {")' $tracing -run TestExportFailuresAreLoggedAtMostOncePerPeriod
mutate "a request span is not named by its route" internal/tracing/http.go \
  's.replace("\tspan.SetName(pattern)\n", "")' $tracing -run TestARequestSpanIsNamedByItsRouteAndHoldsNoPath
mutate "a request span holds the path" internal/tracing/http.go \
  's.replace("trace.WithAttributes(attribute.String(\"http.request.method\", method)))", "trace.WithAttributes(attribute.String(\"http.request.method\", method), attribute.String(\"url.path\", r.URL.String())))")' $tracing -run TestARequestSpanIsNamedByItsRouteAndHoldsNoPath
mutate "any method names a span" internal/tracing/http.go \
  's.replace("method := methodName(r.Method)", "method := r.Method")' $tracing -run TestARequestWithoutARouteKeepsItsMethod
mutate "a server error does not mark its span" internal/tracing/http.go \
  's.replace("if rec.status >= http.StatusInternalServerError {", "if rec.status > 999 {")' $tracing -run TestAServerErrorMarksTheSpan
mutate "a query span names what follows FROM" internal/tracing/db.go \
  's.replace("if after[strings.ToUpper(w)] \x26\x26 isIdentifier(words[i+1]) {", "if after[strings.ToUpper(w)] {")' $tracing -run TestAStatementIsNamedByOperationAndTableOnly
mutate "a query span names any first word" internal/tracing/db.go \
  's.replace("if !operations[operation] {", "if false {")' $tracing -run TestAStatementIsNamedByOperationAndTableOnly
mutate "a database error is described by its detail" internal/tracing/db.go \
  's.replace("return \"sqlstate \" + pg.Code", "return pg.Detail")' $tracing -run TestAnErrorIsDescribedByItsKindOnly
mutate "a query in a trace not kept makes a span" internal/tracing/db.go \
  's.replace("parent.IsValid() \x26\x26 !parent.IsSampled() {", "false \x26\x26 parent.IsValid() {")' $tracing -run TestAQueryInATraceNotKeptMakesNoSpan
mutate "the end of a query ends the span around it" internal/tracing/db.go \
  's.replace("span, ok := ctx.Value(querySpan{}).(trace.Span)\n\tif !ok {\n\t\treturn\n\t}", "span := trace.SpanFromContext(ctx)")' $tracing -run TestAQueryInATraceNotKeptMakesNoSpan
mutate "an event leaves without its trace" internal/tracing/queue.go \
  's.replace("\tpropagator.Inject(ctx, propagation.HeaderCarrier(http.Header(headers)))\n", "")' $tracing -run TestAnEventCarriesItsTraceThroughTheQueue
mutate "an event is handled outside its trace" internal/tracing/queue.go \
  's.replace("\tctx = propagator.Extract(ctx, propagation.HeaderCarrier(http.Header(headers)))\n", "")' $tracing -run TestAnEventCarriesItsTraceThroughTheQueue
mutate "a log line does not name its span" internal/tracing/log.go \
  's.replace("r.AddAttrs(slog.String(\"trace_id\", sc.TraceID().String()), slog.String(\"span_id\", sc.SpanID().String()))", "_ = sc")' $tracing -run TestALogLineNamesItsSpan
mutate "a logger with attributes loses the span" internal/tracing/log.go \
  's.replace("return logHandler{h.Handler.WithAttrs(attrs)}", "return h.Handler.WithAttrs(attrs)")' $tracing -run TestALogLineNamesItsSpan
mutate "the pool makes no query spans" internal/db/db.go \
  's.replace("\t\tctx = q.trace.TraceQueryStart(ctx, conn, d)\n", "")' $app -run TestTracesOfTheWholeApplicationHoldNoContent
mutate "requests are not traced" internal/httpapi/server.go \
  's.replace("\t\ttraced = append(traced, tracing.Middleware)\n", "\t\t_ = traced\n")' $app -run TestTracesOfTheWholeApplicationHoldNoContent
mutate "the router does not name the span" internal/httpapi/router.go \
  's.replace("\t\ttracing.NameRoute(req.Context(), pattern)\n", "")' $app -run TestTracesOfTheWholeApplicationHoldNoContent
mutate "a listed log event is renamed" internal/queue/queue.go \
  's.replace("\"nats disconnected\"", "\"the queue went away\"")' $app -run TestEveryListedLogEventIsStillWritten
mutate "the sealer is not traced" internal/app/ops.go \
  's.replace("ctx, end := tracing.Span(ctx, \"audit seal\", \"\")", "ctx, end := tracing.Span(ctx, \"audit\", \"\")")' $app -run TestTracesOfTheWholeApplicationHoldNoContent

# Spec 063: the event stream keeps a copy per NATS node, and the database holds the
# settings PgBouncer would drop at connect.
mutate "five copies are refused" internal/config/config.go \
  's.replace("c.NATSReplicas != 3 \x26\x26 c.NATSReplicas != 5:", "c.NATSReplicas != 3:")' ./internal/config/ -run TestAClusterKeepsThreeOrFiveCopies
mutate "the embedded server takes three copies" internal/config/config.go \
  's.replace("case c.NATSReplicas > 1 \x26\x26 c.NATSURL == \"\":", "case false:")' ./internal/config/ -run TestTheEmbeddedServerRefusesMoreThanOneCopy
mutate "the stream ignores the copies it is told to keep" internal/queue/queue.go \
  's.replace("\t\tReplicas:   want,\n", "\t\tReplicas:   1,\n")' ./internal/queue/ -run TestOneServerRefusesThreeCopies
mutate "a node lowers the copies of the stream" internal/queue/queue.go \
  's.replace("return max(existing, configured)", "return configured")' ./internal/queue/ -run TestTheStreamGetsMoreCopiesButNeverFewer
mutate "the database does not hold plan_cache_mode" internal/db/migrations/0050_session_settings.sql \
  's.replace("SET plan_cache_mode = force_custom_plan", "SET application_name = force_custom_plan")' ./internal/db/ -run TestANewSessionPlansWithItsValuesWithoutAskingForIt
mutate "the database does not hold jit" internal/db/migrations/0050_session_settings.sql \
  's.replace("SET jit = off", "SET application_name = off")' ./internal/db/ -run TestANewSessionPlansWithItsValuesWithoutAskingForIt
mutate "a role that does not own the database stops the migration" internal/db/migrations/0050_session_settings.sql \
  's.replace("EXCEPTION WHEN insufficient_privilege THEN", "EXCEPTION WHEN division_by_zero THEN")' ./internal/db/ -run TestTheSessionSettingsAskNothingTheOwnerMayNotDo

# Spec 065: a database restored into a fresh one gets the defaults of 0050 again.
mutate "applying the session settings again does nothing" internal/db/session_settings.go \
  's.replace("_, err := q.Exec(ctx, SessionSettingsSQL)", "_, err := q.Exec(ctx, \"SELECT 1\")")' ./internal/db/ -run TestTheSessionSettingsComeBackOnADatabaseThatLostThem
mutate "a node on a restored database leaves the settings lost" internal/app/upgrade.go \
  's.replace("though it is recorded.\n\t\treturn db.ApplySessionSettings(ctx, owner)", "though it is recorded.\n\t\treturn nil")' $app -run TestANodeStartingOnARestoredDatabaseSetsTheSessionSettingsAgain
mutate "migrate leaves the settings lost" cmd/diffluence/main.go \
  's.replace("if err := db.ApplySessionSettings(ctx, pool); err != nil {", "if err := error(nil); err != nil {")' ./cmd/diffluence/ -run TestMigrateSetsTheSessionSettingsAgainWithNothingToMigrate
# Spec 064, wave DC1: readers see approved versions only, every change starts review again,
# a publish supersedes the round, a space workflow wins, and no workflow lacks a way out.
mutate "a never-approved page shows to readers" internal/permissions/evaluate.go \
  's.replace("\tif p.ControlHidden {", "\tif false {")' $perms -run TestMatrixEvaluate
mutate "an edit restriction does not stop an author seeing the head" internal/permissions/evaluate.go \
  's.replace("return len(in.Page.EditSubjects) == 0 || in.User.matchesAny(in.Page.EditSubjects)", "return true")' $perms -run TestMatrixEvaluate
mutate "the visible list shows hidden controlled pages" internal/db/migrations/0051_document_control.sql \
  's.replace("      AND p.id NOT IN (SELECT c.page_id FROM perm_control_hidden_pages(u) c)\n", "")' $perms -run TestMatrixSQLAgreesWithEvaluate
mutate "a withdrawn page shows to readers in SQL" internal/db/migrations/0051_document_control.sql \
  's.replace("WHERE pc.approved_version IS NULL OR pc.state_kind = \x27obsolete\x27))", "WHERE pc.approved_version IS NULL))")' $perms -run TestMatrixSQLAgreesWithEvaluate
mutate "a change to approved content reaches readers" internal/pages/pages.go \
  's.replace("\tif controlled \x26\x26 ctl.approved() {\n\t\tworking := p", "\tif false {\n\t\tworking := p")' $app -run TestReadersSeeTheApprovedVersionWhileAChangeWaits
mutate "a change does not start review again" internal/pages/pages.go \
  's.replace("return working, s.restartReview(ctx, tx, c, p, ctl, number, cause)", "return working, nil")' $app -run TestWhatRestartsReview
mutate "an unchanged text still waits for review" internal/pages/control.go \
  's.replace("\t\tif same {\n", "\t\tif same \x26\x26 false {\n")' $app -run TestWhatRestartsReview
mutate "a publish leaves the round open" internal/pages/control.go \
  's.replace("\tif err := endRound(ctx, tx, p.ID, roundStatusSuperseded, cause); err != nil {", "\tif err := error(nil); err != nil {")' $app -run TestAPublishSupersedesTheRoundAndItsAnswers
mutate "an answer about a replaced text counts" internal/pages/control.go \
  's.replace("\t\tif in.Version != ctl.headVersion {", "\t\tif false {")' $app -run TestAPublishSupersedesTheRoundAndItsAnswers
mutate "a reader gets the head" internal/pages/control.go \
  's.replace("\tif !d.Can[permissions.PageEdit] {", "\tif false {")' $app -run TestReadersSeeTheApprovedVersionWhileAChangeWaits
mutate "a reader reaches every version of a controlled page" internal/pages/control.go \
  's.replace("return historyView{only: max(currentVersion, 1)}, nil", "return historyView{}, nil")' $app -run TestReadersSeeTheApprovedVersionWhileAChangeWaits
mutate "the feed shows a version waiting for review" internal/feed/feed.go \
  's.replace("WHERE NOT v.silent AND ` + readable(\"v\") + `", "WHERE NOT v.silent")' $app -run TestTheFeedNeverShowsAVersionWaitingForReview
mutate "a space workflow keeps the page workflows" internal/pages/assign.go \
  's.replace("\"DELETE FROM page_workflows pw USING pages p WHERE p.id = pw.page_id AND p.space_id = $1\"", "\"SELECT $1::uuid\"")' $app -run TestTheSpaceWorkflowOverridesPageWorkflows
mutate "a page gets its own workflow under a space workflow" internal/pages/assign.go \
  's.replace("\t\tif spaceWorkflow {", "\t\tif false {")' $app -run TestTheSpaceWorkflowOverridesPageWorkflows
mutate "a branch takes over page workflows unasked" internal/pages/assign.go \
  's.replace("len(replaced) \x3e 0 \x26\x26 !in.ReplaceChildren \x26\x26 !in.Preview", "false")' $app -run TestABranchWorkflowFollowsMovesInAndOut
mutate "a workflow may lack a way to approval" internal/workflow/workflow.go \
  's.replace("\t\tif !leadsToApproval(s.ID, next, states) {", "\t\tif false {")' ./internal/workflow/ -run TestNoAcceptedWorkflowLacksAWayToApproval
mutate "a state may be unreachable" internal/workflow/workflow.go \
  's.replace("\t\tif !reached[s.ID] {", "\t\tif !reached[s.ID] \x26\x26 false {")' ./internal/workflow/ -run TestEachRuleOfAWorkflowIsChecked
mutate "page.workflow is not split off page.edit in SQL" internal/db/migrations/0051_document_control.sql \
  's.replace(", \x27database.edit\x27, \x27page.workflow\x27]", ", \x27database.edit\x27]")' $perms -run TestTheSplitIsTheSameInGoAndSQL
mutate "the migration gives administrators no workflows" internal/db/migrations/0051_document_control.sql \
  's.replace("SELECT rbac_split_roles_from(\x27space.administer\x27, ARRAY[\x27space.workflows\x27]);", "SELECT 1;")' $perms -run TestBuiltInRolesMatchTheTemplates
mutate "a bundle from before spec 064 gives editors no workflows" internal/exchange/bundlein.go \
  's.replace("permissions.PageWorkflow:      6,", "permissions.PageWorkflow:      1,")' ./internal/exchange/ -run TestABundleOfVersionFiveGivesEditorsAndAdministratorsTheWorkflowPermissions
finish_mutations
if [ "$survived" -gt 0 ]; then
  echo "$survived mutation(s) were not caught"
  exit 1
fi
# Nothing run is not a pass: say so, so a builder never reads it as one.
if [ "$ran" -eq 0 ]; then
  echo "no mutation ran for the changed paths"
  exit 2
fi
echo "all mutations were caught ($ran run)"
