// SPDX-License-Identifier: GPL-2.0-or-later

// Package web serves tally's local-first app shell and its static assets. The
// app itself (data model, persistence, all interaction) runs client-side in
// static/app.js against browser IndexedDB — there is no server-side data layer.
// This package renders the static shell ([AppPage]) and serves static/, and the
// same rendering feeds a static export ([Export]) so GitHub Pages publishes the
// whole app. The widget gallery ([GalleryPage]) is kept at /design as a living
// style reference.
package web

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

//go:embed static
var staticFS embed.FS

// Handler returns the tally HTTP handler for the live build. It serves the app
// shell, static assets, and one small data endpoint — GET /api/version, which
// tells the header chip whether a newer release exists (see version.go). The
// notch data stays client-side; there is no server-side data layer.
func Handler() http.Handler {
	return handler(newVersionChecker())
}

// handler builds the mux with an explicit version checker, so tests can inject a
// stub instead of reaching GitHub.
func handler(vc *versionChecker) http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// staticFS is embedded at build time; a missing subdir is a build bug.
		panic(fmt.Sprintf("web: static assets: %v", err))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("GET /design", page(GalleryPage()))
	// The update check lives only on the served build: the static demo export
	// (GitHub Pages) never registers this route, so it never offers an upgrade.
	mux.HandleFunc("GET /api/version", vc.handle)
	// Served (tailnet or -local): the live build. app.js persists to IndexedDB,
	// so a reload keeps your notches — unlike the demo-mode static export.
	mux.HandleFunc("GET /{$}", page(AppPage(false)))
	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

// page returns a handler that renders a templ component as HTML.
func page(c interface {
	Render(context.Context, io.Writer) error
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := c.Render(r.Context(), w); err != nil {
			log.Printf("web: render: %v", err)
		}
	}
}

// Export writes the app shell and its static assets into dir as a
// self-contained static site: index.html plus static/. Because the app is
// client-only, the published export is the whole product — GitHub Pages serves
// it and the browser's IndexedDB holds the data.
func Export(ctx context.Context, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("web: export: %w", err)
	}

	index, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return fmt.Errorf("web: export: %w", err)
	}
	defer index.Close()
	// The static export is the demo build: no server backs it, so app.js runs
	// entirely in memory with the demo banner and reload-to-reset behaviour.
	if err := AppPage(true).Render(ctx, index); err != nil {
		return fmt.Errorf("web: export: render: %w", err)
	}

	return copyStatic(dir)
}

// copyStatic mirrors the embedded static/ tree into dir/static/.
func copyStatic(dir string) error {
	return fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, path)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		src, err := staticFS.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, src); err != nil {
			return err
		}
		return nil
	})
}
