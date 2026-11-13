// SPDX-License-Identifier: Apache-2.0

package keys

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"os"
)

// minWorkerSecret is the shortest backup key taken. setup-secrets.sh writes 32 random bytes
// in base64, which is 45 characters.
const minWorkerSecret = 32

// Worker holds the keys the backup service derives from its backup key (spec 070). The server
// never reads that file, so it can neither sign a checkpoint in the worker's name nor make the
// code that confirms a restore.
type Worker struct {
	secret []byte
}

// LoadWorker reads the backup key. Surrounding white space does not count. A missing file
// keeps os.ErrNotExist in the error.
func LoadWorker(path string) (*Worker, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- the operator names the key file of the backup service.
	if err != nil {
		return nil, fmt.Errorf("keys: read the backup key: %w", err)
	}
	secret := bytes.TrimSpace(b)
	if len(secret) < minWorkerSecret {
		return nil, fmt.Errorf("keys: the backup key has %d characters, fewer than %d", len(secret), minWorkerSecret)
	}
	return &Worker{secret: secret}, nil
}

func (w *Worker) derive(purpose string, size int) ([]byte, error) {
	return hkdf.Key(sha256.New, w.secret, nil, "diffluence backup service "+purpose+" v1", size)
}

// CheckpointKey is the Ed25519 key with which the backup service signs audit checkpoints.
func (w *Worker) CheckpointKey() (ed25519.PrivateKey, error) {
	seed, err := w.derive("audit checkpoint signing", ed25519.SeedSize)
	if err != nil {
		return nil, err
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// MAC is an HMAC-SHA256 of message with a key derived for purpose.
func (w *Worker) MAC(purpose string, message []byte) []byte {
	key, err := w.derive("mac "+purpose, sha256.Size)
	if err != nil {
		// hkdf.Key fails only for a length beyond 255 hashes; sha256.Size never is.
		panic(err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}
