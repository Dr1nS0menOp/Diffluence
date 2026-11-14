// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/diffluence/diffluence/internal/apperr"
)

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		// Fields names the refused fields of a form, each with why (spec 074, TPL-324).
		Fields []apperr.FieldProblem `json:"fields,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

// writeError sends {"error":{"code":..., "message":...}}. Messages are meant
// for users: never put internal details in them.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeErrorFields(w, status, code, message, nil)
}

// writeErrorFields is writeError with the refused fields of a form.
func writeErrorFields(w http.ResponseWriter, status int, code, message string, fields []apperr.FieldProblem) {
	var b errorBody
	b.Error.Code, b.Error.Message, b.Error.Fields = code, message, fields
	writeJSON(w, status, b)
}

var errBadJSON = errors.New("invalid JSON body")

// decodeJSON reads exactly one JSON object with known fields only.
func decodeJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) error {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		return errBadJSON
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errBadJSON
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errBadJSON
	}
	return nil
}
