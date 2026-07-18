// SPDX-License-Identifier: GPL-2.0-or-later

// Package version exposes build and version information for tally.
package version

import "runtime"

// Version is the tally release version. Override it at build time with:
//
//	-ldflags "-X github.com/clarkbar-sys/tally/internal/version.Version=v1.2.3"
//
// It defaults to "dev" for local and untagged builds.
var Version = "dev"

// String returns a human-readable version string including the Go runtime.
func String() string {
	return Version + " (" + runtime.Version() + ")"
}
