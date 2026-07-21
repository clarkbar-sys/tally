// SPDX-License-Identifier: GPL-2.0-or-later

package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func get(path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	// These shell/static/version tests don't touch persistence, so a nil store is
	// fine — the /api/state routes just aren't mounted. State endpoints have their
	// own coverage in state_test.go with a real store.
	Handler(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	rec := get("/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "ok" {
		t.Fatalf("body = %q, want %q", got, "ok")
	}
}

func TestRootRendersAppShell(t *testing.T) {
	rec := get("/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	// The shell carries the version, mounts the client app, and loads app.js.
	// Served pages run the live build (data-mode="live"), so app.js persists to
	// the server via /api/state rather than running in demo mode.
	for _, want := range []string{versionString(), `id="view"`, "static/app.js", "local-first", `data-mode="live"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("app shell missing %q", want)
		}
	}
}

func TestDesignRendersGallery(t *testing.T) {
	rec := get("/design")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"mark-cell", `class="check`, "Buttons", "Tally-marks"} {
		if !strings.Contains(body, want) {
			t.Fatalf("gallery missing %q", want)
		}
	}
}

func TestStaticAssetsServed(t *testing.T) {
	css := get("/static/app.css")
	if css.Code != http.StatusOK || !strings.Contains(css.Body.String(), "--bg") {
		t.Fatalf("app.css not served correctly (status %d)", css.Code)
	}
	js := get("/static/app.js")
	if js.Code != http.StatusOK || !strings.Contains(js.Body.String(), "demoSeed") {
		t.Fatalf("app.js not served correctly (status %d)", js.Code)
	}
}

func TestUnknownPathNotFound(t *testing.T) {
	if rec := get("/nope"); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestExportWritesSelfContainedSite(t *testing.T) {
	dir := t.TempDir()
	if err := Export(context.Background(), dir); err != nil {
		t.Fatalf("Export: %v", err)
	}

	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(index), versionString()) {
		t.Fatal("exported index.html missing version")
	}
	// The static export is the demo build (data-mode="demo"): no server backs
	// it, so app.js must run in memory-only demo mode, not try to persist.
	if !strings.Contains(string(index), `data-mode="demo"`) {
		t.Fatal("exported index.html should render the demo build (data-mode=\"demo\")")
	}
	for _, asset := range []string{"app.css", "app.js"} {
		if _, err := os.Stat(filepath.Join(dir, "static", asset)); err != nil {
			t.Fatalf("exported static/%s missing: %v", asset, err)
		}
	}
}
