# 0001 — Language and stack

- **Status:** Accepted
- **Date:** 2026-07-18
- **Issue:** [#2](https://github.com/clarkbar-sys/tally/issues/2)
- **Supersedes:** —

## Context

Tally is a website served on the tailnet at `tally.taileafxyz.net`, single-user
for now. Its purpose is an annotation UI over financial data — tag transactions,
attach markdown notes, thread transactions into stories that span months.
"Aggregation is table stakes; the annotation layer is the product."

The pull toward a heavy JS frontend was the assumption that a rich annotation UI
needs a rich client. Walking the actual interactions weakens that:

- Tag apply/remove + autocomplete (#13), inline markdown notes (#14), list
  filter/search (#11), balances (#12) — all partial-update, server-round-trip
  shaped. HTMX covers these directly, with no meaningful client state.
- The only genuinely app-like interaction is stories (#15) — the "Trello-ish"
  drag-to-thread. Even that collapses to a button/checkbox ("attach transaction
  to story") without drag; "Trello-ish" is aspirational, not a spec. Worst case
  it is one localized island of vanilla JS / Alpine, not a reason to adopt an
  SPA build pipeline.

Meanwhile the app's real shape — solo, self-hosted, tailnet-only, deployed onto
the box and ideally folded into `jaassh-machine` (#16) — rewards operational
simplicity over client richness.

## Options considered

1. **TypeScript full-stack** — matches `clark-app` / `jaashboard` idioms; shared
   org tooling. Heaviest runtime, an SPA build pipeline, more deploy surface.
2. **Go + server-rendered (templ / HTMX)** — single static binary, trivial
   systemd deploy, matches `hush`. The annotation UI takes marginally more effort
   to make feel app-like in its one interactive corner (stories).
3. **Go API + TS frontend** — best-feeling UI, but two build pipelines to
   maintain for a one-person app. Worst cost/benefit here.

## Decision

**Go + `templ` + HTMX**, server-rendered, with `net/http` (chi if routing needs
it) and SQLite (see [0002](0002-datastore.md)). Drop to a small amount of
vanilla JS / Alpine only where an interaction genuinely needs it (stories).

## Consequences

- **Deploy is a single static binary + a systemd unit** — directly simplifies
  #16 (reproducible, foldable into `jaassh-machine`) and #17 (backups: fewer
  moving parts around irreplaceable annotation data).
- **One build pipeline.** CI (#5) is Go-native: `go vet`, `gofmt`/`gofumpt`,
  `go test`, plus `gitleaks` in the pre-push gate (#6).
- **Matches `hush`**, an existing, liked pattern in the org.
- **Accepted tradeoff:** if stories later wants to feel genuinely drag-native,
  that one corner is fiddlier than it would be in React. Contained bet, not a
  whole-app cost. Revisit with a superseding ADR only if that corner grows past
  a sprinkle of JS.
- Adding a second data-source provider later (#8) is unaffected by this choice —
  the adapter interface is plain Go regardless.
