// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/office"
	"github.com/diffluence/diffluence/internal/permissions"
)

// DraftContent is what a Word file adds to a draft at the cursor (EXC-029).
type DraftContent struct {
	Content  []document.Node `json:"content"`
	Warnings []string        `json:"warnings"`
}

// WordIntoDraft reads a Word file for the editor. Its images become attachments of
// the page, and nothing is published.
func (s *Service) WordIntoDraft(ctx context.Context, c identity.Caller, pageID uuid.UUID, body io.Reader) (DraftContent, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.PageEdit, uuid.Nil); err != nil {
		return DraftContent{}, err
	}
	data, err := io.ReadAll(io.LimitReader(body, maxWordBytes+1))
	if err != nil {
		return DraftContent{}, err
	}
	if int64(len(data)) > maxWordBytes {
		return DraftContent{}, apperr.TooLarge("This Word file is too large.")
	}
	n := 0
	got, err := office.ReadDOCX(data, func(name string, image []byte) (map[string]any, bool) {
		n++
		a, err := s.attachments.Upload(ctx, c, pageID, fmt.Sprintf("%d-%s", n, name), bytes.NewReader(image))
		if err != nil {
			return nil, false
		}
		return map[string]any{"attachment_id": a.ID.String()}, true
	})
	if err != nil {
		return DraftContent{}, err
	}
	doc := checked(got.Document, &got.Warnings)
	if got.Warnings == nil {
		got.Warnings = []string{}
	}
	return DraftContent{Content: doc.Content, Warnings: got.Warnings}, nil
}
