// SPDX-License-Identifier: Apache-2.0

package permissions_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/permissions"
)

// TestAReviewerReadsTheirPageThroughRestrictionsForTheRoundOnly is DOC-203 and DOC-1224 on
// the pure rule: being asked opens the page past its restrictions and past "never
// approved", for approval.read only, and only while the person is a reviewer. Nothing else
// opens: not viewing, not editing, not a template or a page in the trash, not a guest.
func TestAReviewerReadsTheirPageThroughRestrictionsForTheRoundOnly(t *testing.T) {
	me, author, pageID, parentID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	keptOut := []permissions.RestrictionLevel{
		{PageID: parentID, Subjects: []permissions.Subject{{Type: permissions.SubjectUser, ID: author}}},
		{PageID: pageID, Subjects: []permissions.Subject{{Type: permissions.SubjectUser, ID: author}}},
	}
	in := func(userType string, p permissions.PageFacts, action string, grants ...string) permissions.Input {
		p.ID, p.CreatedBy = pageID, author
		var list []permissions.Grant
		for _, g := range grants {
			list = append(list, permissions.Grant{Subject: permissions.Subject{Type: permissions.SubjectUser, ID: me}, Permission: g})
		}
		return permissions.Input{
			User:   permissions.UserFacts{ID: me, Type: userType, Active: true, Global: []string{"use_product"}},
			Space:  permissions.SpaceFacts{Status: "active", Grants: list},
			Page:   &p,
			Action: action,
		}
	}
	reviewing := permissions.PageFacts{Status: "published", Kind: "page", ViewChain: keptOut, ControlHidden: true, Reviewer: true}
	if !permissions.Evaluate(in("member", reviewing, permissions.ActionApprovalRead, permissions.SpaceView)).Allowed {
		t.Fatal("a reviewer cannot read the page they are asked about")
	}
	former := reviewing
	former.Reviewer = false
	trashed := reviewing
	trashed.Status = "trashed"
	template := reviewing
	template.Kind = permissions.KindTemplate
	refused := map[string]permissions.Input{
		"a former reviewer":                   in("member", former, permissions.ActionApprovalRead, permissions.SpaceView),
		"a guest named as a reviewer":         in("guest", reviewing, permissions.ActionApprovalRead, permissions.SpaceView),
		"a reviewer without the space":        in("member", reviewing, permissions.ActionApprovalRead),
		"a reviewer viewing the page as such": in("member", reviewing, permissions.ActionPageView, permissions.SpaceView),
		"a reviewer editing":                  in("member", reviewing, permissions.PageEdit, permissions.SpaceView, permissions.PageEdit),
		"a reviewer of a page in the trash":   in("member", trashed, permissions.ActionApprovalRead, permissions.SpaceView),
		"a reviewer of a template":            in("member", template, permissions.ActionApprovalRead, permissions.SpaceView),
	}
	for name, input := range refused {
		if permissions.Evaluate(input).Allowed {
			t.Errorf("%s is let in", name)
		}
	}
}
