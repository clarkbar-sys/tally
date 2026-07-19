#!/bin/sh
# tally installer. Run on the box:
#
#     sudo sh install.sh
#
# Installs the tally binary, a dedicated service user, and the systemd unit,
# then (re)starts the tailnet service. Idempotent — safe to re-run to upgrade.
#
# Prerequisites (see deploy/README.md):
#   - Run from a checkout of this repo on the box.
#   - A Go toolchain to build from source, OR a prebuilt ./tally binary at the
#     repo root.
#   - A *tagged* Tailscale auth key (tag:tally) at /etc/tally/ts-authkey.
#   - MagicDNS + HTTPS certificates enabled in the tailnet (for https://).
set -eu

SERVICE_USER=tally
BIN=/usr/local/bin/tally
UNIT=/etc/systemd/system/tally.service
CRED=/etc/tally/ts-authkey

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)

if [ "$(id -u)" -ne 0 ]; then
	echo "must run as root (try: sudo sh install.sh)" >&2
	exit 1
fi

echo "==> service user: $SERVICE_USER"
if ! id "$SERVICE_USER" >/dev/null 2>&1; then
	useradd --system --home-dir /var/lib/tally --shell /usr/sbin/nologin "$SERVICE_USER"
fi

echo "==> installing binary -> $BIN"
NEWBIN="$BIN.new"
if [ -x "$REPO_ROOT/tally" ]; then
	echo "    using prebuilt $REPO_ROOT/tally"
	install -m 0755 "$REPO_ROOT/tally" "$NEWBIN"
elif command -v go >/dev/null 2>&1; then
	echo "    building from source"
	( cd "$REPO_ROOT" && go build -o "$NEWBIN" ./cmd/tally )
	chmod 0755 "$NEWBIN"
else
	echo "no prebuilt ./tally binary and no Go toolchain to build one" >&2
	exit 1
fi
# Rename over the (possibly running) binary — atomic, avoids ETXTBSY.
mv "$NEWBIN" "$BIN"

echo "==> credential dir: /etc/tally"
mkdir -p /etc/tally
chgrp "$SERVICE_USER" /etc/tally
chmod 0750 /etc/tally

echo "==> installing systemd unit -> $UNIT"
install -m 0644 "$SCRIPT_DIR/tally.service" "$UNIT"
systemctl daemon-reload
systemctl enable tally >/dev/null 2>&1 || true

if [ ! -s "$CRED" ]; then
	cat >&2 <<EOF

  !! No Tailscale auth key at $CRED — not starting the service yet.

     Create a *tagged* auth key (tag:tally) in the Tailscale admin console, then:

       printf '%s' 'tskey-auth-XXXXXXXX' | sudo tee $CRED >/dev/null
       sudo chgrp $SERVICE_USER $CRED && sudo chmod 0640 $CRED
       sudo systemctl restart tally

     Everything else is installed. See deploy/README.md.
EOF
	exit 0
fi

echo "==> (re)starting service"
# NB: 'systemctl enable --now' does NOT restart an already-running unit; restart
# explicitly so unit and binary changes actually take effect (trap from #16).
systemctl restart tally

echo "==> waiting for service to settle"
i=0
while [ "$i" -lt 10 ]; do
	if systemctl is-active --quiet tally; then
		echo "    tally is active"
		echo
		echo "done. verify from another tailnet device:"
		echo "    curl -fsS https://tally.taileafxyz.net/healthz    # expect: ok"
		exit 0
	fi
	i=$((i + 1))
	sleep 1
done

echo "    tally did not become active — inspect: journalctl -u tally -e" >&2
exit 1
