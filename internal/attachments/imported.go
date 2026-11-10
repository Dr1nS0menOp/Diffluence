// SPDX-License-Identifier: Apache-2.0

package attachments

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/storage"
)

// ImportedFile is a file from a bundle, which keeps the id its documents point to.
type ImportedFile struct {
	ID       uuid.UUID
	Origin   uuid.UUID
	SpaceID  uuid.UUID
	PageID   uuid.UUID
	Filename string
	Author   uuid.UUID
}

// AddImported stores a file within the transaction that imports its page. The caller
// has decided who may do this, and removes the returned object if the import fails.
func (s *Service) AddImported(ctx context.Context, tx pgx.Tx, f ImportedFile, body io.Reader) (string, error) {
	name, err := cleanFilename(f.Filename)
	if err != nil {
		return "", err
	}
	values, err := s.settings.GetIn(ctx, tx)
	if err != nil {
		return "", err
	}
	limit := values.MaxUploadBytes()
	reader := bufio.NewReaderSize(body, 512)
	head, err := reader.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	contentType, _, _ := strings.Cut(http.DetectContentType(head), ";")
	key := "attachments/" + ids.New().String()
	obj, err := s.store.Put(ctx, key, reader, limit)
	if errors.Is(err, storage.ErrTooLarge) {
		return "", apperr.TooLarge("A file in the bundle is larger than the upload limit.")
	}
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO attachments (id, origin_instance_id, space_id, page_id, filename, status, created_by)
		VALUES ($1, $2, $3, $4, $5, 'active', $6)`, f.ID, f.Origin, f.SpaceID, f.PageID, name, f.Author); err != nil {
		return key, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO attachment_versions (id, attachment_id, number, object_key, sha256, size, content_type, uploaded_by)
		VALUES ($1, $2, 1, $3, $4, $5, $6, $7)`, ids.New(), f.ID, key, obj.SHA256, obj.Size, contentType, f.Author)
	return key, err
}
