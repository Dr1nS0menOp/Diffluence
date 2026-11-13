// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/config"
)

// The schema ops steers the restore, so it is left out of the list pg_restore follows;
// everything in public stays, also names that merely contain "ops".
func TestARestoreLeavesTheSchemaOpsAlone(t *testing.T) {
	list := `;
; Archive created at 2026-11-11 21:00:00 UTC
;
3; 2615 16385 SCHEMA - ops diffluence_owner
4; 2615 2200 SCHEMA - public diffluence_owner
220; 1259 16400 TABLE public pages diffluence_owner
221; 1259 16500 TABLE ops restores diffluence_owner
222; 1259 16501 TABLE ops nodes diffluence_owner
4001; 0 16400 TABLE DATA public pages diffluence_owner
4002; 0 16500 TABLE DATA ops restores diffluence_owner
4100; 1259 16600 INDEX ops restores_one_open diffluence_owner
4200; 0 0 ACL - SCHEMA ops diffluence_owner
4300; 1259 16700 TABLE public stops diffluence_owner
`
	kept := withoutOps(list)
	// The schema public is made again just before the restore, so its own entry goes too.
	for _, gone := range []string{"SCHEMA - ops", "TABLE ops restores", "TABLE ops nodes", "TABLE DATA ops", "INDEX ops", "ACL - SCHEMA ops", "SCHEMA - public"} {
		if strings.Contains(kept, gone) {
			t.Errorf("%q is still in the list", gone)
		}
	}
	for _, stays := range []string{"TABLE public pages", "TABLE DATA public pages", "TABLE public stops", "; Archive created"} {
		if !strings.Contains(kept, stays) {
			t.Errorf("%q was left out", stays)
		}
	}
}

// The newest migration in a dump decides whether this release may restore it.
func TestTheNewestMigrationIsReadFromTheDump(t *testing.T) {
	dump := `--
-- Data for Name: schema_migrations; Type: TABLE DATA; Schema: public; Owner: diffluence_owner
--

COPY public.schema_migrations (version, name, phase, checksum, applied_at) FROM stdin;
1	init	expand	abc	2026-11-01 10:00:00+00
48	backups	expand	def	2026-11-11 10:00:00+00
9	other	expand	ghi	2026-11-02 10:00:00+00
\.

`
	got, err := newestMigration(strings.NewReader(dump))
	if err != nil || got != 48 {
		t.Fatalf("newest migration: %d %v", got, err)
	}
	if _, err := newestMigration(strings.NewReader("")); err == nil {
		t.Fatal("a dump without migrations was accepted")
	}
	if _, err := newestMigration(strings.NewReader("COPY public.schema_migrations (version) FROM stdin;\nseven\n\\.\n")); err == nil {
		t.Fatal("an odd row was accepted")
	}
}

// The files step empties only a data folder of Diffluence: one that holds a master.key, or
// one a restore left halfway; never a folder given by mistake, nor a relative path.
func TestTheFilesStepEmptiesOnlyADataFolder(t *testing.T) {
	ctx := t.Context()
	other := t.TempDir()
	keep := filepath.Join(other, "precious.txt")
	if err := os.WriteFile(keep, []byte("not ours"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{other, "relative/data", "/"} {
		s := &shellSteps{dir: t.TempDir(), cfg: config.Config{DataDir: dir}}
		if err := s.RestoreFiles(ctx, "diffluence-20261111T071618Z"); err == nil || !strings.Contains(err.Error(), "restore:") {
			t.Fatalf("%q was taken as a data folder: %v", dir, err)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("a folder that is not a data folder was emptied")
	}
}

// A backup name never leaves the backup folder.
func TestABackupNameStaysInTheBackupFolder(t *testing.T) {
	s := &shellSteps{dir: "/backups"}
	for _, bad := range []string{"../secrets", "diffluence-../../etc", "diffluence-x/y", `diffluence-x\y`, "other"} {
		if _, err := s.backup(bad); err == nil {
			t.Errorf("%q was taken as a backup", bad)
		}
	}
	if dir, err := s.backup("diffluence-20261111T071618Z"); err != nil || dir != "/backups/diffluence-20261111T071618Z" {
		t.Fatalf("a real backup: %q %v", dir, err)
	}
}
