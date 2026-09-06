package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func key(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
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

		t.Run("a pane has no name of its own", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "l")
			at(t, m, paneIDs(t, tm, "work", "edit")[0])
			press(m, "r")

			require.Equal(t, browsing, m.mode)
			require.Contains(t, m.status, "panes take their name")
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

		t.Run("a session cannot be marked", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "work")
			press(m, " ")

			require.Empty(t, m.marked)
			require.Contains(t, m.status, "mark windows and panes")
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
}
