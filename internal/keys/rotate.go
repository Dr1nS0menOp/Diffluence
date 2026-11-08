// SPDX-License-Identifier: Apache-2.0

package keys

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Replacing the KEK takes three steps, each safe to stop after and to run again (spec 034):
//
//  1. AddNext writes a new key to `<file>.next`. Every node must load it before step 2, so
//     every node can open what the new key seals.
//  2. Promote makes the new key the one that seals, and keeps the old one in
//     `<file>.previous`. Values sealed before still open.
//  3. After every value and file is sealed again with the new key, RetirePrevious removes
//     the old key.
//
// At every moment in between, each key that sealed anything is still in one of the files.

// ErrPreviousPresent means an earlier change has not finished: its old key is still in use.
var ErrPreviousPresent = errors.New("keys: an older key is still being taken out; finish that first")

// ErrNoNext means there is no key on its way in.
var ErrNoNext = errors.New("keys: there is no new key to bring in; add one first")

// AddNext writes a new random key beside the key file and names it. It refuses when a new
// key is already waiting or an old one is still being taken out.
func AddNext(path string) (string, error) {
	if _, err := os.Stat(path + PreviousSuffix); err == nil {
		return "", ErrPreviousPresent
	}
	b := make([]byte, kekSize)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	f, err := os.OpenFile(path+NextSuffix, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return "", errors.New("keys: a new key is already waiting; promote it first")
	}
	if err != nil {
		return "", err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		_ = os.Remove(path + NextSuffix)
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	e, err := newEntry(b)
	if err != nil {
		return "", err
	}
	return idString(e), nil
}

// Promote makes the waiting key the one that seals. The old key moves to `<file>.previous`,
// and the derivation root stays the same. Run again after a crash, it finishes the job.
func Promote(path string) (string, error) {
	next, err := os.ReadFile(path + NextSuffix)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNoNext
	}
	if err != nil {
		return "", err
	}
	nextEntry, err := newEntry(next)
	if err != nil {
		return "", fmt.Errorf("keys: next key: %w", err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("keys: read key file: %w", err)
	}
	kek, root, err := parseFile(current)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(kek, next) {
		// The old key goes out of the key file only after it is safe in the previous file.
		previous, err := os.ReadFile(path + PreviousSuffix)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := writeFile(path+PreviousSuffix, kek); err != nil {
				return "", err
			}
		case err != nil:
			return "", err
		case !bytes.Equal(previous, kek):
			return "", ErrPreviousPresent
		}
		body := append(append(append([]byte(nil), fileMagic...), root...), next...)
		if err := writeFile(path, body); err != nil {
			return "", err
		}
	}
	if err := os.Remove(path + NextSuffix); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return idString(nextEntry), nil
}

// RetirePrevious removes the old key. Call it only after a check has shown that nothing is
// sealed with it any more: without it, such a value can no longer be read.
func RetirePrevious(path string) error {
	err := os.Remove(path + PreviousSuffix)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// writeFile replaces a key file whole: written beside it, synced, then renamed over it, so a
// crash leaves either the old or the new file, never half of one.
func writeFile(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func idString(e entry) string {
	k := Keyring{current: e}
	return k.KeyID()
}
