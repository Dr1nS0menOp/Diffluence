// SPDX-License-Identifier: Apache-2.0

package health

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

// healthy is an installation with nothing to say: every other test changes one fact.
func healthy() Facts {
	return Facts{
		Now:            now,
		DiskFree:       500 << 30,
		DiskTotal:      1000 << 30,
		DatabaseBytes:  2 << 30,
		Growth30d:      1 << 30,
		GrowthKnown:    true,
		LastBackup:     now.Add(-3 * time.Hour),
		MailConfigured: true,
		BuildDate:      now.AddDate(0, 0, -10),
		Version:        "v1.2.3",
	}
}

func find(t *testing.T, checks []Check, name string) Check {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %q in %+v", name, checks)
	return Check{}
}

func expect(t *testing.T, f Facts, name string, state State, code string) Check {
	t.Helper()
	c := find(t, Evaluate(f), name)
	if c.State != state || c.Code != code {
		t.Fatalf("%s: got %s/%s, want %s/%s", name, c.State, c.Code, state, code)
	}
	return c
}

func TestAHealthyInstallationHasNothingToSay(t *testing.T) {
	checks := Evaluate(healthy())
	for _, c := range checks {
		if c.State != OK {
			t.Errorf("%s is %s (%s)", c.Name, c.State, c.Code)
		}
	}
	if len(checks) != len(Names) {
		t.Fatalf("got %d checks, want one for each of %v", len(checks), Names)
	}
	if problems, warnings := Count(checks); problems != 0 || warnings != 0 {
		t.Fatalf("counted %d problems and %d warnings", problems, warnings)
	}
}

func TestADiskBelowTwoPercentIsAProblemThatRefusesLargeWrites(t *testing.T) {
	f := healthy()
	f.DiskFree, f.DiskTotal = 19, 1000
	expect(t, f, "disk", Problem, "disk_full")
	if !RefusesLargeWrites(19, 1000) {
		t.Fatal("1.9% free must refuse large writes")
	}
}

func TestADiskAtTwoPercentWarnsButStillTakesFiles(t *testing.T) {
	f := healthy()
	f.DiskFree, f.DiskTotal = 20, 1000
	expect(t, f, "disk", Warn, "disk_low")
	if RefusesLargeWrites(20, 1000) {
		t.Fatal("2% free must still take files")
	}
}

func TestADiskBelowTenPercentWarns(t *testing.T) {
	f := healthy()
	f.DiskFree, f.DiskTotal = 99, 1000
	expect(t, f, "disk", Warn, "disk_low")
}

func TestADiskAtTenPercentIsFine(t *testing.T) {
	f := healthy()
	f.DiskFree, f.DiskTotal, f.Growth30d = 100, 1000, 0
	expect(t, f, "disk", OK, "disk_ok")
}

func TestADiskThatAMonthOfGrowthWouldFillWarns(t *testing.T) {
	f := healthy()
	f.DiskFree, f.DiskTotal, f.Growth30d = 400<<30, 1000<<30, 401<<30
	expect(t, f, "disk", Warn, "disk_fills")
	f.Growth30d = 400 << 30
	expect(t, f, "disk", OK, "disk_ok")
}

func TestGrowthThatIsNotKnownYetNeverWarns(t *testing.T) {
	f := healthy()
	f.Growth30d, f.GrowthKnown = 900<<30, false
	expect(t, f, "disk", OK, "disk_ok")
	expect(t, f, "database", OK, "database_size")
}

func TestADiskOfUnknownSizeNeverWarnsAndNeverRefuses(t *testing.T) {
	f := healthy()
	f.DiskFree, f.DiskTotal = 0, 0
	expect(t, f, "disk", OK, "disk_unknown")
	if RefusesLargeWrites(0, 0) {
		t.Fatal("an unknown size must not refuse")
	}
}

func TestADatabaseOfUnknownSizeSaysSo(t *testing.T) {
	f := healthy()
	f.DatabaseBytes = 0
	expect(t, f, "database", OK, "database_unknown")
}

func TestNoBackupNotedWarns(t *testing.T) {
	f := healthy()
	f.LastBackup = time.Time{}
	expect(t, f, "backup", Warn, "backup_none")
}

func TestABackupOlderThanTwentySixHoursWarns(t *testing.T) {
	f := healthy()
	f.LastBackup = now.Add(-26*time.Hour - time.Second)
	c := expect(t, f, "backup", Warn, "backup_old")
	if c.Values["hours"] != 26 {
		t.Fatalf("hours: %v", c.Values["hours"])
	}
}

func TestABackupOfTwentySixHoursIsStillFine(t *testing.T) {
	f := healthy()
	f.LastBackup = now.Add(-26 * time.Hour)
	expect(t, f, "backup", OK, "backup_ok")
}

// A contract migration waits for its grace period on purpose (spec 048 D1); only a day on
// top of that is news.
func TestAContractMigrationWarnsADayAfterItsGrace(t *testing.T) {
	f := healthy()
	f.PendingContract, f.ContractGrace = 1, 24*time.Hour
	f.LastMigrationAt = now.Add(-47 * time.Hour)
	expect(t, f, "migrations", OK, "contract_pending")
	f.LastMigrationAt = now.Add(-48*time.Hour - time.Second)
	expect(t, f, "migrations", Warn, "contract_waiting")
}

func TestAContractMigrationWaitingMoreThanADayWarns(t *testing.T) {
	f := healthy()
	f.PendingContract, f.LastMigrationAt = 1, now.Add(-24*time.Hour-time.Second)
	expect(t, f, "migrations", Warn, "contract_waiting")
	f.LastMigrationAt = now.Add(-24 * time.Hour)
	expect(t, f, "migrations", OK, "contract_pending")
	f.PendingContract = 0
	f.LastMigrationAt = now.AddDate(-1, 0, 0)
	expect(t, f, "migrations", OK, "migrations_ok")
}

func TestAContractMigrationWithoutATimeDoesNotWarn(t *testing.T) {
	f := healthy()
	f.PendingContract = 2
	expect(t, f, "migrations", OK, "contract_pending")
}

func TestAnEventStuckInTheOutboxIsAProblem(t *testing.T) {
	f := healthy()
	f.OutboxStuck, f.FailedJobs7d = 1, 3
	expect(t, f, "work", Problem, "outbox_stuck")
}

func TestFailedJobsOrEventsWarn(t *testing.T) {
	f := healthy()
	f.FailedJobs7d = 1
	expect(t, f, "work", Warn, "work_failed")
	f.FailedJobs7d, f.EventFailures = 0, 1
	expect(t, f, "work", Warn, "work_failed")
}

func TestAuditEventsWaitingLongerThanAMinuteWarn(t *testing.T) {
	f := healthy()
	f.AuditWaiting, f.OldestAuditWaiting = 3, now.Add(-AuditSealWarnAfter-time.Second)
	c := expect(t, f, "audit", Warn, "audit_lagging")
	if c.Values["count"] != int64(3) || c.Values["seconds"] != 61 {
		t.Fatalf("values %+v", c.Values)
	}
}

func TestAuditEventsWaitingAMomentAreNormal(t *testing.T) {
	f := healthy()
	f.AuditWaiting, f.OldestAuditWaiting = 40, now.Add(-AuditSealWarnAfter)
	expect(t, f, "audit", OK, "audit_ok")
}

func TestAuditEventsWaitingLongerThanFifteenMinutesAreAProblem(t *testing.T) {
	f := healthy()
	f.AuditWaiting, f.OldestAuditWaiting = 1, now.Add(-AuditSealProblemAfter-time.Second)
	expect(t, f, "audit", Problem, "audit_stuck")
	f.OldestAuditWaiting = now.Add(-AuditSealProblemAfter)
	expect(t, f, "audit", Warn, "audit_lagging")
}

func TestMailThatIsOffOrFailingWarns(t *testing.T) {
	f := healthy()
	f.MailConfigured = false
	expect(t, f, "mail", Warn, "mail_off")
	f.MailConfigured, f.MailFailed1d = true, 1
	expect(t, f, "mail", Warn, "mail_failed")
}

func TestAnAccountWhosePasswordLoginStoppedWarns(t *testing.T) {
	f := healthy()
	f.PasswordStops = 1
	expect(t, f, "login", Warn, "password_stopped")
}

func TestAReleaseOlderThanSixtyDaysWarns(t *testing.T) {
	f := healthy()
	f.BuildDate = now.AddDate(0, 0, -60).Add(-time.Second)
	c := expect(t, f, "version", Warn, "version_old")
	if c.Values["days"] != 60 {
		t.Fatalf("days: %v", c.Values["days"])
	}
	f.BuildDate = now.AddDate(0, 0, -60)
	expect(t, f, "version", OK, "version_ok")
}

func TestABuildWithoutADateNeverWarns(t *testing.T) {
	f := healthy()
	f.BuildDate = time.Time{}
	expect(t, f, "version", OK, "version_unknown")
}

func TestCountSeparatesProblemsFromWarnings(t *testing.T) {
	f := healthy()
	f.OutboxStuck, f.MailConfigured, f.LastBackup = 1, false, time.Time{}
	if problems, warnings := Count(Evaluate(f)); problems != 1 || warnings != 2 {
		t.Fatalf("counted %d problems and %d warnings", problems, warnings)
	}
}

// The console says the same as the command line, in its translation strings: every code
// and every check name has one there, so a new code cannot reach a page as a bare word.
func TestTheConsoleHasATextForEveryCodeAndName(t *testing.T) {
	raw, err := os.ReadFile("../../web/src/locales/en.json")
	if err != nil {
		t.Fatal(err)
	}
	var messages struct {
		Health map[string]string `json:"health"`
	}
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatal(err)
	}
	for _, code := range Codes {
		if messages.Health[code] == "" && messages.Health[code+"_other"] == "" {
			t.Errorf("en.json has no health.%s", code)
		}
	}
	for _, name := range Names {
		if messages.Health["name_"+name] == "" {
			t.Errorf("en.json has no health.name_%s", name)
		}
	}
	for _, state := range []State{OK, Warn, Problem} {
		if messages.Health["state_"+string(state)] == "" {
			t.Errorf("en.json has no health.state_%s", state)
		}
	}
}

func TestEveryCodeHasASentenceAndWhatToDo(t *testing.T) {
	for _, code := range Codes {
		if Sentences[code] == "" {
			t.Errorf("%s has no sentence", code)
		}
	}
	for _, c := range Evaluate(healthy()) {
		if Sentences[c.Code] == "" {
			t.Errorf("%s has no sentence", c.Code)
		}
	}
}
