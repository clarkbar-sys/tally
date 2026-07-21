// SPDX-License-Identifier: GPL-2.0-or-later

// Package updater checks GitHub for a newer tally release. tally never updates
// itself — the box is upgraded by re-running install.sh — so this only reports
// whether a newer tag exists. It feeds the header's version chip and its
// upgrade popup (see internal/web) and is used only by the served build; the
// static demo export has no server to reach GitHub.
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/clarkbar-sys/tally/internal/version"
)

// Repo is the owner/name whose releases tally checks — the same repo install.sh
// fetches release binaries from.
const Repo = "clarkbar-sys/tally"

// apiBase is the GitHub REST API root. It is a var only so tests can point it at
// an httptest server; production never changes it.
var apiBase = "https://api.github.com"

// maxBody caps the release JSON we read so a broken or hostile response can't
// exhaust memory.
const maxBody = 1 << 20 // 1 MiB

type ghRelease struct {
	TagName string `json:"tag_name"`
}

// LatestRelease returns the tag_name of the repo's latest published release
// (e.g. "v1.2.3"). The caller supplies the HTTP client, so timeout and transport
// are its choice.
func LatestRelease(ctx context.Context, client *http.Client) (string, error) {
	url := apiBase + "/repos/" + Repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("updater: GitHub returned %s", resp.Status)
	}

	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&rel); err != nil {
		return "", fmt.Errorf("updater: decode release: %w", err)
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("updater: release has no tag_name")
	}
	return rel.TagName, nil
}

// Check reports the running version, the latest released version, and whether
// the latest is newer. A "dev" build (an untagged local binary) never reports an
// update available: there is no meaningful version to compare, so it never nags.
func Check(ctx context.Context, client *http.Client) (current, latest string, updateAvailable bool, err error) {
	current = version.Version
	latest, err = LatestRelease(ctx, client)
	if err != nil {
		return current, "", false, err
	}
	updateAvailable = current != "dev" && Newer(latest, current)
	return current, latest, updateAvailable, nil
}

// Newer reports whether semantic version a is strictly greater than b. Versions
// parse as [v]MAJOR.MINOR.PATCH; any pre-release ("-rc.1") or build ("+meta")
// suffix is ignored. Unparseable input compares as not-newer, so a malformed tag
// never triggers a spurious update prompt.
func Newer(a, b string) bool {
	amaj, amin, apat, aok := parseSemver(a)
	bmaj, bmin, bpat, bok := parseSemver(b)
	if !aok || !bok {
		return false
	}
	if amaj != bmaj {
		return amaj > bmaj
	}
	if amin != bmin {
		return amin > bmin
	}
	return apat > bpat
}

func parseSemver(s string) (major, minor, patch int, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	// Drop any pre-release ("-rc.1") or build ("+meta") suffix.
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	var err error
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, 0, false
	}
	if minor, err = strconv.Atoi(parts[1]); err != nil {
		return 0, 0, 0, false
	}
	if patch, err = strconv.Atoi(parts[2]); err != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}
