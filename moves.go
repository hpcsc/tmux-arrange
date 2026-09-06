package main

import (
	"fmt"
	"strings"
)

type rowKind int

const (
	sessionRow rowKind = iota
	windowRow
	paneRow
)

type row struct {
	kind    rowKind
	session *session
	window  *window
	pane    *pane
	marked  bool
	cut     bool
}

func (r row) id() string {
	switch r.kind {
	case windowRow:
		return r.window.id
	case paneRow:
		return r.pane.id
	}
	return r.session.id
}

func (r row) label() string {
	switch r.kind {
	case windowRow:
		return r.window.name
	case paneRow:
		if r.pane.title != "" {
			return r.pane.title
		}
		return r.pane.command
	}
	return r.session.name
}

type item struct {
	kind  rowKind
	id    string
	label string
}

// endOf names the free index past a session's last window, where tmux puts a
// window moved to a session rather than to a window of it. Sessions are named
// by id, so a rename between reading the tree and moving cannot misdirect it.
func endOf(sessionID string) string { return sessionID + ":" }

// pasteCommands moves every clipboard item next to the target row: after it, or
// before it when before is set. A pane landing on a session becomes a window of
// its own; a window landing on a pane lands next to that pane's window.
func pasteCommands(clip []item, dst row, before bool) [][]string {
	windowTarget := ""
	if dst.window != nil {
		windowTarget = dst.window.id
	}
	ordered := clip
	if !before && dst.kind != sessionRow {
		ordered = make([]item, len(clip))
		for i, it := range clip {
			ordered[len(clip)-1-i] = it
		}
	}
	var cmds [][]string
	for _, it := range ordered {
		if it.id == dst.id() || (it.kind == windowRow && it.id == windowTarget) {
			continue
		}
		cmds = append(cmds, pasteOne(it, dst, before))
	}
	return cmds
}

func pasteOne(it item, dst row, before bool) []string {
	if dst.kind == sessionRow {
		if it.kind == paneRow {
			return []string{"break-pane", "-d", "-s", it.id, "-t", endOf(dst.session.id)}
		}
		return []string{"move-window", "-d", "-s", it.id, "-t", endOf(dst.session.id)}
	}
	side := "-a"
	if before {
		side = "-b"
	}
	if it.kind == windowRow {
		return []string{"move-window", "-d", side, "-s", it.id, "-t", dst.window.id}
	}
	paneTarget := dst.window.id
	if dst.kind == paneRow {
		paneTarget = dst.pane.id
	}
	cmd := []string{"join-pane", "-d"}
	if before {
		cmd = append(cmd, "-b")
	}
	return append(cmd, "-s", it.id, "-t", paneTarget)
}

// killCommands closes every item, each by the id of the thing itself: a session
// goes with its windows, a window with its panes.
func killCommands(items []item) [][]string {
	var cmds [][]string
	for _, it := range items {
		verb := "kill-window"
		switch it.kind {
		case sessionRow:
			verb = "kill-session"
		case paneRow:
			verb = "kill-pane"
		}
		cmds = append(cmds, []string{verb, "-t", it.id})
	}
	return cmds
}

// appendCommands puts every item at the end of the session, a window as a
// window and a pane as a window of its own.
func appendCommands(clip []item, dst string) [][]string {
	var cmds [][]string
	for _, it := range clip {
		verb := "move-window"
		if it.kind == paneRow {
			verb = "break-pane"
		}
		cmds = append(cmds, []string{verb, "-d", "-s", it.id, "-t", endOf(dst)})
	}
	return cmds
}

// newSession moves the clipboard into a session of its own. The window tmux
// creates the session with is killed once the clipboard is in, so the session
// holds only what was pasted; a session with no windows at all cannot exist,
// which is why it goes last.
func (t tmux) newSession(name string, clip []item) error {
	out, err := t.run("new-session", "-d", "-s", name, "-P", "-F", "#{session_id}\t#{window_id}")
	if err != nil {
		return err
	}
	made := strings.Split(strings.TrimSpace(out), "\t")
	if len(made) != 2 {
		return fmt.Errorf("tmux did not name the new session: %q", out)
	}
	if err := t.apply(appendCommands(clip, made[0])); err != nil {
		return err
	}
	_, err = t.run("kill-window", "-t", made[1])
	return err
}
