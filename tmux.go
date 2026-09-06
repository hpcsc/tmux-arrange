package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

type pane struct {
	id      string
	index   int
	active  bool
	command string
	title   string
	path    string
}

type window struct {
	id      string
	session string
	index   int
	name    string
	active  bool
	panes   []pane
}

type session struct {
	id       string
	name     string
	attached bool
	windows  []window
}

type tmux struct{ socket string }

func (t tmux) run(args ...string) (string, error) {
	if t.socket != "" {
		args = append([]string{"-S", t.socket}, args...)
	}
	cmd := exec.Command(tmuxPath(), args...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return out.String(), nil
}

// A key binding runs with the tmux server's environment, whose PATH is the one
// tmux was started with and on macOS often has no Homebrew in it.
func tmuxPath() string {
	if path, err := exec.LookPath("tmux"); err == nil {
		return path
	}
	for _, path := range []string{"/opt/homebrew/bin/tmux", "/usr/local/bin/tmux", "/usr/bin/tmux"} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "tmux"
}

var treeFormat = strings.Join([]string{
	"#{session_id}",
	"#{session_name}",
	"#{?session_attached,1,0}",
	"#{window_id}",
	"#{window_index}",
	"#{window_name}",
	"#{?window_active,1,0}",
	"#{pane_id}",
	"#{pane_index}",
	"#{?pane_active,1,0}",
	"#{pane_current_command}",
	// A pane's name is its title, but only where tmux will hold on to it:
	// with allow-set-title on (1), whatever runs in the pane overwrites the
	// title at its next prompt, and the shells that do would fill the tree
	// with names nobody chose. An untouched title is the hostname.
	"#{?#{==:#{allow-set-title},0},#{?#{==:#{pane_title},#{host}},,#{pane_title}},}",
	"#{pane_current_path}",
}, "\t")

func (t tmux) tree() ([]session, error) {
	out, err := t.run("list-panes", "-a", "-F", treeFormat)
	if err != nil {
		return nil, err
	}
	return parseTree(out), nil
}

func parseTree(out string) []session {
	var sessions []session
	sessionAt := map[string]int{}
	windowAt := map[string]int{}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 13 {
			continue
		}
		s, seen := sessionAt[f[0]]
		if !seen {
			sessions = append(sessions, session{id: f[0], name: f[1], attached: f[2] == "1"})
			s = len(sessions) - 1
			sessionAt[f[0]] = s
		}
		// A window linked into two sessions arrives once per session, and its
		// place in one session's list says nothing about the other's.
		key := f[0] + "\x00" + f[3]
		w, seen := windowAt[key]
		if !seen {
			sessions[s].windows = append(sessions[s].windows, window{
				id:      f[3],
				session: f[0],
				index:   atoi(f[4]),
				name:    f[5],
				active:  f[6] == "1",
			})
			w = len(sessions[s].windows) - 1
			windowAt[key] = w
		}
		sessions[s].windows[w].panes = append(sessions[s].windows[w].panes, pane{
			id:      f[7],
			index:   atoi(f[8]),
			active:  f[9] == "1",
			command: f[10],
			title:   f[11],
			path:    f[12],
		})
	}
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].name < sessions[j].name })
	for s := range sessions {
		ws := sessions[s].windows
		sort.SliceStable(ws, func(i, j int) bool { return ws[i].index < ws[j].index })
		for w := range ws {
			ps := ws[w].panes
			sort.SliceStable(ps, func(i, j int) bool { return ps[i].index < ps[j].index })
		}
	}
	return sessions
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// option reads a user setting from tmux, e.g. @arrange-key, falling back when
// it is unset or empty.
func (t tmux) option(name, fallback string) string {
	out, err := t.run("show-option", "-gqv", name)
	if err != nil {
		return fallback
	}
	if value := strings.TrimSpace(out); value != "" {
		return value
	}
	return fallback
}

func (t tmux) sessionOf(client string) string {
	args := []string{"display-message", "-p"}
	if client != "" {
		args = append(args, "-c", client)
	}
	out, err := t.run(append(args, "#{session_id}")...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (t tmux) apply(commands [][]string) error {
	for _, c := range commands {
		if _, err := t.run(c...); err != nil {
			return err
		}
	}
	return nil
}

// namePane titles the pane and stops programs in it from titling it themselves,
// so the name stays until it is cleared, which hands the title back to them.
func (t tmux) namePane(id, name string) error {
	if _, err := t.run("select-pane", "-t", id, "-T", name); err != nil {
		return err
	}
	args := []string{"set-option", "-p", "-t", id, "allow-set-title", "off"}
	if name == "" {
		args = []string{"set-option", "-p", "-t", id, "-u", "allow-set-title"}
	}
	_, err := t.run(args...)
	return err
}

func (t tmux) switchTo(client string, r row) error {
	args := []string{"switch-client"}
	if client != "" {
		args = append(args, "-c", client)
	}
	if _, err := t.run(append(args, "-t", r.session.id)...); err != nil {
		return err
	}
	if r.window != nil {
		if _, err := t.run("select-window", "-t", r.window.id); err != nil {
			return err
		}
	}
	if r.pane != nil {
		if _, err := t.run("select-pane", "-t", r.pane.id); err != nil {
			return err
		}
	}
	return nil
}
