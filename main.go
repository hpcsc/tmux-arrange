// tmux-arrange rearranges tmux sessions, windows and panes from a tree in a
// popup. Cut a window or a pane with x, put the cursor where it belongs and
// paste it with p; J and K move it within its session; M merges the session
// under the cursor into the one the popup was opened from; S sends what is cut
// to a session of its own. r renames what the cursor is on and d closes it, once
// y confirms. L draws the panes of a window as boxes to point at, where they can
// be pushed around and resized. Enter goes to the row under the cursor.
//
//	tmux-arrange --popup <client>   open the tree in a popup over that client
//	tmux-arrange [client]           the tree itself, in the terminal it is run in
//	tmux-arrange --version          the version this binary was built at
//
// The key binding needs both halves: display-popup leaves #{client_name}
// unexpanded, so the binding runs --popup through run-shell, which expands it,
// and the popup passes the name back in. Enter switches that client, rather
// than whichever one tmux would call current for a popup's own tty.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// version is stamped at build time; a binary built by hand says so.
var version = "dev"

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--version" {
		fmt.Println(version)
		return
	}
	if len(args) > 0 && args[0] == "--popup" {
		if err := popup(first(args[1:])); err != nil {
			fail(err)
		}
		return
	}
	m, err := newModel(tmux{}, first(args))
	if err != nil {
		fail(err)
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fail(err)
	}
}

func first(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func popup(client string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	t := tmux{}
	args := []string{"display-popup", "-E",
		"-w", t.option("@arrange-width", "80%"),
		"-h", t.option("@arrange-height", "80%"),
		"-T", t.option("@arrange-title", " arrange "),
	}
	if client != "" {
		args = append(args, "-c", client)
	}
	_, err = t.run(append(args, quote(self)+" "+quote(client))...)
	return err
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "tmux-arrange:", err)
	os.Exit(1)
}
