// SPDX-License-Identifier: GPL-2.0-or-later

// Command tally is the entrypoint for the tally personal-ledger service.
//
// This is a scaffolding stub: it prints version information and exits. The HTTP
// service, SQLite datastore, source adapters, and annotation UI arrive in later
// issues (#7 onward). See docs/adr for the stack and datastore decisions.
package main

import (
	"fmt"

	"github.com/clarkbar-sys/tally/internal/version"
)

func main() {
	fmt.Printf("tally %s\n", version.String())
}
