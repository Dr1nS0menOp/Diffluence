// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/restore"
)

// ErrRestoring means a restore from the console began: the node stopped serving and must
// wait until it is done (spec 059).
var ErrRestoring = errors.New("app: a restore began")

// restoreWatchEvery is how often a node says what it does and looks for a restore. The
// worker counts a node as gone after restore.NodeGoneAfter, so this is well below it.
const restoreWatchEvery = 5 * time.Second

// watchRestore says every few seconds that this node serves, and closes started once a
// restore begins. One insert and one indexed read on a table of a few rows.
func (a *App) watchRestore(ctx context.Context, started chan<- struct{}) {
	tick := time.NewTicker(restoreWatchEvery)
	defer tick.Stop()
	for {
		if err := restore.Seen(ctx, a.pool, a.nodeID, restore.ModeServing); err != nil && ctx.Err() == nil {
			a.log.Warn("could not say this node serves", "error", err)
		}
		r, err := restore.MustWait(ctx, a.pool)
		switch {
		case err != nil && ctx.Err() == nil:
			a.log.Warn("could not look for a restore", "error", err)
		case r != nil:
			a.log.Warn("a restore began: this node stops serving", "restore", r.ID, "backup", r.BackupName)
			close(started)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// restoreBegan is whether a restore keeps this node from serving, read once before it
// starts to serve.
func (a *App) restoreBegan(ctx context.Context) bool {
	r, err := restore.MustWait(ctx, a.pool)
	if err != nil {
		a.log.Warn("could not look for a restore", "error", err)
		return false
	}
	return r != nil
}

// WaitWhileRestoring keeps a node off the database while a restore runs. When none runs it
// returns at once. Otherwise it serves the waiting page on addr, says every few seconds that
// it waits, and returns once the restore is done or the way back ran. It reads only the
// schema ops, which a restore never touches, through a pool of one connection.
func WaitWhileRestoring(ctx context.Context, databaseURL, addr string, log *slog.Logger) error {
	pool, err := db.Open(ctx, databaseURL)
	if err != nil {
		// The database is not there yet; the start says so as before.
		return nil
	}
	defer pool.Close()
	r, err := restore.MustWait(ctx, pool)
	if err != nil || r == nil {
		return nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return waitForRestore(ctx, pool, ln, r, uuid.New(), restoreWatchEvery, log)
}

// waitForRestore is WaitWhileRestoring on a listener (tests use port 0).
func waitForRestore(ctx context.Context, pool *pgxpool.Pool, ln net.Listener, r *restore.Restore, node uuid.UUID,
	every time.Duration, log *slog.Logger) error {
	page := &restoringPage{}
	page.set(r)
	log.Error("waiting while a restore runs", "restore", r.ID, "state", r.State)
	srv := &http.Server{Handler: page, ReadHeaderTimeout: 5 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	stop := func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), stop())
		case err := <-served:
			return err
		case <-tick.C:
		}
		// While the database is being made again, a look may fail: keep waiting.
		_ = restore.Seen(ctx, pool, node, restore.ModeWaiting)
		now, err := restore.MustWait(ctx, pool)
		if err != nil {
			continue
		}
		if now == nil {
			log.Info("the restore is over: this node starts again")
			return stop()
		}
		page.set(now)
	}
}

// restoringPage answers every request of a node that waits for a restore. Anyone can open
// it, so it names the step but no backup, date or person; those are in the log and in ops.
type restoringPage struct {
	failed atomic.Bool
	step   atomic.Pointer[string]
}

func (p *restoringPage) set(r *restore.Restore) {
	p.failed.Store(r.State == restore.StateFailed)
	step := r.Step
	p.step.Store(&step)
}

func (p *restoringPage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	status, failed, step := "restoring", p.failed.Load(), ""
	if s := p.step.Load(); s != nil {
		step = *s
	}
	if failed {
		status = "restore_failed"
	}
	switch {
	case r.URL.Path == "/livez":
		h.Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("alive, waiting while a restore runs\n"))
	case r.URL.Path == "/readyz":
		writeWaitingJSON(w, map[string]any{"status": status})
	case strings.HasPrefix(r.URL.Path, "/api/"):
		h.Set("Retry-After", waitingRetryAfter)
		writeWaitingJSON(w, map[string]any{"error": map[string]any{
			"code": status, "message": "The installation is being restored from a backup. Try again in a minute."}})
	default:
		h.Set("Retry-After", waitingRetryAfter)
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = restoringTemplate.Execute(w, map[string]any{"Failed": failed, "Step": step})
	}
}

var restoringTemplate = template.Must(template.New("restoring").Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{if .Failed}}A restore stopped{{else}}Restoring from a backup{{end}}</title>
<style>body{font-family:system-ui,sans-serif;max-inline-size:40rem;margin:4rem auto;padding:0 1rem;line-height:1.5;color:#1d1d1f}
code{display:block;background:#f2f2f4;padding:.4rem .6rem;border-radius:4px;margin:.3rem 0;overflow-wrap:anywhere}</style></head>
<body>{{if .Failed}}<h1>A restore stopped</h1>
<p>An administrator restored this installation from a backup, and the restore stopped: {{.Step}}. Nothing is served until it is put right, so nothing more can change.</p>
<p>A backup of the installation as it was just before is kept. An administrator goes back to it with this, in the folder with compose.yaml:</p>
<code>docker compose run --rm backup way-back</code>
{{else}}<h1>Restoring from a backup</h1>
<p>An administrator is restoring this installation from a backup. It is back as soon as that is done; this page looks again every minute.</p>
{{if .Step}}<p>Now: {{.Step}}.</p>{{end}}{{end}}
<p>The full story is in the log: <code>docker compose logs backup</code></p>
</body></html>
`))
