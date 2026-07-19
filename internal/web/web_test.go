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
	"time"

	"github.com/clarkbar-sys/tally/internal/ingest"
	"github.com/clarkbar-sys/tally/internal/source/demo"
	"github.com/clarkbar-sys/tally/internal/store"
)

// newHandler opens a store, syncs the demo adapter into it, and returns the
// wired handler — the same path production uses.
func newHandler(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "tally.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := ingest.Apply(context.Background(), st, demo.New(), time.Time{}); err != nil {
		t.Fatalf("ingest.Apply: %v", err)
	}
	return Handler(st), st
}

func TestHealthz(t *testing.T) {
	h, _ := newHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "ok" {
		t.Fatalf("body = %q, want %q", got, "ok")
	}
}

func TestLedgerRendersDemoData(t *testing.T) {
	h, _ := newHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"Everyday Checking", "GREEN VALLEY GROCERS", "class=\"tx-list\"", "account-balance"} {
		if !strings.Contains(body, want) {
			t.Fatalf("ledger body missing %q", want)
		}
	}
}

func TestLedgerSearchFilters(t *testing.T) {
	h, _ := newHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?q=payroll", nil))

	body := rec.Body.String()
	if !strings.Contains(body, "PAYROLL DIRECT DEPOSIT") {
		t.Fatal("search for 'payroll' should surface payroll rows")
	}
	if strings.Contains(body, "DAYBREAK COFFEE") {
		t.Fatal("search for 'payroll' should exclude unrelated rows")
	}
}

func TestStaticAssetServed(t *testing.T) {
	h, _ := newHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/app.css", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("static asset status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "--bg") {
		t.Fatal("app.css did not serve expected content")
	}
}

func TestUnknownPathNotFound(t *testing.T) {
	h, _ := newHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestExportWritesSelfContainedSite(t *testing.T) {
	_, st := newHandler(t)
	dir := t.TempDir()
	if err := Export(context.Background(), st, dir); err != nil {
		t.Fatalf("Export: %v", err)
	}

	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(index), "GREEN VALLEY GROCERS") {
		t.Fatal("exported index.html missing demo data")
	}
	if _, err := os.Stat(filepath.Join(dir, "static", "app.css")); err != nil {
		t.Fatalf("exported static/app.css missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "static", "theme.js")); err != nil {
		t.Fatalf("exported static/theme.js missing: %v", err)
	}
}
