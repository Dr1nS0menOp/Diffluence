// SPDX-License-Identifier: Apache-2.0

package attachments

import (
	"bytes"
	"path"
	"slices"
	"strings"

	"github.com/diffluence/diffluence/internal/apperr"
)

// extensionFamilies are other names for the same type, so a block on one holds for both.
var extensionFamilies = map[string][]string{"htm": {"html"}, "jpeg": {"jpg"}}

// detectedExtensions says which extension a detected type really is. A name can lie, so
// a block on a type looks at the content too (FIL-020).
var detectedExtensions = map[string]string{"text/html": "html", "application/pdf": "pdf", "application/zip": "zip",
	"application/x-gzip": "gz", "application/x-rar-compressed": "rar", "image/png": "png", "image/jpeg": "jpg",
	"image/gif": "gif", "image/webp": "webp", "text/xml": "xml"}

var windowsProgramMark = []byte("MZ")

var officeExtensions = []string{"doc", "docx", "odt", "rtf", "xls", "xlsx", "ods", "ppt", "pptx", "odp"}

func extensionOf(name string) string {
	return strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
}

// IsOffice reports whether a helper can turn this file into a PDF.
func IsOffice(filename string) bool { return slices.Contains(officeExtensions, extensionOf(filename)) }

func isBlocked(blocked []string, name, contentType string, head []byte) bool {
	extension := extensionOf(name)
	found := []string{extension, detectedExtensions[contentType]}
	found = append(found, extensionFamilies[extension]...)
	if bytes.HasPrefix(head, windowsProgramMark) {
		found = append(found, "exe")
	}
	return slices.ContainsFunc(found, func(kind string) bool { return kind != "" && slices.Contains(blocked, kind) })
}

func errBlocked() error { return apperr.Invalid("Files of this type are not allowed here.") }
