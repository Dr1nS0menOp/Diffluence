// SPDX-License-Identifier: Apache-2.0

// Package labels holds what a label is, for pages and for files alike.
package labels

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
)

const maxRunes = 64

// Clean writes a label the one allowed way: small letters, dashes for spaces, and
// letters of any language, digits, dashes and underscores.
func Clean(raw string) (string, error) {
	label := strings.Join(strings.Fields(strings.ToLower(raw)), "-")
	bad := apperr.Invalid("A label has letters, digits, dashes and underscores, and at most 64 characters.")
	if label == "" || utf8.RuneCountInString(label) > maxRunes {
		return "", bad
	}
	for _, r := range label {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return "", bad
		}
	}
	return label, nil
}

// Refused reports whether the fixed list of a space leaves this label out.
func Refused(ctx context.Context, q db.Querier, spaceID uuid.UUID, label string) (bool, error) {
	var refused bool
	err := q.QueryRow(ctx, `SELECT s.fixed_labels AND NOT EXISTS
			(SELECT 1 FROM space_fixed_labels f WHERE f.space_id = s.id AND f.label = $2)
		FROM spaces s WHERE s.id = $1`, spaceID, label).Scan(&refused)
	return refused, err
}
