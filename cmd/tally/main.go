// SPDX-License-Identifier: GPL-2.0-or-later

// Command tally serves the UI on the tailnet.
//
// It joins the tailnet as its own node via tsnet (Hostname "tally") and serves
// over that identity — so tally is reachable at tally.<tailnet>.ts.net with an
// access domain separate from the box it runs on (the day-one constraint from
// #3). The served page is currently the hello-world design shell (theme +
// widget gallery); the data layer is being reworked.
//
// With -export DIR it instead renders the UI to a self-contained static site in
// DIR and exits, without touching the tailnet — the build step behind the
// GitHub Pages preview.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"tailscale.com/tsnet"

	"github.com/clarkbar-sys/tally/internal/version"
	"github.com/clarkbar-sys/tally/internal/web"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("tally: ")

	exportDir := flag.String("export", "", "render the UI to this directory as a static site and exit (no tailnet)")
	local := flag.Bool("local", false, "serve the app on a local address instead of the tailnet — for trying tally in a browser")
	addr := flag.String("addr", "127.0.0.1:8080", "address to bind in -local mode")
	flag.Parse()

	if *exportDir != "" {
		if err := runExport(*exportDir); err != nil {
			log.Fatalf("export: %v", err)
		}
		return
	}
	if *local {
		if err := runLocal(*addr); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := runServe(); err != nil {
		log.Fatal(err)
	}
}

// runLocal serves the app shell over plain HTTP on addr, without the tailnet —
// the quickest way to open tally in a browser. The app is local-first: all data
// lives in the browser's IndexedDB, so this only serves static files. Ctrl-C
// shuts it down.
func runLocal(addr string) error {
	log.Printf("starting %s", version.String())
	srv := &http.Server{
		Addr:              addr,
		Handler:           web.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("serving http://%s (local mode) — data stays in your browser", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	sctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-sctx.Done()

	log.Print("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// runExport renders the UI to dir as a static site, then exits.
func runExport(dir string) error {
	if err := web.Export(context.Background(), dir); err != nil {
		return err
	}
	log.Printf("exported static site to %s", dir)
	return nil
}

// runServe serves the UI on the tailnet.
func runServe() error {
	log.Printf("starting %s", version.String())

	hostname := env("TALLY_HOSTNAME", "tally")
	stateDir := env("TALLY_STATE_DIR", "/var/lib/tally/tsnet")
	httpOnly := os.Getenv("TALLY_HTTP_ONLY") != ""

	key, err := authKey()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}

	ts := &tsnet.Server{
		Hostname: hostname,
		Dir:      stateDir,
		AuthKey:  key,
		Logf:     func(format string, args ...any) { log.Printf("tsnet: "+format, args...) },
	}
	defer ts.Close()

	ln, scheme, err := listen(ts, httpOnly)
	if err != nil {
		return err
	}
	defer ln.Close()
	log.Printf("serving %s on the tailnet as %q", scheme, hostname)

	srv := &http.Server{
		Handler:           web.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	sctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-sctx.Done()

	log.Print("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	return nil
}

// listen binds the tailnet listener: HTTPS on 443 by default (uses the tailnet's
// MagicDNS certificate), or plain HTTP on 80 when TALLY_HTTP_ONLY is set — a
// fallback for tailnets without HTTPS certificates enabled.
func listen(ts *tsnet.Server, httpOnly bool) (net.Listener, string, error) {
	if httpOnly {
		ln, err := ts.Listen("tcp", ":80")
		return ln, "http", err
	}
	ln, err := ts.ListenTLS("tcp", ":443")
	return ln, "https", err
}

// authKey resolves the Tailscale auth key from TS_AUTHKEY, or from the
// systemd-provided credential file (see deploy/tally.service). It is never read
// from a committed file or a flag.
func authKey() (string, error) {
	if k := os.Getenv("TS_AUTHKEY"); k != "" {
		return k, nil
	}
	if dir := os.Getenv("CREDENTIALS_DIRECTORY"); dir != "" {
		b, err := os.ReadFile(filepath.Join(dir, "ts-authkey"))
		if err == nil {
			return strings.TrimSpace(string(b)), nil
		}
	}
	return "", errors.New("set TS_AUTHKEY or provide the ts-authkey systemd credential (see deploy/README.md)")
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
