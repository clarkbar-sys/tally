#!/usr/bin/env sh
# tally installer — sets up tally as a systemd service that joins the tailnet
# as its own node (tsnet) and serves the UI at https://tally.<tailnet>.ts.net.
#
# Fetches the prebuilt release binary and the systemd unit straight from
# GitHub — no Go toolchain, no git clone required on the target box. Runs
# tally under a dedicated, unprivileged "tally" system user (never root).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/clarkbar-sys/tally/main/install.sh | sudo sh
#
# Installs the binary to /usr/local/bin, the unit to /etc/systemd/system, and
# an editable env file to /etc/tally/tally.env — never clobbered on re-run, so
# local edits survive upgrades. Safe to re-run: re-running upgrades the binary
# and reinstalls the unit.
#
# The Tailscale auth key is NOT fetched or embedded — it's a bearer secret you
# place at /etc/tally/ts-authkey yourself (see below and deploy/README.md).
# If it's missing the installer sets everything else up and tells you exactly
# what to run.
#
# On immutable-root distros (SteamOS, Fedora Silverblue/Kinoite) /usr is
# read-only, so /usr/local/bin can't be written. There the installer falls
# back to a writable directory automatically and rewrites the unit's
# ExecStart to match. Force a specific location with TALLY_BIN_DIR=/some/dir.
#
# Targets systemd + shadow-utils (useradd) distros: Debian, Ubuntu, Fedora,
# Arch, RHEL, openSUSE. Not Alpine (OpenRC) or NixOS, and not macOS (no
# systemd). To build and install from a checkout instead (Go toolchain, or a
# prebuilt ./tally binary at the repo root), use deploy/install.sh.

set -eu

REPO="clarkbar-sys/tally"
REF="main"
RAW_BASE="https://raw.githubusercontent.com/$REPO/$REF"
SERVICE_USER="${TALLY_USER:-tally}"
SERVICE_GROUP="${TALLY_GROUP:-tally}"
CONFIG_DIR="/etc/tally"
UNIT_DIR="/etc/systemd/system"
CRED="$CONFIG_DIR/ts-authkey"

require_root() {
  if [ "$(id -u)" != "0" ]; then
    echo "error: must run as root (sudo) — tally installs as a systemd service" >&2
    echo "  curl -fsSL https://raw.githubusercontent.com/$REPO/main/install.sh | sudo sh" >&2
    exit 1
  fi
}

os() {
  case "$(uname -s)" in
    Linux) echo linux ;;
    Darwin)
      echo "error: this installer sets up a systemd service, which macOS doesn't have." >&2
      echo "  Build from source and run it yourself instead:" >&2
      echo "    go install github.com/$REPO/cmd/tally@latest" >&2
      exit 1
      ;;
    *)
      echo "error: unsupported OS '$(uname -s)' — this installer is systemd-only (Linux)." >&2
      exit 1
      ;;
  esac
}

arch() {
  case "$(uname -m)" in
    x86_64 | amd64) echo amd64 ;;
    aarch64 | arm64) echo arm64 ;;
    *)
      echo "error: unsupported architecture '$(uname -m)' — build from source instead:" >&2
      echo "  go install github.com/$REPO/cmd/tally@latest" >&2
      exit 1
      ;;
  esac
}

# resolve_bin_dir picks where the binary lands and sets BIN_DIR. The usual
# /usr/local/bin is unwritable on immutable-root distros (SteamOS, Fedora
# Silverblue/Kinoite) where /usr is mounted read-only, so fall back to a
# writable path there. An explicit TALLY_BIN_DIR always wins. Must run after
# require_root, since the fallbacks live outside the user's home.
resolve_bin_dir() {
  if [ -n "${TALLY_BIN_DIR:-}" ]; then
    BIN_DIR="$TALLY_BIN_DIR"
    if ! mkdir -p "$BIN_DIR" 2>/dev/null || [ ! -w "$BIN_DIR" ]; then
      echo "error: TALLY_BIN_DIR='$BIN_DIR' is not writable" >&2
      exit 1
    fi
    return
  fi
  for cand in /usr/local/bin /opt/tally/bin /var/lib/tally/bin; do
    if mkdir -p "$cand" 2>/dev/null && [ -w "$cand" ]; then
      BIN_DIR="$cand"
      if [ "$cand" != /usr/local/bin ]; then
        echo "note: /usr/local/bin is read-only; installing the binary to $cand instead" >&2
        echo "  (set TALLY_BIN_DIR to pick a different location)" >&2
      fi
      return
    fi
  done
  echo "error: found no writable install dir (tried /usr/local/bin, /opt/tally/bin, /var/lib/tally/bin)" >&2
  echo "  set TALLY_BIN_DIR=/some/writable/dir and re-run" >&2
  exit 1
}

create_user() {
  if id "$SERVICE_USER" >/dev/null 2>&1; then
    return
  fi
  echo "creating system user '$SERVICE_USER'" >&2
  useradd --system --home-dir /var/lib/tally --shell /usr/sbin/nologin \
    --user-group "$SERVICE_USER"
}

# curl_network_error inspects a failed curl's exit code and, if it looks like
# a connectivity problem (DNS, connection refused, timeout, TLS) rather than a
# real HTTP response, prints a network-specific diagnosis and returns 0. It
# returns 1 for everything else (notably exit 22, curl's -f code for HTTP >=
# 400 — a genuine 404). Callers use it so a transient DNS/network blip is not
# reported as "no tagged release / unsupported platform": the two are
# indistinguishable without the exit code, and conflating them sends people
# down the build-from-source path when the release is fine and only their
# resolver hiccuped.
curl_network_error() {
  rc="$1"
  url="$2"
  host="$(printf '%s\n' "$url" | sed -e 's#^[a-z]*://##' -e 's#/.*##')"
  case "$rc" in
    6)
      echo "error: couldn't resolve $host — DNS/name resolution failed on this machine." >&2
      echo "  This is a local network problem, not a missing release. The download URL is valid:" >&2
      echo "    $url" >&2
      echo "  Check DNS (e.g. 'getent hosts $host', your resolv.conf / upstream resolver)," >&2
      echo "  then re-run the installer — no need to build from source." >&2
      return 0
      ;;
    7 | 28 | 35 | 56)
      echo "error: couldn't reach $host (curl exit $rc: connection/timeout/TLS) — a network" >&2
      echo "  problem on this machine, not a missing release. URL: $url" >&2
      echo "  Check this box's connectivity/proxy/firewall to $host, then re-run." >&2
      return 0
      ;;
  esac
  return 1
}

fetch_binary() {
  name="$1"
  url="https://github.com/$REPO/releases/latest/download/${name}_${OS_NAME}_${ARCH_NAME}.tar.gz"
  echo "downloading $name ($OS_NAME/$ARCH_NAME)..." >&2
  rc=0
  curl -fsSL "$url" -o "$TMP_DIR/$name.tar.gz" || rc=$?
  if [ "$rc" -ne 0 ]; then
    curl_network_error "$rc" "$url" && exit 1
    echo "error: no release binary found at $url" >&2
    echo "  (no tagged release yet, or unsupported platform — build from a checkout" >&2
    echo "   with deploy/install.sh, or: go install github.com/$REPO/cmd/$name@latest)" >&2
    exit 1
  fi
  tar -xzf "$TMP_DIR/$name.tar.gz" -C "$TMP_DIR" "$name"
  mkdir -p "$BIN_DIR"
  # Rename over the (possibly running) binary via a temp path — atomic, avoids
  # ETXTBSY when upgrading a live service.
  install -o root -g root -m 0755 "$TMP_DIR/$name" "$BIN_DIR/$name.new"
  mv "$BIN_DIR/$name.new" "$BIN_DIR/$name"
  echo "installed $BIN_DIR/$name" >&2
}

fetch_unit() {
  unit="$1"
  url="$RAW_BASE/deploy/$unit"
  echo "downloading $unit..." >&2
  rc=0
  curl -fsSL "$url" -o "$TMP_DIR/$unit" || rc=$?
  if [ "$rc" -ne 0 ]; then
    curl_network_error "$rc" "$url" && exit 1
    echo "error: couldn't fetch $url (curl exit $rc)" >&2
    exit 1
  fi
  # Point ExecStart at the resolved BIN_DIR so a non-default location — e.g.
  # the SteamOS read-only-/usr fallback — is reflected in the unit. A no-op
  # when BIN_DIR is /usr/local/bin.
  if [ "$BIN_DIR" != /usr/local/bin ]; then
    sed "s#/usr/local/bin#$BIN_DIR#g" "$TMP_DIR/$unit" >"$TMP_DIR/$unit.patched"
    mv "$TMP_DIR/$unit.patched" "$TMP_DIR/$unit"
  fi
  install -o root -g root -m 0644 "$TMP_DIR/$unit" "$UNIT_DIR/$unit"
  echo "installed $UNIT_DIR/$unit" >&2
}

# install_env_file writes the editable env template once and never clobbers it,
# so admin edits persist across re-runs/upgrades.
install_env_file() {
  dest="$CONFIG_DIR/tally.env"
  if [ -f "$dest" ]; then
    echo "kept existing $dest" >&2
    return
  fi
  cat >"$dest" <<-'EOF'
	# tally environment overrides — edit, then: systemctl restart tally
	# Never overwritten by install.sh; only written once, on first install.
	#TALLY_HOSTNAME=tally
	#TALLY_STATE_DIR=/var/lib/tally/tsnet
	# Uncomment if the tailnet has no MagicDNS HTTPS certs (serves plain HTTP
	# on :80 on the tailnet instead of HTTPS on :443):
	#TALLY_HTTP_ONLY=1
	EOF
  chown "root:$SERVICE_GROUP" "$dest"
  chmod 0640 "$dest"
  echo "wrote $dest" >&2
}

# start_or_wait_for_key restarts the service when the auth key is present, or
# installs everything and stops with instructions when it's missing — the key
# is a bearer secret this installer deliberately never fetches or embeds.
start_or_wait_for_key() {
  if [ ! -s "$CRED" ]; then
    cat >&2 <<EOF

  !! No Tailscale auth key at $CRED — not starting the service yet.

     Create a *tagged* auth key (tag:tally) in the Tailscale admin console, then:

       printf '%s' 'tskey-auth-XXXXXXXX' | sudo tee $CRED >/dev/null
       sudo chgrp $SERVICE_GROUP $CRED && sudo chmod 0640 $CRED
       sudo systemctl restart tally

     Everything else is installed. See deploy/README.md.
EOF
    exit 0
  fi

  echo "(re)starting service" >&2
  # NB: 'systemctl enable --now' does NOT restart an already-running unit;
  # restart explicitly so unit and binary changes actually take effect.
  systemctl restart tally

  echo "waiting for service to settle" >&2
  i=0
  while [ "$i" -lt 10 ]; do
    if systemctl is-active --quiet tally; then
      echo "    tally is active" >&2
      echo >&2
      echo "done. verify from another tailnet device:" >&2
      echo "    curl -fsS https://tally.taileafxyz.net/healthz    # expect: ok" >&2
      exit 0
    fi
    i=$((i + 1))
    sleep 1
  done
  echo "    tally did not become active — inspect: journalctl -u tally -e" >&2
  exit 1
}

command -v curl >/dev/null 2>&1 || {
  echo "error: curl is required but not found — install curl and re-run" >&2
  exit 1
}
command -v systemctl >/dev/null 2>&1 || {
  echo "error: systemctl not found — this installer is systemd-only (Linux)." >&2
  exit 1
}

require_root
resolve_bin_dir
OS_NAME="$(os)"
ARCH_NAME="$(arch)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

create_user

# The credential dir is root-owned but group-readable by the service user, so
# it can read the ts-authkey credential delivered via systemd LoadCredential.
install -d -o root -g "$SERVICE_GROUP" -m 0750 "$CONFIG_DIR"

fetch_binary tally
fetch_unit tally.service
install_env_file
systemctl daemon-reload
systemctl enable tally >/dev/null 2>&1 || true
start_or_wait_for_key
