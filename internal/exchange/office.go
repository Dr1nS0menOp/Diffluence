// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/diffluence/diffluence/internal/apperr"
)

const (
	libreOfficeConvertPath = "/forms/libreoffice/convert"
	libreOfficeField       = "files"
	collaboraConvertPath   = "/cool/convert-to/pdf"
	collaboraField         = "data"
	officeDocumentName     = "document"
)

// officeHelper is who turns an office file into a PDF: Collabora when an administrator
// configured it (ADR-026), or else the LibreOffice of the PDF helper.
func (s *Service) officeHelper() (address, field string, ok bool) {
	if s.collaboraURL != "" {
		return strings.TrimRight(s.collaboraURL, "/") + collaboraConvertPath, collaboraField, true
	}
	if s.pdfURL != "" {
		return strings.TrimRight(s.pdfURL, "/") + libreOfficeConvertPath, libreOfficeField, true
	}
	return "", "", false
}

// OfficeAvailable says whether office files can open as a PDF.
func (s *Service) OfficeAvailable() bool {
	_, _, ok := s.officeHelper()
	return ok
}

// OfficeToPDF sends a file to the helper and hands the PDF to write. The helper only gets
// the extension, which it needs to read the file, and not the name someone gave it.
func (s *Service) OfficeToPDF(parent context.Context, filename string, file io.Reader, write func(pdf io.Reader) error) error {
	address, field, ok := s.officeHelper()
	if !ok {
		return apperr.Conflict("no_preview", "No helper is connected to open this file as a PDF.")
	}
	ctx, cancel := context.WithTimeout(parent, pdfTimeout)
	defer cancel()
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	form := multipart.NewWriter(writer)
	go func() {
		part, err := form.CreateFormFile(field, officeDocumentName+path.Ext(filename))
		if err == nil {
			_, err = io.Copy(part, file)
		}
		if err == nil {
			err = form.Close()
		}
		_ = writer.CloseWithError(err)
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return apperr.Invalid("The PDF helper did not answer. Try again later, or ask an administrator.")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return apperr.Invalid("The PDF helper could not read this file. Try again later, or ask an administrator.")
	}
	return write(resp.Body)
}
