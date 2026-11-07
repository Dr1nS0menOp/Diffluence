#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Do the tests catch real bugs? This script breaks a security rule on purpose,
# in a throw-away copy of the code, and fails when the tests stay green.
# Run it in the development toolbox: scripts/dev bash scripts/mutation-check.sh
set -uo pipefail

src=$(pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
survived=0

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

# mutate_web <name> <file below web> <expression on s> <test file>: the same for the frontend.
mutate_web() {
  local name=$1 file=$2 expr=$3 test=$4
  rm -rf "$work/web"
  mkdir -p "$work/web"
  tar -C "$src/web" --exclude=./node_modules --exclude=./e2e --exclude=./test-results -cf - . | tar -C "$work/web" -xf -
  ln -s "$src/web/node_modules" "$work/web/node_modules"
  if ! apply "$work/web/$file" "$expr"; then
    echo "BROKEN  $name (the mutation no longer applies; update this script)"
    survived=$((survived + 1))
    return
  fi
  if (cd "$work/web" && npx vitest run "$test" >"$work/out.txt" 2>&1); then
    echo "MISSED  $name"
    survived=$((survived + 1))
  elif grep -a -q -E 'Tests .*failed' "$work/out.txt"; then
    echo "caught  $name"
  else
    echo "BROKEN  $name (no test failed: $(grep -a -m1 . "$work/out.txt"))"
    survived=$((survived + 1))
  fi
}

# mutate <name> <file> <python expression on s> <go test args...>
mutate() {
  local name=$1 file=$2 expr=$3
  shift 3
  rm -rf "$work/repo"
  mkdir -p "$work/repo"
  tar -C "$src" --exclude=./web --exclude=./collab --exclude=./bin --exclude=./.git -cf - . | tar -C "$work/repo" -xf -
  if ! apply "$work/repo/$file" "$expr"; then
    echo "BROKEN  $name (the mutation no longer applies; update this script)"
    survived=$((survived + 1))
    return
  fi
  # A mutation that does not compile proves nothing, so it must not count as caught.
  if ! (cd "$work/repo" && go vet ./internal/... >"$work/out.txt" 2>&1); then
    echo "BROKEN  $name (the mutated code does not compile: $(grep -a -m1 -E '\.go:' "$work/out.txt"))"
    survived=$((survived + 1))
    return
  fi
  if (cd "$work/repo" && go test -count=1 "$@" >"$work/out.txt" 2>&1); then
    echo "MISSED  $name"
    survived=$((survived + 1))
  elif failed=$(grep -a -m1 -E '^--- FAIL' "$work/out.txt"); then
    echo "caught  $name: $failed"
  else
    echo "BROKEN  $name (no test failed: $(grep -a -m1 . "$work/out.txt"))"
    survived=$((survived + 1))
  fi
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
  's.replace("if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {\n\t\treturn Detail{}, err\n\t}", "")' $app
mutate "downloads skip the check" internal/attachments/attachments.go \
  's.replace("if err := s.perms.RequirePage(ctx, s.pool, c.UserID, a.PageID, permissions.ActionPageView, uuid.Nil); err != nil {\n\t\treturn a, nil, err\n\t}\n\tf, err :=", "f, err :=")' $app
mutate "any member can import a font" internal/httpapi/handlers_appearance.go \
  's.replace("admin := SessionOnly(Permission(identity.PermAdministrator))", "admin := SessionOnly(Permission(identity.PermUseProduct))")' $app
mutate "any file passes as a font" internal/settings/font.go \
  's.replace(`return "", false`, `return "woff2", true`)' $app
mutate "font changes are not audited" internal/settings/font.go \
  's.replace(`c.Event("settings.font_changed"`, `c.Event("settings.updated"`)' $app
mutate "a replaced font file stays on disk" internal/settings/font.go \
  's.replace("if found {", "if false && found {")' $app
mutate "publishing skips the check" internal/pages/versions.go \
  's.replace("if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {\n\t\t\treturn err\n\t\t}\n\t\tp, err := lockPage(ctx, tx, id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif p.CurrentVersion != in.BaseVersion", "p, err := lockPage(ctx, tx, id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif p.CurrentVersion != in.BaseVersion")' $app
mutate "search ignores permissions" internal/search/search.go \
  's.replace("JOIN perm_visible_pages($1) v ON v.page_id = p.id\n", "JOIN (SELECT id AS page_id FROM pages, (SELECT $1::uuid) x) v ON v.page_id = p.id\n")' $app
mutate "export ignores permissions" internal/pages/export.go \
  's.replace("JOIN perm_visible_pages($1) vis ON vis.page_id = p.id\n", "JOIN (SELECT id AS page_id FROM pages, (SELECT $1::uuid) x) vis ON vis.page_id = p.id\n")' $app
mutate "moves can drop restrictions" internal/pages/tree.go \
  's.replace("if !slices.Contains(after, r) {", "if !slices.Contains(after, r) && len(before) < 0 {")' $app
mutate "moves can create loops" internal/pages/tree.go \
  's.replace("if slices.Contains(tree, *parent) {", "if slices.Contains(tree, *parent) && len(tree) < 0 {")' $app
mutate "a page returns while its parent is in the trash" internal/pages/tree.go \
  's.replace("if parent.Status == StatusTrashed {", "if parent.Status == StatusTrashed && len(parent.Title) < 0 {")' $app
mutate "SVG shown inline" internal/attachments/attachments.go \
  's.replace(`"image/webp": true}`, `"image/webp": true, "text/xml; charset=utf-8": true, "text/plain": true, "text/xml": true}`)' $app
mutate "publish loses its audit event" internal/pages/pages.go \
  's.replace("return p, record(ctx, tx, c, \"page.created\", p, map[string]any{\"parent_id\": in.parent})", "return p, nil")' ./internal/app/ -run TestAuditAndOutboxPerStateChange
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
  's.replace("JOIN perm_listed_pages($1) v ON v.page_id = a.page_id", "JOIN (SELECT $1::uuid AS u) v ON true")' $app -run TestQuickSearch
mutate "a row menu counts as a visit" internal/pages/details.go \
  's.replace("out.Can, err = s.capabilities(ctx, c, p)\n\treturn out, err", "s.touchRecent(ctx, c.UserID, p)\n\tout.Can, err = s.capabilities(ctx, c, p)\n\treturn out, err")' $app -run TestOpeningARowMenu

mutate "archived pages stay writable" internal/permissions/evaluate.go \
  's.replace("if p.Archived && !worksOnArchivedPage(in.Action) {", "if p.Archived && len(in.Action) < 0 {")' $perms
mutate "templates open as pages" internal/permissions/evaluate.go \
  's.replace("p.Kind != KindTemplate, true)", "true, true)")' $perms
mutate "SQL: lists show archived pages" internal/db/migrations/0008_organising.sql \
  's.replace("WHERE p.archived_at IS NULL\n$$;", "WHERE true\n$$;")' $perms
mutate "SQL: templates show in lists" internal/db/migrations/0008_organising.sql \
  's.replace("AND p.kind <> \x27template\x27\n", "\n")' $perms
mutate "a label reveals hidden pages" internal/pages/labels.go \
  's.replace("JOIN perm_listed_pages($1) v ON v.page_id = l.page_id", "JOIN (SELECT $1::uuid AS u) v ON true")' $app -run TestLabelsAreCleanedAndNeverRevealHiddenPages
mutate "a spelling suggestion uses hidden titles" internal/search/search.go \
  's.replace("SELECT p.title FROM pages p JOIN perm_listed_pages($1) v ON v.page_id = p.id", "SELECT p.title FROM pages p JOIN (SELECT $1::uuid AS u) v ON true")' $app -run TestSearchFiltersNeverRevealHiddenPages
mutate "a move skips hidden pages" internal/pages/move.go \
  's.replace("if hidden {\n", "if hidden && len(tree) < 0 {\n")' $app -run TestMoveBetweenSpacesIsRefusedWhenItWouldTouchTooMuch
mutate "a restore drops a restriction" internal/pages/archive.go \
  's.replace("if len(parents) > 0 && !withParents {", "if len(parents) < 0 && !withParents {")' $app -run TestARestoreBelowAnArchivedPageCannotDropARestriction
mutate "publishing ignores required approval" internal/pages/versions.go \
  's.replace("if err := s.requireApproved(ctx, tx, p, title, doc); err != nil {", "if err := s.requireApproved(ctx, tx, p, title, doc); err != nil && len(title) < 0 {")' $app -run TestASpaceCanRequireApprovalOfTheExactText
mutate "a scheduled publish ignores lost rights" internal/pages/schedule.go \
  's.replace("_, err = s.Publish(ctx, identity.Caller{UserID: due.user}, due.page,", "var author uuid.UUID\n\t\t_ = s.pool.QueryRow(ctx, \"SELECT created_by FROM pages WHERE id = $1\", due.page).Scan(&author)\n\t\t_, err = s.Publish(ctx, identity.Caller{UserID: author}, due.page,")' $app -run TestScheduledPublishingStopsWhenTheRightsAreGone
mutate "handing over gives delete own" internal/permissions/service.go \
  's.replace("SELECT space_id, status, created_by, kind,", "SELECT space_id, status, COALESCE(owner_id, created_by), kind,")' $app -run TestHandingOverAPageGivesNoRightToDeleteIt
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
  's.replace("status = \x27active\x27 AND (expires_at IS NULL OR expires_at > now())", "status = \x27active\x27")' $perms -run TestMatrixSQL
mutate "a guest sees everyone" internal/permissions/evaluate.go \
  's.replace("return target.ID == viewer.ID || slices.Contains(viewer.Global, globalViewProfiles) || target.SharedSpace", "return true")' $perms -run TestMatrixPeople
mutate "SQL: a guest sees everyone" internal/db/migrations/0010_people.sql \
  's.replace("AND (t.id = u\n        OR perm_has_global(u, \x27view_profiles\x27)", "AND (true\n        OR perm_has_global(u, \x27view_profiles\x27)")' $perms -run TestMatrixPeople
mutate "a reset link works twice" internal/identity/factors.go \
  's.replace("\"DELETE FROM auth_challenges WHERE token_hash = $1 AND kind = $2 RETURNING user_id, data, expires_at\"", "\"SELECT user_id, data, expires_at FROM auth_challenges WHERE token_hash = $1 AND kind = $2\"")' $app -run TestAResetLink
mutate "a reset says whether the address is known" internal/identity/lifecycle.go \
  's.replace("if errors.Is(err, pgx.ErrNoRows) {\n\t\treturn nil\n\t}\n\tif err != nil {\n\t\treturn err\n\t}\n\tif _, err := s.pool.Exec(ctx, \"DELETE FROM auth_challenges WHERE kind = $1 AND user_id = $2\"", "if errors.Is(err, pgx.ErrNoRows) {\n\t\treturn s.send(ctx, AccountMail{To: email, Kind: MailReset})\n\t}\n\tif err != nil {\n\t\treturn err\n\t}\n\tif _, err := s.pool.Exec(ctx, \"DELETE FROM auth_challenges WHERE kind = $1 AND user_id = $2\"")' $app -run TestAResetLink
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
  's.replace("\t\te, err := f.read(payload, part)", "\t\tuncensored := f\n\t\tuncensored.Censor = nil\n\t\te, err := uncensored.read(payload, part)")' $app -run TestAnAdministratorSeesACensored
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
mutate "a comment for editors reaches everyone" internal/comments/list.go \
  's.replace("if v.EditorsOnly && !a.canEdit {", "if v.EditorsOnly && !a.canEdit && len(v.Status) < 0 {")' $app -run TestInlineThreadsResolveAndStayWithEditorsUntilPublishing
mutate "a mention reaches a group of any size" internal/notify/fanout.go \
  's.replace("if len(members) <= maxGroupMembers {", "if len(members) >= 0 {")' $app -run TestGroupMentionsAreOffAndHaveALimit
mutate "a silent update reaches watchers" internal/notify/fanout.go \
  's.replace("if p.Silent {\n\t\treturn nil\n\t}\n\twatchers", "if p.Silent && len(p.Mentions) < 0 {\n\t\treturn nil\n\t}\n\twatchers")' $app -run TestWatchersHearOnceAndLosingAccessEndsIt

mutate "a stored mention keeps the name" internal/document/attrs.go \
  's.replace("out[\"kind\"] = MentionUser\n\t}\n\treturn nil", "out[\"kind\"] = MentionUser\n\t}\n\tout[\"label\"] = in[\"label\"]\n\treturn nil")' $app -run TestAMentionStoresTheIdAndNeverTheName
mutate "search does not find a mentioned person" internal/search/search.go \
  's.replace("WHERE (x.tsv || n.tsv) @@ q.query", "WHERE x.tsv @@ q.query")' $app -run TestAMentionStoresTheIdAndNeverTheName
mutate "an unpublished sentence reaches readers" internal/comments/comments.go \
  's.replace("if reachedReaders(t.anchor, doc) {", "if t.id != uuid.Nil || reachedReaders(t.anchor, doc) {")' $app -run TestInlineThreadsResolveAndStayWithEditorsUntilPublishing
mutate "a reaction changes an archived space" internal/permissions/evaluate.go \
  's.replace("case SpaceView, ActionPageView, ActionApprovalRead, SpaceExport, SpaceAdminister, SpaceManageAccess, AnalyticsView:", "case SpaceView, ActionPageView, ActionApprovalRead, SpaceExport, SpaceAdminister, SpaceManageAccess, AnalyticsView, ActionReact:")' $app -run TestAReactionChangesNothingInAnArchivedSpace
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
mutate "an edit restriction does not count for a thread" internal/db/migrations/0008_organising.sql \
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
  's.replace("if err := s.requireGlobal(ctx, c, identity.PermInvitePeople); err != nil {\n\t\t\treturn err\n\t\t}\n\t\tspace, _, err", "if err := s.requireGlobal(ctx, c, identity.PermInvitePeople); err != nil \x26\x26 len(in.Email) < 0 {\n\t\t\treturn err\n\t\t}\n\t\tspace, _, err")' $app -run TestInvitationMatrix
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
  's.replace("if !mayInvite {", "if !mayInvite \x26\x26 r.email == \"\" {")' $app -run TestApprovalFailsWhenTheRequesterLostTheRole
mutate "an approval goes through while invitations are off" internal/sharing/invitations.go \
  's.replace("r, err := lockWaitingInvitation(ctx, tx, id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif err := s.requireInvitationsOn(ctx); err != nil {\n\t\t\treturn err\n\t\t}", "r, err := lockWaitingInvitation(ctx, tx, id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}")' $app -run TestRequestsWaitOnlyWhileInvitationsAreOn
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
  's.replace("if err := s.checkParent(ctx, tx, c, p.SpaceID, source); err != nil {", "if err := s.checkParent(ctx, tx, c, p.SpaceID, source); err != nil \x26\x26 title == \"\" {")' $app -run TestFollowingAPlaceholderNeedsTheRightToCreate
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
mutate "search finds a page by the label of a trashed file" internal/search/search.go \
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

if [ "$survived" -gt 0 ]; then
  echo "$survived mutation(s) were not caught"
  exit 1
fi
echo "all mutations were caught"
