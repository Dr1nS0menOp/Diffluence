// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// RetryUpgradeEvery is how often a waiting node checks again whether it may migrate.
const RetryUpgradeEvery = time.Minute

// waitingRetryAfter is what a waiting node tells clients to wait, in seconds.
const waitingRetryAfter = "60"

// WaitWhileRefused serves addr while an upgrade is refused, so the process stays alive
// and people see why instead of a restart loop (spec 054). It touches no database. It
// returns nil when retry fires, with the address free again for the real server, and the
// context's error when the node is stopped.
func WaitWhileRefused(ctx context.Context, addr string, refused *UpgradeRefused, retry <-chan struct{}, log *slog.Logger) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return waitOn(ctx, ln, refused, retry, log)
}

// waitOn is WaitWhileRefused on an existing listener (tests use port 0).
func waitOn(ctx context.Context, ln net.Listener, refused *UpgradeRefused, retry <-chan struct{}, log *slog.Logger) error {
	log.Error("waiting before the upgrade: "+refused.Error(), "retry_every", RetryUpgradeEvery.String())
	srv := &http.Server{Handler: waitingHandler(refused), ReadHeaderTimeout: 5 * time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	var stopped error
	select {
	case <-ctx.Done():
		stopped = ctx.Err()
	case <-retry:
	case err := <-served:
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return errors.Join(stopped, err)
	}
	if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.Join(stopped, err)
	}
	return stopped
}

// waitingHandler answers every request of a waiting node. The page and /readyz name only
// the kinds of problem: anyone can open them, and free space or backup dates are an
// administrator's business. The full reasons are in the log.
func waitingHandler(refused *UpgradeRefused) http.Handler {
	kinds := refused.Kinds()
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		switch {
		case r.URL.Path == "/livez":
			h.Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("alive, waiting before the upgrade\n"))
		case r.URL.Path == "/readyz":
			writeWaitingJSON(w, map[string]any{"status": "waiting_for_upgrade", "problems": names})
		case strings.HasPrefix(r.URL.Path, "/api/"):
			h.Set("Retry-After", waitingRetryAfter)
			writeWaitingJSON(w, map[string]any{"error": map[string]any{
				"code": "upgrade_waiting", "message": "The server waits before an upgrade. Try again in a minute.",
				"problems": names}})
		default:
			h.Set("Retry-After", waitingRetryAfter)
			h.Set("Content-Type", "text/html; charset=utf-8")
			h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = waitingPage.Execute(w, waitingFixes(kinds))
		}
	})
}

func writeWaitingJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(body)
}

// waitingFix is what the page says about one kind of problem.
type waitingFix struct {
	Title    string
	Commands []string
}

func waitingFixes(kinds []ProblemKind) []waitingFix {
	fixes := make([]waitingFix, 0, len(kinds))
	for _, k := range kinds {
		switch k {
		case ProblemBackup:
			fixes = append(fixes, waitingFix{"No recent backup was noted", []string{
				"docker compose run --rm backup once",
				"bash scripts/backup.sh /path/to/backups /path/to/backup.key",
				"docker compose run --rm diffluence migrate --without-backup   (once, without a backup)",
			}})
		case ProblemDisk:
			fixes = append(fixes, waitingFix{"The data folder is nearly full", []string{"docker compose logs diffluence"}})
		case ProblemQueue:
			fixes = append(fixes, waitingFix{"The queue does not answer", []string{"docker compose logs diffluence"}})
		case ProblemExtension:
			fixes = append(fixes, waitingFix{"A PostgreSQL extension is missing", []string{"docker compose logs diffluence"}})
		}
	}
	return fixes
}

var waitingPage = template.Must(template.New("waiting").Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Waiting before an upgrade</title>
<style>body{font-family:system-ui,sans-serif;max-inline-size:40rem;margin:4rem auto;padding:0 1rem;line-height:1.5;color:#1d1d1f}
code{display:block;background:#f2f2f4;padding:.4rem .6rem;border-radius:4px;margin:.3rem 0;overflow-wrap:anywhere}</style></head>
<body><h1>Waiting before an upgrade</h1>
<p>A new version is installed, and it waits before it changes the database. It tries again every minute, and starts as soon as it may. Nothing has changed yet.</p>
{{range .}}<h2>{{.Title}}</h2><p>An administrator can fix it with one of these, in the folder with compose.yaml:</p>{{range .Commands}}<code>{{.}}</code>{{end}}{{end}}
<p>The full reasons are in the log: <code>docker compose logs diffluence</code></p>
</body></html>
`))
