// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// openAPIVersion is the version of the description, which grows with the API.
const openAPIVersion = "1.0.0"

var pathParameter = regexp.MustCompile(`\{([a-z]+)\}`)

func operation(method, path string, a Access) map[string]any {
	parts := strings.Split(strings.TrimPrefix(path, "/api/v1/"), "/")
	parameters := []any{}
	for _, m := range pathParameter.FindAllStringSubmatch(path, -1) {
		parameters = append(parameters, map[string]any{"name": m[1], "in": "path", "required": true, "schema": map[string]any{"type": "string"}})
	}
	answers := map[string]any{
		"200":     map[string]any{"description": "Done. Most routes answer with JSON."},
		"default": map[string]any{"description": `An error, as {"error": {"code", "message"}}.`},
	}
	return map[string]any{
		"operationId": strings.ToLower(method) + " " + path,
		"tags":        []string{parts[0]},
		"parameters":  parameters,
		"x-access":    a.String(),
		"responses":   answers,
	}
}

// OpenAPI describes the API from the route table, so no route can go missing and no
// entry can outlive its route (spec 015, D19). It covers routes and rules, and not
// every field.
func OpenAPI(routes []Route) ([]byte, error) {
	paths := map[string]map[string]any{}
	for _, r := range routes {
		method, path, found := strings.Cut(r.Pattern, " ")
		if !found {
			continue // the static web app has no method
		}
		if paths[path] == nil {
			paths[path] = map[string]any{}
		}
		paths[path][strings.ToLower(method)] = operation(method, path, r.Access)
	}
	raw, err := json.MarshalIndent(map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{"title": "Diffluence API", "version": openAPIVersion,
			"description": "See docs/api/README.md for what stays stable."},
		"servers": []any{map[string]any{"url": "/"}},
		"components": map[string]any{"securitySchemes": map[string]any{
			"token":   map[string]any{"type": "http", "scheme": "bearer"},
			"session": map[string]any{"type": "apiKey", "in": "cookie", "name": "diffluence_session"}}},
		"paths": paths,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	raw, err := OpenAPI(s.Routes())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(raw); err != nil {
		s.deps.Log.Error("writing the API description failed", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
}
