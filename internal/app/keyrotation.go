// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/storage"
	"github.com/diffluence/diffluence/internal/webhooks"
)

// sealedColumn is one column whose values the keyring seals (spec 034). Its two queries
// are written out in full, so no SQL is put together while the program runs.
type sealedColumn struct {
	table, column, id string
	context           func(id string) (string, error)
	// readPage reads one page of ids and values after the id $1, at most $2 rows.
	readPage string
	// reseal writes the value $2 of the id $1, while the old value is still $3.
	reseal string
}

func fixedContext(c string) func(string) (string, error) {
	return func(string) (string, error) { return c, nil }
}

// sealedColumns is every place a sealed value is kept. A new one must be added here, or a
// new master key would leave it behind; TestEverySealedColumnIsKnown keeps this list honest.
var sealedColumns = []sealedColumn{
	{table: "totp_credentials", column: "secret_sealed", id: "user_id", context: func(id string) (string, error) {
		u, err := uuid.Parse(id)
		if err != nil {
			return "", err
		}
		return identity.TOTPContext(u), nil
	},
		readPage: `SELECT user_id::text, secret_sealed FROM totp_credentials
			WHERE secret_sealed IS NOT NULL AND user_id::text > $1 ORDER BY user_id::text LIMIT $2`,
		reseal: `UPDATE totp_credentials SET secret_sealed = $2 WHERE user_id::text = $1 AND secret_sealed = $3`},
	{table: "webhook_endpoints", column: "secret", id: "id", context: fixedContext(webhooks.SecretContext),
		readPage: `SELECT id::text, secret FROM webhook_endpoints
			WHERE secret IS NOT NULL AND id::text > $1 ORDER BY id::text LIMIT $2`,
		reseal: `UPDATE webhook_endpoints SET secret = $2 WHERE id::text = $1 AND secret = $3`},
	{table: "webhook_endpoints", column: "old_secret", id: "id", context: fixedContext(webhooks.SecretContext),
		readPage: `SELECT id::text, old_secret FROM webhook_endpoints
			WHERE old_secret IS NOT NULL AND id::text > $1 ORDER BY id::text LIMIT $2`,
		reseal: `UPDATE webhook_endpoints SET old_secret = $2 WHERE id::text = $1 AND old_secret = $3`},
	{table: "mail_queue", column: "sealed", id: "id", context: fixedContext(accountMailContext),
		readPage: `SELECT id::text, sealed FROM mail_queue
			WHERE sealed IS NOT NULL AND id::text > $1 ORDER BY id::text LIMIT $2`,
		reseal: `UPDATE mail_queue SET sealed = $2 WHERE id::text = $1 AND sealed = $3`},
}

// keyPageSize is how many rows a check or reseal reads at once, so memory stays flat.
const keyPageSize = 500

// KeyFailure is one value or file that does not open with the loaded keys. It names where it
// is, never what it holds.
type KeyFailure struct {
	Where  string
	Reason string
}

// KeyReport says, per key id, how many values and files each loaded key protects, and which
// do not open at all.
type KeyReport struct {
	CurrentKey string
	Values     map[string]int // key id, or "legacy" for values from before key ids
	Files      map[string]int
	PlainFiles int
	Failures   []KeyFailure
}

// AllCurrent says whether everything opens and is sealed with the current key, which is
// what must hold before the previous key may be removed.
func (r KeyReport) AllCurrent() bool {
	if len(r.Failures) > 0 || r.PlainFiles > 0 {
		return false
	}
	for id := range r.Values {
		if id != r.CurrentKey {
			return false
		}
	}
	for id := range r.Files {
		if id != r.CurrentKey {
			return false
		}
	}
	return true
}

func idOrLegacy(sealed []byte) string {
	if id, ok := keys.IDOf(sealed); ok {
		return id
	}
	return "legacy"
}

func failureReason(err error) string {
	switch {
	case errors.Is(err, keys.ErrNoKey):
		return "its key is not loaded"
	case errors.Is(err, storage.ErrUnreadable):
		return "it does not decrypt"
	case errors.Is(err, storage.ErrNotEncrypted):
		return "it is plain while every file must be encrypted"
	default:
		return "it does not open"
	}
}

// eachSealed calls fn for every non-empty value of one column, one page at a time.
func eachSealed(ctx context.Context, pool *pgxpool.Pool, c sealedColumn, fn func(id string, sealed []byte) error) error {
	after := ""
	for {
		rows, err := pool.Query(ctx, c.readPage, after, keyPageSize)
		if err != nil {
			return fmt.Errorf("app: read %s.%s: %v", c.table, c.column, err)
		}
		type row struct {
			id     string
			sealed []byte
		}
		page, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
			var x row
			return x, r.Scan(&x.id, &x.sealed)
		})
		if err != nil {
			return fmt.Errorf("app: read %s.%s: %v", c.table, c.column, err)
		}
		for _, x := range page {
			if err := fn(x.id, x.sealed); err != nil {
				return err
			}
		}
		if len(page) < keyPageSize {
			return nil
		}
		after = page[len(page)-1].id
	}
}

// sortedKeys lists the stored files in name order, which the cursors of the file jobs follow.
func sortedKeys(ctx context.Context, store *storage.Encrypted) ([]string, error) {
	listed, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(listed))
	for _, l := range listed {
		names = append(names, l.Key)
	}
	sort.Strings(names)
	return names, nil
}

// CheckKeys proves that every sealed value and every stored file opens with the loaded keys,
// and counts which key protects each. With deep, every byte of every file is read, which
// also finds damage inside a file; without it, the key and the end of each file are checked.
func CheckKeys(ctx context.Context, pool *pgxpool.Pool, store *storage.Encrypted, k *keys.Keyring, deep bool) (KeyReport, error) {
	rep := KeyReport{CurrentKey: k.KeyID(), Values: map[string]int{}, Files: map[string]int{}}
	for _, c := range sealedColumns {
		err := eachSealed(ctx, pool, c, func(id string, sealed []byte) error {
			where := c.table + "." + c.column + " " + id
			purpose, err := c.context(id)
			if err != nil {
				rep.Failures = append(rep.Failures, KeyFailure{Where: where, Reason: "its row id is not valid"})
				return nil
			}
			if _, err := k.Open(sealed, purpose); err != nil {
				rep.Failures = append(rep.Failures, KeyFailure{Where: where, Reason: failureReason(err)})
				return nil
			}
			rep.Values[idOrLegacy(sealed)]++
			return nil
		})
		if err != nil {
			return rep, err
		}
	}
	names, err := sortedKeys(ctx, store)
	if err != nil {
		return rep, err
	}
	for _, name := range names {
		if ctx.Err() != nil {
			return rep, ctx.Err()
		}
		wrapped, err := store.WrappedKey(ctx, name)
		if errors.Is(err, storage.ErrNotFound) {
			continue // removed while the check ran
		}
		if errors.Is(err, storage.ErrNotEncrypted) {
			rep.PlainFiles++
			continue
		}
		if err != nil {
			rep.Failures = append(rep.Failures, KeyFailure{Where: "file " + name, Reason: failureReason(err)})
			continue
		}
		if err := openFile(ctx, store, name, deep); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			rep.Failures = append(rep.Failures, KeyFailure{Where: "file " + name, Reason: failureReason(err)})
			continue
		}
		rep.Files[idOrLegacy(wrapped)]++
	}
	return rep, nil
}

func openFile(ctx context.Context, store *storage.Encrypted, name string, deep bool) error {
	f, err := store.Open(ctx, name)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if !deep {
		return nil
	}
	_, err = io.Copy(io.Discard, f)
	return err
}

// ResealReport says what one run of ResealKeys did.
type ResealReport struct {
	Values  int // values sealed again
	Files   int // files sealed again
	Skipped int // values or files that do not open, left as they are for CheckKeys to name
	Done    bool
}

// ResealKeys seals every value and file that an older key protects again with the current
// key. It runs beside a working instance: a value is only replaced when nobody changed it
// meanwhile, and a file is replaced whole. Each run stops after budget, having finished at
// least one thing, and records where it stopped, so the next run goes on from there. Done is
// true after a run that found nothing left to seal again; what does not open is skipped, and
// CheckKeys names it.
func ResealKeys(ctx context.Context, pool *pgxpool.Pool, store *storage.Encrypted, k *keys.Keyring, budget time.Duration) (ResealReport, error) {
	r := resealer{pool: pool, store: store, keyring: k, deadline: time.Now().Add(budget)}
	return r.run(ctx)
}

// resealer is one run of ResealKeys.
type resealer struct {
	pool     *pgxpool.Pool
	store    *storage.Encrypted
	keyring  *keys.Keyring
	deadline time.Time
	rep      ResealReport
	// beforeUpdate runs between reading a value and replacing it; tests change the value there.
	beforeUpdate func(c sealedColumn, id string)
}

// stop says whether the run is over: past its budget, once something was done.
func (r *resealer) stop() bool {
	return r.rep.Values+r.rep.Files+r.rep.Skipped > 0 && time.Now().After(r.deadline)
}

func (r *resealer) run(ctx context.Context) (ResealReport, error) {
	for _, c := range sealedColumns {
		if err := r.column(ctx, c); err != nil {
			return r.rep, err
		}
		if r.stop() {
			return r.rep, nil
		}
	}
	valuesDone := r.rep.Values == 0
	filesDone, err := r.files(ctx)
	if err != nil {
		return r.rep, err
	}
	r.rep.Done = valuesDone && filesDone
	return r.rep, nil
}

func (r *resealer) column(ctx context.Context, c sealedColumn) error {
	// The old value is part of the condition of c.reseal, so a value that changed meanwhile is
	// left alone: its new value was sealed with the current key already.
	return eachSealed(ctx, r.pool, c, func(id string, sealed []byte) error {
		if r.keyring.SealedWithCurrent(sealed) || r.stop() {
			return nil
		}
		purpose, err := c.context(id)
		if err != nil {
			r.rep.Skipped++
			return nil
		}
		plain, err := r.keyring.Open(sealed, purpose)
		if err != nil {
			r.rep.Skipped++
			return nil
		}
		fresh, err := r.keyring.Seal(plain, purpose)
		clear(plain)
		if err != nil {
			return err
		}
		if r.beforeUpdate != nil {
			r.beforeUpdate(c, id)
		}
		if _, err := r.pool.Exec(ctx, c.reseal, id, fresh, sealed); err != nil {
			return fmt.Errorf("app: seal %s.%s again: %v", c.table, c.column, err)
		}
		r.rep.Values++
		return nil
	})
}

// files walks the stored files from where the last run stopped. It reports done after a whole
// pass, begun at the first file, in which no file was sealed again.
func (r *resealer) files(ctx context.Context) (bool, error) {
	var cursor string
	if err := r.pool.QueryRow(ctx, "SELECT reseal_cursor FROM storage_state").Scan(&cursor); err != nil {
		return false, fmt.Errorf("app: reseal cursor: %v", err)
	}
	names, err := sortedKeys(ctx, r.store)
	if err != nil {
		return false, err
	}
	fromStart := cursor == ""
	before := r.rep.Files
	for _, name := range names {
		if name <= cursor {
			continue
		}
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if r.stop() {
			return false, saveCursor(ctx, r.pool, "reseal_cursor", cursor)
		}
		cursor = name
		if err := r.file(ctx, name); err != nil {
			return false, err
		}
	}
	// A whole pass ends here: the next one starts at the first file again.
	if err := saveCursor(ctx, r.pool, "reseal_cursor", ""); err != nil {
		return false, err
	}
	return fromStart && r.rep.Files == before, nil
}

func (r *resealer) file(ctx context.Context, name string) error {
	wrapped, err := r.store.WrappedKey(ctx, name)
	switch {
	case errors.Is(err, storage.ErrNotFound), errors.Is(err, storage.ErrNotEncrypted):
		return nil // gone, or still plain: the encryption job seals plain files with the current key
	case err != nil:
		r.rep.Skipped++
		return nil
	case r.keyring.SealedWithCurrent(wrapped):
		return nil
	}
	err = r.store.Reseal(ctx, name)
	switch {
	case err == nil:
		r.rep.Files++
		return nil
	case errors.Is(err, storage.ErrNotFound):
		return nil
	case errors.Is(err, storage.ErrUnreadable), errors.Is(err, keys.ErrNoKey):
		r.rep.Skipped++
		return nil
	default:
		return fmt.Errorf("app: seal file %s again: %w", name, err)
	}
}

// ErrOldKeyStillNeeded means something still uses the key that would be removed.
var ErrOldKeyStillNeeded = errors.New("app: the old key is still needed; seal everything again first")

// RetireOldKey removes the previous master key, but only after a check shows that every value
// and file opens and is sealed with the current key. Without that check a value of the old key
// would become unreadable for good.
func RetireOldKey(ctx context.Context, t *KeyTools, keyFile string) (KeyReport, error) {
	rep, err := CheckKeys(ctx, t.Pool, t.Store, t.Keyring, false)
	if err != nil {
		return rep, err
	}
	if !rep.AllCurrent() {
		return rep, ErrOldKeyStillNeeded
	}
	if err := keys.RetirePrevious(keyFile); err != nil {
		return rep, err
	}
	return rep, RecordKeyEvent(ctx, t.Pool, "keys.retired", rep.CurrentKey, nil)
}

// cursorUpdates are the cursors the file jobs keep, each with its query written out in
// full, so no SQL is put together while the program runs.
var cursorUpdates = map[string]string{
	"encrypt_cursor": "UPDATE storage_state SET encrypt_cursor = $1",
	"reseal_cursor":  "UPDATE storage_state SET reseal_cursor = $1",
}

// saveCursor records where a file job stopped.
func saveCursor(ctx context.Context, pool *pgxpool.Pool, column, value string) error {
	query, ok := cursorUpdates[column]
	if !ok {
		return fmt.Errorf("app: no cursor is called %q", column)
	}
	if _, err := pool.Exec(ctx, query, value); err != nil {
		return fmt.Errorf("app: save %s: %v", column, err)
	}
	return nil
}

// RecordKeyEvent writes an audit event for a step of replacing the master key. It names the
// key by its id only.
func RecordKeyEvent(ctx context.Context, pool *pgxpool.Pool, action, keyID string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	details["key_id"] = keyID
	return audit.RecordStandalone(ctx, pool, audit.Event{ActorType: audit.ActorSystem, ActorID: "cli",
		Action: action, ObjectType: "master_key", ObjectID: keyID, Result: audit.ResultSuccess, Details: details})
}
