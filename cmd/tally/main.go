// SPDX-License-Identifier: GPL-2.0-or-later

// Command tally serves the personal-ledger service on the tailnet.
//
// It joins the tailnet as its own node via tsnet (Hostname "tally") and serves
// over that identity — so tally is reachable at tally.<tailnet>.ts.net with an
// access domain separate from the box it runs on (the day-one constraint from
// #3). During the scaffolding phase (#16) it serves only a health endpoint and
// a proof-of-life page: enough to prove the deploy path end to end before the
// ledger itself exists. Datastore, adapters, and the annotation UI land later.
package main

import (
	"context"
	"errors"
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
	log.Printf("starting %s", version.String())

	hostname := env("TALLY_HOSTNAME", "tally")
	stateDir := env("TALLY_STATE_DIR", "/var/lib/tally/tsnet")
	httpOnly := os.Getenv("TALLY_HTTP_ONLY") != ""

	key, err := authKey()
	if err != nil {
		log.Fatalf("auth key: %v", err)
	}

	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		log.Fatalf("state dir %s: %v", stateDir, err)
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
		log.Fatalf("listen: %v", err)
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	log.Print("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
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
