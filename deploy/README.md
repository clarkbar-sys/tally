# Deploying tally

tally runs on the box as a single Go binary and joins the tailnet **as its own
node** (`tally`) via [tsnet](https://tailscale.com/kb/1244/tsnet) — reachable at
`https://tally.taileafxyz.net`, tailnet-only, never funnelled. See
[ADR-0003](../docs/adr/0003-tailnet-integration.md) for why tsnet over
`tailscale serve`.

This is the **proof-of-life** deploy: the service serves a health endpoint and
an "alive" page. It exists to prove the whole path — systemd, a separate tailnet
identity, HTTPS — works before the ledger is built on top of it.

## One-line install

The root [`install.sh`](../install.sh) is a bootstrap: it clones (or updates)
this repo onto the box and then runs `deploy/install.sh` for you.

```sh
curl -fsSL https://raw.githubusercontent.com/clarkbar-sys/tally/main/install.sh | sudo sh
```

**Caveat while this repo is private:** an anonymous `curl` of a private
GitHub repo returns 404, so the one-liner above won't work as-is — the fetch
of `install.sh` itself needs authentication. Until the repo is public, either:

- clone the repo yourself and run `sudo sh deploy/install.sh` directly (see
  below), or
- fetch `install.sh` with an authenticated request (e.g. a token in the
  `Authorization` header, or `gh api`) and pipe that into `sudo sh`.

It becomes a clean anonymous `curl | sudo sh` one-liner once the repo goes
public. It's safe to re-run any time to pull and install the latest `main`.

## Prerequisites

1. A checkout of this repo on the box.
2. A Go toolchain on the box (to build from source), **or** a prebuilt `tally`
   binary at the repo root. Note: the tsnet dependency currently requires a
   recent Go toolchain (see `go.mod`).
3. **A tagged Tailscale auth key.** In the admin console, create an auth key
   tagged `tag:tally`. Tagging is what makes tally a distinct identity in the
   tailnet — and what lets ACLs deny the family surface (see below).
4. **MagicDNS + HTTPS certificates enabled** in the tailnet, so tsnet can serve
   `https://`. Without it, run with `TALLY_HTTP_ONLY=1` (plain HTTP on the
   tailnet) as a fallback.

## Install

```sh
sudo sh deploy/install.sh
```

The script creates the `tally` service user, installs the binary to
`/usr/local/bin/tally` and the unit to `/etc/systemd/system/tally.service`,
`daemon-reload`s, and — once the auth key is in place — restarts the service. If
the key is missing it installs everything and stops, printing exactly what to
run. Provide the key and restart:

```sh
printf '%s' 'tskey-auth-XXXXXXXX' | sudo tee /etc/tally/ts-authkey >/dev/null
sudo chgrp tally /etc/tally/ts-authkey && sudo chmod 0640 /etc/tally/ts-authkey
sudo systemctl restart tally
```

## Verify

```sh
# from another tailnet device
curl -fsS https://tally.taileafxyz.net/healthz    # expect: ok

# on the box
systemctl status tally
journalctl -u tally -f
```

First start takes a moment while the node registers with the tailnet and fetches
its certificate.

## The auth key is a secret

`/etc/tally/ts-authkey` is a bearer credential (`root:tally`, `0640`), delivered
to the process via systemd `LoadCredential` — never in the environment or the
repo. It follows the same handling and rotation rules as the SimpleFIN URL; see
[`docs/security/secrets.md`](../docs/security/secrets.md). Prefer an **ephemeral,
reusable, tagged** key, or an OAuth client, so it can be rotated without touching
tally.

## Access domain (#3)

Because tally is its own tagged node, the tailnet ACL is the access boundary —
a *different authz system* from the family-shareable surface, which is exactly
what [#3](https://github.com/clarkbar-sys/tally/issues/3) requires. Grant only
your own devices to `tag:tally`, e.g.:

```jsonc
// tailnet policy (illustrative)
"grants": [
  { "src": ["you@example.com"], "dst": ["tag:tally"], "ip": ["443"] },
]
```

A device with a *family* invite is simply not in `src`, so it is denied at the
tailnet layer before a request ever reaches tally. This is the interim answer
until #3 is decided in full; tally does not yet enforce app-level auth.

## Folding into jaassh-machine (later)

Per [#16](https://github.com/clarkbar-sys/tally/issues/16), the reproducible
path is an Ansible role in `jaassh-machine` that invokes this same
`install.sh` (one source of truth for install logic) — not a hand-placed unit.

## Uninstall

```sh
sudo systemctl disable --now tally
sudo rm -f /usr/local/bin/tally /etc/systemd/system/tally.service
sudo systemctl daemon-reload
# state + secret, remove if you mean it:
sudo rm -rf /var/lib/tally /etc/tally
```
