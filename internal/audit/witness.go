// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"github.com/diffluence/diffluence/internal/db"
)

// The witness is a copy of every checkpoint in the application log, so it reaches a
// SIEM and lives outside the database (ADR-039). Someone who owns the database can
// remove the tail of the chain together with its checkpoints, but not the lines a log
// collector already took.
const (
	// WitnessMessage is the message of a witness line in the application log.
	WitnessMessage = "audit checkpoint"
	// witnessFormat names the shape of a witness line, so a later shape can be told apart.
	witnessFormat = "audit-checkpoint-1"
	// maxWitnessLine is the longest log line read. Other lines in the same log can be long.
	maxWitnessLine = 1 << 20
	// witnessBatch is how many checkpoints are looked up in one query.
	witnessBatch = 1000
)

// WitnessAttrs are the fields of the witness line for c. They hold no personal data:
// a position, two hashes and a public key.
func (c Checkpoint) WitnessAttrs() []any {
	return []any{"witness", witnessFormat, "seq", c.Seq, "hash", hex.EncodeToString(c.Hash),
		"signature", hex.EncodeToString(c.Signature), "public_key", hex.EncodeToString(c.PublicKey)}
}

type witnessLine struct {
	Witness   string `json:"witness"`
	Seq       int64  `json:"seq"`
	Hash      string `json:"hash"`
	Signature string `json:"signature"`
	PublicKey string `json:"public_key"`
}

// ReadWitness takes the checkpoints out of an application log. Lines that are not
// witness lines are skipped, also lines with a prefix such as a timestamp from the
// container runtime. A witness line that cannot be read is an error, because a log
// that was tampered with must not pass by being skipped.
func ReadWitness(r io.Reader) ([]Checkpoint, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), maxWitnessLine)
	var out []Checkpoint
	for number := 1; scanner.Scan(); number++ {
		line := scanner.Bytes()
		start := bytes.IndexByte(line, '{')
		if start < 0 || !bytes.Contains(line, []byte(witnessFormat)) {
			continue
		}
		var w witnessLine
		if err := json.Unmarshal(line[start:], &w); err != nil || w.Witness != witnessFormat {
			return nil, fmt.Errorf("audit: witness line %d cannot be read", number)
		}
		c, err := w.checkpoint()
		if err != nil {
			return nil, fmt.Errorf("audit: witness line %d: %w", number, err)
		}
		out = append(out, c)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("audit: read witness log: %w", err)
	}
	return out, nil
}

func (w witnessLine) checkpoint() (Checkpoint, error) {
	hash, err := hex.DecodeString(w.Hash)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("hash: %w", err)
	}
	sig, err := hex.DecodeString(w.Signature)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("signature: %w", err)
	}
	public, err := hex.DecodeString(w.PublicKey)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("public key: %w", err)
	}
	if w.Seq <= 0 {
		return Checkpoint{}, fmt.Errorf("position %d", w.Seq)
	}
	return Checkpoint{Seq: w.Seq, Hash: hash, Signature: sig, PublicKey: public}, nil
}

// WitnessResult is what a comparison with the witness found.
type WitnessResult struct {
	// Compared counts the checkpoints that matched the chain.
	Compared int `json:"compared"`
	// Pruned counts the checkpoints from before the retention period, which have no rows left.
	Pruned      int    `json:"pruned"`
	OK          bool   `json:"ok"`
	FirstBadSeq int64  `json:"first_bad_seq,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// CompareWitness checks every checkpoint of the witness against the key and against
// the chain in the database, oldest first, and names the first that disagrees. Run it
// after Check: it trusts that the chain itself is whole.
func CompareWitness(ctx context.Context, q db.Querier, public ed25519.PublicKey, witness []Checkpoint) (WitnessResult, error) {
	res := WitnessResult{OK: true}
	sorted := slices.Clone(witness)
	slices.SortFunc(sorted, func(a, b Checkpoint) int { return cmp.Compare(a.Seq, b.Seq) })
	start, err := latestAnchor(ctx, q)
	if err != nil {
		return res, err
	}
	var prunedUpTo, head int64
	if start != nil {
		prunedUpTo = start.seq
	}
	if err := q.QueryRow(ctx, "SELECT COALESCE(max(seq), 0) FROM audit_events").Scan(&head); err != nil {
		return res, err
	}
	fail := func(seq int64, reason string) (WitnessResult, error) {
		res.OK, res.FirstBadSeq, res.Reason = false, seq, reason
		return res, nil
	}
	for from := 0; from < len(sorted); from += witnessBatch {
		batch := sorted[from:min(from+witnessBatch, len(sorted))]
		hashes, err := chainHashesAt(ctx, q, batch)
		if err != nil {
			return res, err
		}
		for _, c := range batch {
			if !ed25519.Verify(public, checkpointMessage(c.Seq, c.Hash), c.Signature) {
				return fail(c.Seq, "a checkpoint in the witness log has a bad signature")
			}
			if c.Seq <= prunedUpTo {
				res.Pruned++
				continue
			}
			if c.Seq > head {
				return fail(c.Seq, "the chain ends before a checkpoint in the witness log (its tail was removed)")
			}
			chain, found := hashes[c.Seq]
			if !found {
				return fail(c.Seq, "the event of a checkpoint in the witness log is missing")
			}
			if !bytes.Equal(chain, c.Hash) {
				return fail(c.Seq, "the chain differs from a checkpoint in the witness log")
			}
			res.Compared++
		}
	}
	return res, nil
}

// chainHashesAt reads the hashes of the chain at the positions of the checkpoints.
func chainHashesAt(ctx context.Context, q db.Querier, batch []Checkpoint) (map[int64][]byte, error) {
	positions := make([]int64, len(batch))
	for i, c := range batch {
		positions[i] = c.Seq
	}
	rows, err := q.Query(ctx, "SELECT seq, hash FROM audit_events WHERE seq = ANY($1)", positions)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]byte, len(batch))
	for rows.Next() {
		var seq int64
		var hash []byte
		if err := rows.Scan(&seq, &hash); err != nil {
			return nil, err
		}
		out[seq] = hash
	}
	return out, rows.Err()
}
