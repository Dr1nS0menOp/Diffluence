// SPDX-License-Identifier: Apache-2.0

// Package webui serves the built frontend from inside the binary, so one file is
// all an installation needs.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// The frontend build writes into dist/. An empty placeholder keeps go:embed
// working when only the backend is built.
//
//go:embed all:dist
var dist embed.FS

// contentPolicy is the Content Security Policy of the app. Everything comes from
// our own origin and no inline script can run, so an injected <script> does
// nothing even if every other defence fails. Style attributes are allowed
// because the editor sets table column widths that way; they cannot run code.
// ProseMirrorClipboard is the editor's own Trusted Types policy for pasted HTML.
const contentPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; style-src-attr 'unsafe-inline'; " +
	"img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; manifest-src 'self'; " +
	"base-uri 'none'; form-action 'self'; frame-ancestors 'none'; " +
	"require-trusted-types-for 'script'; trusted-types ProseMirrorClipboard"

// Handler serves static assets and falls back to index.html, so client-side
// routes work on reload. notFound answers everything the app does not own.
func Handler(notFound http.HandlerFunc) http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method != http.MethodGet && r.Method != http.MethodHead) ||
			strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/scim/") {
			notFound(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", contentPolicy)
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" {
			if f, err := root.Open(name); err == nil {
				info, statErr := f.Stat()
				_ = f.Close()
				if statErr == nil && !info.IsDir() {
					// Vite puts a content hash in these names, so they never change.
					if strings.HasPrefix(name, "assets/") {
						h.Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					files.ServeHTTP(w, r)
					return
				}
			}
			// A missing asset must not turn into the app page.
			if path.Ext(name) != "" {
				notFound(w, r)
				return
			}
		}
		index, err := fs.ReadFile(root, "index.html")
		if err != nil {
			http.Error(w, "The frontend was not built. Run: make web", http.StatusServiceUnavailable)
			return
		}
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-store")
		_, _ = w.Write(index)
	})
}
