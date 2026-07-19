# tally

[![CI](https://github.com/clarkbar-sys/tally/actions/workflows/ci.yml/badge.svg)](https://github.com/clarkbar-sys/tally/actions/workflows/ci.yml)
[![License: GPL-2.0-or-later](https://img.shields.io/badge/license-GPL--2.0--or--later-blue.svg)](LICENSE)

A generic container for notes, checklists, and reminders — closer to a Trello card than a ledger. Make a **tally**, then attach whatever's relevant to it: notes, checklist items, due dates, and eventually financial transactions. A grocery list is a tally full of checklist items; "the kitchen renovation" is a tally full of notes, reminders, and eventually the transactions that belong to it.

A [tally stick](https://en.wikipedia.org/wiki/Tally_stick): notched wood, split so both parties hold half the record — annotation is the point, aggregation (of anything, finance included) is just one input to it.

**v1 is local-first**: a client-only app, no server, no auth, no sync yet. See [epic #1](https://github.com/clarkbar-sys/tally/issues/1) for the full pivot writeup and what's deferred to later (finance sync via SimpleFIN, tailnet deploy, auth).

## Stack

Go + [templ](https://templ.guide/) + HTMX render the app shell today. The earlier finance-ledger build (SQLite store, source-adapter interface, tsnet tailnet deploy) is shelved in git history, not deleted — it returns once a real backend is justified. See [`docs/adr/`](docs/adr/) for the decisions behind it and why.

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
