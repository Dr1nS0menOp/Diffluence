// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/restore"
)

// The backup worker restores through these commands (spec 059). The console asks; the
// worker checks the backup, stops every node, makes a safety backup with its shell script,
// and then restores here, as the owner of the tables, with gpg, pg_restore and tar.

// appUID is the user the server runs as in its image (distroless nonroot): the restored
// data folder is handed to it.
const appUID = "65532:65532"

// restoringMark stands in the data folder while its files are being restored.
const restoringMark = ".restoring"

// restoreFlags are the folders of the worker: where backups are, and the key that opens them.
type restoreFlags struct {
	dir, key *string
}

func addRestoreFlags(fs *flag.FlagSet) restoreFlags {
	return restoreFlags{
		dir: fs.String("dir", "/backups", "the folder that holds the backups"),
		key: fs.String("key", "/key/backup.key", "the key that opens the backups"),
	}
}

// openRunner connects as the owner of the tables and builds the runner with the real steps.
func openRunner(ctx context.Context, f restoreFlags, log *slog.Logger) (restore.Runner, func(), error) {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return restore.Runner{}, nil, err
	}
	migs, err := db.Migrations()
	if err != nil {
		return restore.Runner{}, nil, err
	}
	url, err := ownerURL(cfg)
	if err != nil {
		return restore.Runner{}, nil, err
	}
	owner, err := db.Open(ctx, url)
	if err != nil {
		return restore.Runner{}, nil, err
	}
	steps := &shellSteps{cfg: cfg, ownerURL: url, owner: owner, dir: *f.dir, key: *f.key, migs: migs, log: log}
	// The key that opens the backups also confirms the restore (spec 070).
	r := restore.Runner{Pool: owner, Steps: steps, Gate: workerGate(*f.key, os.Stderr), NewestKnown: migs[len(migs)-1].Version,
		StopWithin: 3 * time.Minute, Poll: 2 * time.Second, Log: log}
	return r, owner.Close, nil
}

func parseRequest(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, errors.New("give --request as the id the worker was handed")
	}
	return id, nil
}

// cmdBackupRestorePrepare checks the backup while the nodes serve, then stops every node.
// It exits 0 once every node waits and the safety backup may begin.
func cmdBackupRestorePrepare(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backup restore-prepare", flag.ContinueOnError)
	request := fs.String("request", "", "the restore the worker was handed")
	f := addRestoreFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := parseRequest(*request)
	if err != nil {
		return err
	}
	r, done, err := openRunner(ctx, f, app.NewLogger(slog.LevelInfo))
	if err != nil {
		return err
	}
	defer done()
	if err := r.Prepare(ctx, id); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "every node waits: make the safety backup")
	return nil
}

// cmdBackupRestoreRun restores after the safety backup.
func cmdBackupRestoreRun(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backup restore-run", flag.ContinueOnError)
	request := fs.String("request", "", "the restore the worker was handed")
	safety := fs.String("safety", "", "the safety backup just made")
	f := addRestoreFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := parseRequest(*request)
	if err != nil {
		return err
	}
	r, done, err := openRunner(ctx, f, app.NewLogger(slog.LevelInfo))
	if err != nil {
		return err
	}
	defer done()
	if err := r.Run(ctx, id, *safety); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "restored; the nodes serve again within a minute")
	return nil
}

// cmdBackupRestoreAbort ends a restore whose safety backup failed: nothing was changed.
func cmdBackupRestoreAbort(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backup restore-abort", flag.ContinueOnError)
	request := fs.String("request", "", "the restore the worker was handed")
	step := fs.String("step", "", "what failed")
	f := addRestoreFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := parseRequest(*request)
	if err != nil {
		return err
	}
	r, done, err := openRunner(ctx, f, app.NewLogger(slog.LevelInfo))
	if err != nil {
		return err
	}
	defer done()
	err = r.Abort(ctx, id, *step)
	fmt.Fprintln(stdout, "the restore ended without changing anything; the nodes serve again")
	return err
}

// cmdBackupRestoreWayBack restores the safety backup of a restore that failed after it
// changed something.
func cmdBackupRestoreWayBack(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backup restore-way-back", flag.ContinueOnError)
	f := addRestoreFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, done, err := openRunner(ctx, f, app.NewLogger(slog.LevelInfo))
	if err != nil {
		return err
	}
	defer done()
	rs, err := r.WayBack(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "went back to %s; the nodes serve again within a minute\n", rs.BackupName)
	return nil
}

// cmdBackupRestoreRequest asks for a restore from the command line, for an operator who
// has the machine and no console. Shell access to the worker is more than the console's
// second factor, so none is asked; the audit log says it came from the command line. Run
// inside the backup service, it holds the backup key and fills in the confirmation code
// itself; elsewhere the request waits for the code in the backup service's log (spec 070).
func cmdBackupRestoreRequest(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backup restore-request", flag.ContinueOnError)
	name := fs.String("name", "", "the backup to restore, such as diffluence-20261111T071618Z")
	confirm := fs.String("confirm", "", "the name of this installation, to confirm")
	keyPath := fs.String("key", defaultBackupKey, "the backup key; when it can be read, the request needs no code")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	rs, err := restore.Request(ctx, pool, restore.Ask{BackupName: *name, Confirm: *confirm, Installation: cfg.InstanceName,
		Caller: identity.SystemCaller("cli"), CallerName: "the command line"})
	if err != nil {
		return err
	}
	if w, err := keys.LoadWorker(*keyPath); err == nil {
		if err := restore.PresetCode(ctx, pool, restore.NewGate(w.MAC), rs); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "restore %s of %s asked for and confirmed; the backup worker takes it within a minute\n", rs.ID, rs.BackupName)
		return nil
	}
	fmt.Fprintf(stdout, "restore %s of %s asked for; within a minute the backup service writes a confirmation code to "+
		"its log (docker compose logs backup): enter it in the console\n", rs.ID, rs.BackupName)
	return nil
}

// shellSteps are the real steps of a restore.
type shellSteps struct {
	cfg      config.Config
	ownerURL string
	owner    *pgxpool.Pool
	dir, key string
	migs     []db.Migration
	log      *slog.Logger
}

// backup is the folder of one backup, refusing a name that could leave the backup folder.
func (s *shellSteps) backup(name string) (string, error) {
	if !strings.HasPrefix(name, "diffluence-") || strings.ContainsAny(name, `/\`) || name != filepath.Base(name) {
		return "", fmt.Errorf("restore: %q is not the name of a backup", name)
	}
	return filepath.Join(s.dir, name), nil
}

// shell runs a bash script with set -o pipefail. Paths and the key travel as environment
// variables, never inside the script, so no name can become a command.
func (s *shellSteps) shell(ctx context.Context, script string, env map[string]string, stdout io.Writer) error {
	gnupg := filepath.Join(os.TempDir(), "gnupg-restore")
	if err := os.MkdirAll(gnupg, 0o700); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "bash", "-c", "set -eo pipefail\n"+script)
	cmd.Env = append(os.Environ(), "GNUPGHOME="+gnupg, "KEY="+s.key)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = stdout, &stderr
	if cmd.Stdout == nil {
		cmd.Stdout = io.Discard
	}
	if err := cmd.Run(); err != nil {
		// The last lines of what the tool said, without the database address.
		lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
		msg := strings.Join(lines[max(0, len(lines)-3):], " / ")
		msg = strings.ReplaceAll(msg, s.ownerURL, "<database>")
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

const decrypt = `gpg --batch --quiet --no-symkey-cache --pinentry-mode loopback --passphrase-file "$KEY" --decrypt`

// Check compares every part with SHA256SUMS and proves the key opens the backup.
func (s *shellSteps) Check(ctx context.Context, name string) error {
	dir, err := s.backup(name)
	if err != nil {
		return err
	}
	sums, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		return fmt.Errorf("restore: the backup has no SHA256SUMS: %w", err)
	}
	seen := 0
	for line := range strings.Lines(string(sums)) {
		want, file, ok := strings.Cut(strings.TrimSpace(line), "  ")
		if !ok {
			continue
		}
		if file != filepath.Base(file) {
			return fmt.Errorf("restore: SHA256SUMS names %q outside the backup", file)
		}
		got, err := fileSHA256(filepath.Join(dir, file))
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("restore: %s does not match its checksum", file)
		}
		seen++
	}
	for _, part := range []string{"database.dump.gpg", "files.tar.gz.gpg", "master.key.gpg"} {
		if _, err := os.Stat(filepath.Join(dir, part)); err != nil {
			return fmt.Errorf("restore: the backup lacks %s", part)
		}
	}
	if seen == 0 {
		return errors.New("restore: SHA256SUMS lists nothing")
	}
	return s.shell(ctx, decrypt+` "$DIR/master.key.gpg" >/dev/null`, map[string]string{"DIR": dir}, nil)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// NewestMigration reads the rows of schema_migrations out of the dump.
func (s *shellSteps) NewestMigration(ctx context.Context, name string) (int, error) {
	dir, err := s.backup(name)
	if err != nil {
		return 0, err
	}
	var out bytes.Buffer
	if err := s.shell(ctx, decrypt+` "$DIR/database.dump.gpg" | pg_restore --data-only --table=schema_migrations --file=-`,
		map[string]string{"DIR": dir}, &out); err != nil {
		return 0, err
	}
	return newestMigration(&out)
}

// newestMigration finds the highest version in the COPY of schema_migrations.
func newestMigration(r io.Reader) (int, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	in, newest := false, 0
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "COPY ") && strings.Contains(line, "schema_migrations"):
			in = true
		case in && line == `\.`:
			in = false
		case in:
			field, _, _ := strings.Cut(line, "\t")
			v, err := strconv.Atoi(field)
			if err != nil {
				return 0, fmt.Errorf("restore: an odd row in schema_migrations: %q", field)
			}
			newest = max(newest, v)
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	if newest == 0 {
		return 0, errors.New("restore: the backup's database has no migrations")
	}
	return newest, nil
}

// StopOthers ends the sessions of every other role, so a node that missed the waiting
// page cannot write while the tables are made again. A role that may not do it only logs.
func (s *shellSteps) StopOthers(ctx context.Context) error {
	var ended int
	err := s.owner.QueryRow(ctx, `SELECT count(*) FILTER (WHERE pg_terminate_backend(pid)) FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid() AND usename <> current_user`).Scan(&ended)
	if err != nil {
		s.log.Warn("could not end the other sessions; the nodes wait on their own", "error", err)
		return nil
	}
	s.log.Info("ended the other sessions on the database", "sessions", ended)
	return nil
}

// RestoreDatabase restores the dump in one transaction, leaving out the schema ops, which
// steers this restore. A failure leaves the database as it was.
//
// The list leaves out the schema public itself too: it is made again just before.
func (s *shellSteps) RestoreDatabase(ctx context.Context, name string) error {
	dir, err := s.backup(name)
	if err != nil {
		return err
	}
	var list bytes.Buffer
	if err := s.shell(ctx, decrypt+` "$DIR/database.dump.gpg" | pg_restore --list`, map[string]string{"DIR": dir}, &list); err != nil {
		return err
	}
	kept, err := os.CreateTemp("", "restore-list-*")
	if err != nil {
		return err
	}
	defer os.Remove(kept.Name())
	if _, err := kept.WriteString(withoutOps(list.String())); err != nil {
		kept.Close()
		return err
	}
	if err := kept.Close(); err != nil {
		return err
	}
	// One transaction: public is dropped and made again empty, then filled from the dump,
	// so a table a newer release added cannot linger, and a failure leaves the database
	// exactly as it was. The schema ops stays, because it is neither dropped nor restored.
	return s.shell(ctx, `{ printf 'DROP SCHEMA public CASCADE;\nCREATE SCHEMA public;\nGRANT USAGE ON SCHEMA public TO PUBLIC;\n'; `+
		decrypt+` "$DIR/database.dump.gpg" | pg_restore --use-list="$LIST" --file=-; } | `+
		`psql --dbname="$OWNER_URL" --no-psqlrc --quiet -v ON_ERROR_STOP=1 --single-transaction >/dev/null`,
		map[string]string{"DIR": dir, "LIST": kept.Name(), "OWNER_URL": s.ownerURL}, nil)
}

// withoutOps drops every entry of the schema ops from a pg_restore list, and the entry
// that makes the schema public, which the restore makes itself.
func withoutOps(list string) string {
	var b strings.Builder
	for line := range strings.Lines(list) {
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)
		entry := !strings.HasPrefix(trimmed, ";") && strings.Contains(trimmed, ";")
		makesPublic := len(fields) >= 6 && fields[3] == "SCHEMA" && fields[4] == "-" && fields[5] == "public"
		if entry && (slices.Contains(fields, "ops") || makesPublic) {
			continue
		}
		b.WriteString(line)
	}
	return b.String()
}

// RestoreFiles empties the data folder and fills it from the backup, with its keys. It
// refuses a folder without a master.key: that is not a data folder of Diffluence. The files
// are unpacked into a new folder and moved up, so tar never touches the data folder itself,
// whose mode the worker may not change; everything goes to the server's user at the end.
func (s *shellSteps) RestoreFiles(ctx context.Context, name string) error {
	dir, err := s.backup(name)
	if err != nil {
		return err
	}
	data := s.cfg.DataDir
	if !filepath.IsAbs(data) || filepath.Clean(data) == "/" {
		return fmt.Errorf("restore: the data folder must be an absolute path, not %q", data)
	}
	// A restore that stopped in this step leaves its mark, so the way back may empty the
	// folder again although its master.key is gone.
	_, keyErr := os.Stat(filepath.Join(data, "master.key"))
	_, markErr := os.Stat(filepath.Join(data, restoringMark))
	if keyErr != nil && markErr != nil {
		return fmt.Errorf("restore: %s holds no master.key, so it is not emptied", data)
	}
	return s.shell(ctx, `
find "$DATA" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
: >"$DATA/$MARK"
staging="$DATA/.restoring-files"
mkdir "$staging"
`+decrypt+` "$DIR/files.tar.gz.gpg" | tar -C "$staging" --no-same-owner -xzf -
find "$staging" -mindepth 1 -maxdepth 1 -exec mv -t "$DATA" -- {} +
rmdir "$staging"
for extra in next previous; do
  if [ -f "$DIR/master.key.$extra.gpg" ]; then
    `+decrypt+` "$DIR/master.key.$extra.gpg" >"$DATA/master.key.$extra"
    chmod 600 "$DATA/master.key.$extra"
  fi
done
`+decrypt+` "$DIR/master.key.gpg" >"$DATA/master.key"
chmod 600 "$DATA/master.key"
chown -R "$OWNER" "$DATA"
rm -f "$DATA/$MARK"
`, map[string]string{"DIR": dir, "DATA": data, "OWNER": appUID, "MARK": restoringMark}, nil)
}

// Migrate brings the restored database up to this release, as the owner, and gives the
// application role its rights. The backup check is not asked: the safety backup is fresh.
func (s *shellSteps) Migrate(ctx context.Context) error {
	_, err := migrateDatabase(ctx, s.cfg, s.migs, db.Options{Log: s.log})
	return err
}

// Record writes the restore into the restored audit log. The restore rewound the log with
// everything else, so the request is written again here, next to the result; and the
// safety backup is noted for the checks before the next upgrade.
func (s *shellSteps) Record(ctx context.Context, r restore.Restore) error {
	details := map[string]any{"backup": r.BackupName, "safety_backup": r.SafetyName,
		"requested_by": r.RequestedByName, "requested_at": r.RequestedAt.UTC().Format(time.RFC3339)}
	for _, action := range []string{"backup.restore_requested", "backup.restored"} {
		if err := audit.RecordSealed(ctx, s.owner, audit.Event{ActorType: audit.ActorSystem, ActorID: "backup",
			Action: action, ObjectType: "backup", ObjectID: r.BackupName, Result: audit.ResultSuccess, Details: details}); err != nil {
			return err
		}
	}
	return app.NoteBackup(ctx, s.owner)
}

// Verify checks the restored audit chain and that every secret and file opens with the
// restored keys.
func (s *shellSteps) Verify(ctx context.Context) error {
	key, err := signingKey(s.cfg)
	if err != nil {
		return err
	}
	public, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return errors.New("unexpected key type")
	}
	if _, err := audit.SealWaiting(ctx, s.owner); err != nil {
		return err
	}
	res, err := audit.Check(ctx, s.owner, public)
	if err != nil {
		return err
	}
	if !res.OK {
		return fmt.Errorf("the audit chain is broken at seq %d: %s", res.FirstBadSeq, res.Reason)
	}
	tools, err := app.OpenKeyTools(ctx, s.cfg)
	if err != nil {
		return err
	}
	defer tools.Close()
	rep, err := app.CheckKeys(ctx, tools.Pool, tools.Store, tools.Keyring, false)
	if err != nil {
		return err
	}
	if len(rep.Failures) > 0 {
		return fmt.Errorf("%d secret(s) or file(s) do not open with the restored keys", len(rep.Failures))
	}
	return nil
}
