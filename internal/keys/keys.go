// SPDX-License-Identifier: Apache-2.0

// Package keys protects secret fields with envelope encryption (ADR-011).
// Every secret gets its own AES-256-GCM data key, which is wrapped by the
// key-encryption key (KEK). The KEK comes from a key file for now; a KMS can
// replace it in phase 3 without changing callers. The primitives are the Go
// standard library; this package only arranges them.
package keys

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const kekSize = 32

// format is the first byte of every sealed value, so the layout can change later.
const format = 1

// Keyring holds the key-encryption key.
type Keyring struct {
	kek []byte
}

// New wraps an existing key.
func New(kek []byte) (*Keyring, error) {
	if len(kek) != kekSize {
		return nil, fmt.Errorf("keys: the key must be %d bytes, got %d", kekSize, len(kek))
	}
	return &Keyring{kek: append([]byte(nil), kek...)}, nil
}

// LoadOrCreate reads the key file. When create is true and the file does not
// exist yet, a new key is generated. Back this file up separately from the data:
// without it, encrypted secrets cannot be read.
func LoadOrCreate(path string, create bool) (*Keyring, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		return New(b)
	}
	if !errors.Is(err, os.ErrNotExist) || !create {
		return nil, fmt.Errorf("keys: read key file: %w", err)
	}
	b = make([]byte, kekSize)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// O_EXCL: when two processes start at once, one wins and the other re-reads.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return LoadOrCreate(path, false)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(b); err != nil {
		return nil, err
	}
	return New(b)
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

// Seal encrypts a secret. context binds it to its purpose (for example
// "totp:<user id>"), so a sealed value cannot be moved to another row.
func (k *Keyring) Seal(plaintext []byte, context string) ([]byte, error) {
	dek := make([]byte, kekSize)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	wrapped, err := seal(k.kek, dek, []byte(context))
	if err != nil {
		return nil, err
	}
	body, err := seal(dek, plaintext, []byte(context))
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 2+len(wrapped)+len(body))
	out = append(out, format, byte(len(wrapped)))
	out = append(out, wrapped...)
	return append(out, body...), nil
}

// Open decrypts a value made by Seal with the same context.
func (k *Keyring) Open(sealed []byte, context string) ([]byte, error) {
	if len(sealed) < 2 || sealed[0] != format || len(sealed) < 2+int(sealed[1]) {
		return nil, errors.New("keys: unknown format")
	}
	n := 2 + int(sealed[1])
	dek, err := open(k.kek, sealed[2:n], []byte(context))
	if err != nil {
		return nil, err
	}
	return open(dek, sealed[n:], []byte(context))
}

// SigningKey derives the Ed25519 key that signs audit checkpoints.
func (k *Keyring) SigningKey() (ed25519.PrivateKey, error) {
	seed, err := hkdf.Key(sha256.New, k.kek, nil, "diffluence audit checkpoint signing v1", ed25519.SeedSize)
	if err != nil {
		return nil, err
	}
	return ed25519.NewKeyFromSeed(seed), nil
}
