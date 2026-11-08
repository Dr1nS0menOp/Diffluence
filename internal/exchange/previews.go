// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/storage"
)

// previewPrefix holds derived files (ARC-195). They can always be made again, so a backup may
// leave them out, and dropping them only costs a conversion.
const previewPrefix = "previews/"

// maxPreviewBytes bounds what a helper may hand back.
const maxPreviewBytes = 512 << 20

// previewKey names the PDF of a file by the hash of its content, so a conversion is done once per
// version, and copies of the same file share it (ARC-093, ARC-196). Whoever asks has already been
// allowed to read the source file, and the PDF holds nothing that file does not.
func previewKey(sha256Hex string) (string, error) {
	raw, err := hex.DecodeString(sha256Hex)
	if err != nil || len(raw) != 32 {
		return "", fmt.Errorf("exchange: preview of a file without a valid hash")
	}
	return previewPrefix + hex.EncodeToString(raw) + "-pdf", nil
}

// OfficePreview hands the PDF of an office file to write. The first request converts it and keeps
// the result; later requests read what was kept.
func (s *Service) OfficePreview(ctx context.Context, sha256Hex, filename string, file io.Reader, write func(pdf io.Reader) error) error {
	key, err := previewKey(sha256Hex)
	if err != nil {
		return err
	}
	if kept, err := s.store.Open(ctx, key); err == nil {
		defer func() { _ = kept.Close() }()
		return write(kept)
	} else if !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	err = s.OfficeToPDF(ctx, filename, file, func(pdf io.Reader) error {
		_, err := s.store.Put(ctx, key, pdf, maxPreviewBytes)
		return err
	})
	if err != nil {
		return err
	}
	kept, err := s.store.Open(ctx, key)
	if err != nil {
		return err
	}
	defer func() { _ = kept.Close() }()
	return write(kept)
}

// PurgePreviews removes kept PDFs whose source no version of any file has any more.
func (s *Service) PurgePreviews(ctx context.Context) error {
	listed, err := s.store.List(ctx)
	if err != nil {
		return err
	}
	kept := map[string]string{} // hash in hex to key
	hashes := [][]byte{}
	for _, obj := range listed {
		if !strings.HasPrefix(obj.Key, previewPrefix) {
			continue
		}
		hash := strings.TrimSuffix(strings.TrimPrefix(obj.Key, previewPrefix), "-pdf")
		raw, err := hex.DecodeString(hash)
		if err != nil {
			continue // not a name this code makes; leave it for a person to look at
		}
		kept[hash] = obj.Key
		hashes = append(hashes, raw)
	}
	if len(hashes) == 0 {
		return nil
	}
	// One pass over the versions for all kept PDFs, instead of one per PDF.
	rows, err := s.pool.Query(ctx, "SELECT DISTINCT sha256 FROM attachment_versions WHERE sha256 = ANY($1)", hashes)
	if err != nil {
		return fmt.Errorf("exchange: previews in use: %v", err)
	}
	used, err := pgx.CollectRows(rows, pgx.RowTo[[]byte])
	if err != nil {
		return fmt.Errorf("exchange: previews in use: %v", err)
	}
	for _, raw := range used {
		delete(kept, hex.EncodeToString(raw))
	}
	for _, key := range kept {
		if err := s.store.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
