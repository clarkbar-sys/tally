// SPDX-License-Identifier: GPL-2.0-or-later

package version

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	Version = "v0.0.0-test"
	got := String()

	if !strings.Contains(got, "v0.0.0-test") {
		t.Errorf("String() = %q, want it to contain the version", got)
	}
	if !strings.Contains(got, "go") {
		t.Errorf("String() = %q, want it to contain the Go runtime version", got)
	}
}

func TestStringDefault(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	Version = "dev"
	if got := String(); !strings.HasPrefix(got, "dev ") {
		t.Errorf("String() = %q, want it to start with %q", got, "dev ")
	}
}
