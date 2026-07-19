// SPDX-License-Identifier: GPL-2.0-or-later

// Package web holds tally's HTTP handler and server-rendered UI. The current
// page is a hello-world design shell — the Ember Terminal theme plus a widget
// gallery ([Page]) — rendered with templ. The same rendering feeds a static
// export ([Export]) so GitHub Pages can publish it without a running server.
// There is no data layer yet; the domain model is being reworked.
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

// Handler returns the tally HTTP handler.
func Handler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// staticFS is embedded at build time; a missing subdir is a build bug.
		panic(fmt.Sprintf("web: static assets: %v", err))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("/", pageHandler)
	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

// pageHandler serves the design shell at "/".
func pageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := Page().Render(r.Context(), w); err != nil {
		log.Printf("page: render: %v", err)
	}
}

// Export writes the page and its static assets into dir as a self-contained
// static site: index.html plus static/. It renders exactly what the live
// handler serves, so a published export is a faithful snapshot of the real UI —
// the basis for the GitHub Pages preview.
func Export(ctx context.Context, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("web: export: %w", err)
	}

	index, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return fmt.Errorf("web: export: %w", err)
	}
	defer index.Close()
	if err := Page().Render(ctx, index); err != nil {
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
