package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// grid is a window of four panes, two by two, named by where they sit.
func grid() layout {
	return layout{
		shape: shape{width: 80, height: 24, boxes: []box{
			{id: "%0", index: 0, left: 0, top: 0, right: 39, bottom: 11},
			{id: "%1", index: 1, left: 0, top: 13, right: 39, bottom: 23},
			{id: "%2", index: 2, left: 41, top: 0, right: 79, bottom: 11},
			{id: "%3", index: 3, left: 41, top: 13, right: 79, bottom: 23},
		}},
	}
}

func splitInto(t *testing.T, tm tmux, target string, times int) {
	t.Helper()
	for range times {
		_, err := tm.run("split-window", "-d", "-t", target)
		require.NoError(t, err)
	}
}

// openLayoutOn opens the tree, puts the cursor on the window and opens its map.
func openLayoutOn(t *testing.T, tm tmux, session, window string) *model {
	t.Helper()
	m := openOn(t, tm, session)
	at(t, m, windowID(t, tm, session, window))
	press(m, "L")
	require.NotNil(t, m.layout, "the layout view did not open")
	return m
}

func boxOf(t *testing.T, m *model, id string) box {
	t.Helper()
	for _, b := range m.layout.shape.boxes {
		if b.id == id {
			return b
		}
	}
	t.Fatalf("no pane %s in the map", id)
	return box{}
}

func TestLayout(t *testing.T) {
	t.Run("neighbour", func(t *testing.T) {
		t.Run("finds the pane across each side", func(t *testing.T) {
			l := grid()

			require.Equal(t, 2, l.neighbour(eastward))
			require.Equal(t, 1, l.neighbour(southward))
			require.Equal(t, -1, l.neighbour(westward))
			require.Equal(t, -1, l.neighbour(northward))
		})

		t.Run("does not reach a pane that only sits diagonally", func(t *testing.T) {
			l := grid()
			l.cursor = 1

			require.Equal(t, 3, l.neighbour(eastward))
			require.Equal(t, 0, l.neighbour(northward))
		})

		t.Run("takes the nearest of the panes on that side", func(t *testing.T) {
			l := layout{shape: shape{width: 80, height: 24, boxes: []box{
				{id: "%0", left: 0, top: 0, right: 19, bottom: 23},
				{id: "%1", left: 21, top: 0, right: 49, bottom: 23},
				{id: "%2", left: 51, top: 0, right: 79, bottom: 23},
			}}}

			require.Equal(t, 1, l.neighbour(eastward))
		})

		t.Run("prefers the pane most across from it", func(t *testing.T) {
			l := layout{shape: shape{width: 80, height: 24, boxes: []box{
				{id: "%0", left: 0, top: 12, right: 39, bottom: 23},
				{id: "%1", left: 41, top: 0, right: 79, bottom: 5},
				{id: "%2", left: 41, top: 7, right: 79, bottom: 23},
			}}}

			require.Equal(t, 2, l.neighbour(eastward))
		})

		t.Run("a window of one pane has no neighbours", func(t *testing.T) {
			l := layout{shape: shape{width: 80, height: 24, boxes: []box{
				{id: "%0", left: 0, top: 0, right: 79, bottom: 23},
			}}}

			require.Equal(t, -1, l.neighbour(eastward))
			require.Equal(t, -1, l.neighbour(southward))
		})
	})

	t.Run("open", func(t *testing.T) {
		t.Run("L on a window shows every pane of it", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			splitInto(t, tm, "=work:edit", 2)

			m := openLayoutOn(t, tm, "work", "edit")

			require.Len(t, m.layout.shape.boxes, 3)
			require.Equal(t, "work · edit", m.layout.name)
		})

		t.Run("L on a pane opens on that pane", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			splitInto(t, tm, "=work:edit", 1)
			m := openOn(t, tm, "work")
			second := paneIDs(t, tm, "work", "edit")[1]

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "l")
			at(t, m, second)
			press(m, "L")

			require.Equal(t, second, m.layout.at().id)
		})

		t.Run("L on a session says where to point instead", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			m := openOn(t, tm, "work")

			atSession(t, m, tm, "work")
			press(m, "L")

			require.Nil(t, m.layout)
			require.Contains(t, m.status, "point at a window")
		})

		t.Run("esc goes back to the tree at that window", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			splitInto(t, tm, "=work:edit", 1)
			m := openLayoutOn(t, tm, "work", "edit")

			press(m, "esc")

			require.Nil(t, m.layout)
			require.Equal(t, windowID(t, tm, "work", "edit"), m.current().id())
		})
	})

	t.Run("point", func(t *testing.T) {
		t.Run("l moves to the pane on the right", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")
			m.layout.focus(paneIDs(t, tm, "work", "edit")[0])

			press(m, "l")

			require.Equal(t, paneIDs(t, tm, "work", "edit")[1], m.layout.at().id)
		})

		t.Run("says when there is no pane that way", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")
			m.layout.focus(paneIDs(t, tm, "work", "edit")[0])

			press(m, "h")

			require.Equal(t, paneIDs(t, tm, "work", "edit")[0], m.layout.at().id)
			require.Contains(t, m.status, "no pane that way")
		})
	})

	t.Run("push", func(t *testing.T) {
		t.Run("L swaps the pane with the one to its right", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")
			before := paneIDs(t, tm, "work", "edit")
			m.layout.focus(before[0])

			press(m, "L")

			require.Equal(t, []string{before[1], before[0]}, paneIDs(t, tm, "work", "edit"))
		})

		t.Run("the cursor follows the pane it pushed", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")
			moved := paneIDs(t, tm, "work", "edit")[0]
			m.layout.focus(moved)

			press(m, "L")

			require.Equal(t, moved, m.layout.at().id)
			require.Equal(t, 1, m.layout.cursor)
		})

		t.Run("u puts the pane back where it was", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")
			before := paneIDs(t, tm, "work", "edit")
			m.layout.focus(before[0])

			press(m, "L", "u")

			require.Equal(t, before, paneIDs(t, tm, "work", "edit"))
			require.Equal(t, "undid the move", m.status)
		})
	})

	t.Run("size", func(t *testing.T) {
		t.Run("s then l moves the border right", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")
			m.layout.focus(paneIDs(t, tm, "work", "edit")[0])
			before := m.layout.at().width()

			press(m, "s", "l", "l")

			require.Equal(t, before+2, m.layout.at().width())
			require.True(t, m.layout.sizing)
		})

		t.Run("esc leaves sizing without leaving the map", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")

			press(m, "s", "l", "esc")

			require.False(t, m.layout.sizing)
			require.NotNil(t, m.layout)
		})

		t.Run("u puts a whole run of nudges back in one step", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")
			m.layout.focus(paneIDs(t, tm, "work", "edit")[0])
			before := m.layout.at().width()

			press(m, "s", "l", "l", "l", "esc", "u")

			require.Equal(t, before, m.layout.at().width())
			require.Equal(t, "undid the resize", m.status)
		})

		t.Run("a run that changed nothing leaves nothing to undo", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")

			press(m, "s", "esc", "u")

			require.Contains(t, m.status, "nothing to undo")
		})
	})

	t.Run("presets", func(t *testing.T) {
		t.Run("= lays the panes out evenly across the window", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			splitInto(t, tm, "=work:edit", 2)
			m := openLayoutOn(t, tm, "work", "edit")

			press(m, "=")

			require.Equal(t, "even-horizontal", m.status)
			for _, b := range m.layout.shape.boxes {
				require.Equal(t, m.layout.shape.height, b.height())
			}
		})

		t.Run("= steps on to the next layout each time", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			splitInto(t, tm, "=work:edit", 2)
			m := openLayoutOn(t, tm, "work", "edit")

			press(m, "=", "=")

			require.Equal(t, "even-vertical", m.status)
			for _, b := range m.layout.shape.boxes {
				require.Equal(t, m.layout.shape.width, b.width())
			}
		})

		t.Run("u goes back to the layout that was there", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			splitInto(t, tm, "=work:edit", 2)
			m := openLayoutOn(t, tm, "work", "edit")
			before := m.layout.at().width()

			press(m, "=", "u")

			require.Equal(t, before, m.layout.at().width())
		})
	})

	t.Run("zoom", func(t *testing.T) {
		t.Run("z gives the pane the whole window, and gives it back", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")

			press(m, "z")
			require.True(t, m.layout.shape.zoomed)

			press(m, "z")
			require.False(t, m.layout.shape.zoomed)
		})

		t.Run("a zoomed window refuses the keys that would rearrange it", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")
			before := paneIDs(t, tm, "work", "edit")

			press(m, "z", "H")

			require.Equal(t, before, paneIDs(t, tm, "work", "edit"))
			require.Contains(t, m.status, "zoomed")
		})
	})

	t.Run("cut and place", func(t *testing.T) {
		t.Run("x cuts the pane and p puts it on the side pointed at", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			_, err = tm.run("split-window", "-d", "-v", "-t", "=work:edit.1")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")
			panes := paneIDs(t, tm, "work", "edit")
			m.layout.focus(panes[2])

			press(m, "x")
			m.layout.focus(panes[0])
			press(m, "p", "k")

			above := m.layout.at()
			require.Equal(t, panes[2], above.id)
			require.Less(t, above.top, boxOf(t, m, panes[0]).top)
		})

		t.Run("the clipboard carries from the tree into the map", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")
			edit := paneIDs(t, tm, "work", "edit")[0]

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "l")
			at(t, m, edit)
			press(m, "x")
			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "L", "p", "l")

			require.Equal(t, []string{"work:1 run"}, windows(t, tm))
			require.Len(t, paneIDs(t, tm, "work", "run"), 2)
			require.Empty(t, m.clip)
		})

		t.Run("x again puts back the pane it cut", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")

			press(m, "x", "x")

			require.Empty(t, m.clip)
		})

		t.Run("placing where it already is changes nothing", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")
			before := paneIDs(t, tm, "work", "edit")

			press(m, "x", "p", "l")

			require.Equal(t, before, paneIDs(t, tm, "work", "edit"))
			require.Contains(t, m.status, "already there")
		})

		t.Run("any other key leaves what is cut where it is", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")
			before := paneIDs(t, tm, "work", "edit")

			press(m, "x", "p", "esc")

			require.Equal(t, before, paneIDs(t, tm, "work", "edit"))
			require.Len(t, m.clip, 1)
			require.NotNil(t, m.layout)
		})

		t.Run("p with nothing cut says so", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")

			press(m, "p")

			require.Contains(t, m.status, "nothing cut")
		})

		t.Run("a window cut in the tree cannot be placed beside a pane", func(t *testing.T) {
			tm := server(t, []string{"work", "edit", "run"})
			m := openOn(t, tm, "work")

			at(t, m, windowID(t, tm, "work", "edit"))
			press(m, "x")
			at(t, m, windowID(t, tm, "work", "run"))
			press(m, "L", "p")

			require.Contains(t, m.status, "paste it in the tree")
			require.False(t, m.layout.placing)
		})

		t.Run("the map shows what is cut", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")

			press(m, "x")

			require.Contains(t, m.View(), "✂ ")
		})
	})

	t.Run("the map", func(t *testing.T) {
		t.Run("draws a box per pane, with the cursor on one of them", func(t *testing.T) {
			tm := server(t, []string{"work", "edit"})
			_, err := tm.run("split-window", "-d", "-h", "-t", "=work:edit")
			require.NoError(t, err)
			m := openLayoutOn(t, tm, "work", "edit")

			view := m.View()

			require.Contains(t, view, "work · edit")
			require.Contains(t, view, "2 panes · 80x24")
			require.Contains(t, view, "▶ ")
			require.Contains(t, view, "┌")
			require.Contains(t, view, "└")
		})
	})
}
