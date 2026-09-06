#!/usr/bin/env bash

# TPM entry point: bind the key, and make sure the binary the key needs exists.
#
# The binding goes through run-shell rather than display-popup because
# display-popup leaves #{client_name} unexpanded, and the popup has to be told
# which client to switch when you press Enter — a popup's own tty tells tmux
# nothing about that.

set -u

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

key="$(tmux show-option -gqv @arrange-key)"
[ -n "$key" ] || key=W

tmux bind-key -N "Arrange windows and panes across sessions in a popup" "$key" \
	run-shell "$DIR/scripts/arrange.sh '#{client_name}'"

# TPM runs this file with its output discarded, so the install reports into a
# log of its own. It runs in the background to keep tmux starting promptly.
"$DIR/scripts/install.sh" --if-needed >>"$DIR/install.log" 2>&1 &
