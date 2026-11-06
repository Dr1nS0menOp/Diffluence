// SPDX-License-Identifier: Apache-2.0

// Package app wires the modules together and runs the roles (api, worker, all).
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/blocks"
	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/console"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/example"
	"github.com/diffluence/diffluence/internal/exchange"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/feed"
	"github.com/diffluence/diffluence/internal/httpapi"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/instance"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/mail"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/people"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/queue"
	"github.com/diffluence/diffluence/internal/scim"
	"github.com/diffluence/diffluence/internal/search"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/sharing"
	"github.com/diffluence/diffluence/internal/spaces"
	"github.com/diffluence/diffluence/internal/sso"
	"github.com/diffluence/diffluence/internal/storage"
	"github.com/diffluence/diffluence/internal/tasks"
	"github.com/diffluence/diffluence/internal/templates"
	"github.com/diffluence/diffluence/internal/trash"
	"github.com/diffluence/diffluence/internal/version"
	"github.com/diffluence/diffluence/internal/watches"
)

// NewLogger returns the JSON logger used everywhere.
func NewLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

// App is one running Diffluence process.
type App struct {
	cfg        config.Config
	log        *slog.Logger
	pool       *pgxpool.Pool
	queue      *queue.Queue
	identity   *identity.Service
	people     *people.Service
	console    *console.Service
	perms      *permissions.Service
	settings   *settings.Service
	features   *features.Service
	templates  *templates.Service
	store      storage.Store
	attach     *attachments.Service
	pages      *pages.Service
	spaces     *spaces.Service
	trash      *trash.Service
	jobs       *jobs.Service
	exchange   *exchange.Service
	example    *example.Service
	search     *search.Service
	scim       *scim.Service
	sso        *sso.Provider
	comments   *comments.Service
	watches    *watches.Service
	tasks      *tasks.Service
	notify     *notify.Service
	mail       *mail.Service
	sharing    *sharing.Service
	feed       *feed.Service
	keyring    *keys.Keyring
	nodeID     uuid.UUID
	api        *httpapi.Server
	instanceID uuid.UUID
	migrations []db.Migration
	draining   atomic.Bool
}

// New connects to the database and queue and builds the modules.
func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	a := &App{cfg: cfg, log: log}
	var err error
	if a.migrations, err = db.Migrations(); err != nil {
		return nil, err
	}
	if a.pool, err = db.Open(ctx, cfg.DatabaseURL); err != nil {
		return nil, err
	}
	if cfg.MigrateOnStart {
		if err := a.migrate(ctx, db.Options{Log: log}); err != nil {
			a.Close()
			return nil, err
		}
	}
	if a.instanceID, err = instance.Ensure(ctx, a.pool, cfg.InstanceName); err != nil {
		a.Close()
		return nil, fmt.Errorf("app: instance: %w", err)
	}
	if a.queue, err = queue.Open(ctx, cfg.NATSURL, cfg.DataDir, log); err != nil {
		a.Close()
		return nil, err
	}
	if a.keyring, err = keys.LoadOrCreate(cfg.KeyFile(), cfg.MasterKeyFile == ""); err != nil {
		a.Close()
		return nil, err
	}
	wa, err := identity.NewWebAuthn(cfg.InstanceName, cfg.PublicURL.Hostname(), cfg.PublicOrigin())
	if err != nil {
		a.Close()
		return nil, fmt.Errorf("app: passkeys: %w", err)
	}
	blocklist, err := identity.LoadBlocklist(cfg.PasswordBlocklistFile)
	if err != nil {
		a.Close()
		return nil, err
	}
	a.identity = identity.NewService(a.pool, a.instanceID, identity.Options{
		SessionIdleTimeout: cfg.SessionIdleTimeout,
		SessionMaxAge:      cfg.SessionMaxAge,
		Keyring:            a.keyring,
		WebAuthn:           wa,
		Issuer:             cfg.InstanceName,
		PasswordBlocklist:  blocklist,
	})
	if a.store, err = storage.NewDisk(filepath.Join(cfg.DataDir, "objects")); err != nil {
		a.Close()
		return nil, err
	}
	a.perms = permissions.NewService(a.pool)
	a.settings = settings.NewService(a.pool, a.store)
	a.identity.SetPolicy(a.accessPolicy())
	a.people = people.NewService(a.pool, a.perms, a.store, a.identity)
	a.console = console.NewService(a.pool, a.store)
	a.features = features.NewService(a.pool, a.settings)
	a.jobs = jobs.NewService(a.pool, log)
	a.jobs.HoldWhile(a.inMaintenance)
	a.attach = attachments.NewService(a.pool, a.perms, a.store, a.settings, a.instanceID)
	a.pages = pages.NewService(a.pool, a.perms, a.attach, a.settings, a.features, a.jobs, a.instanceID)
	a.spaces = spaces.NewService(a.pool, a.perms, a.pages, a.settings, a.features, a.store, a.instanceID)
	a.templates = templates.NewService(a.pool, a.perms, a.pages, a.spaces, a.identity, a.attach)
	a.trash = trash.NewService(a.pool, a.perms, a.identity, log)
	a.trash.Register(trash.TypePage, a.pages)
	a.trash.Register(trash.TypeAttachment, a.attach)
	a.trash.Register(trash.TypeSpace, a.spaces)
	a.comments = comments.NewService(a.pool, a.perms, a.settings, a.features, a.instanceID)
	a.trash.Register(trash.TypeComment, a.comments)
	a.watches = watches.NewService(a.pool, a.perms)
	a.tasks = tasks.NewService(a.pool)
	a.feed = feed.NewService(a.pool)
	a.notify = notify.NewService(a.pool, a.perms, a.settings, a.watches, notify.Options{
		InstanceName: cfg.InstanceName, PublicOrigin: cfg.PublicOrigin(), Keyring: a.keyring})
	a.mail = mail.NewService(a.pool, mailSender(cfg), log)
	a.mail.SetRenderer(a.renderMail)
	a.mail.HoldWhile(a.inMaintenance)
	if a.mail.Configured() {
		a.identity.SetMailer(accountMailer{pool: a.pool, keyring: a.keyring}, cfg.PublicOrigin())
	}
	a.sharing = sharing.NewService(a.pool, a.perms, a.identity, a.settings, a.notify, a.features)
	a.search = search.NewService(a.pool)
	a.exchange = exchange.NewService(a.pool, a.perms, a.pages, a.attach, a.store, a.jobs, cfg.DataDir)
	a.example = example.NewService(a.identity, a.settings, a.spaces, a.pages, a.attach, a.perms, a.comments, a.templates)
	a.scim = scim.NewService(a.pool, a.identity, cfg.PublicOrigin(), log)
	if cfg.OIDC.Issuer != "" {
		a.sso = sso.New(sso.Config{Issuer: cfg.OIDC.Issuer, ClientID: cfg.OIDC.ClientID, ClientSecret: cfg.OIDC.ClientSecret,
			RedirectURL: cfg.PublicOrigin() + "/api/v1/auth/oidc/callback", DisplayName: cfg.OIDC.DisplayName,
			AutoCreate: cfg.OIDC.AutoCreate, GroupsClaim: cfg.OIDC.GroupsClaim}, a.pool, a.identity)
	}

	if cfg.Role.RunsAPI() {
		a.api, err = httpapi.New(httpapi.Deps{Config: cfg, Pool: a.pool, Identity: a.identity,
			Permissions: a.perms, Spaces: a.spaces, Pages: a.pages, Attachments: a.attach, Blocks: blocks.NewService(a.pool), People: a.people, Console: a.console, Search: a.search,
			Trash: a.trash, Settings: a.settings, Jobs: a.jobs, Exchange: a.exchange, Example: a.example,
			SCIM: a.scim, SSO: a.sso, Comments: a.comments, Watches: a.watches, Tasks: a.tasks, Notify: a.notify, Mail: a.mail,
			Sharing: a.sharing, Feed: a.feed, Features: a.features, Templates: a.templates,
			SystemStatus: a.SystemStatus, CheckAudit: a.CheckAudit,
			InstanceID: a.instanceID, Log: log, Ready: a.Ready})
		if err != nil {
			a.Close()
			return nil, err
		}
	}
	return a, nil
}

// accessPolicy hands the rules of the Access settings to identity. A failed read
// falls back to the stricter of the two: the configured limit, and the rule on file.
func (a *App) accessPolicy() identity.Policy {
	read := func(ctx context.Context) settings.Access {
		v := settings.DefaultAccess
		if err := a.settings.ReadSection(ctx, settings.SectionAccess, &v); err != nil {
			a.log.Error("reading access settings failed", "error", err)
		}
		return v
	}
	return identity.Policy{
		IdleLimit:    func(ctx context.Context) time.Duration { return time.Duration(read(ctx).IdleMinutes) * time.Minute },
		SecondFactor: func(ctx context.Context) string { return a.secondFactorRule(ctx, read(ctx).SecondFactor) },
	}
}

// secondFactorRule gives the rule that holds now. A new instance gets time before its
// administrators need a second factor. When its age cannot be read, the rule holds.
func (a *App) secondFactorRule(ctx context.Context, rule string) string {
	if rule != settings.FactorAfterGrace {
		return rule
	}
	var created time.Time
	if err := a.pool.QueryRow(ctx, "SELECT created_at FROM instance").Scan(&created); err != nil {
		a.log.Error("reading the age of the instance failed", "error", err)
		return settings.FactorAdministrators
	}
	if time.Since(created) < settings.SecondFactorGrace {
		return settings.FactorNone
	}
	return settings.FactorAdministrators
}

func (a *App) inMaintenance(ctx context.Context) (bool, error) {
	state, err := a.settings.Maintenance(ctx)
	return state.On, err
}

// mailSender is the mail server, or nothing, and then mail waits in the queue.
func mailSender(cfg config.Config) mail.Sender {
	if cfg.SMTP.Host == "" {
		return nil
	}
	return mail.SMTP{Config: cfg.SMTP, ListID: cfg.InstanceName + " <notifications." + cfg.PublicURL.Hostname() + ">"}
}

// migrate applies migrations, as the owner of the tables when one is configured.
func (a *App) migrate(ctx context.Context, opts db.Options) error {
	if a.cfg.MigrateDatabaseURL == "" {
		_, err := db.Migrate(ctx, a.pool, a.migrations, opts)
		return err
	}
	owner, err := db.Open(ctx, a.cfg.MigrateDatabaseURL)
	if err != nil {
		return err
	}
	defer owner.Close()
	if _, err := db.Migrate(ctx, owner, a.migrations, opts); err != nil {
		return err
	}
	role, err := db.RoleOf(a.cfg.DatabaseURL)
	if err != nil {
		return err
	}
	return db.GrantApp(ctx, owner, role)
}

// Identity exposes the identity service (used by the CLI and tests).
func (a *App) Identity() *identity.Service { return a.identity }

// Pool exposes the database pool (used by the CLI and tests).
func (a *App) Pool() *pgxpool.Pool { return a.pool }

// Routes lists the API routes and their access rules (nil for role=worker).
func (a *App) Routes() []httpapi.Route {
	if a.api == nil {
		return nil
	}
	return a.api.Routes()
}

// Handler returns the HTTP handler for this role.
func (a *App) Handler() http.Handler {
	if a.api != nil {
		return a.api.Handler()
	}
	return httpapi.HealthOnly(a.Ready, a.log)
}

// Ready reports whether this node should receive traffic.
func (a *App) Ready(ctx context.Context) error {
	if a.draining.Load() {
		return httpapi.ReadyError("draining")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := a.pool.Ping(ctx); err != nil {
		a.log.Warn("readiness: database unreachable", "error", err)
		return httpapi.ReadyError("database")
	}
	n, err := db.PendingExpand(ctx, a.pool, a.migrations)
	if err != nil || n > 0 {
		return httpapi.ReadyError("migrations")
	}
	if !a.queue.Healthy() {
		return httpapi.ReadyError("queue")
	}
	if err := a.store.Healthy(ctx); err != nil {
		a.log.Warn("readiness: storage unavailable", "error", err)
		return httpapi.ReadyError("storage")
	}
	return nil
}

// Run serves until ctx is cancelled, then shuts down gracefully: first report
// "not ready" so load balancers stop sending traffic, then finish open requests.
func (a *App) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.cfg.HTTPAddr)
	if err != nil {
		return err
	}
	return a.Serve(ctx, ln)
}

// Serve is Run on an existing listener (tests use port 0).
func (a *App) Serve(ctx context.Context, ln net.Listener) error {
	var wg sync.WaitGroup
	workCtx, stopWork := context.WithCancel(context.Background())
	defer stopWork()

	if a.cfg.Role.RunsWorker() {
		relay := &outbox.Relay{Pool: a.pool, Publisher: a.queue, Log: a.log}
		wg.Add(5)
		go func() { defer wg.Done(); relay.Run(workCtx) }()
		go func() { defer wg.Done(); a.jobs.Run(workCtx) }()
		go func() { defer wg.Done(); a.jobs.RunPeriodic(workCtx, a.PeriodicTasks()) }()
		go func() { defer wg.Done(); a.mail.Run(workCtx) }()
		go func() {
			defer wg.Done()
			if err := a.queue.Consume(workCtx, notificationConsumer, a.handleEvent); err != nil {
				a.log.Error("notifications stopped", "error", err)
			}
		}()
	}

	wg.Add(1)
	go func() { defer wg.Done(); a.heartbeat(workCtx) }()

	srv := &http.Server{
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(a.log.Handler(), slog.LevelWarn),
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	a.log.Info("diffluence started", "version", version.Version, "role", a.cfg.Role, "addr", ln.Addr().String(),
		"instance_id", a.instanceID)

	var runErr error
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = err
		}
	}

	a.draining.Store(true)
	a.log.Info("shutting down", "delay", a.cfg.ShutdownDelay)
	time.Sleep(a.cfg.ShutdownDelay)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && runErr == nil {
		runErr = err
	}
	stopWork()
	wg.Wait()
	a.log.Info("stopped")
	return runErr
}

// notificationConsumer is the durable name of the fan-out in the event stream.
const notificationConsumer = "notifications"

// handleEvent turns one event from the stream into notifications.
func (a *App) handleEvent(ctx context.Context, data []byte) error {
	return a.notify.Publish(ctx, "", data, "")
}

// Notify exposes notifications (used by tests).
func (a *App) Notify() *notify.Service { return a.notify }

// Mail exposes the mail queue (used by tests).
func (a *App) Mail() *mail.Service { return a.mail }

// Logger exposes the logger (used by tests).
func (a *App) Logger() *slog.Logger { return a.log }

// DeliverEvents hands every waiting event straight to the notifications, as the
// worker does through the queue. Tests use it to stay in step.
func (a *App) DeliverEvents(ctx context.Context) error {
	relay := &outbox.Relay{Pool: a.pool, Publisher: a.notify, Log: a.log}
	for {
		n, err := relay.RunOnce(ctx)
		if err != nil || n == 0 {
			return err
		}
	}
}

// Jobs exposes the job runner (used by tests).
func (a *App) Jobs() *jobs.Service { return a.jobs }

// Trash exposes the trash (used by tests).
func (a *App) Trash() *trash.Service { return a.trash }

// Pages exposes pages (used by tests).
func (a *App) Pages() *pages.Service { return a.pages }

// Attachments exposes attachments (used by tests).
func (a *App) Attachments() *attachments.Service { return a.attach }

// exampleSpaceCheck is short, because the first person logs in right after
// creating the administrator. Once decided, a check only reads one setting.
const exampleSpaceCheck = 10 * time.Second

// PeriodicTasks is the work the worker does on a schedule.
func (a *App) PeriodicTasks() []jobs.Task {
	tasks := []jobs.Task{
		{Name: "purge-trash", Every: time.Hour, Run: func(ctx context.Context) error {
			n, err := a.trash.PurgeExpired(ctx)
			if n > 0 {
				a.log.Info("purged expired trash", "items", n)
			}
			return err
		}},
		{Name: "drain-storage", Every: time.Minute, Run: func(ctx context.Context) error {
			_, err := a.attach.DrainDeletions(ctx)
			return err
		}},
		{Name: "fail-stale-jobs", Every: 10 * time.Minute, Run: a.jobs.FailStale},
		{Name: "purge-identity", Every: 10 * time.Minute, Run: a.identity.PurgeExpired},
		{Name: "end-expired-accounts", Every: 10 * time.Minute, Run: a.identity.EndExpiredAccounts},
		{Name: "purge-outbox", Every: time.Hour, Run: func(ctx context.Context) error {
			return outbox.PurgePublished(ctx, a.pool, 7*24*time.Hour)
		}},
		{Name: "audit-verify", Every: 24 * time.Hour, Run: a.verifyAudit},
		{Name: "audit-checkpoint", Every: 24 * time.Hour, Run: a.signCheckpoint},
		{Name: "contract-migrations", Every: time.Minute, Run: a.runContractMigrations},
		{Name: "publish-scheduled", Every: time.Minute, Run: a.pages.PublishDue},
		{Name: "remind-reviews", Every: 24 * time.Hour, Run: a.pages.RemindReviews},
		{Name: "expire-drafts", Every: 24 * time.Hour, Run: a.pages.ExpireDrafts},
		{Name: "task-reminders", Every: time.Hour, Run: a.notify.RemindDue},
		{Name: "notification-digests", Every: time.Hour, Run: a.notify.SendDigests},
		{Name: "purge-notifications", Every: 24 * time.Hour, Run: a.notify.Purge},
	}
	if a.cfg.ExampleSpace {
		tasks = append(tasks, jobs.Task{Name: "example-space", Every: exampleSpaceCheck, Run: a.example.OnNewInstance})
	}
	return tasks
}

// Close releases connections.
func (a *App) Close() {
	if a.queue != nil {
		a.queue.Close()
	}
	if a.pool != nil {
		a.pool.Close()
	}
}
