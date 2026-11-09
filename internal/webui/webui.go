// SPDX-License-Identifier: Apache-2.0

// Package webui serves the built frontend from inside the binary, so one file is
// all an installation needs.
package webui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
)

// buildOnce and buildID hold the build id. The embedded files never change while the binary
// runs, so it is worked out once: logically constant data, not shared state.
var (
	buildOnce sync.Once
	buildID   string
)

// Build is a short id of the frontend inside this binary: a hash of index.html, which names
// every asset by its content, so each frontend build gets a new id. An open tab compares it
// to find out that a newer version runs (spec 041). It tells nothing a visitor cannot already
// download. It is empty when the frontend was not built.
func Build() string {
	buildOnce.Do(func() {
		root, err := fs.Sub(dist, "dist")
		if err != nil {
			return
		}
		buildID = buildOf(root)
	})
	return buildID
}

// buildOf hashes index.html of a frontend, or gives "" when there is none.
func buildOf(root fs.FS) string {
	index, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(index)
	return hex.EncodeToString(sum[:8])
}

// The frontend build writes into dist/. An empty placeholder keeps go:embed
// working when only the backend is built.
//
//go:embed all:dist
var dist embed.FS

// contentPolicy gives the Content Security Policy of the app. Everything comes from
// our own origin and no inline script can run, so an injected <script> does
// nothing even if every other defence fails. Style attributes are allowed
// because the editor sets table column widths that way; they cannot run code.
// ProseMirrorClipboard is the editor's own Trusted Types policy for pasted HTML.
// Only our own viewers may be framed, and a drawing service if an administrator set one.
func contentPolicy(drawingOrigin, publicOrigin string) string {
	frames := "frame-src " + publicOrigin + "/" + vendorFolder
	if drawingOrigin != "" {
		frames += " " + drawingOrigin
	}
	frames += "; "
	return "default-src 'none'; script-src 'self'; style-src 'self'; style-src-attr 'unsafe-inline'; " +
		"img-src 'self' data: blob:; media-src 'self'; font-src 'self'; connect-src 'self'; manifest-src 'self'; " +
		frames + "base-uri 'none'; form-action 'self'; frame-ancestors 'none'; " +
		"require-trusted-types-for 'script'; trusted-types ProseMirrorClipboard"
}

// vendorFolder holds the web builds of PdfCraft and GridCraft (spec 019).
const vendorFolder = "vendor/"

// viewerPolicy runs a viewer in a sandbox with no origin of its own. It cannot read
// the session, open a window, or leave the frame, and a file it opens can only go
// back to the person as a download. Its WebAssembly is why it has its own policy:
// the app itself never gets 'wasm-unsafe-eval'. A sandboxed page has no self, so
// the address is named as well.
func viewerPolicy(publicOrigin string) string {
	return "default-src 'none'; script-src 'self' " + publicOrigin + " 'wasm-unsafe-eval'; connect-src 'self' " + publicOrigin +
		"; style-src 'unsafe-inline'; img-src blob: data:; font-src 'self' " + publicOrigin +
		"; base-uri 'none'; form-action 'none'; frame-ancestors 'self'; sandbox allow-scripts allow-downloads"
}

// serveViewerFile sends one file of a viewer. Its name holds a hash of its content,
// and a sandboxed frame needs an open door to load it, which is safe for public code.
func serveViewerFile(w http.ResponseWriter, r *http.Request, root fs.FS, name, publicOrigin string, notFound http.HandlerFunc) {
	stored := name
	packed := false
	if path.Ext(name) == ".wasm" {
		stored, packed = name+".gz", true
	}
	data, err := fs.ReadFile(root, stored)
	if err != nil {
		notFound(w, r)
		return
	}
	h := w.Header()
	h.Del("X-Frame-Options")
	h.Set("Content-Security-Policy", viewerPolicy(publicOrigin))
	h.Set("Cross-Origin-Resource-Policy", "cross-origin")
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	if path.Ext(name) == ".html" {
		h.Set("Cache-Control", "no-cache")
	}
	if packed {
		h.Set("Content-Encoding", "gzip")
		h.Set("Content-Type", "application/wasm")
	} else if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		h.Set("Content-Type", contentType)
	}
	_, _ = w.Write(data)
}

// Handler serves static assets and falls back to index.html, so client-side
// routes work on reload. notFound answers everything the app does not own.
func Handler(notFound http.HandlerFunc, drawingOrigin, publicOrigin string) http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return newHandler(root, notFound, drawingOrigin, publicOrigin)
}

func newHandler(root fs.FS, notFound http.HandlerFunc, drawingOrigin, publicOrigin string) http.Handler {
	policy := contentPolicy(drawingOrigin, publicOrigin)
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method != http.MethodGet && r.Method != http.MethodHead) ||
			strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/scim/") {
			notFound(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", policy)
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if strings.HasPrefix(name, vendorFolder) {
			serveViewerFile(w, r, root, name, publicOrigin, notFound)
			return
		}
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
