// SPDX-License-Identifier: GPL-2.0-or-later

package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubChecker builds a versionChecker whose check func returns fixed values, so
// the endpoint and cache can be tested without reaching GitHub.
func stubChecker(cur, latest string, avail bool, err error) *versionChecker {
	return &versionChecker{
		check: func(context.Context) (string, string, bool, error) {
			return cur, latest, avail, err
		},
	}
}

func getFrom(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestVersionEndpointReportsUpdate(t *testing.T) {
	h := handler(stubChecker("v1.0.0", "v1.1.0", true, nil))
	rec := getFrom(h, "/api/version")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type = %q, want JSON", ct)
	}
	var st versionStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.Current != "v1.0.0" || st.Latest != "v1.1.0" || !st.UpdateAvailable {
		t.Fatalf("status = %+v, want current v1.0.0 latest v1.1.0 update true", st)
	}
}

func TestVersionEndpointDegradesOnError(t *testing.T) {
	h := handler(stubChecker("v1.0.0", "", false, errors.New("boom")))
	rec := getFrom(h, "/api/version")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var st versionStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Even when GitHub is unreachable the current build is still reported, so the
	// chip keeps showing a version; only the update offer is withheld.
	if st.Current != "v1.0.0" {
		t.Fatalf("current = %q, want v1.0.0", st.Current)
	}
	if st.UpdateAvailable {
		t.Fatal("update must not be offered when the check failed")
	}
	if st.Error == "" {
		t.Fatal("error should be surfaced to the client")
	}
}

func TestVersionCheckerCaches(t *testing.T) {
	calls := 0
	vc := &versionChecker{
		check: func(context.Context) (string, string, bool, error) {
			calls++
			return "v1.0.0", "v1.0.0", false, nil
		},
	}

	vc.status(context.Background(), false)
	vc.status(context.Background(), false)
	if calls != 1 {
		t.Fatalf("check called %d times, want 1 (second should hit cache)", calls)
	}

	// A forced check bypasses the cache.
	vc.status(context.Background(), true)
	if calls != 2 {
		t.Fatalf("check called %d times after force, want 2", calls)
	}
}
