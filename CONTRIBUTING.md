# Contributing to tally

tally is a single-user, tailnet-only personal ledger. Decisions live in
[`docs/adr/`](docs/adr/); security posture in [`docs/security/`](docs/security/).
Read those first — they explain why the stack is Go + templ + HTMX, why the
datastore is SQLite, and why secret handling is the way it is.

## Setup

Requires **Go 1.24+** and [**gitleaks**](https://github.com/gitleaks/gitleaks).

```sh
make hooks      # point core.hooksPath at .githooks (the pre-push gate)
```

`make hooks` is per-clone — git does not enable committed hooks automatically.
Run it once after cloning.

## The pre-push gate

[`.githooks/pre-push`](.githooks/pre-push) runs and blocks the push on failure:

- `gofmt -l` — formatting
- `go vet ./...`
- `go test ./...`
- `gitleaks detect` — **not optional for this repo** ([#6](https://github.com/clarkbar-sys/tally/issues/6)):
  tally handles a bearer credential, so nothing that looks like one may leave
  your machine.

CI ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) runs the same checks
plus `-race` and a full-history gitleaks scan. Green on every PR is the merge
bar.

## Handy targets

```sh
make generate   # regenerate *_templ.go after editing any .templ file
make fmt        # gofmt -w .
make vet
make test
make build
make gitleaks
make ci         # everything CI runs, locally (incl. templ-generate freshness)
```

The UI is written in [templ](https://templ.guide/): edit `.templ` files, then
`make generate` to refresh the committed `*_templ.go`. CI fails if they are
stale, so commit the regenerated output alongside the template change.

## GitHub Pages (preview)

[`.github/workflows/pages.yml`](.github/workflows/pages.yml) builds the real
UI as a static site — `go run ./cmd/tally -export`, rendered from the demo
adapter (synthetic data, no credentials, no network) — and deploys it to the
`gh-pages` branch: production at the branch root on push to `main`, and a
preview under `pr-<number>/` for every same-repo pull request, with a sticky
comment linking to it. The preview is the actual server-rendered app, so it
can't drift from what ships.

Reproduce a preview locally with `go run ./cmd/tally -export ./_site` and open
`_site/index.html`.

**One-time repo setting**, required for this to work: **Settings → Pages →
Build and deployment → Source** must be **"Deploy from a branch"**, branch
`gh-pages`, folder `/ (root)` — not "GitHub Actions" (the branch doesn't
exist until the workflow's first run creates it, so this can't be set until
after that first push to `main`).

## Fixture policy — synthetic, always

No real API response is ever committed, even redacted. Any credential-shaped
value in tests or docs is an obvious placeholder. See
[`docs/security/secrets.md`](docs/security/secrets.md). The gitleaks gate
enforces this.

## Conventions

- Commits: short imperative subject, referencing the issue (e.g. `#12`).
- One ADR per irreversible decision; supersede rather than edit.
- Provider data and tally-owned annotation (tags, notes, stories) stay
  separated at the schema level ([#7](https://github.com/clarkbar-sys/tally/issues/7))
  so a re-sync never clobbers annotation.
