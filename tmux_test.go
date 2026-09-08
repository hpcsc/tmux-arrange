package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// server starts a throwaway tmux server holding one session per name, each with
// the given windows, and returns a client bound to it.
// needsPaneNames skips a test that names a pane where tmux cannot hold the name.
// allow-set-title arrived in 3.5, and Ubuntu still ships 3.4.
func needsPaneNames(t *testing.T, tm tmux) {
	t.Helper()
	if !tm.holdsTitles() {
		t.Skip("this tmux has no allow-set-title; naming a pane needs 3.5")
	}
}

func server(t *testing.T, sessions ...[]string) tmux {
	t.Helper()
	t.Setenv("TMUX", "")
	// A unix socket path is capped near 104 bytes, which t.TempDir() names overrun.
	dir, err := os.MkdirTemp("", "tmxa")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	tm := tmux{socket: filepath.Join(dir, "s")}
	for i, spec := range sessions {
		name, windows := spec[0], spec[1:]
		args := []string{"new-session", "-d", "-s", name, "-x", "80", "-y", "24"}
		if i == 0 {
			args = append([]string{"-f", "/dev/null"}, args...)
		}
		if len(windows) > 0 {
			args = append(args, "-n", windows[0])
		}
		_, err = tm.run(args...)
		require.NoError(t, err)
		for _, w := range windows[1:] {
			_, err := tm.run("new-window", "-d", "-t", "="+name+":", "-n", w)
			require.NoError(t, err)
		}
	}
	t.Cleanup(func() { tm.run("kill-server") })
	return tm
}

func windows(t *testing.T, tm tmux) []string {
	t.Helper()
	out, err := tm.run("list-windows", "-a", "-F", "#{session_name}:#{window_index} #{window_name}")
	require.NoError(t, err)
	return lines(out)
}

func panes(t *testing.T, tm tmux) []string {
	t.Helper()
	out, err := tm.run("list-panes", "-a", "-F", "#{session_name}:#{window_index}.#{pane_index} #{pane_id}")
	require.NoError(t, err)
	return lines(out)
}

func lines(out string) []string {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func sessionID(t *testing.T, tm tmux, name string) string {
	t.Helper()
	out, err := tm.run("list-sessions", "-F", "#{session_id} #{session_name}")
	require.NoError(t, err)
	for _, l := range lines(out) {
		if f := strings.SplitN(l, " ", 2); len(f) == 2 && f[1] == name {
			return f[0]
		}
	}
	t.Fatalf("no session named %s", name)
	return ""
}

func windowID(t *testing.T, tm tmux, session, name string) string {
	t.Helper()
	out, err := tm.run("list-windows", "-t", "="+session, "-F", "#{window_id} #{window_name}")
	require.NoError(t, err)
	for _, l := range lines(out) {
		if f := strings.Fields(l); len(f) == 2 && f[1] == name {
			return f[0]
		}
	}
	t.Fatalf("no window %s in session %s", name, session)
	return ""
}

func TestTmux(t *testing.T) {
	t.Run("tree", func(t *testing.T) {
		t.Run("groups every pane under its window and session", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"}, []string{"notes", "read"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)

			tree, err := tm.tree()
			require.NoError(t, err)

			require.Len(t, tree, 2)
			require.Equal(t, "notes", tree[0].name)
			require.Equal(t, "work", tree[1].name)
			require.Len(t, tree[1].windows, 2)
			require.Equal(t, "edit", tree[1].windows[0].name)
			require.Len(t, tree[1].windows[0].panes, 2)
			require.Len(t, tree[1].windows[1].panes, 1)
		})

		t.Run("orders windows by index, not by creation", func(t *testing.T) {
			tm := server(t, []string{"work", "one", "two", "three"})
			_, err := tm.run("move-window", "-d", "-b", "-s", windowID(t, tm, "work", "three"), "-t", "=work:0")
			require.NoError(t, err)

			tree, err := tm.tree()
			require.NoError(t, err)

			var names []string
			for _, w := range tree[0].windows {
				names = append(names, w.name)
			}
			require.Equal(t, []string{"three", "one", "two"}, names)
		})

		t.Run("reads the window name, index and pane command", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})

			tree, err := tm.tree()
			require.NoError(t, err)

			w := tree[0].windows[0]
			require.Equal(t, "edit", w.name)
			require.Equal(t, 0, w.index)
			require.True(t, w.active)
			require.NotEmpty(t, w.panes[0].command)
			require.NotEmpty(t, w.panes[0].path)
		})

		t.Run("reads the name a pane has been given", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			needsPaneNames(t, tm)
			tree, err := tm.tree()
			require.NoError(t, err)
			require.NoError(t, tm.namePane(tree[0].windows[0].panes[0].id, "logs"))

			tree, err = tm.tree()
			require.NoError(t, err)

			require.Equal(t, "logs", tree[0].windows[0].panes[0].title)
		})

		t.Run("a title the pane's own program can overwrite is no name", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("select-pane", "-t", "=work:edit", "-T", "passing through")
			require.NoError(t, err)

			tree, err := tm.tree()
			require.NoError(t, err)

			require.Empty(t, tree[0].windows[0].panes[0].title)
		})

		t.Run("a pane nobody has named has no name", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})

			tree, err := tm.tree()
			require.NoError(t, err)

			require.Empty(t, tree[0].windows[0].panes[0].title)
		})

		t.Run("clearing a name hands the title back to the pane", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			needsPaneNames(t, tm)
			tree, err := tm.tree()
			require.NoError(t, err)
			id := tree[0].windows[0].panes[0].id
			require.NoError(t, tm.namePane(id, "logs"))

			require.NoError(t, tm.namePane(id, ""))

			tree, err = tm.tree()
			require.NoError(t, err)
			require.Empty(t, tree[0].windows[0].panes[0].title)
			out, err := tm.run("show-options", "-p", "-t", id, "allow-set-title")
			require.NoError(t, err)
			require.Empty(t, out)
		})

		t.Run("a window linked into two sessions shows under both", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"}, []string{"notes", "read"})
			_, err := tm.run("link-window", "-d", "-s", "=work:edit", "-t", "=notes:")
			require.NoError(t, err)

			tree, err := tm.tree()
			require.NoError(t, err)

			require.Len(t, tree, 2)
			require.Equal(t, []string{"read", "edit"}, []string{tree[0].windows[0].name, tree[0].windows[1].name})
			require.Equal(t, "edit", tree[1].windows[0].name)
			require.Len(t, tree[1].windows[0].panes, 1)
		})

		t.Run("an empty server yields no sessions", func(t *testing.T) {
			require.Empty(t, parseTree(""))
		})
	})

	t.Run("options", func(t *testing.T) {
		t.Run("a set option is read", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("set-option", "-g", "@arrange-width", "60%")
			require.NoError(t, err)

			require.Equal(t, "60%", tm.option("@arrange-width", "80%"))
		})

		t.Run("an unset option falls back", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})

			require.Equal(t, "80%", tm.option("@arrange-width", "80%"))
		})
	})

	t.Run("new session", func(t *testing.T) {
		t.Run("holds the moved windows and nothing else", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run", "logs"})
			clip := []item{
				{kind: windowRow, id: windowID(t, tm, "work", "run")},
				{kind: windowRow, id: windowID(t, tm, "work", "logs")},
			}

			require.NoError(t, tm.newSession("side", clip))

			require.Equal(t, []string{"side:1 run", "side:2 logs", "work:0 edit"}, windows(t, tm))
		})

		t.Run("a pane becomes a window of the new session", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-t", "=work:edit")
			require.NoError(t, err)
			tree, _ := tm.tree()
			second := tree[0].windows[0].panes[1]

			require.NoError(t, tm.newSession("side", []item{{kind: paneRow, id: second.id}}))

			require.Len(t, windows(t, tm), 2)
			require.Contains(t, panes(t, tm)[0], second.id)
		})

		t.Run("a move that fails takes the half-made session with it", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})

			err := tm.newSession("side", []item{{kind: windowRow, id: "@404"}})

			require.Error(t, err)
			out, err := tm.run("list-sessions", "-F", "#{session_name}")
			require.NoError(t, err)
			require.Equal(t, []string{"work"}, lines(out))
		})

		t.Run("a name already taken is reported", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})

			err := tm.newSession("work", []item{{kind: windowRow, id: windowID(t, tm, "work", "edit")}})

			require.Error(t, err)
			require.Contains(t, err.Error(), "duplicate session")
			require.Equal(t, []string{"work:0 edit"}, windows(t, tm))
		})
	})
}
