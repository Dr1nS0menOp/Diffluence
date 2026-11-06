// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	pdfTimeout        = 5 * time.Minute
	pdfConvertPath    = "/forms/chromium/convert/html"
	maxPDFDesignRunes = 200
)

// Sizes of paper in inches, the unit of the helper.
var paperSizes = map[string][2]string{"A4": {"8.27", "11.7"}, "Letter": {"8.5", "11"}}

// PDFDesign is what a space puts around its PDF files.
type PDFDesign struct {
	CoverTitle    string     `json:"cover_title"`
	CoverSubtitle string     `json:"cover_subtitle"`
	LogoID        *uuid.UUID `json:"logo_id"`
	Header        string     `json:"header"`
	Footer        string     `json:"footer"`
}

// PDFAvailable says whether a helper process is connected.
func (s *Service) PDFAvailable() bool { return s.pdfURL != "" }

// SetPDFHelper points to the helper process. Tests use it to fit a fake one.
func (s *Service) SetPDFHelper(url string) { s.pdfURL = url }

// PDFDesignOf reads the design of a space.
func (s *Service) PDFDesignOf(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (PDFDesign, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return PDFDesign{}, err
	}
	return s.loadPDFDesign(ctx, spaceID)
}

func (s *Service) loadPDFDesign(ctx context.Context, spaceID uuid.UUID) (PDFDesign, error) {
	var d PDFDesign
	err := s.pool.QueryRow(ctx, `SELECT cover_title, cover_subtitle, logo_id, header, footer FROM space_pdf_design WHERE space_id = $1`, spaceID).
		Scan(&d.CoverTitle, &d.CoverSubtitle, &d.LogoID, &d.Header, &d.Footer)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, nil
	}
	return d, err
}

// SetPDFDesign stores the design of a space.
func (s *Service) SetPDFDesign(ctx context.Context, c identity.Caller, spaceID uuid.UUID, d PDFDesign) error {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return err
	}
	for _, text := range []string{d.CoverTitle, d.CoverSubtitle, d.Header, d.Footer} {
		if len([]rune(text)) > maxPDFDesignRunes {
			return apperr.Invalid("Keep these texts under 200 characters.")
		}
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO space_pdf_design (space_id, cover_title, cover_subtitle, logo_id, header, footer)
			VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (space_id) DO UPDATE SET cover_title = EXCLUDED.cover_title,
			cover_subtitle = EXCLUDED.cover_subtitle, logo_id = EXCLUDED.logo_id, header = EXCLUDED.header, footer = EXCLUDED.footer`,
			spaceID, d.CoverTitle, d.CoverSubtitle, d.LogoID, d.Header, d.Footer); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("space.pdf_design_changed", "space", spaceID.String(), spaceID.String(), nil)); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "space.pdf_design_changed", map[string]any{"space_id": spaceID})
		return err
	})
}

// marginText turns the words of a header or footer into what the helper fills in.
func marginText(text, title string) string {
	if text == "" {
		return ""
	}
	out := strings.NewReplacer(
		"{page}", `<span class="pageNumber"></span>`,
		"{pages}", `<span class="totalPages"></span>`,
		"{date}", `<span class="date"></span>`,
		"{title}", html.EscapeString(title),
	).Replace(html.EscapeString(text))
	return `<html><body><div style="font-size:9px;inline-size:100%;text-align:center;font-family:sans-serif">` + out + `</div></body></html>`
}

// pdfBody is the one HTML file of a PDF: cover, contents, then every page.
func (w *exportWriter) pdfBody(ctx context.Context, design PDFDesign, images map[string][]byte) string {
	var body strings.Builder
	cover := design.CoverTitle
	if cover != "" || design.CoverSubtitle != "" {
		fmt.Fprintf(&body, `<section class="cover"><h1>%s</h1><p>%s</p></section>`, html.EscapeString(cover), html.EscapeString(design.CoverSubtitle))
	}
	if w.run.Options.Contents && len(w.items) > 1 {
		body.WriteString(`<nav class="contents"><h2>Contents</h2><ol>`)
		for _, item := range w.items {
			fmt.Fprintf(&body, `<li><a href="#p-%s">%s</a></li>`, item.page.ID, html.EscapeString(item.page.Title))
		}
		body.WriteString(`</ol></nav>`)
	}
	sections := map[uuid.UUID]bool{}
	for _, item := range w.items {
		sections[item.page.ID] = true
	}
	for _, item := range w.items {
		options := document.HTMLOptions{
			ImageSrc: func(id uuid.UUID) (string, bool) { return w.pdfImage(ctx, item, id, images) },
			PageHref: func(id uuid.UUID) (string, bool) { return "#p-" + id.String(), sections[id] },
		}
		fmt.Fprintf(&body, `<section id="p-%s" class="pdf-page"><h1>%s</h1>%s</section>`, item.page.ID,
			html.EscapeString(item.page.Title), document.ToHTML(w.withComments(ctx, item), options))
	}
	return body.String()
}

func (w *exportWriter) pdfImage(ctx context.Context, item exportItem, id uuid.UUID, images map[string][]byte) (string, bool) {
	a, ok := item.files[id]
	if !ok {
		return "", false
	}
	ext := strings.ToLower(path.Ext(a.Filename))
	if imageExtensions[ext] == "" {
		return "", false
	}
	data, err := w.readFile(ctx, a)
	if err != nil {
		return "", false
	}
	name := id.String() + ext
	images[name] = data
	return name, true
}

func (w *exportWriter) pdf(parent context.Context) error {
	if !w.s.PDFAvailable() {
		return apperr.Invalid("PDF is not available on this instance.")
	}
	design, err := w.s.loadPDFDesign(parent, w.run.SpaceID)
	if err != nil {
		return err
	}
	images := map[string][]byte{}
	body := w.pdfBody(parent, design, images)
	page := `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>` + html.EscapeString(w.title) + `</title><style>` +
		document.ExportStyle + `</style></head><body><main>` + body + `</main></body></html>`

	form, contentType, err := w.pdfForm(page, design, images)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, pdfTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(w.s.pdfURL, "/")+pdfConvertPath, form)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return apperr.Invalid("The PDF helper did not answer. Try again later, or ask an administrator.")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return apperr.Invalid("The PDF helper could not make this file. Try again later, or ask an administrator.")
	}
	_, err = io.Copy(w.out, resp.Body)
	return err
}

func (w *exportWriter) pdfForm(page string, design PDFDesign, images map[string][]byte) (*bytes.Buffer, string, error) {
	form := &bytes.Buffer{}
	mw := multipart.NewWriter(form)
	addFile := func(name string, data []byte) error {
		part, err := mw.CreateFormFile("files", name)
		if err != nil {
			return err
		}
		_, err = part.Write(data)
		return err
	}
	if err := addFile(indexFile, []byte(page)); err != nil {
		return nil, "", err
	}
	if header := marginText(design.Header, w.title); header != "" {
		if err := addFile("header.html", []byte(header)); err != nil {
			return nil, "", err
		}
	}
	if footer := marginText(design.Footer, w.title); footer != "" {
		if err := addFile("footer.html", []byte(footer)); err != nil {
			return nil, "", err
		}
	}
	for name, data := range images {
		if err := addFile(name, data); err != nil {
			return nil, "", err
		}
	}
	size := paperSizes[w.run.Options.Paper]
	fields := map[string]string{"paperWidth": size[0], "paperHeight": size[1], "printBackground": "true",
		"landscape": fmt.Sprint(w.run.Options.Orientation == "landscape"), "generateDocumentOutline": "true", "generateTaggedPdf": "true"}
	for key, value := range fields {
		if err := mw.WriteField(key, value); err != nil {
			return nil, "", err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return form, mw.FormDataContentType(), nil
}
