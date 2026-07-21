// SPDX-License-Identifier: GPL-2.0-or-later

package web

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/clarkbar-sys/tally/internal/version"
)

// iconCheck is the checkmark drawn inside a checked checkbox.
const iconCheck = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12l5 5L20 6"/></svg>`

// versionString is the running build's version, shown in the header. Kept in Go
// so the template stays declarative.
func versionString() string { return version.Version }

// pageMode is the value of the app shell's data-mode attribute, the one signal
// static/app.js reads to pick its storage backend. The static export (GitHub
// Pages / PR preview) renders "demo" — in-memory only, wiped on reload — while
// the build tally serves on the tailnet renders "live", persisting to the
// browser's IndexedDB. See AppPage and the "storage mode" note in app.js.
func pageMode(demo bool) string {
	if demo {
		return "demo"
	}
	return "live"
}

// markCount renders a tally-mark specimen's count label.
func markCount(n int) string { return strconv.Itoa(n) }

// tallyMarkSVG renders n as tally strokes — the mark tally is named for — with
// every fifth stroke crossed by a diagonal slash. Returns an inline SVG string
// the template drops in via templ.Raw. n is expected small (specimen counts);
// callers pass 1..12 in the gallery.
func tallyMarkSVG(n int) string {
	if n <= 0 {
		return ""
	}
	const h, gap, groupGap = 15, 5, 8
	x := 0
	var lines strings.Builder
	groups := (n + 4) / 5
	for g := 0; g < groups; g++ {
		inGroup := n - g*5
		if inGroup > 5 {
			inGroup = 5
		}
		start := x
		strokes := inGroup
		if strokes > 4 {
			strokes = 4
		}
		for i := 0; i < strokes; i++ {
			fmt.Fprintf(&lines, `<line x1="%d" y1="1" x2="%d" y2="%d"/>`, x, x, h)
			x += gap
		}
		if inGroup == 5 {
			fmt.Fprintf(&lines, `<line x1="%d" y1="%d" x2="%d" y2="0"/>`, start-2, h+1, start+3*gap+2)
		}
		x += groupGap
	}
	width := x - groupGap
	if width < h {
		width = h
	}
	return fmt.Sprintf(
		`<svg viewBox="0 0 %d %d" width="%d" height="%d" aria-hidden="true">`+
			`<g stroke="currentColor" stroke-width="2" stroke-linecap="round">%s</g></svg>`,
		width, h+2, width, h+2, lines.String(),
	)
}
