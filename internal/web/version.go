// SPDX-License-Identifier: GPL-2.0-or-later

package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/clarkbar-sys/tally/internal/updater"
)

// versionStatus is the JSON the app shell fetches from /api/version to drive the
// header's version chip and its "update available" popup. Only the live build
// (tailnet or -local) serves this route; the static demo export has no backend,
// so the chip there stays a plain version label and no update is ever offered.
type versionStatus struct {
	Current         string `json:"current"`             // running build, e.g. "v1.2.0" or "dev"
	Latest          string `json:"latest,omitempty"`    // latest released tag
	UpdateAvailable bool   `json:"updateAvailable"`     // latest is newer than current
	CheckedAt       string `json:"checkedAt,omitempty"` // RFC3339, when GitHub was last asked
	Error           string `json:"error,omitempty"`     // last check failure, if any
}

// versionCheckTTL bounds how often we ask GitHub for the latest release, so a
// busy tailnet can't turn every page load into an API call — GitHub rate-limits
// unauthenticated requests hard. A forced check (?force=1) bypasses the cache.
const versionCheckTTL = time.Hour

// versionChecker caches the last release check behind a short TTL. The check
// func is injected so tests can stub it; in production it talks to GitHub over
// the host's normal network with a plain client and a tight timeout (tsnet is
// for serving tally, not for outbound calls).
type versionChecker struct {
	check func(context.Context) (current, latest string, updateAvailable bool, err error)

	mu       sync.Mutex
	cached   versionStatus
	cachedAt time.Time
	fresh    bool
}

func newVersionChecker() *versionChecker {
	client := &http.Client{Timeout: 5 * time.Second}
	return &versionChecker{
		check: func(ctx context.Context) (string, string, bool, error) {
			return updater.Check(ctx, client)
		},
	}
}

// status returns the current/latest version, serving the cached result when it
// is fresh, error-free, and not forced. On a failed check it keeps serving the
// last good latest/updateAvailable with an Error attached, so a transient GitHub
// blip doesn't make the chip flicker between states.
func (v *versionChecker) status(ctx context.Context, force bool) versionStatus {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.fresh && !force && v.cached.Error == "" && time.Since(v.cachedAt) < versionCheckTTL {
		return v.cached
	}

	current, latest, avail, err := v.check(ctx)
	st := versionStatus{Current: current, Latest: latest, UpdateAvailable: avail}
	if err != nil {
		st.Error = err.Error()
		if v.fresh && v.cached.Latest != "" {
			st.Latest = v.cached.Latest
			st.UpdateAvailable = v.cached.UpdateAvailable
		}
	} else {
		st.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	}

	v.cached = st
	v.cachedAt = time.Now()
	v.fresh = true
	return st
}

// handle serves the version status as JSON.
func (v *versionChecker) handle(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") != ""
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v.status(r.Context(), force)); err != nil {
		log.Printf("web: encode version: %v", err)
	}
}
