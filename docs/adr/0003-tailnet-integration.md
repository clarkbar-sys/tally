# 0003 — Tailnet integration

- **Status:** Accepted
- **Date:** 2026-07-19
- **Issues:** [#16](https://github.com/clarkbar-sys/tally/issues/16) (deploy),
  bears on [#3](https://github.com/clarkbar-sys/tally/issues/3) (auth)
- **Supersedes:** —

## Context

tally must be reachable at `tally.taileafxyz.net`, tailnet-only, never
funnelled. It also must **never share an access domain** with the
family-shareable surface (the day-one constraint from #3).

`tailscale serve` on the box publishes under the *box's* node name and rides the
box's tailnet identity — so it gives neither the distinct `tally.` hostname nor a
separate identity. Getting both means tally must be its own tailnet node.

## Options considered

1. **Embed `tsnet`** — the Go binary joins the tailnet as its own node
   (`Hostname: "tally"`) and serves directly. Native hostname, own identity,
   own node state; no `tailscale serve` config on the box.
2. **`tailscale serve` on the box** — one command, but wrong hostname and shared
   identity.
3. **Separate tailscale node / container named `tally`** — achieves the goal but
   adds a whole node (or container) to run and maintain.

## Decision

**Embed `tsnet`.** tally appears as node `tally`, tagged `tag:tally`, and serves
HTTPS via the tailnet's MagicDNS certificate (`ListenTLS` on 443), with a plain
HTTP fallback (`TALLY_HTTP_ONLY`) for tailnets without HTTPS enabled.

## Consequences

- **Separate access domain, mostly for free (#3).** Because tally is a distinct
  tagged node, the **tailnet ACL** is the access boundary — a different authz
  system from the family surface. A family-invited device that isn't granted to
  `tag:tally` is denied at the tailnet layer, never reaching tally. This is the
  interim #3 answer; app-level auth is still deferred to #3 proper.
- **The Tailscale auth key is a bearer secret.** Delivered via systemd
  `LoadCredential`, never env or repo — same handling as the SimpleFIN URL
  (docs/security/secrets.md). Prefer an ephemeral, tagged key or an OAuth client.
- **Node state** (keys) lives under `/var/lib/tally` (systemd `StateDirectory`).
- **Dependency + toolchain cost.** Pulling `tailscale.com` adds a large
  transitive tree and raises the minimum Go version (see `go.mod`). Accepted as
  the inherent price of a self-contained tailnet node; the alternative
  (a separate node/container) trades code weight for operational weight.
- **Deploy** is `deploy/install.sh` + `deploy/tally.service`; no `tailscale
  serve` config to manage. Folds into a jaassh-machine Ansible role later (#16).
- **Revisit if** tsnet's footprint becomes a real problem, or a future need
  (e.g. running behind the box's existing node) argues for `serve` after all.
