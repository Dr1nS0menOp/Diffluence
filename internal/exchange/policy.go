// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
)

const (
	limitsKey           = "exchange"
	defaultMaxExportMiB = 2048
	defaultMaxImportMiB = 200
	largestMiB          = 10240
)

// Limits are the instance settings of this module. Both switches start off:
// nothing here is public until an administrator says so.
type Limits struct {
	MaxExportMiB int  `json:"max_export_mib"`
	MaxImportMiB int  `json:"max_import_mib"`
	PublicSites  bool `json:"public_sites_enabled"`
	PublicLinks  bool `json:"public_links_enabled"`
}

// FormatRule says who may use a format. No groups means everyone.
type FormatRule struct {
	Format   string      `json:"format"`
	Enabled  bool        `json:"enabled"`
	GroupIDs []uuid.UUID `json:"group_ids"`
}

// ExportPolicy is the page of an administrator.
type ExportPolicy struct {
	Rules        []FormatRule `json:"rules"`
	Limits       Limits       `json:"limits"`
	PDFAvailable bool         `json:"pdf_available"`
}

// Limits reads the settings of this module.
func (s *Service) Limits(ctx context.Context) (Limits, error) {
	v := Limits{MaxExportMiB: defaultMaxExportMiB, MaxImportMiB: defaultMaxImportMiB}
	err := s.settings.Read(ctx, limitsKey, &v)
	return v, err
}

func (s *Service) rule(ctx context.Context, format string) (FormatRule, error) {
	r := FormatRule{Format: format, Enabled: true, GroupIDs: []uuid.UUID{}}
	err := s.pool.QueryRow(ctx, `SELECT enabled, group_ids FROM export_policy WHERE format = $1`, format).Scan(&r.Enabled, &r.GroupIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, nil
	}
	return r, err
}

func (s *Service) mayUseFormat(ctx context.Context, userID uuid.UUID, format string) (bool, error) {
	if format == FormatPDF && !s.PDFAvailable() {
		return false, nil
	}
	r, err := s.rule(ctx, format)
	if err != nil || !r.Enabled {
		return false, err
	}
	if len(r.GroupIDs) == 0 {
		return true, nil
	}
	var member bool
	err = s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE user_id = $1 AND group_id = ANY($2))`, userID, r.GroupIDs).Scan(&member)
	return member, err
}

// AllowedFormats lists what a person may export to, which is what the dialog offers.
func (s *Service) AllowedFormats(ctx context.Context, c identity.Caller) ([]string, error) {
	out := []string{}
	for _, format := range Formats {
		ok, err := s.mayUseFormat(ctx, c.UserID, format)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, format)
		}
	}
	return out, nil
}

// Policy reads the rules. The route is for administrators.
func (s *Service) Policy(ctx context.Context) (ExportPolicy, error) {
	out := ExportPolicy{PDFAvailable: s.PDFAvailable(), Rules: []FormatRule{}}
	for _, format := range Formats {
		r, err := s.rule(ctx, format)
		if err != nil {
			return out, err
		}
		out.Rules = append(out.Rules, r)
	}
	var err error
	out.Limits, err = s.Limits(ctx)
	return out, err
}

func validateLimits(l Limits) error {
	if l.MaxExportMiB < 1 || l.MaxExportMiB > largestMiB || l.MaxImportMiB < 1 || l.MaxImportMiB > largestMiB {
		return apperr.Invalid("Sizes must be between 1 MiB and 10 GiB.")
	}
	return nil
}

// SetPolicy stores the rules. The route is for administrators.
func (s *Service) SetPolicy(ctx context.Context, c identity.Caller, p ExportPolicy) error {
	if err := validateLimits(p.Limits); err != nil {
		return err
	}
	for _, r := range p.Rules {
		if !slices.Contains(Formats, r.Format) {
			return apperr.Invalid("Unknown format.")
		}
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, r := range p.Rules {
			groups := r.GroupIDs
			if groups == nil {
				groups = []uuid.UUID{}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO export_policy (format, enabled, group_ids) VALUES ($1, $2, $3)
				ON CONFLICT (format) DO UPDATE SET enabled = EXCLUDED.enabled, group_ids = EXCLUDED.group_ids`, r.Format, r.Enabled, groups); err != nil {
				return err
			}
		}
		if _, err := audit.Record(ctx, tx, c.Event("export.policy_changed", "settings", "export-policy", "", map[string]any{"rules": p.Rules})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "export.policy_changed", map[string]any{})
		return err
	})
	if err != nil {
		return err
	}
	return s.settings.Write(ctx, c, limitsKey, p.Limits)
}
