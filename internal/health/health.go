// SPDX-License-Identifier: Apache-2.0

// Package health holds the rules of the health checks (spec 051): from facts an
// installation knows about itself to a short list of what is fine, what needs a look soon
// and what needs a person now. It reads nothing itself, so the console, the metrics and
// "diffluence doctor" judge alike, and every threshold can be tested on its own.
package health

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// State is how a check came out.
type State string

// The states, from good to bad.
const (
	OK      State = "ok"
	Warn    State = "warn"
	Problem State = "problem"
)

// The thresholds (spec 048 D8 to D10, spec 051 D2).
const (
	// DiskWarnPercent is how much of the data folder may be left before administrators are
	// warned.
	DiskWarnPercent = 10
	// DiskRefusePercent is below which uploads, imports and exports are refused, so the
	// room left goes to text.
	DiskRefusePercent = 2
	// BackupWarnAfter tolerates a daily backup that runs a little late.
	BackupWarnAfter = 26 * time.Hour
	// ContractWaitWarnAfter is how long a contract migration may wait for every node, on top
	// of the grace it waits on purpose.
	ContractWaitWarnAfter = 24 * time.Hour
	// ReleaseWarnAfter is when a release counts as behind, from its own build date.
	ReleaseWarnAfter = 60 * 24 * time.Hour
	// GrowthWindow is how far back the growth of the database is measured, and how far
	// ahead the disk check looks.
	GrowthWindow = 30 * 24 * time.Hour
	// AuditSealWarnAfter is how long an audit event may wait for the sealer before
	// administrators are asked to look (spec 052). A worker seals every second, so a minute
	// means no worker seals.
	AuditSealWarnAfter = time.Minute
	// AuditSealProblemAfter is when waiting events need a person: they are not covered by a
	// signed checkpoint or the witness until they are sealed (ADR-041).
	AuditSealProblemAfter = 15 * time.Minute
)

// Names are the checks, in the order the console shows them.
var Names = []string{"disk", "database", "backup", "migrations", "work", "audit", "mail", "login", "version"}

// Facts is what an installation knows about itself. A zero value means "not known" and
// never warns, except where nothing known is itself the news (no backup noted).
type Facts struct {
	Now time.Time
	// DiskFree and DiskTotal are of the folder the files are stored in; zero total = unknown.
	DiskFree, DiskTotal uint64
	// DatabaseBytes is the last measured size of the database; zero = not measured yet.
	DatabaseBytes int64
	// Growth30d is how much the database grew over GrowthWindow; GrowthKnown says whether
	// a measurement that old exists.
	Growth30d   int64
	GrowthKnown bool
	// LastBackup is when a backup was last noted; zero = never.
	LastBackup time.Time
	// BackupFailed says the worker's latest backup failed; RestoreCheckFailed says the
	// latest restore into a throw-away copy failed (spec 058). Both false when none ran.
	BackupFailed       bool
	RestoreCheckFailed bool
	// PendingContract counts contract migrations of this release not applied yet;
	// LastMigrationAt is when the last migration was applied.
	PendingContract int
	LastMigrationAt time.Time
	// ContractGrace is how long contract migrations wait on purpose (DIFFLUENCE_CONTRACT_GRACE).
	ContractGrace time.Duration
	FailedJobs7d  int
	// OutboxStuck counts events that waited more than an hour to reach the queue.
	OutboxStuck   int
	EventFailures int
	// MailConfigured says whether a mail server is set; MailFailed1d counts mails that
	// failed in the last day.
	MailConfigured bool
	MailFailed1d   int
	// AuditWaiting counts audit events that wait for the sealer; OldestAuditWaiting is when
	// the oldest of them happened, zero when none wait.
	AuditWaiting       int64
	OldestAuditWaiting time.Time
	// PasswordStops counts accounts whose password login stopped after wrong guesses.
	PasswordStops  int
	OIDCConfigured bool
	// OneLoginAddress is the address every login of the last day came from, when there
	// were logins of more than one person and that address is a private or loopback one;
	// empty otherwise. It is the trace of a reverse proxy that is not trusted.
	OneLoginAddress string
	// BuildDate is the date of the commit this release was built from; zero = unknown.
	BuildDate time.Time
	Version   string
}

// Check is the result of one rule.
type Check struct {
	Name   string         `json:"name"`
	State  State          `json:"state"`
	Code   string         `json:"code"`
	Values map[string]any `json:"values"`
}

func check(name string, state State, code string, values map[string]any) Check {
	if values == nil {
		values = map[string]any{}
	}
	return Check{Name: name, State: state, Code: code, Values: values}
}

// Evaluate judges the facts. It returns one check per name in Names.
func Evaluate(f Facts) []Check {
	return []Check{
		disk(f), database(f), backup(f), migrations(f), work(f), auditSealing(f), mail(f), login(f), release(f),
	}
}

// auditSealing says whether the worker keeps the audit chain up to date. Events that wait
// only a moment are normal, so the line then reads the same as when none wait.
func auditSealing(f Facts) Check {
	if f.AuditWaiting == 0 || f.OldestAuditWaiting.IsZero() {
		return check("audit", OK, "audit_ok", nil)
	}
	lag := f.Now.Sub(f.OldestAuditWaiting)
	values := map[string]any{"count": f.AuditWaiting, "seconds": int(lag.Seconds())}
	switch {
	case lag > AuditSealProblemAfter:
		return check("audit", Problem, "audit_stuck", values)
	case lag > AuditSealWarnAfter:
		return check("audit", Warn, "audit_lagging", values)
	}
	return check("audit", OK, "audit_ok", values)
}

// percentFree is the free part of a disk in whole percent, rounded down.
func percentFree(free, total uint64) uint64 { return free * 100 / total }

// RefusesLargeWrites says whether the store should refuse files. An unknown size never
// refuses: a store that cannot tell must not stop people.
func RefusesLargeWrites(free, total uint64) bool {
	return total > 0 && percentFree(free, total) < DiskRefusePercent
}

func disk(f Facts) Check {
	if f.DiskTotal == 0 {
		return check("disk", OK, "disk_unknown", nil)
	}
	percent := percentFree(f.DiskFree, f.DiskTotal)
	values := map[string]any{"free": f.DiskFree, "total": f.DiskTotal, "percent": percent}
	switch {
	case RefusesLargeWrites(f.DiskFree, f.DiskTotal):
		return check("disk", Problem, "disk_full", values)
	case percent < DiskWarnPercent:
		return check("disk", Warn, "disk_low", values)
	case f.GrowthKnown && f.Growth30d > 0 && uint64(f.Growth30d) > f.DiskFree:
		return check("disk", Warn, "disk_fills", values)
	}
	return check("disk", OK, "disk_ok", values)
}

func database(f Facts) Check {
	if f.DatabaseBytes == 0 {
		return check("database", OK, "database_unknown", nil)
	}
	values := map[string]any{"bytes": f.DatabaseBytes}
	if f.GrowthKnown {
		values["growth"] = f.Growth30d
	}
	return check("database", OK, "database_size", values)
}

func backup(f Facts) Check {
	if f.LastBackup.IsZero() {
		return check("backup", Warn, "backup_none", nil)
	}
	age := f.Now.Sub(f.LastBackup)
	values := map[string]any{"at": f.LastBackup.UTC().Format(time.RFC3339), "hours": int(age.Hours())}
	// A failure is the news even while an older backup is still recent.
	switch {
	case f.BackupFailed:
		return check("backup", Warn, "backup_failed", values)
	case f.RestoreCheckFailed:
		return check("backup", Warn, "restore_check_failed", values)
	}
	if age > BackupWarnAfter {
		return check("backup", Warn, "backup_old", values)
	}
	return check("backup", OK, "backup_ok", values)
}

func migrations(f Facts) Check {
	if f.PendingContract == 0 {
		return check("migrations", OK, "migrations_ok", nil)
	}
	values := map[string]any{"count": f.PendingContract}
	if !f.LastMigrationAt.IsZero() && f.Now.Sub(f.LastMigrationAt) > f.ContractGrace+ContractWaitWarnAfter {
		return check("migrations", Warn, "contract_waiting", values)
	}
	return check("migrations", OK, "contract_pending", values)
}

func work(f Facts) Check {
	values := map[string]any{"jobs": f.FailedJobs7d, "events": f.EventFailures, "outbox": f.OutboxStuck}
	switch {
	case f.OutboxStuck > 0:
		return check("work", Problem, "outbox_stuck", values)
	case f.FailedJobs7d > 0 || f.EventFailures > 0:
		return check("work", Warn, "work_failed", values)
	}
	return check("work", OK, "work_ok", values)
}

func mail(f Facts) Check {
	switch {
	case !f.MailConfigured:
		return check("mail", Warn, "mail_off", nil)
	case f.MailFailed1d > 0:
		return check("mail", Warn, "mail_failed", map[string]any{"count": f.MailFailed1d})
	}
	return check("mail", OK, "mail_ok", nil)
}

func login(f Facts) Check {
	values := map[string]any{"single_sign_on": f.OIDCConfigured}
	if f.PasswordStops > 0 {
		values["count"] = f.PasswordStops
		return check("login", Warn, "password_stopped", values)
	}
	// Then every person counts as one address, so the login limits of one count for all.
	if f.OneLoginAddress != "" {
		values["address"] = f.OneLoginAddress
		return check("login", Warn, "proxy_not_trusted", values)
	}
	return check("login", OK, "login_ok", values)
}

func release(f Facts) Check {
	values := map[string]any{"version": f.Version}
	if f.BuildDate.IsZero() {
		return check("version", OK, "version_unknown", values)
	}
	age := f.Now.Sub(f.BuildDate)
	values["date"] = f.BuildDate.UTC().Format(time.DateOnly)
	values["days"] = int(age.Hours() / 24)
	if age > ReleaseWarnAfter {
		return check("version", Warn, "version_old", values)
	}
	return check("version", OK, "version_ok", values)
}

// Count says how many checks are problems and how many are warnings.
func Count(checks []Check) (problems, warnings int) {
	for _, c := range checks {
		switch c.State {
		case Problem:
			problems++
		case Warn:
			warnings++
		}
	}
	return problems, warnings
}

// Sentences say each result in plain English, and what to do, for the command line. The
// console has the same sentences in its translation strings. A {name} is replaced by a
// value of the check.
var Sentences = map[string]string{
	"disk_unknown":      "This store cannot tell how much room it has.",
	"disk_ok":           "{percent}% of the disk is free.",
	"disk_low":          "Only {percent}% of the disk is free. Make room or add space before files are refused at 2%.",
	"disk_full":         "Only {percent}% of the disk is free, so uploads, imports and exports are refused. Text still saves. Make room now.",
	"disk_fills":        "{percent}% of the disk is free, but at the growth of the last 30 days it fills within a month. Plan more space.",
	"database_unknown":  "The size of the database is measured once an hour; it has not been measured yet.",
	"database_size":     "The database holds {bytes} bytes.",
	"backup_none":       "No backup was ever noted. Run scripts/backup.sh, or the backup service, on a schedule.",
	"backup_ok":         "The last backup was noted {hours} hours ago.",
	"backup_old":        "The last backup was noted {hours} hours ago. Check that the scheduled backup still runs.",
	"migrations_ok":     "Every migration of this release is applied.",
	"contract_pending":  "{count} contract migration(s) wait until every node runs this release. That is normal during an upgrade.",
	"contract_waiting":  "{count} contract migration(s) have waited a day longer than their grace period. Check that every node was upgraded.",
	"work_ok":           "Background work runs.",
	"work_failed":       "{jobs} job(s) failed in the last 7 days and {events} event(s) gave up. Look under Care.",
	"outbox_stuck":      "{outbox} event(s) have waited more than an hour to reach the queue. Check the queue and the worker.",
	"audit_ok":          "Audit events are sealed in the chain within seconds.",
	"audit_lagging":     "{count} audit event(s) have waited {seconds} seconds to be sealed. Check that a worker runs.",
	"audit_stuck":       "{count} audit event(s) have waited {seconds} seconds to be sealed, so no checkpoint covers them. Start a worker now.",
	"mail_off":          "No mail server is set, so nobody gets mail. Set DIFFLUENCE_SMTP_HOST and DIFFLUENCE_SMTP_FROM.",
	"mail_ok":           "Mail is set up.",
	"mail_failed":       "{count} mail(s) failed in the last day. Check the mail server.",
	"login_ok":          "Logging in works as set up.",
	"password_stopped":  "{count} account(s) can no longer log in with a password after many wrong guesses. Look under Users.",
	"proxy_not_trusted": "Every login of the last day came from {address}, probably the reverse proxy. Add its address to DIFFLUENCE_TRUSTED_PROXIES, or the login limits treat everyone as one.",
	"version_unknown":   "This build has no build date, so its age is not known.",
	"version_ok":        "Release {version} is {days} days old.",
	"version_old":       "Release {version} is {days} days old. A newer release probably exists; see the release notes.",
}

// Codes lists every code a check can give.
var Codes = func() []string {
	out := make([]string, 0, len(Sentences))
	for code := range Sentences {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}()

// Describe says a check in plain English, with its values filled in.
func Describe(c Check) string {
	text := Sentences[c.Code]
	for key, value := range c.Values {
		text = strings.ReplaceAll(text, "{"+key+"}", fmt.Sprint(value))
	}
	return text
}
