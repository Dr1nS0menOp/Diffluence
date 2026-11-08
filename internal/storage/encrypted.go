// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"sync"
	"time"
)

// Sealer wraps and unwraps the data key of one file. keys.Keyring is one.
type Sealer interface {
	Seal(plaintext []byte, context string) ([]byte, error)
	Open(sealed []byte, context string) ([]byte, error)
}

// ErrUnreadable means a stored file could not be decrypted: the key is wrong or missing, or
// the file was changed or cut short. Nothing of such a file is ever handed out.
var ErrUnreadable = errors.New("storage: the file cannot be decrypted")

// ErrNotEncrypted means a plain file was found after every file was encrypted, so it is refused.
var ErrNotEncrypted = errors.New("storage: the file is not encrypted")

// The layout of an encrypted file (SEC-725). Every file has its own data key, wrapped by the
// key-encryption key and bound to the object key, so a file cannot be moved under another name.
// The body is cut in chunks, each sealed with AES-256-GCM, so memory does not grow with the
// file and a range read decrypts only the chunks it needs. The last chunk is sealed as the last,
// so a file cut at a chunk border does not open.
var magic = []byte("DFE1")

const (
	chunkPlain  = 64 << 10
	tagSize     = 16
	chunkCipher = chunkPlain + tagSize
	saltSize    = 8
	dataKeySize = 32
	// Sealed data keys are about 120 bytes; the limit keeps a damaged header from asking for much memory.
	maxWrappedKey = 1024
)

// Encrypted stores files encrypted, on top of another store. It still reads plain files written
// before encryption, until every file has been encrypted; after that it refuses them.
type Encrypted struct {
	inner  Store
	sealer Sealer
	// allDone says whether every stored file has been encrypted. It is read rarely and cached.
	allDone func(ctx context.Context) (bool, error)

	mu        sync.Mutex
	done      bool
	checkedAt time.Time
}

// recheckPlain is how long the answer of allDone is kept.
const recheckPlain = time.Minute

// NewEncrypted wraps a store. allDone reports whether plain files must now be refused.
func NewEncrypted(inner Store, sealer Sealer, allDone func(ctx context.Context) (bool, error)) *Encrypted {
	return &Encrypted{inner: inner, sealer: sealer, allDone: allDone}
}

func keyContext(key string) string { return "storage-file:" + key }

// MaxCiphertext is the stored size of a file of n plain bytes.
func MaxCiphertext(n int64) int64 {
	chunks := n / chunkPlain
	if n%chunkPlain != 0 || n == 0 {
		chunks++
	}
	return int64(len(magic)) + 2 + maxWrappedKey + saltSize + n + chunks*tagSize
}

// Put encrypts r while it is stored. Size and SHA256 describe the plain file.
func (e *Encrypted) Put(ctx context.Context, key string, r io.Reader, maxBytes int64) (Object, error) {
	dataKey := make([]byte, dataKeySize)
	if _, err := rand.Read(dataKey); err != nil {
		return Object{}, err
	}
	wrapped, err := e.sealer.Seal(dataKey, keyContext(key))
	if err != nil {
		return Object{}, fmt.Errorf("storage: seal the file key: %v", err)
	}
	if len(wrapped) > maxWrappedKey {
		return Object{}, errors.New("storage: sealed file key too large")
	}
	enc, err := newEncryptingReader(r, dataKey, wrapped, maxBytes)
	if err != nil {
		return Object{}, err
	}
	if _, err := e.inner.Put(ctx, key, enc, MaxCiphertext(maxBytes)); err != nil {
		if errors.Is(err, errTooLargePlain) || errors.Is(err, ErrTooLarge) {
			return Object{}, ErrTooLarge
		}
		return Object{}, err
	}
	return Object{Size: enc.plainSize, SHA256: enc.hash.Sum(nil)}, nil
}

// Open decrypts a stored file. A wrong key, a changed file or a file cut short fails here, before
// any of it is read, so a caller can still answer with an error instead of half a file.
func (e *Encrypted) Open(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	f, err := e.inner.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	head := make([]byte, len(magic))
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		_ = f.Close()
		return nil, err
	}
	if n < len(magic) || !bytes.Equal(head, magic) {
		return e.openPlain(ctx, f)
	}
	d, err := openDecrypting(f, e.sealer, keyContext(key))
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return d, nil
}

func (e *Encrypted) openPlain(ctx context.Context, f io.ReadSeekCloser) (io.ReadSeekCloser, error) {
	done, err := e.everyFileEncrypted(ctx)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if done {
		_ = f.Close()
		return nil, ErrNotEncrypted
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func (e *Encrypted) everyFileEncrypted(ctx context.Context) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	// Once true it stays true: files are never decrypted back.
	if e.done || time.Since(e.checkedAt) < recheckPlain {
		return e.done, nil
	}
	done, err := e.allDone(ctx)
	if err != nil {
		return false, fmt.Errorf("storage: check encryption state: %v", err)
	}
	e.done, e.checkedAt = done, time.Now()
	return done, nil
}

// IsEncrypted says whether a stored file is already encrypted, by its first bytes only.
func (e *Encrypted) IsEncrypted(ctx context.Context, key string) (bool, error) {
	f, err := e.inner.Open(ctx, key)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, len(magic))
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, err
	}
	return n == len(magic) && bytes.Equal(head, magic), nil
}

// EncryptInPlace rewrites one plain stored file encrypted, under the same key. The new file
// replaces the old one at once, so readers see either the whole old or the whole new file.
func (e *Encrypted) EncryptInPlace(ctx context.Context, key string) error {
	f, err := e.inner.Open(ctx, key)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = e.Put(ctx, key, f, size)
	return err
}

// MarkEveryFileEncrypted tells this store at once that plain files are now refused.
func (e *Encrypted) MarkEveryFileEncrypted() {
	e.mu.Lock()
	e.done = true
	e.mu.Unlock()
}

// Delete removes a file.
func (e *Encrypted) Delete(ctx context.Context, key string) error { return e.inner.Delete(ctx, key) }

// List names the stored files. Sizes are the stored sizes, a little larger than the plain files.
func (e *Encrypted) List(ctx context.Context) ([]Listed, error) { return e.inner.List(ctx) }

// Healthy checks the store below.
func (e *Encrypted) Healthy(ctx context.Context) error { return e.inner.Healthy(ctx) }

// Space passes on the room of the store below, when it knows it.
func (e *Encrypted) Space(ctx context.Context) (free, total uint64, err error) {
	s, ok := e.inner.(Spacer)
	if !ok {
		return 0, 0, errors.New("storage: the store does not know its room")
	}
	return s.Space(ctx)
}

var errTooLargePlain = errors.New("storage: plain file too large")

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func chunkNonce(salt []byte, index uint64) []byte {
	nonce := make([]byte, 12)
	copy(nonce, salt)
	binary.BigEndian.PutUint32(nonce[saltSize:], uint32(index))
	return nonce
}

func chunkAAD(index uint64, last bool) []byte {
	aad := make([]byte, 9)
	binary.BigEndian.PutUint64(aad, index)
	if last {
		aad[8] = 1
	}
	return aad
}

// maxChunks keeps the counter in the nonce from wrapping: 2^32 chunks of 64 KiB is 256 TiB.
const maxChunks = 1 << 32

// encryptingReader turns a plain stream into the stored layout while it is read.
type encryptingReader struct {
	src     io.Reader
	aead    cipher.AEAD
	salt    []byte
	out     bytes.Buffer
	buf     []byte
	next    []byte
	index   uint64
	ended   bool
	maxSize int64

	plainSize int64
	hash      hash.Hash
}

func newEncryptingReader(src io.Reader, dataKey, wrapped []byte, maxSize int64) (*encryptingReader, error) {
	aead, err := newGCM(dataKey)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	r := &encryptingReader{src: src, aead: aead, salt: salt, maxSize: maxSize, hash: sha256.New(),
		buf: make([]byte, 0, chunkPlain), next: make([]byte, 0, chunkPlain)}
	r.out.Write(magic)
	var length [2]byte
	binary.BigEndian.PutUint16(length[:], uint16(len(wrapped)))
	r.out.Write(length[:])
	r.out.Write(wrapped)
	r.out.Write(salt)
	return r, nil
}

func (r *encryptingReader) Read(p []byte) (int, error) {
	for r.out.Len() == 0 {
		if r.ended {
			return 0, io.EOF
		}
		if err := r.fill(); err != nil {
			return 0, err
		}
	}
	return r.out.Read(p)
}

// fill reads one chunk ahead, so it knows whether the chunk in hand is the last one.
func (r *encryptingReader) fill() error {
	if len(r.buf) == 0 && r.index == 0 {
		chunk, err := r.readChunk()
		if err != nil {
			return err
		}
		r.buf = append(r.buf[:0], chunk...)
	}
	more, err := r.readChunk()
	if err != nil {
		return err
	}
	last := len(more) == 0
	if r.index >= maxChunks {
		return errTooLargePlain
	}
	r.out.Write(r.aead.Seal(nil, chunkNonce(r.salt, r.index), r.buf, chunkAAD(r.index, last)))
	r.index++
	r.buf = append(r.buf[:0], more...)
	r.ended = last
	return nil
}

// readChunk reads up to one full chunk of plain bytes, counting and hashing them.
func (r *encryptingReader) readChunk() ([]byte, error) {
	r.next = r.next[:chunkPlain]
	n, err := io.ReadFull(r.src, r.next)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	r.plainSize += int64(n)
	if r.plainSize > r.maxSize {
		return nil, errTooLargePlain
	}
	r.hash.Write(r.next[:n])
	return r.next[:n], nil
}

// decrypting reads a stored file as its plain bytes, one chunk at a time.
type decrypting struct {
	f      io.ReadSeekCloser
	aead   cipher.AEAD
	salt   []byte
	body   int64 // where the first chunk starts
	chunks uint64
	size   int64 // plain size
	pos    int64

	cached int64 // index of the chunk in plain, or -1
	plain  []byte
}

func openDecrypting(f io.ReadSeekCloser, sealer Sealer, context string) (*decrypting, error) {
	var length [2]byte
	if _, err := io.ReadFull(f, length[:]); err != nil {
		return nil, ErrUnreadable
	}
	n := int(binary.BigEndian.Uint16(length[:]))
	if n == 0 || n > maxWrappedKey {
		return nil, ErrUnreadable
	}
	wrapped := make([]byte, n)
	if _, err := io.ReadFull(f, wrapped); err != nil {
		return nil, ErrUnreadable
	}
	dataKey, err := sealer.Open(wrapped, context)
	if err != nil {
		return nil, ErrUnreadable
	}
	aead, err := newGCM(dataKey)
	if err != nil {
		return nil, ErrUnreadable
	}
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(f, salt); err != nil {
		return nil, ErrUnreadable
	}
	body := int64(len(magic) + 2 + n + saltSize)
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}
	stored := end - body
	if stored < tagSize {
		return nil, ErrUnreadable
	}
	chunks := (stored + chunkCipher - 1) / chunkCipher
	lastLen := stored - (chunks-1)*chunkCipher
	if lastLen < tagSize {
		return nil, ErrUnreadable
	}
	d := &decrypting{f: f, aead: aead, salt: salt, body: body, chunks: uint64(chunks),
		size: (chunks-1)*chunkPlain + lastLen - tagSize, cached: -1}
	// Opening the last chunk proves the key, and that the file was not cut short.
	if _, err := d.chunk(chunks - 1); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *decrypting) chunk(index int64) ([]byte, error) {
	if index == d.cached {
		return d.plain, nil
	}
	length := int64(chunkCipher)
	if uint64(index) == d.chunks-1 {
		length = d.size - index*chunkPlain + tagSize
	}
	if _, err := d.f.Seek(d.body+index*chunkCipher, io.SeekStart); err != nil {
		return nil, err
	}
	sealed := make([]byte, length)
	if _, err := io.ReadFull(d.f, sealed); err != nil {
		return nil, ErrUnreadable
	}
	last := uint64(index) == d.chunks-1
	plain, err := d.aead.Open(d.plain[:0], chunkNonce(d.salt, uint64(index)), sealed, chunkAAD(uint64(index), last))
	if err != nil {
		d.cached = -1
		return nil, ErrUnreadable
	}
	d.plain, d.cached = plain, index
	return plain, nil
}

func (d *decrypting) Read(p []byte) (int, error) {
	if d.pos >= d.size {
		return 0, io.EOF
	}
	index := d.pos / chunkPlain
	plain, err := d.chunk(index)
	if err != nil {
		return 0, err
	}
	n := copy(p, plain[d.pos-index*chunkPlain:])
	d.pos += int64(n)
	return n, nil
}

func (d *decrypting) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = d.pos + offset
	case io.SeekEnd:
		next = d.size + offset
	default:
		return 0, errors.New("storage: invalid whence")
	}
	if next < 0 {
		return 0, errors.New("storage: negative position")
	}
	d.pos = next
	return next, nil
}

func (d *decrypting) Close() error { return d.f.Close() }
