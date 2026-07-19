#!/bin/sh
# tally bootstrap installer.
#
# Intended usage, once the repo is public:
#
#     curl -fsSL https://raw.githubusercontent.com/clarkbar-sys/tally/main/install.sh | sudo sh
#
# While the repo is private, an anonymous curl above will fail (private repos
# require authentication to fetch, and this script deliberately never embeds a
# token). Until then, either:
#   - clone the repo yourself and run `sudo sh deploy/install.sh` directly, or
#   - fetch this script with an authenticated request (e.g. a `curl` with a
#     GitHub token in the Authorization header, or `gh api`) and pipe that
#     into `sudo sh`.
#
# What this script does: if it is being run from inside an existing checkout
# (i.e. there's a sibling deploy/install.sh next to it), it just execs that
# script — the real installer. Otherwise it clones (or updates) the repo into
# TALLY_SRC_DIR and then runs deploy/install.sh from there. It is safe to
# re-run: re-running upgrades an existing checkout and reinstalls.
#
# Configuration (env vars, all optional):
#   TALLY_REPO_URL   git remote to clone         (default: git@github.com:clarkbar-sys/tally.git)
#   TALLY_REF        git ref to check out         (default: main)
#   TALLY_SRC_DIR    where to clone the checkout  (default: /opt/tally)
#
# See deploy/README.md for prerequisites (Tailscale auth key, etc.) — this
# script only gets the code onto the box and hands off to deploy/install.sh.
set -eu

TALLY_REPO_URL=${TALLY_REPO_URL:-git@github.com:clarkbar-sys/tally.git}
TALLY_REF=${TALLY_REF:-main}
TALLY_SRC_DIR=${TALLY_SRC_DIR:-/opt/tally}

if [ "$(id -u)" -ne 0 ]; then
	echo "use: curl -fsSL <url>/install.sh | sudo sh" >&2
	exit 1
fi

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)

# Running from inside an existing checkout (curl'd script saved locally, or
# executed straight from a clone) — just hand off to the real installer.
if [ -f "$SCRIPT_DIR/deploy/install.sh" ]; then
	echo "==> running from an existing checkout ($SCRIPT_DIR)"
	exec sh "$SCRIPT_DIR/deploy/install.sh"
fi

if ! command -v git >/dev/null 2>&1; then
	echo "git is required but not found on this system — install git and re-run" >&2
	exit 1
fi

if [ -d "$TALLY_SRC_DIR/.git" ]; then
	echo "==> updating existing checkout at $TALLY_SRC_DIR"
	git -C "$TALLY_SRC_DIR" fetch origin "$TALLY_REF"
	git -C "$TALLY_SRC_DIR" checkout -B "$TALLY_REF" FETCH_HEAD
	git -C "$TALLY_SRC_DIR" reset --hard FETCH_HEAD
else
	echo "==> cloning $TALLY_REPO_URL -> $TALLY_SRC_DIR"
	mkdir -p "$(dirname "$TALLY_SRC_DIR")"
	git clone --branch "$TALLY_REF" "$TALLY_REPO_URL" "$TALLY_SRC_DIR"
fi

echo "==> handing off to $TALLY_SRC_DIR/deploy/install.sh"
sh "$TALLY_SRC_DIR/deploy/install.sh"
