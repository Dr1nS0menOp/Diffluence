// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"strings"

	"github.com/diffluence/diffluence/internal/apperr"
)

const maxImportRows = 1000

// ImportRow is one line of an import file, and what happens to it.
type ImportRow struct {
	Line    int    `json:"line"`
	Email   string `json:"email"`
	Type    string `json:"type"`
	Problem string `json:"problem"`
}

// ParseImport reads a CSV of addresses and types. People choose their own name and
// password, so an import is a set of invitations. A bad line is reported, and never
// stops the others.
func ParseImport(body io.Reader) ([]ImportRow, error) {
	reader := csv.NewReader(body)
	reader.FieldsPerRecord = -1
	var rows []ImportRow
	for line := 1; ; line++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, apperr.Invalid(fmt.Sprintf("Line %d cannot be read as CSV.", line))
		}
		if len(rows) == maxImportRows {
			return nil, apperr.Invalid(fmt.Sprintf("An import holds at most %d lines.", maxImportRows))
		}
		rows = append(rows, parseImportLine(line, record))
	}
}

func parseImportLine(line int, record []string) ImportRow {
	row := ImportRow{Line: line, Email: strings.TrimSpace(record[0]), Type: TypeMember}
	if len(record) > 1 && strings.TrimSpace(record[1]) != "" {
		row.Type = strings.TrimSpace(record[1])
	}
	if _, err := mail.ParseAddress(row.Email); err != nil {
		row.Problem = "invalid_address"
	} else if row.Type != TypeMember && row.Type != TypeGuest {
		row.Problem = "invalid_type"
	}
	return row
}

// PreviewImport marks the lines of addresses that already have an account.
func (s *Service) PreviewImport(ctx context.Context, rows []ImportRow) ([]ImportRow, error) {
	for i := range rows {
		if rows[i].Problem != "" {
			continue
		}
		var exists bool
		err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = lower($1))", rows[i].Email).Scan(&exists)
		if err != nil {
			return nil, err
		}
		if exists {
			rows[i].Problem = "exists"
		}
	}
	return rows, nil
}

// Import invites the people on the good lines, and reports what became of each line.
func (s *Service) Import(ctx context.Context, c Caller, rows []ImportRow) ([]ImportRow, error) {
	checked, err := s.PreviewImport(ctx, rows)
	if err != nil {
		return nil, err
	}
	for i := range checked {
		if checked[i].Problem != "" {
			continue
		}
		if _, err := s.Invite(ctx, c, NewInvitation{Email: checked[i].Email, Type: checked[i].Type}); err != nil {
			e, ok := apperr.As(err)
			if !ok {
				return nil, err
			}
			checked[i].Problem = e.Code
		}
	}
	return checked, nil
}
