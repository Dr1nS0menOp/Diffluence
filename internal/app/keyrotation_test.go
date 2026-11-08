// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/keys"
)

// keyEnv is an instance with one uploaded file and one sealed TOTP seed.
type keyEnv struct {
	*env
	cfg     config.Config
	owner   *user
	address string // the download address of the file
}

func newKeyEnv(t *testing.T) *keyEnv {
	t.Helper()
	var cfg config.Config
	e := newEnvWith(t, func(c *config.Config) { cfg = *c })
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Page", "x")
	status, body := owner.upload(page, "notes.txt", "the content of the file")
	if status != 201 {
		t.Fatalf("upload: %d %s", status, body)
	}
	var att map[string]any
	_ = json.Unmarshal([]byte(body), &att)
	k := &keyEnv{env: e, cfg: cfg, owner: owner, address: "/api/v1/attachments/" + att["id"].(string) + "/content"}
	tools := k.tools()
	defer tools.Close()
	sealed, err := tools.Keyring.Seal([]byte("JBSWY3DPEHPK3PXP"), identity.TOTPContext(uuid.MustParse(owner.id)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tools.Pool.Exec(context.Background(), "INSERT INTO totp_credentials (user_id, secret_sealed) VALUES ($1, $2)", owner.id, sealed); err != nil {
		t.Fatal(err)
	}
	return k
}

func (k *keyEnv) tools() *app.KeyTools {
	k.t.Helper()
	tools, err := app.OpenKeyTools(context.Background(), k.cfg)
	if err != nil {
		k.t.Fatal(err)
	}
	return tools
}

func (k *keyEnv) check(deep bool) app.KeyReport {
	k.t.Helper()
	tools := k.tools()
	defer tools.Close()
	rep, err := app.CheckKeys(context.Background(), tools.Pool, tools.Store, tools.Keyring, deep)
	if err != nil {
		k.t.Fatal(err)
	}
	return rep
}

func (k *keyEnv) newKey() {
	k.t.Helper()
	if _, err := keys.AddNext(k.cfg.KeyFile()); err != nil {
		k.t.Fatal(err)
	}
	if _, err := keys.Promote(k.cfg.KeyFile()); err != nil {
		k.t.Fatal(err)
	}
}

func (k *keyEnv) resealAll() app.ResealReport {
	k.t.Helper()
	tools := k.tools()
	defer tools.Close()
	var total app.ResealReport
	for range 20 {
		rep, err := app.ResealKeys(context.Background(), tools.Pool, tools.Store, tools.Keyring, time.Minute)
		if err != nil {
			k.t.Fatal(err)
		}
		total.Values += rep.Values
		total.Files += rep.Files
		total.Skipped += rep.Skipped
		if rep.Done {
			total.Done = true
			return total
		}
	}
	k.t.Fatal("resealing never finished")
	return total
}

func TestAMissingKeyFileStopsTheKeyToolsInsteadOfMakingANewKey(t *testing.T) {
	k := newKeyEnv(t)
	if err := os.Rename(k.cfg.KeyFile(), k.cfg.KeyFile()+".away"); err != nil {
		t.Fatal(err)
	}
	if _, err := app.OpenKeyTools(context.Background(), k.cfg); err == nil {
		t.Fatal("the key tools must not start without the key")
	}
	if _, err := os.Stat(k.cfg.KeyFile()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the key tools must never create a new key")
	}
}

func TestAWrongKeyIsNamedForEveryValueAndFileWithoutShowingThem(t *testing.T) {
	k := newKeyEnv(t)
	wrong := make([]byte, 32)
	if _, err := rand.Read(wrong); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(k.cfg.KeyFile(), wrong, 0o600); err != nil {
		t.Fatal(err)
	}
	rep := k.check(false)
	if len(rep.Failures) != 2 || rep.AllCurrent() {
		t.Fatalf("a wrong key must fail the value and the file: %+v", rep)
	}
	for _, f := range rep.Failures {
		if strings.Contains(f.Where+f.Reason, "JBSWY3DP") || strings.Contains(f.Where+f.Reason, "content of the file") {
			t.Fatalf("a failure shows a secret: %+v", f)
		}
	}
	tools := k.tools()
	defer tools.Close()
	if _, err := app.RetireOldKey(context.Background(), tools, k.cfg.KeyFile()); !errors.Is(err, app.ErrOldKeyStillNeeded) {
		t.Fatalf("nothing may be retired while values do not open: %v", err)
	}
}

func TestAFileCopiedUnderAnotherNameIsNamedAsUnreadable(t *testing.T) {
	k := newKeyEnv(t)
	files := storedFiles(t, k.cfg.DataDir)
	if len(files) != 1 {
		t.Fatalf("want one stored file, got %v", files)
	}
	raw, _ := os.ReadFile(files[0])
	copied := filepath.Join(filepath.Dir(files[0]), "copied")
	if err := os.WriteFile(copied, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	rep := k.check(false)
	if len(rep.Failures) != 1 || !strings.HasSuffix(rep.Failures[0].Where, "copied") {
		t.Fatalf("the copy must fail, the original must not: %+v", rep.Failures)
	}
}

func TestADeepCheckFindsDamageInsideAFile(t *testing.T) {
	k := newKeyEnv(t)
	// A file of several chunks, with a byte changed in the first one: only reading it all finds it.
	page := k.owner.page(mustSpace(t, k.owner), nil, "Big", "x")
	if status, body := k.owner.upload(page, "big.bin", strings.Repeat("y", 200<<10)); status != 201 {
		t.Fatalf("upload: %d %s", status, body)
	}
	var big string
	for _, f := range storedFiles(t, k.cfg.DataDir) {
		if info, _ := os.Stat(f); info.Size() > 100<<10 {
			big = f
		}
	}
	raw, _ := os.ReadFile(big)
	raw[200] ^= 1
	if err := os.WriteFile(big, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if rep := k.check(false); len(rep.Failures) != 0 {
		t.Fatalf("a quick check reads the key and the end only: %+v", rep.Failures)
	}
	if rep := k.check(true); len(rep.Failures) != 1 {
		t.Fatalf("a deep check must find the damage: %+v", rep.Failures)
	}
}

func mustSpace(t *testing.T, u *user) string {
	t.Helper()
	space, _ := u.space("BIG")
	return space
}

func TestTheOldKeyCannotBeRetiredBeforeEverythingIsSealedAgain(t *testing.T) {
	k := newKeyEnv(t)
	oldID := k.check(false).CurrentKey
	k.newKey()
	rep := k.check(false)
	if len(rep.Failures) != 0 || rep.Values[oldID] != 1 || rep.Files[oldID] != 1 || rep.AllCurrent() {
		t.Fatalf("after promote, everything opens and still uses the old key: %+v", rep)
	}
	tools := k.tools()
	defer tools.Close()
	if _, err := app.RetireOldKey(context.Background(), tools, k.cfg.KeyFile()); !errors.Is(err, app.ErrOldKeyStillNeeded) {
		t.Fatalf("retire must refuse while the old key is in use: %v", err)
	}
	if _, err := os.Stat(k.cfg.KeyFile() + keys.PreviousSuffix); err != nil {
		t.Fatal("a refused retire must leave the old key in place")
	}
}

func TestTheKeyIsReplacedWhileTheInstanceKeepsServing(t *testing.T) {
	k := newKeyEnv(t)
	k.newKey()
	// The running instance still has only the old key loaded, and keeps serving.
	if resp, got := k.do(k.owner.c, "GET", k.address, ""); resp.StatusCode != 200 || got != "the content of the file" {
		t.Fatalf("download during the change: %d %q", resp.StatusCode, got)
	}
	rep := k.resealAll()
	if rep.Values != 1 || rep.Files != 1 || rep.Skipped != 0 {
		t.Fatalf("one value and one file must be sealed again: %+v", rep)
	}
	if again := k.resealAll(); again.Values+again.Files != 0 {
		t.Fatalf("a second run must find nothing to do: %+v", again)
	}
	after := k.check(true)
	if !after.AllCurrent() {
		t.Fatalf("after resealing everything uses the new key: %+v", after)
	}
	tools := k.tools()
	if _, err := app.RetireOldKey(context.Background(), tools, k.cfg.KeyFile()); err != nil {
		t.Fatal(err)
	}
	tools.Close()
	if _, err := os.Stat(k.cfg.KeyFile() + keys.PreviousSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("retire must remove the old key")
	}
	if final := k.check(true); !final.AllCurrent() || final.Values[final.CurrentKey] != 1 {
		t.Fatalf("with only the new key everything opens: %+v", final)
	}
	pool := k.tools()
	defer pool.Close()
	var steps int
	if err := pool.Pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_events WHERE action = 'keys.retired'").Scan(&steps); err != nil {
		t.Fatal(err)
	}
	if steps != 1 {
		t.Fatalf("retiring must be audited once: %d", steps)
	}
}
func TestAValueChangedWhileItIsSealedAgainKeepsTheNewValue(t *testing.T) {
	k := newKeyEnv(t)
	k.newKey()
	tools := k.tools()
	defer tools.Close()
	ctx := context.Background()
	newer, err := tools.Keyring.Seal([]byte("NEWSEED"), identity.TOTPContext(uuid.MustParse(k.owner.id)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.ResealWithHookForTest(ctx, tools, func(table, column, id string) {
		if table == "totp_credentials" {
			if _, err := tools.Pool.Exec(ctx, "UPDATE totp_credentials SET secret_sealed = $2 WHERE user_id = $1", id, newer); err != nil {
				t.Fatal(err)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err := tools.Pool.QueryRow(ctx, "SELECT secret_sealed FROM totp_credentials WHERE user_id = $1", k.owner.id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	plain, err := tools.Keyring.Open(stored, identity.TOTPContext(uuid.MustParse(k.owner.id)))
	if err != nil || string(plain) != "NEWSEED" {
		t.Fatalf("the value written meanwhile must win: %q %v", plain, err)
	}
}

func TestResealingSkipsWhatDoesNotOpenAndStillFinishes(t *testing.T) {
	k := newKeyEnv(t)
	tools := k.tools()
	ctx := context.Background()
	other, _ := keys.New(make([]byte, 32))
	foreign, _ := other.Seal([]byte("x"), "webhook-secret")
	space, _ := k.owner.space("HOOK")
	if _, err := tools.Pool.Exec(ctx, `INSERT INTO webhook_endpoints (id, space_id, address, events, with_titles, secret, owner_id)
		VALUES ($1, $2, 'https://example.test/hook', '{}', false, $3, $4)`, uuid.New(), space, foreign, k.owner.id); err != nil {
		t.Fatal(err)
	}
	tools.Close()
	k.newKey()
	rep := k.resealAll()
	if !rep.Done || rep.Skipped == 0 {
		t.Fatalf("a value that does not open is skipped, and resealing still ends: %+v", rep)
	}
	if after := k.check(false); len(after.Failures) != 1 || !strings.HasPrefix(after.Failures[0].Where, "webhook_endpoints.secret") {
		t.Fatalf("the check must name the value that does not open: %+v", after.Failures)
	}
}

func TestTheEncryptionJobGoesOnWhereTheLastRunStopped(t *testing.T) {
	k := newKeyEnv(t)
	ctx := context.Background()
	space, _ := k.owner.space("MORE")
	page := k.owner.page(space, nil, "More", "x")
	for _, name := range []string{"a.txt", "b.txt"} {
		if status, body := k.owner.upload(page, name, "plain "+name); status != 201 {
			t.Fatalf("upload: %d %s", status, body)
		}
	}
	files := storedFiles(t, k.cfg.DataDir)
	for _, f := range files {
		if err := os.WriteFile(f, []byte("plain bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// With no budget at all, each run still finishes one file and remembers where it stopped.
	for run := 1; run <= len(files); run++ {
		if err := k.app.EncryptStoredFilesWithinForTest(ctx, 0); err != nil {
			t.Fatal(err)
		}
		if plain := countPlain(t, files); plain != len(files)-run {
			t.Fatalf("after run %d, %d file(s) must be left plain, got %d", run, len(files)-run, plain)
		}
	}
}

func countPlain(t *testing.T, files []string) int {
	t.Helper()
	n := 0
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		if string(raw) == "plain bytes" {
			n++
		}
	}
	return n
}

// TestEverySealedColumnIsKnown fails when code seals something in a new place, so that a new
// master key is not forgotten there: such a place must be added to sealedColumns, or listed
// below as not stored.
func TestEverySealedColumnIsKnown(t *testing.T) {
	known := map[string]string{
		"identity/factors.go":   "totp_credentials.secret_sealed",
		"webhooks/webhooks.go":  "webhook_endpoints.secret and old_secret",
		"app/account_mail.go":   "mail_queue.sealed",
		"notify/unsubscribe.go": "not stored: the token is in a mail, and stops working once its key is retired",
		"storage/encrypted.go":  "stored files, which CheckKeys and ResealKeys walk",
		"app/keyrotation.go":    "seals the places above again with a new key",
	}
	seal := regexp.MustCompile(`\.Seal\(`)
	root := filepath.Join("..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "keys/") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if seal.Match(src) && strings.Contains(string(src), "keyring") || seal.Match(src) && strings.Contains(string(src), "sealer") {
			if _, ok := known[rel]; !ok {
				t.Errorf("%s seals a value; add its column to sealedColumns in keyrotation.go, or list it here", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
