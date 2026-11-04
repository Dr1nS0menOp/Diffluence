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

# mutate <name> <file> <python expression on s> <go test args...>
mutate() {
  local name=$1 file=$2 expr=$3
  shift 3
  rm -rf "$work/repo"
  mkdir -p "$work/repo"
  tar -C "$src" --exclude=./web --exclude=./bin --exclude=./.git -cf - . | tar -C "$work/repo" -xf -
  if ! node -e '
    const fs = require("fs");
    const [file, expr] = process.argv.slice(1);
    const s = fs.readFileSync(file, "utf8");
    const out = eval(expr);
    if (out === s) { console.error("mutation did not change the file"); process.exit(3); }
    fs.writeFileSync(file, out);
  ' "$work/repo/$file" "$expr"; then
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
  's.replace("return p, record(ctx, tx, c, \"page.created\", p, map[string]any{\"parent_id\": parent})", "return p, nil")' ./internal/app/ -run TestAuditAndOutboxPerStateChange
mutate "javascript links accepted" internal/document/document.go \
  's.replace("default:\n\t\treturn \"\", bad\n\t}\n\treturn href, nil", "default:\n\t}\n\treturn href, nil")' ./internal/document/

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

mutate "nodes hand out rights at the same time" internal/db/db.go \
  's.replace("\"SELECT pg_advisory_xact_lock($1)\", migrationLockKey", "\"SELECT $1::bigint\", migrationLockKey")' ./internal/db/ -count=5 -run TestGrantApp

if [ "$survived" -gt 0 ]; then
  echo "$survived mutation(s) were not caught"
  exit 1
fi
echo "all mutations were caught"
