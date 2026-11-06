// SPDX-License-Identifier: Apache-2.0

// Package leaked answers whether a password is on a list of leaked passwords. The
// list is stored as a Bloom filter, so no password is in this repository or in the
// binary as text. A rare false positive only means choosing another password.
// cmd/password-filter builds the file; docs/dependencies.md names the source.
package leaked

import (
	"crypto/sha256"
	_ "embed" // the filter is built into the binary
	"encoding/binary"
	"errors"
	"strings"
)

// magic marks the file format.
const (
	magic      = "DFB1"
	headerSize = 16
)

//go:embed passwords.bloom
var embedded []byte

// Filter is a Bloom filter over lower-case passwords.
type Filter struct {
	bits   []byte
	size   uint64
	hashes uint32
}

// New makes an empty filter with room for entries, at bitsPerEntry bits each.
func New(entries, bitsPerEntry uint64, hashes uint32) *Filter {
	size := max(entries*bitsPerEntry, 8)
	return &Filter{bits: make([]byte, (size+7)/8), size: size, hashes: hashes}
}

// positions gives the two hashes that every bit position is derived from.
func positions(password string) (uint64, uint64) {
	sum := sha256.Sum256([]byte(strings.ToLower(password)))
	return binary.LittleEndian.Uint64(sum[0:8]), binary.LittleEndian.Uint64(sum[8:16])
}

// Add puts a password in the filter.
func (f *Filter) Add(password string) {
	first, second := positions(password)
	for i := range uint64(f.hashes) {
		at := (first + i*second) % f.size
		f.bits[at/8] |= 1 << (at % 8)
	}
}

// Contains says whether a password is probably in the filter. It is never wrong
// about a password that was added.
func (f *Filter) Contains(password string) bool {
	first, second := positions(password)
	for i := range uint64(f.hashes) {
		at := (first + i*second) % f.size
		if f.bits[at/8]&(1<<(at%8)) == 0 {
			return false
		}
	}
	return true
}

// Bytes is the file format: magic, number of hashes, number of bits, the bits.
func (f *Filter) Bytes() []byte {
	out := make([]byte, headerSize, headerSize+len(f.bits))
	copy(out, magic)
	binary.LittleEndian.PutUint32(out[4:8], f.hashes)
	binary.LittleEndian.PutUint64(out[8:16], f.size)
	return append(out, f.bits...)
}

// Parse reads the file format.
func Parse(data []byte) (*Filter, error) {
	if len(data) < headerSize || string(data[:4]) != magic {
		return nil, errors.New("leaked: not a password filter")
	}
	f := &Filter{hashes: binary.LittleEndian.Uint32(data[4:8]), size: binary.LittleEndian.Uint64(data[8:16]), bits: data[headerSize:]}
	if f.hashes == 0 || f.size == 0 || uint64(len(f.bits)) != (f.size+7)/8 {
		return nil, errors.New("leaked: the password filter is damaged")
	}
	return f, nil
}

// Embedded returns the filter built into this binary.
func Embedded() (*Filter, error) { return Parse(embedded) }
