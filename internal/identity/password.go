// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Params are Argon2id settings. Stored hashes carry their own parameters, so
// raising them later only means old hashes are upgraded at the next login.
type Params struct {
	Memory  uint32 // KiB
	Time    uint32
	Threads uint8
	SaltLen uint32
	KeyLen  uint32
}

// DefaultParams follow the OWASP Password Storage Cheat Sheet minimum for Argon2id.
var DefaultParams = Params{Memory: 19 * 1024, Time: 2, Threads: 1, SaltLen: 16, KeyLen: 32}

// hashSlots limits how many hashes run at once on this node (ARC-236). Argon2id is meant
// to be expensive, so a rush of logins would otherwise take every core and leave nothing
// for the rest of the node. It is per process on purpose: the limit is the CPU of this
// machine, which no caller owns.
var hashSlots = make(chan struct{}, max(runtime.NumCPU()/2, 1))

// hashKey runs Argon2id when a slot is free.
func hashKey(pw string, salt []byte, p Params, keyLen uint32) []byte {
	hashSlots <- struct{}{}
	defer func() { <-hashSlots }()
	return argon2.IDKey([]byte(pw), salt, p.Time, p.Memory, p.Threads, keyLen)
}

// Password length limits. The upper bound stops hashing from becoming a DoS vector.
const (
	MinPasswordLength = 15   // characters; only new passwords must meet it
	MaxPasswordBytes  = 1024 // bytes
)

var (
	ErrPasswordTooShort = fmt.Errorf("identity: password must have at least %d characters", MinPasswordLength)
	ErrPasswordTooLong  = fmt.Errorf("identity: password must be at most %d bytes", MaxPasswordBytes)
	errMalformedHash    = errors.New("identity: malformed password hash")
)

// ValidatePassword checks the length rules.
func ValidatePassword(pw string) error {
	if len(pw) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}
	if utf8.RuneCountInString(pw) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}

// HashPassword returns a PHC-formatted Argon2id hash.
func HashPassword(pw string, p Params) (string, error) {
	if len(pw) > MaxPasswordBytes {
		return "", ErrPasswordTooLong
	}
	salt := make([]byte, p.SaltLen)
	_, _ = rand.Read(salt) // crypto/rand.Read never fails on supported platforms
	key := hashKey(pw, salt, p, p.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.Memory, p.Time, p.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword checks pw against an encoded hash in constant time.
// needsRehash is true when the hash uses weaker parameters than current.
func VerifyPassword(encoded, pw string, current Params) (ok, needsRehash bool, err error) {
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return false, false, err
	}
	if len(pw) > MaxPasswordBytes {
		return false, false, nil
	}
	got := hashKey(pw, salt, p, uint32(len(key)))
	ok = subtle.ConstantTimeCompare(got, key) == 1
	needsRehash = p.Memory < current.Memory || p.Time < current.Time || p.Threads < current.Threads ||
		uint32(len(key)) < current.KeyLen || uint32(len(salt)) < current.SaltLen
	return ok, needsRehash, nil
}

func decodeHash(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return Params{}, nil, nil, errMalformedHash
	}
	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return Params{}, nil, nil, errMalformedHash
	}
	// Refuse absurd parameters: a tampered hash must not be able to exhaust memory or CPU.
	if p.Memory < 8 || p.Memory > 1024*1024 || p.Time < 1 || p.Time > 16 || p.Threads < 1 || p.Threads > 16 {
		return Params{}, nil, nil, errMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return Params{}, nil, nil, errMalformedHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) < 16 || len(key) > 128 {
		return Params{}, nil, nil, errMalformedHash
	}
	return p, salt, key, nil
}
