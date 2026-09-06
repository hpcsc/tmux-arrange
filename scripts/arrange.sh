#!/usr/bin/env bash

# The key binding runs this with the client name, and it opens the popup. The
# binary is normally in place already; it is installed here when the background
# install at tmux start has not finished or has failed.

set -uo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$DIR/bin/tmux-arrange"

if [ ! -x "$BIN" ]; then
	tmux display-message "tmux-arrange: installing…"
	if ! "$DIR/scripts/install.sh" >>"$DIR/install.log" 2>&1; then
		tmux display-message "tmux-arrange: install failed — see $DIR/install.log"
		exit 0
	fi
fi

exec "$BIN" --popup "${1:-}"
