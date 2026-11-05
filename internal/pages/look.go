// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Title alignments.
const (
	AlignStart  = "start"
	AlignCenter = "center"
)

// maxEmojiRunes leaves room for the longest sequences, such as a family with
// skin tones, and is far too short for a sentence.
const (
	maxEmojiRunes     = 16
	maxHeaderPosition = 100
)

// Look is how a page presents itself. It is page data, not part of a version, so
// restoring old content does not change it (spec 009, D4).
type Look struct {
	Emoji              string     `json:"emoji"`
	FullWidth          bool       `json:"full_width"`
	TitleAlign         string     `json:"title_align"`
	HideChildren       bool       `json:"hide_children"`
	HeaderAttachmentID *uuid.UUID `json:"header_attachment_id"`
	HeaderPosition     int        `json:"header_position"`
}

func cleanEmoji(emoji string) (string, error) {
	emoji = strings.TrimSpace(emoji)
	bad := apperr.Invalid("A page emoji is one emoji.")
	if !utf8.ValidString(emoji) || utf8.RuneCountInString(emoji) > maxEmojiRunes {
		return "", bad
	}
	for _, r := range emoji {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune(`<>&"'`, r) {
			return "", bad
		}
	}
	return emoji, nil
}

func (l Look) validated() (Look, error) {
	emoji, err := cleanEmoji(l.Emoji)
	if err != nil {
		return l, err
	}
	l.Emoji = emoji
	if l.TitleAlign != AlignStart && l.TitleAlign != AlignCenter {
		return l, apperr.Invalid("A title is aligned to the start or the centre.")
	}
	if l.HeaderPosition < 0 || l.HeaderPosition > maxHeaderPosition {
		return l, apperr.Invalid("The position of a header image is between 0 and 100.")
	}
	return l, nil
}

// SetLook changes how a page presents itself.
func (s *Service) SetLook(ctx context.Context, c identity.Caller, id uuid.UUID, in Look) (Page, error) {
	look, err := in.validated()
	if err != nil {
		return Page{}, err
	}
	var out Page
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		if _, err := lockPage(ctx, tx, id); err != nil {
			return err
		}
		if look.HeaderAttachmentID != nil {
			// Only an image of this page: anything else could show a file from a
			// page the reader may not see.
			isImage, err := s.attachments.IsImageOf(ctx, tx, *look.HeaderAttachmentID, id)
			if err != nil {
				return err
			}
			if !isImage {
				return apperr.Invalid("A header image must be an image on this page.")
			}
		}
		out, err = scanPage(tx.QueryRow(ctx, `UPDATE pages SET emoji = $2, full_width = $3, title_align = $4, hide_children = $5,
			header_attachment_id = $6, header_position = $7, updated_at = now() WHERE id = $1 RETURNING `+pageColumns,
			id, look.Emoji, look.FullWidth, look.TitleAlign, look.HideChildren, look.HeaderAttachmentID, look.HeaderPosition))
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "page.look_changed", out, nil)
	})
	return out, err
}
