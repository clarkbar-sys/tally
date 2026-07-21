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

The root [`install.sh`](../install.sh) fetches the **prebuilt release binary**
and the systemd unit straight from GitHub and installs them — no Go toolchain,
no git clone required on the box (the same shape as the sibling
[`hush`](https://github.com/clarkbar-sys/hush) installer). It runs tally under a
dedicated, unprivileged `tally` system user.

```sh
curl -fsSL https://raw.githubusercontent.com/clarkbar-sys/tally/main/install.sh | sudo sh
```

It installs the binary to `/usr/local/bin/tally`, the unit to
`/etc/systemd/system/tally.service`, and an editable `/etc/tally/tally.env`
(written once, never clobbered). Safe to re-run any time — re-running upgrades
the binary to the latest release and reinstalls the unit.

On immutable-root distros (SteamOS, Fedora Silverblue/Kinoite) `/usr` is
read-only; the installer falls back to a writable bin dir automatically and
rewrites the unit's `ExecStart` to match. Force a location with
`TALLY_BIN_DIR=/some/dir`.

**Caveat while this repo is private:** anonymous fetches of a private GitHub
repo (both the raw `install.sh` and the release binary) return 404, so the
one-liner won't work as-is until the repo is public. Until then, build and
install from a checkout instead:

```sh
git clone git@github.com:clarkbar-sys/tally.git && cd tally
sudo sh deploy/install.sh
```

## Prerequisites

The one-line installer above needs only a Linux box with `curl` and `systemd`
— no checkout, no Go toolchain. The `deploy/install.sh` path (build from a
checkout) additionally needs:

1. A checkout of this repo on the box.
2. A Go toolchain on the box (to build from source), **or** a prebuilt `tally`
   binary at the repo root. Note: the tsnet dependency currently requires a
   recent Go toolchain (see `go.mod`).

Both paths need:

3. **A tagged Tailscale auth key.** In the admin console, create an auth key
   tagged `tag:tally`. Tagging is what makes tally a distinct identity in the
   tailnet — and what lets ACLs deny the family surface (see below).
4. **MagicDNS + HTTPS certificates enabled** in the tailnet, so tsnet can serve
   `https://`. Without it, run with `TALLY_HTTP_ONLY=1` (plain HTTP on the
   tailnet) as a fallback.

## Configuration

`install.sh` writes `/etc/tally/tally.env` on first install (commented out,
defaults matching the unit) and never touches it again — edit it to change
`TALLY_HOSTNAME`, `TALLY_STATE_DIR`, or set `TALLY_HTTP_ONLY=1`, then:

```sh
sudo systemctl restart tally
```

Editing `deploy/tally.service` directly instead doesn't stick: re-running
`install.sh` to upgrade always reinstalls the packaged unit, so any hand edit
there is silently lost on the next install/upgrade. The env file is the
persistent override point.

## Install from a checkout (build from source)

When you already have a checkout on the box — or the repo is still private, so
the one-liner can't fetch a release — use `deploy/install.sh` instead. It builds
`tally` from source (or uses a prebuilt `./tally` at the repo root) rather than
downloading a release binary; everything else matches the one-liner.

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
