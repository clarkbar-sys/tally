// SPDX-License-Identifier: GPL-2.0-or-later

// Package web holds tally's HTTP handlers and server-rendered ledger UI (#11,
// #12). Pages are rendered with templ against the canonical model read from the
// store; the same rendering feeds a static export ([Export]) so a PR preview
// can publish the real UI, driven by demo data, without a running server.
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
	"time"

	"github.com/clarkbar-sys/tally/internal/store"
)

//go:embed static
var staticFS embed.FS

// Handler returns the tally HTTP handler backed by st.
func Handler(st *store.Store) http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// staticFS is embedded at build time; a missing subdir is a build bug.
		panic(fmt.Sprintf("web: static assets: %v", err))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("/", ledgerHandler(st))
	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

// ledgerHandler serves the ledger at "/", honouring the ?q= search filter.
func ledgerHandler(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		v, err := buildLedgerView(r.Context(), st, time.Now(), r.URL.Query().Get("q"))
		if err != nil {
			log.Printf("ledger: build view: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := Page(v).Render(r.Context(), w); err != nil {
			log.Printf("ledger: render: %v", err)
		}
	}
}

// Export writes the ledger and its static assets into dir as a self-contained
// static site: index.html plus static/. It renders exactly what the live
// handler serves (unfiltered), so a published export is a faithful snapshot of
// the real UI — the basis for the PR preview. Server-driven interactions
// (search) are inert in the snapshot; the rendered page is otherwise identical.
func Export(ctx context.Context, st *store.Store, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("web: export: %w", err)
	}

	v, err := buildLedgerView(ctx, st, time.Now(), "")
	if err != nil {
		return fmt.Errorf("web: export: build view: %w", err)
	}
	index, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return fmt.Errorf("web: export: %w", err)
	}
	defer index.Close()
	if err := Page(v).Render(ctx, index); err != nil {
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
