// SPDX-License-Identifier: Apache-2.0

// Package console answers what the admin console needs to know about the whole
// instance: numbers and sizes, and never the content of a page (ADM-012).
package console

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/storage"
)

const (
	largestRows     = 20
	unusedMinAge    = 24 * time.Hour
	expiryWarning   = "7 days"
	tokenWarning    = "30 days"
	staleNodeMinute = 5
)

// Service reads the state of the instance.
type Service struct {
	pool  *pgxpool.Pool
	store storage.Store
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, store storage.Store) *Service {
	return &Service{pool: pool, store: store}
}

// Summary is the first screen of the console.
type Summary struct {
	Users      int   `json:"users"`
	Spaces     int   `json:"spaces"`
	Pages      int   `json:"pages"`
	Files      int   `json:"files"`
	Storage    int64 `json:"storage"`
	FailedJobs int   `json:"failed_jobs"`
}

// Summary counts what the instance holds.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	var out Summary
	err := s.pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM users WHERE status = 'active' AND type <> 'service'),
			(SELECT count(*) FROM spaces WHERE status <> 'trashed'),
			(SELECT count(*) FROM pages WHERE status = 'published'),
			(SELECT count(*) FROM attachments WHERE status = 'active'),
			(SELECT COALESCE(sum(size), 0)::bigint FROM attachment_versions),
			(SELECT count(*) FROM jobs WHERE status = 'failed')`).
		Scan(&out.Users, &out.Spaces, &out.Pages, &out.Files, &out.Storage, &out.FailedJobs)
	return out, err
}

// Item is something that needs an administrator's attention.
type Item struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// Attention lists what needs a look. An empty list is good news.
func (s *Service) Attention(ctx context.Context, mailReady bool) ([]Item, error) {
	out := []Item{}
	if !mailReady {
		out = append(out, Item{Kind: "mail_not_set"})
	}
	var failed, expiring, removal, oldNodes, tokens int
	var damaged bool
	err := s.pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM jobs WHERE status = 'failed'),
			(SELECT count(*) FROM users WHERE status = 'active' AND expires_at IS NOT NULL
				AND expires_at <= now() + $1::interval),
			(SELECT count(*) FROM audit_events a WHERE a.action = 'user.removal_requested'
				AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id::text = a.object_id AND u.anonymised_at IS NOT NULL)),
			(SELECT GREATEST(count(DISTINCT version) - 1, 0) FROM nodes WHERE last_seen_at > now() - make_interval(mins => $2)),
			(SELECT count(*) FROM scim_tokens WHERE revoked_at IS NULL AND expires_at > now() AND expires_at <= now() + $3::interval)
				+ (SELECT count(*) FROM api_tokens t JOIN users u ON u.id = t.user_id
					WHERE u.type = 'service' AND t.revoked_at IS NULL AND t.expires_at > now() AND t.expires_at <= now() + $3::interval),
			COALESCE((SELECT a.result <> 'success' FROM audit_events a
				WHERE a.action = 'audit.verified' AND a.actor_id = 'worker' ORDER BY a.seq DESC LIMIT 1), false)`,
		expiryWarning, staleNodeMinute, tokenWarning).Scan(&failed, &expiring, &removal, &oldNodes, &tokens, &damaged)
	if err != nil {
		return nil, err
	}
	add := func(kind string, n int) {
		if n > 0 {
			out = append(out, Item{Kind: kind, Count: n})
		}
	}
	add("failed_jobs", failed)
	add("accounts_expiring", expiring)
	add("removal_requests", removal)
	add("nodes_differ", oldNodes)
	add("tokens_expiring", tokens)
	// The result of the daily job: pressing Verify cannot take this away (SEC-814).
	if damaged {
		out = append(out, Item{Kind: "audit_damaged"})
	}
	return out, nil
}

// Sized is one entry of a report: a name and a size, and nothing of the content.
type Sized struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Report lists what takes the most room.
type Report struct {
	Spaces []Sized `json:"spaces"`
	Files  []Sized `json:"files"`
}

func collectSized(rows pgx.Rows, err error) ([]Sized, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Sized, error) {
		var v Sized
		err := r.Scan(&v.Name, &v.Size)
		return v, err
	})
	if out == nil {
		out = []Sized{}
	}
	return out, err
}

// Largest lists the largest spaces and files, by size only.
func (s *Service) Largest(ctx context.Context) (Report, error) {
	var out Report
	rows, err := s.pool.Query(ctx, `SELECT sp.key, COALESCE(sum(v.size), 0)::bigint FROM spaces sp
		LEFT JOIN attachments a ON a.space_id = sp.id
		LEFT JOIN attachment_versions v ON v.attachment_id = a.id
		WHERE sp.status <> 'trashed' GROUP BY sp.key ORDER BY 2 DESC, sp.key LIMIT $1`, largestRows)
	if out.Spaces, err = collectSized(rows, err); err != nil {
		return out, err
	}
	rows, err = s.pool.Query(ctx, `SELECT sp.key, v.size FROM attachment_versions v
		JOIN attachments a ON a.id = v.attachment_id JOIN spaces sp ON sp.id = a.space_id
		ORDER BY v.size DESC LIMIT $1`, largestRows)
	out.Files, err = collectSized(rows, err)
	return out, err
}

// knownKeys lists every stored file something still points at.
func (s *Service) knownKeys(ctx context.Context) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT object_key FROM attachment_versions
		UNION SELECT picture_key FROM user_profiles WHERE picture_key <> ''
		UNION SELECT value->>'object_key' FROM settings WHERE value ? 'object_key'
		UNION SELECT object_key FROM storage_deletions`)
	if err != nil {
		return nil, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out, nil
}

// UnusedFile is a stored file nothing points at.
type UnusedFile struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
}

// UnusedFiles compares the store with the database. Files younger than a day are
// skipped, because an upload may still be on its way to the database.
func (s *Service) UnusedFiles(ctx context.Context) ([]UnusedFile, error) {
	known, err := s.knownKeys(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("console: list files: %v", err)
	}
	out := []UnusedFile{}
	for _, o := range stored {
		if !known[o.Key] && time.Since(o.Modified) > unusedMinAge {
			out = append(out, UnusedFile{Key: o.Key, Size: o.Size})
		}
	}
	return out, nil
}

// RemoveUnusedFiles queues what UnusedFiles reports for removal. It finds them
// again here: what someone saw a moment ago may be in use by now.
func (s *Service) RemoveUnusedFiles(ctx context.Context, c identity.Caller) (int, error) {
	list, err := s.UnusedFiles(ctx)
	if err != nil {
		return 0, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, f := range list {
			if err := storage.QueueDeletion(ctx, tx, f.Key); err != nil {
				return err
			}
		}
		if _, err := audit.Record(ctx, tx, c.Event("storage.unused_removed", "storage", "", "", map[string]any{"files": len(list)})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "storage.unused_removed", map[string]any{"files": len(list)})
		return err
	})
	return len(list), err
}
