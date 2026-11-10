// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
)

const oneMiB = 1 << 20

func TestAFileInTheTrashStillTakesRoomUntilItIsPurged(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	file := owner.file(page, "big.txt", strings.Repeat("x", oneMiB))
	admin.setLimit(space, 1)
	owner.ok("DELETE", "/api/v1/attachments/"+file, nil)

	if status, body := owner.upload(page, "small.txt", "x"); status != 413 {
		t.Fatalf("a file in the trash freed its room: %d %s", status, body)
	}
	item := e.scalar("SELECT id::text FROM trash_items WHERE object_id = $1", file)
	owner.ok("POST", "/api/v1/trash/"+item+"/restore", nil)
	if e.count("SELECT count(*) FROM attachments WHERE id = $1 AND status = 'active'", file) != 1 {
		t.Fatal("a restore over a full space failed, though its file never left the count")
	}
	owner.ok("DELETE", "/api/v1/attachments/"+file, nil)
	item = e.scalar("SELECT id::text FROM trash_items WHERE object_id = $1", file)
	owner.ok("DELETE", "/api/v1/trash/"+item, nil)
	if status, body := owner.upload(page, "small.txt", "x"); status != 201 {
		t.Fatalf("a purge did not free the room: %d %s", status, body)
	}
}

func TestAVersionMadeCurrentAgainCountsItsStoredFileOnce(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	file := owner.file(page, "plan.txt", strings.Repeat("x", oneMiB))
	if status, body := owner.newVersion(file, "plan.txt", "y"); status != 201 {
		t.Fatalf("new version: %d %s", status, body)
	}
	owner.ok("POST", "/api/v1/attachments/"+file+"/versions/1/current", nil)
	admin.setLimit(space, 2)

	// One stored megabyte, one byte and this upload fit in two; counting the first twice would not.
	if status, body := owner.upload(page, "notes.txt", strings.Repeat("z", 900<<10)); status != 201 {
		t.Fatalf("the same stored file counted twice: %d %s", status, body)
	}
	usage := owner.ok("GET", "/api/v1/spaces/"+space+"/files/usage", nil)
	if got := int(usage["bytes"].(float64)); got != oneMiB+1+900<<10 {
		t.Fatalf("the Files screen total: %d", got)
	}
}

// fullSpaces makes a space that is full and one with a page whose file needs room.
type fullSpaces struct {
	e                  *env
	owner, admin       *user
	full, other        string
	page, withFile     string
	pages, attachments int
}

func newFullSpaces(t *testing.T) fullSpaces {
	e := newEnv(t)
	owner, _, _, full := e.team("ENG")
	other, _ := owner.space("OPS")
	f := fullSpaces{e: e, owner: owner, full: full, other: other}
	f.admin = e.user("admin@example.com", identity.AllGlobalPermissions...)
	f.page = owner.page(full, nil, "Full page", "x")
	owner.file(f.page, "first.txt", strings.Repeat("x", oneMiB))
	f.withFile = owner.page(other, nil, "Carries a file", "x")
	owner.file(f.withFile, "big.txt", strings.Repeat("y", oneMiB))
	f.admin.setLimit(full, 1)
	f.pages = e.count("SELECT count(*) FROM pages")
	f.attachments = e.count("SELECT count(*) FROM attachments")
	return f
}

func (f fullSpaces) leftNothing(t *testing.T, way string) {
	t.Helper()
	if got := f.e.count("SELECT count(*) FROM pages"); got != f.pages {
		t.Fatalf("%s: a refused action left %d pages, want %d", way, got, f.pages)
	}
	if got := f.e.count("SELECT count(*) FROM attachments"); got != f.attachments {
		t.Fatalf("%s: a refused action left %d files, want %d", way, got, f.attachments)
	}
}

func TestEveryWayThatCopiesFilesIntoAFullSpaceIsRefusedWhole(t *testing.T) {
	f := newFullSpaces(t)
	owner := f.owner

	owner.want(413, "POST", "/api/v1/pages/"+f.page+"/copy", map[string]any{})
	f.leftNothing(t, "a copy beside itself")
	owner.want(413, "POST", "/api/v1/pages/"+f.withFile+"/copy", map[string]any{"space_id": f.full})
	f.leftNothing(t, "a copy into the space")
	owner.want(413, "POST", "/api/v1/pages/"+f.page+"/save-as-template", map[string]any{"name": "Too big"})
	f.leftNothing(t, "saving as a template")
	if n := f.e.count("SELECT count(*) FROM templates"); n != 0 {
		t.Fatalf("a refused template stayed: %d", n)
	}

	f.admin.setLimit(f.full, 0)
	template := owner.ok("POST", "/api/v1/pages/"+f.page+"/save-as-template", map[string]any{"name": "Pictured"})["id"].(string)
	f.admin.setLimit(f.full, 2)
	f.pages = f.e.count("SELECT count(*) FROM pages")
	f.attachments = f.e.count("SELECT count(*) FROM attachments")
	owner.want(413, "POST", "/api/v1/templates/"+template+"/use", map[string]any{"space_id": f.full, "title": "From it"})
	f.leftNothing(t, "a page from a template")

	f.admin.setLimit(f.full, 5)
	owner.ok("POST", "/api/v1/pages/"+f.page+"/copy", map[string]any{})
	owner.ok("POST", "/api/v1/templates/"+template+"/use", map[string]any{"space_id": f.full, "title": "From it"})
}

func TestABranchMovesIntoAFullSpaceOnlyWithRoomForItsFiles(t *testing.T) {
	f := newFullSpaces(t)
	owner := f.owner

	impact := owner.ok("GET", "/api/v1/pages/"+f.withFile+"/move-impact?space_id="+f.full, nil)
	if impact["fits"] != false {
		t.Fatalf("the move dialog was not warned: %v", impact)
	}
	owner.want(413, "POST", "/api/v1/pages/"+f.withFile+"/move", map[string]any{"space_id": f.full})
	if got := f.e.scalar("SELECT space_id::text FROM pages WHERE id = $1", f.withFile); got != f.other {
		t.Fatalf("a refused move moved the page to %s", got)
	}
	if got := f.e.count("SELECT count(*) FROM attachments WHERE space_id = $1", f.full); got != 1 {
		t.Fatalf("a refused move moved its file: %d", got)
	}

	f.admin.setLimit(f.full, 3)
	if impact := owner.ok("GET", "/api/v1/pages/"+f.withFile+"/move-impact?space_id="+f.full, nil); impact["fits"] != true {
		t.Fatalf("with room: %v", impact)
	}
	owner.ok("POST", "/api/v1/pages/"+f.withFile+"/move", map[string]any{"space_id": f.full})
}

func TestTheCopyDialogIsWarnedWhenTheFilesDoNotFit(t *testing.T) {
	f := newFullSpaces(t)
	if got := f.owner.ok("GET", "/api/v1/pages/"+f.page+"/copy-impact", nil); got["fits"] != false {
		t.Fatalf("beside itself: %v", got)
	}
	if got := f.owner.ok("GET", "/api/v1/pages/"+f.withFile+"/copy-impact?space_id="+f.full, nil); got["fits"] != false {
		t.Fatalf("into the full space: %v", got)
	}
	if got := f.owner.ok("GET", "/api/v1/pages/"+f.withFile+"/copy-impact", nil); got["fits"] != true {
		t.Fatalf("a space without a limit: %v", got)
	}
}

func TestALargeCopyReportsThePagesWhoseFilesNoLongerFit(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	top := owner.page(space, nil, "Big branch", "x")
	var heavy string
	for i := range 51 {
		id := owner.page(space, top, fmt.Sprintf("Part %d", i), "x")
		if i == 7 {
			heavy = id
		}
	}
	owner.file(heavy, "big.txt", strings.Repeat("x", oneMiB))
	admin.setLimit(space, 1)

	job := owner.ok("POST", "/api/v1/pages/"+top+"/copy", map[string]any{
		"parent_id": nil, "include_children": true, "prefix": "New "})["job_id"].(string)
	result := e.runJob(owner, job)
	failed := result["failed"].([]any)
	if len(failed) != 1 || failed[0].(map[string]any)["id"] != heavy {
		t.Fatalf("report: %v", result)
	}
	if !strings.Contains(failed[0].(map[string]any)["reason"].(string), "storage limit") {
		t.Fatalf("the reason: %v", failed[0])
	}
	if n := e.count("SELECT count(*) FROM pages WHERE title = 'New Part 7'"); n != 0 {
		t.Fatalf("a page whose files did not fit was half copied: %d", n)
	}
	if n := e.count("SELECT count(*) FROM attachments WHERE space_id = $1", space); n != 1 {
		t.Fatalf("files: %d", n)
	}
}
