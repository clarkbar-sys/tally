// SPDX-License-Identifier: GPL-2.0-or-later

// Command tally serves the UI on the tailnet.
//
// It joins the tailnet as its own node via tsnet (Hostname "tally") and serves
// over that identity — so tally is reachable at tally.<tailnet>.ts.net with an
// access domain separate from the box it runs on (the day-one constraint from
// #3). The served page is the live app, persisting to a server-side SQLite store
// (internal/store) at -db / $TALLY_DB — shared across every device on the tailnet.
//
// With -export DIR it instead renders the UI to a self-contained static site in
// DIR and exits, without touching the tailnet — the build step behind the
// GitHub Pages preview.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
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

	"github.com/clarkbar-sys/tally/internal/store"
	"github.com/clarkbar-sys/tally/internal/version"
	"github.com/clarkbar-sys/tally/internal/web"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("tally: ")

	exportDir := flag.String("export", "", "render the UI to this directory as a static site and exit (no tailnet)")
	local := flag.Bool("local", false, "serve the app on a local address instead of the tailnet — for trying tally in a browser")
	addr := flag.String("addr", "127.0.0.1:8080", "address to bind in -local mode")
	dbPath := flag.String("db", "", "path to the SQLite database (default: $TALLY_DB, else /var/lib/tally/tally.db when served, ./tally.db in -local mode)")
	flag.Parse()

	if *exportDir != "" {
		if err := runExport(*exportDir); err != nil {
			log.Fatalf("export: %v", err)
		}
		return
	}
	if *local {
		if err := runLocal(*addr, dbFile(*dbPath, "tally.db")); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := runServe(dbFile(*dbPath, "/var/lib/tally/tally.db")); err != nil {
		log.Fatal(err)
	}
}

// dbFile resolves the database path: the -db flag wins, then $TALLY_DB, then the
// mode's default. The live build always persists to this file — that is the
// server-side store that replaces the old per-browser IndexedDB.
func dbFile(flagVal, def string) string {
	if flagVal != "" {
		return flagVal
	}
	return env("TALLY_DB", def)
}

// openStore opens (creating if needed) the SQLite store at path, making its
// parent directory first so a default like /var/lib/tally/tally.db works on a
// fresh box.
func openStore(path string) (*store.Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create db dir %s: %w", dir, err)
		}
	}
	return store.Open(context.Background(), path)
}

// runLocal serves the app over plain HTTP on addr, without the tailnet — the
// quickest way to open tally in a browser. It runs the live build, persisting to
// the SQLite store at dbPath (so a reload keeps your notches), and serves the
// static assets. Ctrl-C shuts it down.
func runLocal(addr, dbPath string) error {
	log.Printf("starting %s", version.String())

	st, err := openStore(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	srv := &http.Server{
		Addr:              addr,
		Handler:           web.Handler(st),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("serving http://%s (local mode) — data persists to %s", addr, dbPath)
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
func runServe(dbPath string) error {
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

	st, err := openStore(dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	log.Printf("persisting to %s", dbPath)

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
		Handler:           web.Handler(st),
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
