# tally

[![CI](https://github.com/clarkbar-sys/tally/actions/workflows/ci.yml/badge.svg)](https://github.com/clarkbar-sys/tally/actions/workflows/ci.yml)
[![License: GPL-2.0-or-later](https://img.shields.io/badge/license-GPL--2.0--or--later-blue.svg)](LICENSE)

A generic container for notes, checklists, and reminders — closer to a Trello card than a ledger. Make a **notch** — a mark on your tally — then attach whatever's relevant: notes, checklist items, tags, due dates, **sub-notches**, and eventually financial transactions. A grocery list is a notch full of checklist items; "the kitchen renovation" is a notch full of notes, reminders, and sub-notches. (Resolving notches into a running *tally* is a later concept.)

A [tally stick](https://en.wikipedia.org/wiki/Tally_stick): notched wood, split so both parties hold half the record — annotation is the point, aggregation (of anything, finance included) is just one input to it.

**v1 is local-first**: a client-only app, no server, no auth, no sync yet. See [epic #1](https://github.com/clarkbar-sys/tally/issues/1) for the full pivot writeup and what's deferred to later (finance sync via SimpleFIN, auth).

## Install

To reach tally from your other devices, run it on your tailnet. A single
Go binary joins the tailnet as its own node via [tsnet](https://tailscale.com/kb/1244/tsnet)
and serves the app at `https://tally.<tailnet>.ts.net` — tailnet-only, its own
identity (see [ADR-0003](docs/adr/0003-tailnet-integration.md)). `install.sh`
fetches the prebuilt release binary and sets it up as a systemd service under a
dedicated, unprivileged `tally` user — no Go toolchain, no git clone on the box.
It **must run as root**:

```sh
curl -fsSL https://raw.githubusercontent.com/clarkbar-sys/tally/main/install.sh | sudo sh
```

You supply a *tagged* Tailscale auth key (`tag:tally`) as a secret; the
installer prints exactly what to do if it's missing. It's systemd-only (Linux).
See [`deploy/README.md`](deploy/README.md) for the full walkthrough, the
build-from-a-checkout path ([`deploy/install.sh`](deploy/install.sh)), and how
the tailnet ACL becomes tally's access boundary.

Just want to try it in a browser with no tailnet? Use `-local` (see
[Development](#development) below) — data stays in your browser.

### Staying up to date

The tailnet build shows its running version in the header and checks GitHub for
newer releases (an hourly, cached, unauthenticated call to the releases API). A
`dev` build never nags, and if the check fails the pill just shows the version
as before. When a newer release exists the pill lights up — click it for a popup
with the exact command to upgrade the box over SSH (re-run `install.sh`, which
fetches the latest release binary and restarts the service). This lives only on
the served build; the [browser demo](https://clarkbar-sys.github.io/tally/) has
no server to check and never offers an upgrade.

## Stack

Go + [templ](https://templ.guide/) render the static app shell; the app itself is vanilla JavaScript over the browser's [IndexedDB](https://developer.mozilla.org/en-US/docs/Web/API/IndexedDB_API) — client-only, no server. The Go binary also serves that shell on the tailnet via [tsnet](https://tailscale.com/kb/1244/tsnet) (see [Install](#install)). The earlier finance-ledger build (SQLite store, source-adapter interface, and its templ + HTMX server) is shelved in git history, not deleted — it returns once a real backend is justified. See [`docs/adr/`](docs/adr/) for the decisions behind it and why.

## Development

Requires Go 1.24+ and [gitleaks](https://github.com/gitleaks/gitleaks).

```sh
make hooks   # enable the pre-push gate (once per clone)
make ci      # gofmt, vet, build, test, gitleaks
```

Run the app locally — data stays in your browser via IndexedDB, nothing is
served but static files:

```sh
go run ./cmd/tally -local   # http://127.0.0.1:8080
```

See [CONTRIBUTING.md](CONTRIBUTING.md). Security posture: [`docs/security/`](docs/security/).

## License

[GPL-2.0-or-later](LICENSE).
