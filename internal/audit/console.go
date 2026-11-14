// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/db"
)

// What the console's audit area shows beside the check of the chain (spec 075, CSL-530 to
// CSL-533): the newest checkpoint, the keys that sign, and the newest result of a comparison
// with the witness, which an operator runs outside and reports with "audit verify --witness
// --report". The console never compares the witness itself (CSL-1006).

// KeyStartLength is how much of a public key the console shows: enough to tell keys apart,
// as a fingerprint is.
const KeyStartLength = 16

// maxWitnessReason bounds the reason a report carries.
const maxWitnessReason = 300

// WitnessCheck is one reported comparison of the chain with the witness.
type WitnessCheck struct {
	CheckedAt   time.Time `json:"checked_at"`
	OK          bool      `json:"ok"`
	Compared    int       `json:"compared"`
	Pruned      int       `json:"pruned"`
	FirstBadSeq int64     `json:"first_bad_seq,omitempty"`
	Reason      string    `json:"reason,omitempty"`
}

func (w WitnessCheck) validate() error {
	switch {
	case w.Compared < 0 || w.Pruned < 0 || w.FirstBadSeq < 0:
		return errors.New("audit: a witness report counts from zero")
	case w.OK && (w.FirstBadSeq != 0 || w.Reason != ""):
		return errors.New("audit: a witness report that matched names no position that did not")
	case !w.OK && w.Reason == "":
		return errors.New("audit: a witness report that did not match says why")
	case utf8.RuneCountInString(w.Reason) > maxWitnessReason || strings.ContainsAny(w.Reason, "\r\n\x00"):
		return errors.New("audit: the reason of a witness report is one short line")
	}
	return nil
}

// RecordWitnessCheck stores the result of a comparison with the witness, as of now.
func RecordWitnessCheck(ctx context.Context, q db.Querier, w WitnessCheck) error {
	if err := w.validate(); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `INSERT INTO audit_witness_checks (ok, compared, pruned, first_bad_seq, reason)
		VALUES ($1, $2, $3, $4, $5)`, w.OK, w.Compared, w.Pruned, w.FirstBadSeq, w.Reason)
	if err != nil {
		return fmt.Errorf("audit: record the witness report: %w", err)
	}
	return nil
}

// LatestWitnessCheck is the newest reported comparison, or nil when none was ever reported.
func LatestWitnessCheck(ctx context.Context, q db.Querier) (*WitnessCheck, error) {
	var w WitnessCheck
	err := q.QueryRow(ctx, `SELECT checked_at, ok, compared, pruned, first_bad_seq, reason
		FROM audit_witness_checks ORDER BY checked_at DESC, id DESC LIMIT 1`).
		Scan(&w.CheckedAt, &w.OK, &w.Compared, &w.Pruned, &w.FirstBadSeq, &w.Reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("audit: read the witness report: %w", err)
	}
	return &w, nil
}

// WitnessEverReported says whether any comparison with the witness was reported.
func WitnessEverReported(ctx context.Context, q db.Querier) (bool, error) {
	w, err := LatestWitnessCheck(ctx, q)
	return w != nil, err
}

// SigningKey is one key that signs checkpoints, as the console shows it.
type SigningKey struct {
	// FromSeq is the first position it signs.
	FromSeq int64 `json:"from_seq"`
	// PublicKeyStart is the start of its public key in hex.
	PublicKeyStart string `json:"public_key_start"`
	// Installation marks the key derived from master.key, which needs no endorsement.
	Installation bool `json:"installation"`
	// Endorsed says whether the installation's key signed this key in.
	Endorsed bool `json:"endorsed"`
}

// NewestCheckpoint is the newest signed head of the chain.
type NewestCheckpoint struct {
	Seq int64     `json:"seq"`
	At  time.Time `json:"at"`
}

// Overview is the state of the chain's signatures.
type Overview struct {
	Checkpoint *NewestCheckpoint `json:"checkpoint"`
	Keys       []SigningKey      `json:"keys"`
}

func keyStart(public []byte) string {
	whole := hex.EncodeToString(public)
	return whole[:min(KeyStartLength, len(whole))]
}

// ReadOverview reads the newest checkpoint and the keys, oldest first, the installation's
// own key first. Unlike a check of the chain, a key without its endorsement is shown and
// marked, so an administrator sees it.
func ReadOverview(ctx context.Context, q db.Querier, installation ed25519.PublicKey) (Overview, error) {
	out := Overview{Keys: []SigningKey{{FromSeq: 1, PublicKeyStart: keyStart(installation), Installation: true, Endorsed: true}}}
	var cp NewestCheckpoint
	err := q.QueryRow(ctx, "SELECT seq, created_at FROM audit_checkpoints ORDER BY seq DESC LIMIT 1").Scan(&cp.Seq, &cp.At)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return out, fmt.Errorf("audit: read the newest checkpoint: %w", err)
	default:
		out.Checkpoint = &cp
	}
	rows, err := q.Query(ctx, "SELECT from_seq, public_key, endorsement FROM audit_signing_keys ORDER BY from_seq")
	if err != nil {
		return out, fmt.Errorf("audit: read the signing keys: %w", err)
	}
	keys, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SigningKey, error) {
		var k SigningKey
		var public, endorsement []byte
		if err := row.Scan(&k.FromSeq, &public, &endorsement); err != nil {
			return k, err
		}
		k.PublicKeyStart = keyStart(public)
		k.Endorsed = len(public) == ed25519.PublicKeySize && ed25519.Verify(installation, keyChangeMessage(k.FromSeq, public), endorsement)
		return k, nil
	})
	if err != nil {
		return out, fmt.Errorf("audit: read the signing keys: %w", err)
	}
	out.Keys = append(out.Keys, keys...)
	return out, nil
}

// LatestOf is the newest event of an action, or nil when there is none. The console uses it
// for markers such as the start of an upgrade.
func LatestOf(ctx context.Context, q db.Querier, action string) (*Event, error) {
	list, err := List(ctx, q, Filter{Action: action, Limit: 1})
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0].Event, nil
}
