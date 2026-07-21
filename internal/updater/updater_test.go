// SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clarkbar-sys/tally/internal/version"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v1.2.4", "v1.2.3", true},
		{"v1.3.0", "v1.2.9", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.3", "v1.2.4", false},
		{"1.2.4", "v1.2.3", true}, // "v" optional on either side
		{"v1.2.4-rc.1", "v1.2.3", true},
		{"v1.2.3", "v1.2.3-rc.1", false}, // suffixes ignored → equal → not newer
		{"garbage", "v1.2.3", false},     // unparseable never nags
		{"v1.2.3", "garbage", false},
		{"v1.2", "v1.2.3", false}, // wrong arity
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// withAPIBase points the GitHub API base at a stub server for the duration of a
// test and restores it after.
func withAPIBase(t *testing.T, tagJSON string, status int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+Repo+"/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(tagJSON))
	}))
	orig := apiBase
	apiBase = srv.URL
	t.Cleanup(func() {
		apiBase = orig
		srv.Close()
	})
}

func TestLatestRelease(t *testing.T) {
	withAPIBase(t, `{"tag_name":"v1.4.0"}`, http.StatusOK)
	got, err := LatestRelease(context.Background(), http.DefaultClient)
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if got != "v1.4.0" {
		t.Fatalf("tag = %q, want v1.4.0", got)
	}
}

func TestLatestReleaseHTTPError(t *testing.T) {
	withAPIBase(t, `{}`, http.StatusForbidden) // e.g. rate-limited
	if _, err := LatestRelease(context.Background(), http.DefaultClient); err == nil {
		t.Fatal("want error on non-200 response, got nil")
	}
}

func TestCheckUpdateAvailable(t *testing.T) {
	orig := version.Version
	t.Cleanup(func() { version.Version = orig })
	version.Version = "v1.0.0"

	withAPIBase(t, `{"tag_name":"v1.1.0"}`, http.StatusOK)
	current, latest, avail, err := Check(context.Background(), http.DefaultClient)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if current != "v1.0.0" || latest != "v1.1.0" || !avail {
		t.Fatalf("Check = (%q, %q, %v), want (v1.0.0, v1.1.0, true)", current, latest, avail)
	}
}

func TestCheckDevNeverUpdates(t *testing.T) {
	orig := version.Version
	t.Cleanup(func() { version.Version = orig })
	version.Version = "dev"

	withAPIBase(t, `{"tag_name":"v9.9.9"}`, http.StatusOK)
	_, _, avail, err := Check(context.Background(), http.DefaultClient)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if avail {
		t.Fatal("a dev build must never report an update available")
	}
}
