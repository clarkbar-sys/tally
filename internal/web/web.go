// SPDX-License-Identifier: GPL-2.0-or-later

// Package web holds tally's HTTP handlers. During the scaffolding phase this is
// just a health endpoint and a proof-of-life page; the annotation UI arrives in
// the browse phase (#11 onward).
package web

import (
	"fmt"
	"net/http"

	"github.com/clarkbar-sys/tally/internal/version"
)

// Handler returns the tally HTTP handler.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/", index)
	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

func index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w,
		"<!doctype html>\n<meta charset=utf-8>\n<title>tally</title>\n"+
			"<h1>tally is alive</h1>\n<p>%s</p>\n",
		version.String(),
	)
}
