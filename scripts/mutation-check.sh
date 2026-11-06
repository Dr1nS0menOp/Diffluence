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
  tar -C "$src" --exclude=./web --exclude=./bin --exclude=./.git -cf - . | tar -C "$work/repo" -xf -
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
mutate "SQL: guests count as all members" internal/db/migrations/0003_permission_functions.sql \
  's.replace(`(g.subject_type = '"'"'all_members'"'"' AND x.type = '"'"'member'"'"')`, `(g.subject_type = '"'"'all_members'"'"')`)' $perms
mutate "reading a page skips the check" internal/pages/pages.go \
  's.replace("if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {\n\t\treturn Detail{}, err\n\t}", "")' $app
mutate "downloads skip the check" internal/attachments/attachments.go \
  's.replace("if err := s.perms.RequirePage(ctx, s.pool, c.UserID, a.PageID, permissions.ActionPageView, uuid.Nil); err != nil {\n\t\treturn a, nil, err\n\t}\n\tf, err :=", "f, err :=")' $app
mutate "any member can import a font" internal/httpapi/handlers_appearance.go \
  's.replace("admin := Permission(identity.PermAdministrator)", "admin := Permission(identity.PermUseProduct)")' $app
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
  's.replace("case SpaceView, ActionPageView, ActionApprovalRead, SpaceExport, SpaceAdminister:", "case SpaceView, ActionPageView, ActionApprovalRead, SpaceExport, SpaceAdminister, ActionReact:")' $app -run TestAReactionChangesNothingInAnArchivedSpace
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
  's.replace(".filter((key) => key.startsWith(PERSONAL_PREFIX))\n    .forEach((key) => localStorage.removeItem(key));\n  sessionStorage.clear();", ".filter((key) => key.startsWith(\"nothing\"))\n    .forEach((key) => localStorage.removeItem(key));")' src/personal.test.ts
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
  's.replace("case ActionPageView, ActionApprovalRead, SpaceExport, SpaceAdminister, ActionPageDelete, PageArchive, RestrictionManage:", "case ActionPageView, ActionApprovalRead, ActionReact, SpaceExport, SpaceAdminister, ActionPageDelete, PageArchive, RestrictionManage:")' $app -run TestAReactionChangesNothingOnAnArchivedPageOrInAnArchivedSpace
mutate "a removed version lives on in a quote" internal/pages/versions.go \
  's.replace("if err := comments.BlankQuotes(ctx, tx, c, id, removed, current); err != nil {", "_ = removed\n\t\tif err := comments.BlankQuotes(ctx, tx, c, id, current, current); err != nil {")' $app -run TestRemovingAVersionBlanksTheQuotesThatHoldIt
mutate "nodes hand out rights at the same time" internal/db/db.go \
  's.replace("\"SELECT pg_advisory_xact_lock($1)\", migrationLockKey", "\"SELECT $1::bigint\", migrationLockKey")' ./internal/db/ -count=5 -run TestGrantApp

if [ "$survived" -gt 0 ]; then
  echo "$survived mutation(s) were not caught"
  exit 1
fi
echo "all mutations were caught"
