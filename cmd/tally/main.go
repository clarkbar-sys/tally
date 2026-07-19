// SPDX-License-Identifier: GPL-2.0-or-later

// Command tally serves the personal-ledger service on the tailnet.
//
// It joins the tailnet as its own node via tsnet (Hostname "tally") and serves
// over that identity — so tally is reachable at tally.<tailnet>.ts.net with an
// access domain separate from the box it runs on (the day-one constraint from
// #3). On startup it opens the SQLite store, syncs the configured source
// adapter into it, and serves the browse UI (#11, #12) rendered from that data.
//
// With -export DIR it instead renders the UI to a self-contained static site in
// DIR and exits, without touching the tailnet — the build step behind the
// GitHub Pages preview, which publishes the real UI driven by the demo adapter.
//
// The provider defaults to "demo" (synthetic data, no credentials) so the app
// is useful before the SimpleFIN adapter (#9) exists; set TALLY_PROVIDER to
// switch once other adapters are registered.
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

	"github.com/clarkbar-sys/tally/internal/ingest"
	"github.com/clarkbar-sys/tally/internal/source"
	_ "github.com/clarkbar-sys/tally/internal/source/demo" // registers the "demo" provider
	"github.com/clarkbar-sys/tally/internal/store"
	"github.com/clarkbar-sys/tally/internal/version"
	"github.com/clarkbar-sys/tally/internal/web"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("tally: ")

	exportDir := flag.String("export", "", "render the UI to this directory as a static site and exit (no tailnet)")
	flag.Parse()

	if *exportDir != "" {
		if err := runExport(*exportDir); err != nil {
			log.Fatalf("export: %v", err)
		}
		return
	}
	if err := runServe(); err != nil {
		log.Fatal(err)
	}
}

// openAndSync opens the store at dbPath and performs an initial sync of the
// configured provider into it, returning the ready store.
func openAndSync(ctx context.Context, dbPath string) (*store.Store, error) {
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return nil, err
	}

	provider := env("TALLY_PROVIDER", "demo")
	adapter, err := source.Open(ctx, provider)
	if err != nil {
		st.Close()
		return nil, err
	}
	res, err := ingest.Apply(ctx, st, adapter, time.Time{})
	if err != nil {
		st.Close()
		return nil, err
	}
	log.Printf("synced %q: %d accounts, %d transactions", provider, res.Accounts, res.Transactions)
	return st, nil
}

// runExport renders the UI to dir as a static site, then exits. It uses a
// throwaway database so the export is reproducible and leaves no state behind.
func runExport(dir string) error {
	ctx := context.Background()

	tmp, err := os.MkdirTemp("", "tally-export-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	st, err := openAndSync(ctx, filepath.Join(tmp, "export.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	if err := web.Export(ctx, st, dir); err != nil {
		return err
	}
	log.Printf("exported static site to %s", dir)
	return nil
}

// runServe opens the store, syncs, and serves the UI on the tailnet.
func runServe() error {
	log.Printf("starting %s", version.String())
	ctx := context.Background()

	st, err := openAndSync(ctx, env("TALLY_DB", "/var/lib/tally/tally.db"))
	if err != nil {
		return err
	}
	defer st.Close()

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
