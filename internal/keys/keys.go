// SPDX-License-Identifier: Apache-2.0

// Package keys protects secret fields with envelope encryption (ADR-011).
// Every secret gets its own AES-256-GCM data key, which is wrapped by the
// key-encryption key (KEK). The KEK comes from a key file for now; a KMS can
// replace it in phase 3 without changing callers. The primitives are the Go
// standard library; this package only arranges them.
//
// The KEK can be replaced while the instance runs (spec 034). Next to the key file,
// `<file>.next` holds a key that is being brought in and `<file>.previous` a key that is
// being taken out. Both open values but never seal them, so every node can read what any
// other node writes while the key file changes one node at a time.
package keys

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const kekSize = 32

// idSize is the length of a key id inside a sealed value.
const idSize = 8

// The layouts of a sealed value. legacyFormat values carry no key id and come from before
// keys could be replaced; every key that is loaded is tried on them.
const (
	legacyFormat = 1
	format       = 2
)

// fileMagic starts a key file that keeps a derivation root apart from its KEK. A file of
// exactly kekSize bytes is the first layout, where the KEK is also the root.
var fileMagic = []byte("DFK2")

// Suffixes of the files that hold a key on its way in or out.
const (
	NextSuffix     = ".next"
	PreviousSuffix = ".previous"
)

// ErrNoKey means a sealed value names a key that is not loaded.
var ErrNoKey = errors.New("keys: the key of this value is not loaded")

type entry struct {
	id  [idSize]byte
	kek []byte
}

// Keyring holds the key-encryption keys.
type Keyring struct {
	current entry
	// opens are the keys that open values but never seal them: one on its way in, one on its way out.
	opens []entry
	// root derives the fingerprint and signing keys. It stays when the KEK is replaced, so
	// fingerprints, audit checkpoints and bundles keep their meaning (spec 034, D3).
	root []byte
}

func newEntry(kek []byte) (entry, error) {
	if len(kek) != kekSize {
		return entry{}, fmt.Errorf("keys: the key must be %d bytes, got %d", kekSize, len(kek))
	}
	e := entry{kek: append([]byte(nil), kek...)}
	mac := hmac.New(sha256.New, e.kek)
	mac.Write([]byte("diffluence key id v1"))
	copy(e.id[:], mac.Sum(nil))
	return e, nil
}

// New wraps an existing key, which is also the derivation root.
func New(kek []byte) (*Keyring, error) {
	e, err := newEntry(kek)
	if err != nil {
		return nil, err
	}
	return &Keyring{current: e, root: e.kek}, nil
}

// parseFile reads the contents of a key file: the KEK and the root.
func parseFile(b []byte) (kek, root []byte, err error) {
	switch {
	case len(b) == kekSize:
		return b, b, nil
	case len(b) == len(fileMagic)+2*kekSize && bytes.Equal(b[:len(fileMagic)], fileMagic):
		root = b[len(fileMagic) : len(fileMagic)+kekSize]
		return b[len(fileMagic)+kekSize:], root, nil
	default:
		return nil, nil, fmt.Errorf("keys: the key file has %d bytes, which is no known layout", len(b))
	}
}

// LoadOrCreate reads the key file, and the keys on their way in or out beside it. When
// create is true and the file does not exist yet, a new key is generated. Back these files
// up separately from the data: without them, encrypted secrets and files cannot be read.
func LoadOrCreate(path string, create bool) (*Keyring, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := createKeyFile(path); err != nil {
			return nil, err
		}
		return LoadOrCreate(path, false)
	}
	if err != nil {
		return nil, fmt.Errorf("keys: read key file: %w", err)
	}
	kek, root, err := parseFile(b)
	if err != nil {
		return nil, err
	}
	k, err := New(kek)
	if err != nil {
		return nil, err
	}
	k.root = append([]byte(nil), root...)
	for _, suffix := range []string{NextSuffix, PreviousSuffix} {
		extra, err := os.ReadFile(path + suffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("keys: read %s key: %w", suffix[1:], err)
		}
		e, err := newEntry(extra)
		if err != nil {
			return nil, fmt.Errorf("keys: %s key: %w", suffix[1:], err)
		}
		if e.id != k.current.id {
			k.opens = append(k.opens, e)
		}
	}
	return k, nil
}

func createKeyFile(path string) error {
	b := make([]byte, kekSize)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// O_EXCL: when two processes start at once, one wins and the other reads its key.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// KeyID names the key that seals new values. It is derived from the key one way, so it can
// be shown and logged without giving anything away.
func (k *Keyring) KeyID() string { return hex.EncodeToString(k.current.id[:]) }

// KeyIDs names every loaded key, the sealing one first.
func (k *Keyring) KeyIDs() []string {
	ids := []string{k.KeyID()}
	for _, e := range k.opens {
		ids = append(ids, hex.EncodeToString(e.id[:]))
	}
	return ids
}

// IDOf names the key a value was sealed with. It reads no secret. ok is false for a value
// from before key ids, or one that is no sealed value at all.
func IDOf(sealed []byte) (id string, ok bool) {
	if len(sealed) < 1+idSize || sealed[0] != format {
		return "", false
	}
	return hex.EncodeToString(sealed[1 : 1+idSize]), true
}

// SealedWithCurrent says whether a value was sealed with the key that seals new values now.
func (k *Keyring) SealedWithCurrent(sealed []byte) bool {
	id, ok := IDOf(sealed)
	return ok && id == k.KeyID()
}

func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func seal(key, plaintext, aad []byte) ([]byte, error) {
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, plaintext, aad), nil
}

func open(key, sealed, aad []byte) ([]byte, error) {
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < a.NonceSize() {
		return nil, errors.New("keys: value too short")
	}
	return a.Open(nil, sealed[:a.NonceSize()], sealed[a.NonceSize():], aad)
}

// Seal encrypts a secret with the current key. context binds it to its purpose (for
// example "totp:<user id>"), so a sealed value cannot be moved to another row.
func (k *Keyring) Seal(plaintext []byte, context string) ([]byte, error) {
	dek := make([]byte, kekSize)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	wrapped, err := seal(k.current.kek, dek, []byte(context))
	if err != nil {
		return nil, err
	}
	body, err := seal(dek, plaintext, []byte(context))
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 2+idSize+len(wrapped)+len(body))
	out = append(out, format)
	out = append(out, k.current.id[:]...)
	out = append(out, byte(len(wrapped)))
	out = append(out, wrapped...)
	return append(out, body...), nil
}

// Open decrypts a value made by Seal with the same context, with whichever loaded key sealed it.
func (k *Keyring) Open(sealed []byte, context string) ([]byte, error) {
	if len(sealed) < 2 {
		return nil, errors.New("keys: unknown format")
	}
	switch sealed[0] {
	case format:
		if len(sealed) < 2+idSize || len(sealed) < 2+idSize+int(sealed[1+idSize]) {
			return nil, errors.New("keys: unknown format")
		}
		e, ok := k.byID(sealed[1 : 1+idSize])
		if !ok {
			return nil, ErrNoKey
		}
		n := 2 + idSize + int(sealed[1+idSize])
		return openWith(e.kek, sealed[2+idSize:n], sealed[n:], context)
	case legacyFormat:
		if len(sealed) < 2+int(sealed[1]) {
			return nil, errors.New("keys: unknown format")
		}
		n := 2 + int(sealed[1])
		// Without an id, every loaded key is tried; GCM refuses a wrong one.
		var lastErr error
		for _, e := range k.all() {
			plain, err := openWith(e.kek, sealed[2:n], sealed[n:], context)
			if err == nil {
				return plain, nil
			}
			lastErr = err
		}
		return nil, lastErr
	default:
		return nil, errors.New("keys: unknown format")
	}
}

func openWith(kek, wrapped, body []byte, context string) ([]byte, error) {
	dek, err := open(kek, wrapped, []byte(context))
	if err != nil {
		return nil, err
	}
	return open(dek, body, []byte(context))
}

func (k *Keyring) all() []entry { return append([]entry{k.current}, k.opens...) }

func (k *Keyring) byID(id []byte) (entry, bool) {
	for _, e := range k.all() {
		if bytes.Equal(e.id[:], id) {
			return e, true
		}
	}
	return entry{}, false
}

// Fingerprint gives a keyed hash of a value, for one purpose. It shows that two values
// were the same, and without the key it cannot be reversed or tried against a list.
func (k *Keyring) Fingerprint(purpose string, value []byte) ([]byte, error) {
	key, err := hkdf.Key(sha256.New, k.root, nil, "diffluence fingerprint v1 "+purpose, sha256.Size)
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(value)
	return mac.Sum(nil), nil
}

// SigningKey derives the Ed25519 key that signs audit checkpoints.
func (k *Keyring) SigningKey() (ed25519.PrivateKey, error) {
	seed, err := hkdf.Key(sha256.New, k.root, nil, "diffluence audit checkpoint signing v1", ed25519.SeedSize)
	if err != nil {
		return nil, err
	}
	return ed25519.NewKeyFromSeed(seed), nil
}
