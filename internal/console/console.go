// SPDX-License-Identifier: Apache-2.0

// Package console answers what the admin console needs to know about the whole
// instance: numbers and sizes, and never the content of a page (ADM-012).
package console

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/health"
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

// orphanedRestrictions selects the pages whose view restriction only names people who
// are gone. Nobody can open such a page until an administrator recovers it (ACC-073).
// A group counts as someone: its members can change.
const orphanedRestrictions = `SELECT DISTINCT r.page_id, p.space_id FROM page_restrictions r
	JOIN pages p ON p.id = r.page_id
	WHERE r.kind = 'view' AND p.status <> 'deleted' AND NOT EXISTS (
		SELECT 1 FROM page_restrictions other
		LEFT JOIN users u ON other.subject_type = 'user' AND u.id = other.subject_id
		WHERE other.page_id = r.page_id AND other.kind = 'view'
			AND (other.subject_type <> 'user' OR u.status = 'active'))`

// OrphanedPage is a restricted page nobody can open. The title stays out: an
// administrator has not recovered it yet.
type OrphanedPage struct {
	ID        uuid.UUID `json:"id"`
	SpaceName string    `json:"space_name"`
}

// OrphanedPages lists what an administrator can recover. The route requires
// administrator.
func (s *Service) OrphanedPages(ctx context.Context) ([]OrphanedPage, error) {
	rows, err := s.pool.Query(ctx, `SELECT o.page_id, sp.name FROM (`+orphanedRestrictions+`) o
		JOIN spaces sp ON sp.id = o.space_id ORDER BY sp.name, o.page_id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[OrphanedPage])
	if out == nil {
		out = []OrphanedPage{}
	}
	return out, err
}

// Attention lists what needs a look. An empty list is good news.
func (s *Service) Attention(ctx context.Context, mailReady bool) ([]Item, error) {
	out := []Item{}
	if !mailReady {
		out = append(out, Item{Kind: "mail_not_set"})
	}
	var failed, expiring, removal, oldNodes, tokens, gaveUp, waiting, orphaned, invitations, helpersDown int
	var damaged bool
	err := s.pool.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM jobs WHERE status = 'failed'),
			(SELECT count(*) FROM users WHERE status = 'active' AND expires_at IS NOT NULL
				AND expires_at <= now() + $1::interval),
			(SELECT count(*) FROM audit_entries a WHERE a.action = 'user.removal_requested'
				AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id::text = a.object_id AND u.anonymised_at IS NOT NULL)),
			(SELECT GREATEST(count(DISTINCT version) - 1, 0) FROM nodes WHERE last_seen_at > now() - make_interval(mins => $2)),
			(SELECT count(*) FROM scim_tokens WHERE revoked_at IS NULL AND expires_at > now() AND expires_at <= now() + $3::interval)
				+ (SELECT count(*) FROM api_tokens t JOIN users u ON u.id = t.user_id
					WHERE u.type = 'service' AND t.revoked_at IS NULL AND t.expires_at > now() AND t.expires_at <= now() + $3::interval),
			(SELECT count(*) FROM event_failures WHERE replayed_at IS NULL),
			(SELECT count(*) FROM (`+orphanedRestrictions+`) orphaned),
			(SELECT count(*) FROM outbox_events WHERE published_at IS NULL AND created_at < now() - interval '1 day'),
			(SELECT count(*) FROM invitation_requests WHERE status = 'waiting'),
			(SELECT count(*) FROM helper_health WHERE NOT ok),
			COALESCE((SELECT a.result <> 'success' FROM audit_entries a
				WHERE a.action = 'audit.verified' AND a.actor_id = 'worker' ORDER BY a.occurred_at DESC LIMIT 1), false)`,
		expiryWarning, staleNodeMinute, tokenWarning).Scan(&failed, &expiring, &removal, &oldNodes, &tokens, &gaveUp, &orphaned, &waiting, &invitations, &helpersDown, &damaged)
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
	add("invitations_waiting", invitations)
	add("events_failed", gaveUp)
	add("restrictions_orphaned", orphaned)
	// An optional helper that stopped answering, from the worker's minute check (ARC-072).
	add("helpers_down", helpersDown)
	// The stream forgets after 7 days, so this warns while there is time (ARC-053).
	add("events_waiting", waiting)
	// The result of the daily job: pressing Verify cannot take this away (SEC-814).
	if damaged {
		out = append(out, Item{Kind: "audit_damaged"})
	}
	// Uploads fail on a full disk while text still saves (docs/operations/limits.md), so
	// this warns early. The count is the percentage still free.
	if disk, ok := s.store.(storage.Spacer); ok {
		if free, total, err := disk.Space(ctx); err == nil {
			if percent, full := DiskNearlyFull(free, total); full {
				out = append(out, Item{Kind: "disk_nearly_full", Count: percent})
			}
		}
	}
	return out, nil
}

// DiskNearlyFull says whether the free part of a disk is below the warning of the health
// checks, and gives the free percentage. An unknown size never warns.
func DiskNearlyFull(free, total uint64) (percent int, full bool) {
	if total == 0 {
		return 0, false
	}
	percent = int(free * 100 / total)
	return percent, percent < health.DiskWarnPercent
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
	Pages  []Sized `json:"pages"`
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
	if out.Files, err = collectSized(rows, err); err != nil {
		return out, err
	}
	// The key of the space and the id of the page: a title is content.
	rows, err = s.pool.Query(ctx, `SELECT sp.key || '/' || p.id, pg_column_size(v.document)::bigint FROM pages p
		JOIN page_versions v ON v.page_id = p.id AND v.number = p.current_version
		JOIN spaces sp ON sp.id = p.space_id
		WHERE p.status = 'published' ORDER BY 2 DESC LIMIT $1`, largestRows)
	out.Pages, err = collectSized(rows, err)
	return out, err
}

// SpaceTotal is what the list of spaces shows beside a name (ADM-033).
type SpaceTotal struct {
	ID           uuid.UUID  `json:"id"`
	Pages        int        `json:"pages"`
	Bytes        int64      `json:"bytes"`
	LastActivity *time.Time `json:"last_activity"`
	// LimitMiB is the storage limit of the space, and zero means none.
	LimitMiB int `json:"limit_mib"`
}

// SpaceTotals counts pages, stored bytes and the last change of every space. The route
// requires administrator.
func (s *Service) SpaceTotals(ctx context.Context) ([]SpaceTotal, error) {
	rows, err := s.pool.Query(ctx, `SELECT sp.id,
			(SELECT count(*) FROM pages p WHERE p.space_id = sp.id AND p.status = 'published'),
			-- The same count as the storage limit: each stored object once (spec 047, D19).
			(SELECT COALESCE(sum(stored.size), 0)::bigint FROM (SELECT DISTINCT v.object_key, v.size FROM attachments a
				JOIN attachment_versions v ON v.attachment_id = a.id WHERE a.space_id = sp.id) stored),
			(SELECT max(p.updated_at) FROM pages p WHERE p.space_id = sp.id),
			sp.storage_limit_mib
		FROM spaces sp WHERE sp.status <> 'trashed'`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[SpaceTotal])
	if out == nil {
		out = []SpaceTotal{}
	}
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
