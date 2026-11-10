// SPDX-License-Identifier: Apache-2.0

// Package app wires the modules together and runs the roles (api, worker, all).
package app

import (
	"context"
	"encoding/json"
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

	"github.com/diffluence/diffluence/internal/analytics"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/automation"
	"github.com/diffluence/diffluence/internal/blocks"
	"github.com/diffluence/diffluence/internal/collab"
	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/console"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/example"
	"github.com/diffluence/diffluence/internal/exchange"
	"github.com/diffluence/diffluence/internal/failures"
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
	"github.com/diffluence/diffluence/internal/outward"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/people"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/presence"
	"github.com/diffluence/diffluence/internal/queue"
	"github.com/diffluence/diffluence/internal/savedblocks"
	"github.com/diffluence/diffluence/internal/scim"
	"github.com/diffluence/diffluence/internal/search"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/setup"
	"github.com/diffluence/diffluence/internal/sharing"
	"github.com/diffluence/diffluence/internal/sites"
	"github.com/diffluence/diffluence/internal/spaces"
	"github.com/diffluence/diffluence/internal/sso"
	"github.com/diffluence/diffluence/internal/storage"
	"github.com/diffluence/diffluence/internal/tasks"
	"github.com/diffluence/diffluence/internal/templates"
	"github.com/diffluence/diffluence/internal/trash"
	"github.com/diffluence/diffluence/internal/version"
	"github.com/diffluence/diffluence/internal/watches"
	"github.com/diffluence/diffluence/internal/webhooks"
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
	analytics  *analytics.Service
	presence   *presence.Service
	outward    *outward.Service
	automation *automation.Service
	webhooks   *webhooks.Service
	collab     *collab.Service
	templates  *templates.Service
	store      storage.Store
	encrypted  *storage.Encrypted
	attach     *attachments.Service
	pages      *pages.Service
	spaces     *spaces.Service
	trash      *trash.Service
	jobs       *jobs.Service
	exchange   *exchange.Service
	sites      *sites.Service
	example    *example.Service
	search     *search.Service
	scim       *scim.Service
	sso        *sso.Provider
	comments   *comments.Service
	watches    *watches.Service
	tasks      *tasks.Service
	failures   *failures.Service
	setup      *setup.Service
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
	// The queue opens before migrating, so the checks before an upgrade can see it.
	if a.queue, err = queue.Open(ctx, cfg.NATSURL, cfg.DataDir, log); err != nil {
		a.Close()
		return nil, err
	}
	if err := a.prepareSchema(ctx); err != nil {
		a.Close()
		return nil, err
	}
	if a.instanceID, err = instance.Ensure(ctx, a.pool, cfg.InstanceName); err != nil {
		a.Close()
		return nil, fmt.Errorf("app: instance: %w", err)
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
	disk, err := storage.NewDisk(filepath.Join(cfg.DataDir, "objects"))
	if err != nil {
		a.Close()
		return nil, err
	}
	a.encrypted = storage.NewEncrypted(disk, a.keyring, a.everyFileEncrypted)
	a.store = a.encrypted
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
	a.comments = comments.NewService(a.pool, a.perms, a.settings, a.features, a.pages, a.instanceID)
	a.trash.Register(trash.TypeComment, a.comments)
	a.watches = watches.NewService(a.pool, a.perms)
	a.tasks = tasks.NewService(a.pool)
	a.failures = failures.NewService(a.pool, log)
	a.setup = setup.New(a.pool)
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
	a.analytics = analytics.NewService(a.pool, a.perms, a.features)
	a.presence = presence.NewService(a.pool, a.perms, a.identity, a.settings)
	a.outward = outward.NewService(a.pool, outward.Options{AllowPrivate: cfg.OutwardAllowPrivate})
	a.automation = automation.NewService(automation.Deps{Pool: a.pool, Perms: a.perms, Identity: a.identity, Pages: a.pages,
		Comments: a.comments, Watches: a.watches, Templates: a.templates, Notify: a.notify, Jobs: a.jobs, Features: a.features,
		Outward: a.outward, Origin: cfg.PublicOrigin()})
	a.webhooks = webhooks.NewService(a.pool, a.perms, a.identity, a.outward, a.keyring, a.notify)
	a.collab = collab.NewService(a.pool, a.perms, a.identity, collab.Options{URL: cfg.CollabURL, Token: cfg.CollabToken})
	a.exchange = exchange.NewService(a.pool, a.perms, a.pages, a.attach, a.store, a.jobs, a.settings,
		exchange.Options{TempDir: cfg.DataDir, PDFURL: cfg.PDFURL, PDFPassword: cfg.PDFPassword, CollaboraURL: cfg.CollaboraURL, Log: log, Keyring: a.keyring, InstanceID: a.instanceID})
	a.sites = sites.NewService(a.pool, a.perms, a.pages, a.attach, a.exchange, a.jobs,
		sites.Options{PublishDir: cfg.PublishDir, Origin: cfg.PublicOrigin(), Log: log})
	a.example = example.NewService(a.identity, a.settings, a.spaces, a.pages, a.attach, a.perms, a.comments, a.templates)
	a.scim = scim.NewService(a.pool, a.identity, cfg.PublicOrigin(), log)
	if cfg.OIDC.Issuer != "" {
		a.sso = sso.New(sso.Config{Issuer: cfg.OIDC.Issuer, ClientID: cfg.OIDC.ClientID, ClientSecret: cfg.OIDC.ClientSecret,
			RedirectURL: cfg.PublicOrigin() + "/api/v1/auth/oidc/callback", DisplayName: cfg.OIDC.DisplayName,
			AutoCreate: cfg.OIDC.AutoCreate, GroupsClaim: cfg.OIDC.GroupsClaim}, a.pool, a.identity)
	}

	if cfg.Role.RunsAPI() {
		a.api, err = httpapi.New(httpapi.Deps{Config: cfg, Pool: a.pool, Identity: a.identity,
			Permissions: a.perms, Spaces: a.spaces, Pages: a.pages, Attachments: a.attach, Blocks: blocks.NewService(a.pool, a.perms, a.people, a.analytics), People: a.people, Console: a.console, Search: a.search,
			Trash: a.trash, Settings: a.settings, Jobs: a.jobs, Exchange: a.exchange, Sites: a.sites, Example: a.example,
			SCIM: a.scim, SSO: a.sso, Comments: a.comments, Watches: a.watches, Tasks: a.tasks, Notify: a.notify, Mail: a.mail,
			Sharing: a.sharing, SavedBlocks: savedblocks.NewService(a.pool), Feed: a.feed, Features: a.features, Templates: a.templates,
			Analytics: a.analytics, Presence: a.presence, Outward: a.outward, Automation: a.automation, Webhooks: a.webhooks, Collab: a.collab,
			SystemStatus: a.SystemStatus, HelpersDown: a.HelpersDown, CheckAudit: a.CheckAudit, Gauges: a.Gauges, Failures: a.failures,
			Setup: a.setup, InstanceID: a.instanceID, Log: log, Ready: a.Ready})
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
	return a.asOwner(ctx, func(owner *pgxpool.Pool) error {
		_, err := db.Migrate(ctx, owner, a.migrations, opts)
		return err
	})
}

// asOwner runs work with the pool of the owner of the tables when one is configured, and
// then gives the application role its rights: before that, it may not even read what a
// migration made.
func (a *App) asOwner(ctx context.Context, work func(*pgxpool.Pool) error) error {
	if a.cfg.MigrateDatabaseURL == "" {
		return work(a.pool)
	}
	owner, err := db.Open(ctx, a.cfg.MigrateDatabaseURL)
	if err != nil {
		return err
	}
	defer owner.Close()
	if err := work(owner); err != nil {
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

// Setup returns the guard of the first setup.
func (a *App) Setup() *setup.Service { return a.setup }

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
// needsQueueToBeReady says whether a queue outage takes this node out of service.
// An API node only writes to the outbox, which waits in the database, so it can go
// on serving people (ARC-216).
func needsQueueToBeReady(role config.Role) bool { return role.RunsWorker() }

func (a *App) Ready(ctx context.Context) error {
	if a.draining.Load() {
		return httpapi.ReadyError("draining")
	}
	// A ping on a full pool waits for a connection and then fails, which would name the
	// wrong cause. A pool with every connection taken is busy, not gone (ARC-235).
	if stat := a.pool.Stat(); stat.AcquiredConns() >= stat.MaxConns() {
		return httpapi.ReadyError("busy")
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
	if needsQueueToBeReady(a.cfg.Role) && !a.queue.Healthy() {
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
		relay := &outbox.Relay{Pool: a.pool, Publisher: a.queue, Log: a.log, Refused: func(ctx context.Context, data []byte, cause error) error {
			return a.failures.Record(ctx, "relay", data, cause)
		}}
		wg.Add(4)
		go func() { defer wg.Done(); relay.Run(workCtx) }()
		go func() { defer wg.Done(); a.jobs.Run(workCtx) }()
		go func() { defer wg.Done(); a.jobs.RunPeriodic(workCtx, a.PeriodicTasks()) }()
		go func() { defer wg.Done(); a.mail.Run(workCtx) }()
		for durable, handle := range a.eventListeners() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				listener := queue.Listener{Durable: durable, Handle: handle, GiveUp: a.failures.Record}
				if err := a.queue.Consume(workCtx, listener); err != nil {
					a.log.Error("event consumer stopped", "consumer", durable, "error", err)
				}
			}()
		}
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
	if a.cfg.Role.RunsAPI() {
		a.announceSetup(ctx)
	}
	if a.cfg.PlainToOthers() {
		a.log.Warn("DIFFLUENCE_PUBLIC_URL is plain http on another host: cookies go without Secure and no HSTS is sent. Set it to the https address people use.",
			"public_origin", a.cfg.PublicOrigin())
	}

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

// The durable names of the consumers that match events to rules and to webhooks.
const (
	automationConsumer = "automation"
	webhookConsumer    = "webhooks"
	collabConsumer     = "collab"
)

// handleEvent turns one event from the stream into notifications.
func (a *App) handleEvent(ctx context.Context, data []byte) error {
	var m outbox.Message
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if err := a.notify.Handle(ctx, m); err != nil {
		return err
	}
	return a.sites.Handle(ctx, m)
}

// Notify exposes notifications (used by tests).
func (a *App) Notify() *notify.Service { return a.notify }

// Mail exposes the mail queue (used by tests).
func (a *App) Mail() *mail.Service { return a.mail }

// Logger exposes the logger (used by tests).
func (a *App) Logger() *slog.Logger { return a.log }

// everyConsumer hands an event to all consumers at once, as the stream does.
type everyConsumer struct{ a *App }

// Publish implements the publisher of the relay.
func (e everyConsumer) Publish(ctx context.Context, _ string, data []byte, _ string) error {
	if err := e.a.handleEvent(ctx, data); err != nil {
		return err
	}
	if err := e.a.automation.Handle(ctx, data); err != nil {
		return err
	}
	if err := e.a.webhooks.Handle(ctx, data); err != nil {
		return err
	}
	return e.a.collab.Handle(ctx, data)
}

// DeliverEvents hands every waiting event straight to the notifications, as the
// worker does through the queue. Tests use it to stay in step.
func (a *App) DeliverEvents(ctx context.Context) error {
	relay := &outbox.Relay{Pool: a.pool, Publisher: everyConsumer{a}, Log: a.log}
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

// Exchange exposes import and export (used by tests).
func (a *App) Exchange() *exchange.Service { return a.exchange }

// Sites exposes public sites (used by tests).
func (a *App) Sites() *sites.Service { return a.sites }

// Attachments exposes attachments (used by tests).
func (a *App) Attachments() *attachments.Service { return a.attach }

// exampleSpaceCheck is short, because the first person logs in right after
// creating the administrator. Once decided, a check only reads one setting.
const exampleSpaceCheck = 10 * time.Second

// Automation exposes rules (used by tests).
func (a *App) Automation() *automation.Service { return a.automation }

// Webhooks exposes webhooks (used by tests).
func (a *App) Webhooks() *webhooks.Service { return a.webhooks }

// CloseAnalyticsDays runs the daily closing now, for tests and for operators.
func (a *App) CloseAnalyticsDays(ctx context.Context) error { return a.analytics.CloseDays(ctx) }

// eventListeners are the durable consumers, by name.
func (a *App) eventListeners() map[string]failures.Handler {
	return map[string]failures.Handler{
		notificationConsumer: a.handleEvent, automationConsumer: a.automation.Handle,
		webhookConsumer: a.webhooks.Handle, collabConsumer: a.collab.Handle,
	}
}

// Failures exposes the events listeners gave up on (used by tests).
func (a *App) Failures() *failures.Service { return a.failures }

// ReplayEvents runs the replays that were asked for.
func (a *App) ReplayEvents(ctx context.Context) error {
	return a.failures.RunReplays(ctx, a.eventListeners())
}

// PeriodicTasks is the work the worker does on a schedule.
func (a *App) PeriodicTasks() []jobs.Task {
	tasks := []jobs.Task{
		{Name: "close-analytics-days", Every: time.Hour, Run: a.analytics.CloseDays},
		{Name: "forget-presence", Every: time.Minute, Run: a.presence.Forget},
		{Name: "trim-recent-pages", Every: time.Hour, Run: a.pages.TrimRecent},
		{Name: "replay-events", Every: time.Minute, Run: a.ReplayEvents},
		{Name: "purge-event-failures", Every: 24 * time.Hour, Run: a.failures.Purge},
		{Name: "run-schedules", Every: time.Minute, Run: a.automation.RunSchedules},
		{Name: "purge-rule-runs", Every: 24 * time.Hour, Run: a.automation.PurgeRuns},
		{Name: "deliver-webhooks", Every: time.Minute, Run: a.webhooks.DeliverDue},
		{Name: "purge-webhook-log", Every: 24 * time.Hour, Run: a.webhooks.PurgeLog},
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
		{Name: "encrypt-stored-files", Every: 10 * time.Minute, Run: a.encryptStoredFiles},
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
		{Name: "purge-exports", Every: 10 * time.Minute, Run: a.exchange.PurgeExports},
		{Name: "purge-previews", Every: 24 * time.Hour, Run: a.exchange.PurgePreviews},
		{Name: "purge-imports", Every: time.Hour, Run: a.exchange.PurgeImports},
		{Name: "build-sites", Every: time.Minute, Run: a.sites.RunDue},
		{Name: "build-sites-daily", Every: 24 * time.Hour, Run: a.sites.RunDaily},
		{Name: "check-helpers", Every: time.Minute, Run: a.checkHelpers},
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
