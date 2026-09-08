package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

func lastLine(view string) string {
	lines := strings.Split(view, "\n")
	return lines[len(lines)-1]
}

func key(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
}

func press(m *model, keys ...string) {
	for _, k := range keys {
		m.Update(key(k))
	}
}

func typeIn(m *model, text string) {
	for _, r := range text {
		press(m, string(r))
	}
}

// openOn opens the tree as if the popup had been opened from the named session.
func openOn(t *testing.T, tm tmux, here string) *model {
	t.Helper()
	m, err := newModel(tm, "")
	require.NoError(t, err)
	m.here = sessionID(t, tm, here)
	m.width, m.height = 100, 40
	return m
}

func at(t *testing.T, m *model, id string) {
	t.Helper()
	m.focus(id)
	require.Equal(t, id, m.current().id(), "cursor did not reach %s", id)
}

func atSession(t *testing.T, m *model, tm tmux, name string) {
	t.Helper()
	at(t, m, sessionID(t, tm, name))
}

func paneIDs(t *testing.T, tm tmux, session, name string) []string {
	t.Helper()
	out, err := tm.run("list-panes", "-t", "="+session+":"+name, "-F", "#{pane_id}")
	require.NoError(t, err)
	return lines(out)
}

func paneTitle(t *testing.T, tm tmux, id string) string {
	t.Helper()
	out, err := tm.run("display-message", "-p", "-t", id, "#{pane_title}")
	require.NoError(t, err)
	return strings.TrimSpace(out)
}

// paneHolds says whether tmux is keeping the pane's title against whatever runs
// in it, which is what makes a name a name.
func paneHolds(t *testing.T, tm tmux, id string) bool {
	t.Helper()
	out, err := tm.run("show-options", "-p", "-t", id, "allow-set-title")
	require.NoError(t, err)
	return strings.TrimSpace(out) == "allow-set-title off"
}

// openPanes unfolds the window and puts the cursor on the pane at that index.
func openPanes(t *testing.T, m *model, tm tmux, session, window string, pane int) string {
	t.Helper()
	at(t, m, windowID(t, tm, session, window))
	press(m, "l")
	id := paneIDs(t, tm, session, window)[pane]
	at(t, m, id)
	return id
}

func TestModel(t *testing.T) {
	t.Run("cut and paste", func(t *testing.T) {
		t.Run("puts a window after the one under the cursor", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run", "logs"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "logs"))
			press(m, "x")
			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "p")

			require.Equal(t, []string{"work:0 edit", "work:1 logs", "work:2 run"}, windows(t, tm))
		})

		t.Run("puts a window before the one under the cursor", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run", "logs"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "logs"))
			press(m, "x")
			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "P")

			require.Equal(t, []string{"work:0 logs", "work:1 edit", "work:2 run"}, windows(t, tm))
		})

		t.Run("moves a window to another session by pasting onto its name", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"}, []string{"notes", "read"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "x")
			atSession(t, m, tm, "notes")
			press(m, "p")

			require.Equal(t, []string{"notes:0 read", "notes:1 run", "work:0 edit"}, windows(t, tm))
		})

		t.Run("keeps the cut order when several windows are pasted", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "one", "two"}, []string{"notes", "read"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "one"))
			press(m, " ", " ")
			press(m, "x")
			at(t, m, windowID(t, tm, "notes", "read"))
			press(m, "p")

			require.Equal(t, []string{"notes:0 read", "notes:1 one", "notes:2 two", "work:0 edit"}, windows(t, tm))
		})

		t.Run("a marked session is cut with all of its windows", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read", "write"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "notes")
			press(m, " ", "x")
			atSession(t, m, tm, "work")
			press(m, "p")

			require.Equal(t, []string{"work:0 edit", "work:1 read", "work:2 write"}, windows(t, tm))
		})

		t.Run("a window marked inside a marked session moves only once", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read", "write"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "notes")
			press(m, " ")
			at(t, m, windowID(t, tm, "notes", "read"))
			press(m, " ", "x")
			atSession(t, m, tm, "work")
			press(m, "p")

			require.Equal(t, []string{"work:0 edit", "work:1 read", "work:2 write"}, windows(t, tm))
		})

		t.Run("a whole session cut with x lands where it is pasted", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read", "write"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "notes")
			press(m, "x")
			atSession(t, m, tm, "work")
			press(m, "p")

			require.Equal(t, []string{"work:0 edit", "work:1 read", "work:2 write"}, windows(t, tm))
		})

		t.Run("a pane pasted onto a window joins it as a split", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			second := paneIDs(t, tm, "work", "edit")[1]

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "l")
			at(t, m, second)
			press(m, "x")
			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "p")

			require.Len(t, paneIDs(t, tm, "work", "edit"), 1)
			require.Equal(t, []string{paneIDs(t, tm, "work", "run")[1]}, []string{second})
		})

		t.Run("a pane pasted onto a session name becomes a window of its own", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			second := paneIDs(t, tm, "work", "edit")[1]

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "l")
			at(t, m, second)
			press(m, "x")
			atSession(t, m, tm, "notes")
			press(m, "p")

			require.Len(t, windows(t, tm), 3)
			require.Contains(t, panes(t, tm), "notes:1.0 "+second)
		})

		t.Run("a pasted pane shows in the window it joined", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			second := paneIDs(t, tm, "work", "edit")[1]
			run := windowID(t, tm, "work", "run")

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "l")
			at(t, m, second)
			press(m, "x")
			at(t, m, run)
			press(m, "p")

			require.True(t, m.expanded[run])
			at(t, m, second)
		})

		t.Run("pasting a window leaves the folds alone", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run", "logs"})
			m := openOn(t, tm, "work")
			run := windowID(t, tm, "work", "run")

			at(t, m, windowID(t, tm, "work", "logs"))
			press(m, "x")
			at(t, m, run)
			press(m, "p")

			require.Empty(t, m.expanded)
		})

		t.Run("a window whose name holds a colon still moves", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"}, []string{"notes", "read"})
			run := windowID(t, tm, "work", "run")
			_, err := tm.run("rename-window", "-t", run, "run:tests")
			require.NoError(t, err)
			m := openOn(t, tm, "work")

			at(t, m, run)
			press(m, "x")
			atSession(t, m, tm, "notes")
			press(m, "p")

			require.Equal(t, []string{"notes:0 read", "notes:1 run:tests", "work:0 edit"}, windows(t, tm))
		})

		t.Run("pasting a window onto itself changes nothing", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "x", "p")

			require.Equal(t, []string{"work:0 edit", "work:1 run"}, windows(t, tm))
			require.Equal(t, "already there", m.status)
		})

		t.Run("pasting with nothing cut says so", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")

			press(m, "p")

			require.Contains(t, m.status, "nothing cut")
		})
	})

	t.Run("reorder", func(t *testing.T) {
		t.Run("J swaps the window with the one below it", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run", "logs"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "J")

			require.Equal(t, []string{"work:0 run", "work:1 edit", "work:2 logs"}, windows(t, tm))
		})

		t.Run("K keeps the cursor on the window it moved", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")
			run := windowID(t, tm, "work", "run")

			at(t, m, run)
			press(m, "K")

			require.Equal(t, []string{"work:0 run", "work:1 edit"}, windows(t, tm))
			require.Equal(t, run, m.current().id())
		})

		t.Run("K at the top of a session says there is nowhere to go", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "K")

			require.Equal(t, []string{"work:0 edit", "work:1 run"}, windows(t, tm))
			require.Contains(t, m.status, "no window that way")
		})

		t.Run("J swaps a pane with the next one", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			before := paneIDs(t, tm, "work", "edit")

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "l")
			at(t, m, before[0])
			press(m, "J")

			require.Equal(t, []string{before[1], before[0]}, paneIDs(t, tm, "work", "edit"))
		})

		t.Run("a session cannot be moved by hand", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "work")
			press(m, "J")

			require.Contains(t, m.status, "listed by name")
		})
	})

	t.Run("merge", func(t *testing.T) {
		t.Run("moves every window of the session under the cursor into the current one", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read", "write"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "notes")
			press(m, "M")

			require.Equal(t, []string{"work:0 edit", "work:1 read", "work:2 write"}, windows(t, tm))
		})

		t.Run("the emptied session is gone", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "notes")
			press(m, "M")

			out, err := tm.run("list-sessions", "-F", "#{session_name}")
			require.NoError(t, err)
			require.Equal(t, []string{"work"}, lines(out))
		})

		t.Run("folds in every marked session, not only the one at the cursor", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read"}, []string{"docs", "write"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "docs")
			press(m, " ")
			atSession(t, m, tm, "notes")
			press(m, " ")
			press(m, "M")

			require.Equal(t, []string{"work:0 edit", "work:1 write", "work:2 read"}, windows(t, tm))
			require.Equal(t, "merged 2 windows of 2 sessions into work", m.status)
		})

		t.Run("leaves out the session it would merge into", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "notes")
			press(m, " ")
			atSession(t, m, tm, "work")
			press(m, " ", "M")

			require.Equal(t, []string{"work:0 edit", "work:1 read"}, windows(t, tm))
			require.Equal(t, "merged 1 window of notes into work", m.status)
		})

		t.Run("merging the current session into itself is refused", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "work")
			press(m, "M")

			require.Equal(t, []string{"work:0 edit", "work:1 run"}, windows(t, tm))
			require.Contains(t, m.status, "the session you came from")
		})
	})

	t.Run("new session", func(t *testing.T) {
		t.Run("S moves what is cut into a session of the typed name", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "x", "S")
			typeIn(m, "side")
			press(m, "enter")

			require.Equal(t, []string{"side:1 run", "work:0 edit"}, windows(t, tm))
			require.Equal(t, browsing, m.mode)
		})

		t.Run("S with nothing cut takes the row under the cursor", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "S")
			typeIn(m, "side")
			press(m, "enter")

			require.Equal(t, []string{"side:1 run", "work:0 edit"}, windows(t, tm))
		})

		t.Run("esc leaves the windows where they are", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "x", "S")
			typeIn(m, "side")
			press(m, "esc")

			require.Equal(t, []string{"work:0 edit", "work:1 run"}, windows(t, tm))
			require.Equal(t, browsing, m.mode)
		})

		t.Run("a name already in use is reported and nothing moves", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"}, []string{"notes", "read"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "x", "S")
			typeIn(m, "notes")
			press(m, "enter")

			require.Equal(t, []string{"notes:0 read", "work:0 edit", "work:1 run"}, windows(t, tm))
			require.Equal(t, toneFail, m.tone)
		})
	})

	t.Run("rename", func(t *testing.T) {
		t.Run("renames the window under the cursor", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "r")
			typeIn(m, "-tests")
			press(m, "enter")

			require.Equal(t, []string{"work:0 edit", "work:1 run-tests"}, windows(t, tm))
		})

		t.Run("renames a session and keeps following it", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "work")
			press(m, "r")
			typeIn(m, "2")
			press(m, "enter")

			require.Equal(t, []string{"work2:0 edit"}, windows(t, tm))
			require.Equal(t, "work2", m.hereName())
			require.Equal(t, "work2", m.current().session.name)
		})

		t.Run("gives the pane under the cursor a title of its own", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			needsPaneNames(t, tm)
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			first := openPanes(t, m, tm, "work", "edit", 0)

			press(m, "r")
			typeIn(m, "logs")
			press(m, "enter")

			require.Equal(t, "logs", paneTitle(t, tm, first))
			require.Equal(t, "logs", m.current().label())
		})

		t.Run("the name outlasts the program in the pane", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			needsPaneNames(t, tm)
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			first := openPanes(t, m, tm, "work", "edit", 0)

			press(m, "r")
			typeIn(m, "logs")
			press(m, "enter")

			require.True(t, paneHolds(t, tm, first))
		})

		t.Run("offers the title the pane already has", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			needsPaneNames(t, tm)
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			first := openPanes(t, m, tm, "work", "edit", 0)
			require.NoError(t, tm.namePane(first, "logs"))
			m.reload(first)

			press(m, "r")
			typeIn(m, "-old")
			press(m, "enter")

			require.Equal(t, "logs-old", paneTitle(t, tm, first))
		})

		t.Run("an empty name hands the pane back to what runs in it", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			needsPaneNames(t, tm)
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			first := openPanes(t, m, tm, "work", "edit", 0)
			require.NoError(t, tm.namePane(first, "logs"))
			m.reload(first)

			press(m, "r", "backspace", "backspace", "backspace", "backspace")
			press(m, "enter")

			require.Empty(t, paneTitle(t, tm, first))
			require.False(t, paneHolds(t, tm, first))
			require.Equal(t, m.current().pane.command, m.current().label())
			require.NotEmpty(t, m.current().pane.command)
		})

		t.Run("r on a pane follows the tmux it is run on", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			openPanes(t, m, tm, "work", "edit", 0)

			press(m, "r")

			if knowsSetTitle(t, tm) {
				require.Equal(t, renaming, m.mode)
				return
			}
			require.Equal(t, browsing, m.mode)
			require.Contains(t, m.status, "tmux 3.5")
		})

		t.Run("a pane is left unnamed where tmux cannot hold the name", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			m.holdsTitles = false
			openPanes(t, m, tm, "work", "edit", 0)

			press(m, "r")

			require.Equal(t, browsing, m.mode)
			require.Contains(t, m.status, "tmux 3.5")
		})

		t.Run("a window is still renamed where a pane cannot be", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")
			m.holdsTitles = false

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "r")
			typeIn(m, "-two")
			press(m, "enter")

			require.Equal(t, []string{"work:0 edit", "work:1 run-two"}, windows(t, tm))
		})

		t.Run("an empty name leaves a window as it was", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "r", "backspace", "backspace", "backspace")
			press(m, "enter")

			require.Equal(t, []string{"work:0 edit", "work:1 run"}, windows(t, tm))
		})
	})

	t.Run("close", func(t *testing.T) {
		t.Run("closes the window under the cursor once y confirms", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run", "logs"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "d", "y")

			require.Equal(t, []string{"work:0 edit", "work:2 logs"}, windows(t, tm))
			require.Equal(t, "closed run", m.status)
		})

		t.Run("waits for the answer before closing anything", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "d")

			require.Equal(t, confirming, m.mode)
			require.Equal(t, []string{"work:0 edit", "work:1 run"}, windows(t, tm))
		})

		t.Run("any answer other than y keeps it", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "d", "n")

			require.Equal(t, []string{"work:0 edit", "work:1 run"}, windows(t, tm))
			require.Equal(t, browsing, m.mode)
			require.Contains(t, m.status, "nothing closed")
		})

		t.Run("closes a pane and leaves the rest of its window", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			before := paneIDs(t, tm, "work", "edit")
			openPanes(t, m, tm, "work", "edit", 0)

			press(m, "d", "y")

			require.Equal(t, []string{"work:0 edit"}, windows(t, tm))
			require.Equal(t, before[1:], paneIDs(t, tm, "work", "edit"))
		})

		t.Run("closes every marked row at once", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run", "logs"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, " ", " ")
			press(m, "d", "y")

			require.Equal(t, []string{"work:0 edit"}, windows(t, tm))
			require.Equal(t, "closed 2 windows", m.status)
		})

		t.Run("a marked pane inside a marked window goes with the window", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")
			openPanes(t, m, tm, "work", "edit", 0)
			press(m, " ")
			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, " ")

			press(m, "d", "y")

			require.Equal(t, []string{"work:1 run"}, windows(t, tm))
			require.Equal(t, toneDone, m.tone)
		})

		t.Run("closes a whole session", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read", "write"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "notes")
			press(m, "d", "y")

			require.Equal(t, []string{"work:0 edit"}, windows(t, tm))
			require.Equal(t, "closed notes", m.status)
		})

		t.Run("closes every marked session", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read"}, []string{"docs", "write"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "docs")
			press(m, " ")
			atSession(t, m, tm, "notes")
			press(m, " ")
			press(m, "d", "y")

			require.Equal(t, []string{"work:0 edit"}, windows(t, tm))
			require.Equal(t, "closed 2 sessions", m.status)
		})

		t.Run("a window marked inside a marked session is not closed twice", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read", "write"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "notes")
			press(m, " ")
			at(t, m, windowID(t, tm, "notes", "read"))
			press(m, " ")
			press(m, "d", "y")

			require.Equal(t, []string{"work:0 edit"}, windows(t, tm))
			require.Equal(t, toneDone, m.tone)
		})

		t.Run("names a mixed selection by what is in it", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"}, []string{"notes", "read"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "notes")
			press(m, " ")
			at(t, m, windowID(t, tm, "work", "run"))
			press(m, " ")
			press(m, "d", "y")

			require.Equal(t, []string{"work:0 edit"}, windows(t, tm))
			require.Equal(t, "closed 1 session and 1 window", m.status)
		})

		t.Run("refuses a marked session the popup was opened from", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "notes")
			press(m, " ")
			atSession(t, m, tm, "work")
			press(m, " ", "d")

			require.Equal(t, browsing, m.mode)
			require.Contains(t, m.status, "the session you came from")
			require.Equal(t, []string{"notes:0 read", "work:0 edit"}, windows(t, tm))
		})

		t.Run("refuses the session the popup was opened from", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "work")
			press(m, "d")

			require.Equal(t, browsing, m.mode)
			require.Contains(t, m.status, "the session you came from")
			require.Equal(t, []string{"notes:0 read", "work:0 edit"}, windows(t, tm))
		})

		t.Run("what was cut and then closed is no longer cut", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "x", "d", "y")

			require.Empty(t, m.clip)
			require.Equal(t, []string{"work:0 edit"}, windows(t, tm))
		})

		t.Run("the cursor lands on the row below the one that is gone", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run", "logs"})
			m := openOn(t, tm, "work")
			logs := windowID(t, tm, "work", "logs")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "d", "y")

			require.Equal(t, logs, m.current().id())
		})

		t.Run("closing the last row leaves the cursor on the one above", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")
			edit := windowID(t, tm, "work", "edit")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "d", "y")

			require.Equal(t, edit, m.current().id())
		})
	})

	t.Run("status", func(t *testing.T) {
		t.Run("a move that happened reads as done", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"}, []string{"notes", "read"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "x")
			atSession(t, m, tm, "notes")
			press(m, "p")

			require.Equal(t, toneDone, m.tone)
			require.Equal(t, "moved run to notes", m.status)
		})

		t.Run("a key with nothing to act on reads as a hint", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")

			press(m, "p")

			require.Equal(t, toneHint, m.tone)
		})

		t.Run("a rename says what the name is now", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "r")
			typeIn(m, "or")
			press(m, "enter")

			require.Equal(t, toneDone, m.tone)
			require.Equal(t, "renamed to editor", m.status)
		})

		t.Run("moving a window leaves the status quiet", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "J")

			require.Equal(t, toneQuiet, m.tone)
			require.Empty(t, m.status)
		})
	})

	t.Run("marks", func(t *testing.T) {
		t.Run("space marks the row and steps to the next", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")
			edit := windowID(t, tm, "work", "edit")

			at(t, m, edit)
			press(m, " ")

			require.True(t, m.marked[edit])
			require.Equal(t, windowID(t, tm, "work", "run"), m.current().id())
		})

		t.Run("esc drops the marks and the clipboard", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, " ", "esc")

			require.Empty(t, m.marked)
			require.Empty(t, m.clip)
		})

		t.Run("a mark covered by another does not count twice", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read", "write"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "notes")
			press(m, " ")
			at(t, m, windowID(t, tm, "notes", "read"))
			press(m, " ")

			require.Len(t, m.marks(), 1)
			require.Contains(t, m.header(), "1 marked")
		})

		t.Run("a session can be marked like any other row", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read"})
			m := openOn(t, tm, "work")
			notes := sessionID(t, tm, "notes")

			atSession(t, m, tm, "notes")
			press(m, " ")

			require.Equal(t, map[string]bool{notes: true}, m.marked)
		})
	})

	t.Run("fold", func(t *testing.T) {
		t.Run("l shows the panes of the window", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "l")

			require.Len(t, m.rows, 4)
			require.Equal(t, paneRow, m.rows[2].kind)
		})

		t.Run("h on a folded window moves to its session", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "h")

			require.Equal(t, sessionRow, m.current().kind)
			require.Equal(t, "work", m.current().session.name)
		})

		t.Run("h on a session hides its windows", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "work")
			press(m, "h")

			require.Len(t, m.rows, 1)
		})
	})

	t.Run("version", func(t *testing.T) {
		t.Run("sits in the bottom-right corner", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")

			last := lastLine(m.View())

			require.True(t, strings.HasSuffix(last, version), "corner reads %q", last)
			require.Equal(t, m.width-1, lipgloss.Width(last))
		})

		t.Run("stays in the corner while the full help is open", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")

			press(m, "?")

			require.True(t, strings.HasSuffix(lastLine(m.View()), version))
		})

		t.Run("stays in the corner over the layout of a window", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "L")

			require.True(t, strings.HasSuffix(lastLine(m.View()), version))
		})

		t.Run("gives way to the help rather than wrapping it", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")
			m.width = 40

			last := lastLine(m.View())

			require.True(t, strings.HasSuffix(last, version), "corner reads %q", last)
			require.Equal(t, m.width-1, lipgloss.Width(last))
		})
	})

	t.Run("help", func(t *testing.T) {
		t.Run("the tree keeps room for the help it opens", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")

			press(m, "?")

			require.LessOrEqual(t, len(strings.Split(m.View(), "\n")), m.height)
		})
	})
}
