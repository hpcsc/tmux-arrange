#!/usr/bin/env bash

# Put the tmux-arrange binary in bin/: from this version's release asset when
# there is one for the platform, otherwise by building it from the checkout TPM
# already cloned. With --if-needed it returns at once if the binary is current.
#
# The version in bin/ is compared against the VERSION file rather than a
# timestamp, so pulling a new version of the plugin replaces the binary and a
# hand-built one (task build stamps the same version) is left alone.

set -uo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO="hpcsc/tmux-arrange"
BIN="$DIR/bin/tmux-arrange"
LOCK="$DIR/.install.lock"
VERSION="$(cat "$DIR/VERSION" 2>/dev/null || echo dev)"

current() {
	[ -x "$BIN" ] && [ "$("$BIN" --version 2>/dev/null)" = "$VERSION" ]
}

if [ "${1:-}" = "--if-needed" ] && current; then
	exit 0
fi

# tmux start and the key binding can both ask at once; whoever loses the lock
# waits for the binary rather than fetching a second copy over the first.
if ! mkdir "$LOCK" 2>/dev/null; then
	for _ in $(seq 1 60); do
		current && exit 0
		sleep 1
	done
	echo "another install holds $LOCK and did not finish; remove it if it is stale" >&2
	exit 1
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"; rmdir "$LOCK" 2>/dev/null' EXIT

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) arch="$(uname -m)" ;;
esac
asset="tmux-arrange_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/v$VERSION"

fetch() {
	if command -v curl > /dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget > /dev/null 2>&1; then
		wget -qO "$2" "$1"
	else
		return 1
	fi
}

verify() {
	local sums
	sums="$(grep " $asset\$" "$TMP/checksums.txt")" || return 1
	if command -v sha256sum > /dev/null 2>&1; then
		printf '%s\n' "$sums" | (cd "$TMP" && sha256sum -c - > /dev/null)
	elif command -v shasum > /dev/null 2>&1; then
		printf '%s\n' "$sums" | (cd "$TMP" && shasum -a 256 -c - > /dev/null)
	else
		echo "no sha256 tool to check the download with" >&2
		return 1
	fi
}

from_release() {
	fetch "$base/$asset" "$TMP/$asset" || return 1
	fetch "$base/checksums.txt" "$TMP/checksums.txt" || return 1
	verify || return 1
	tar -xzf "$TMP/$asset" -C "$TMP" tmux-arrange || return 1
	mkdir -p "$DIR/bin" || return 1
	mv "$TMP/tmux-arrange" "$BIN"
}

from_source() {
	command -v go > /dev/null 2>&1 || return 1
	(cd "$DIR" && go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$BIN" .)
}

if from_release; then
	echo "$(date -u '+%Y-%m-%dT%H:%M:%SZ') installed tmux-arrange $VERSION for $os/$arch from its release"
elif from_source; then
	echo "$(date -u '+%Y-%m-%dT%H:%M:%SZ') built tmux-arrange $VERSION from source"
else
	echo "$(date -u '+%Y-%m-%dT%H:%M:%SZ') could not install tmux-arrange $VERSION: no release asset for $os/$arch, and no go toolchain to build it with" >&2
	exit 1
fi
