// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
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
	// Language says what the text is written in. Empty means the language of the instance.
	Language string `json:"language"`
}

// languageTag is the short form of a language tag, which ends up in the lang attribute.
var languageTag = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)

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
	if l.Language != "" && !languageTag.MatchString(l.Language) {
		return l, apperr.Invalid("A page language looks like en or pt-BR.")
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
			header_attachment_id = $6, header_position = $7, language = $8, updated_at = now() WHERE id = $1 RETURNING `+pageColumns,
			id, look.Emoji, look.FullWidth, look.TitleAlign, look.HideChildren, look.HeaderAttachmentID, look.HeaderPosition, look.Language))
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "page.look_changed", out, nil)
	})
	return out, err
}

// SetCommentsClosed turns comments off for one page, or back on. Existing comments stay
// readable. The switch is a column of the page, so pages writes it, not comments (ARC-027).
func (s *Service) SetCommentsClosed(ctx context.Context, c identity.Caller, id uuid.UUID, closed bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := scanPage(tx.QueryRow(ctx, "UPDATE pages SET comments_closed = $2 WHERE id = $1 RETURNING "+pageColumns, id, closed))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "page.comments_closed", p, map[string]any{"closed": closed})
	})
}

// CommentsClosed tells whether a page takes no new comments. The comments module asks this
// instead of reading the column itself, so pages stays the only one that decides (ARC-027).
func (s *Service) CommentsClosed(ctx context.Context, q db.Querier, id uuid.UUID) (bool, error) {
	var closed bool
	err := q.QueryRow(ctx, "SELECT comments_closed FROM pages WHERE id = $1", id).Scan(&closed)
	return closed, err
}
