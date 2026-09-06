package main

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// box is a pane as the layout view sees it: what it holds, and the rectangle of
// window cells it covers. tmux leaves a cell between panes for the border, so
// boxes never touch.
type box struct {
	id      string
	index   int
	active  bool
	command string
	title   string
	left    int
	top     int
	right   int
	bottom  int
}

func (b box) label() string {
	if b.title != "" {
		return b.title
	}
	return b.command
}

func (b box) width() int  { return b.right - b.left + 1 }
func (b box) height() int { return b.bottom - b.top + 1 }

// shape is one window's panes and the window they divide up.
type shape struct {
	width  int
	height int
	zoomed bool
	boxes  []box
}

var shapeFormat = strings.Join([]string{
	"#{pane_id}",
	"#{pane_index}",
	"#{?pane_active,1,0}",
	"#{pane_current_command}",
	paneName,
	"#{pane_left}",
	"#{pane_top}",
	"#{pane_right}",
	"#{pane_bottom}",
	"#{window_width}",
	"#{window_height}",
	"#{?window_zoomed_flag,1,0}",
}, "\t")

func (t tmux) shapeOf(window string) (shape, error) {
	out, err := t.run("list-panes", "-t", window, "-F", shapeFormat)
	if err != nil {
		return shape{}, err
	}
	return parseShape(out), nil
}

func parseShape(out string) shape {
	var s shape
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 12 {
			continue
		}
		s.boxes = append(s.boxes, box{
			id:      f[0],
			index:   atoi(f[1]),
			active:  f[2] == "1",
			command: f[3],
			title:   f[4],
			left:    atoi(f[5]),
			top:     atoi(f[6]),
			right:   atoi(f[7]),
			bottom:  atoi(f[8]),
		})
		s.width, s.height, s.zoomed = atoi(f[9]), atoi(f[10]), f[11] == "1"
	}
	return s
}

// layoutOf is the window's geometry as tmux writes it down. select-layout takes
// the string back and restores the sizes, but not which pane sits where, so a
// pushed pane is put back by pushing it the other way instead.
func (t tmux) layoutOf(window string) (string, error) {
	out, err := t.run("display-message", "-p", "-t", window, "#{window_layout}")
	return strings.TrimSpace(out), err
}

// side is a direction to point, push or resize in: the flag resize-pane knows
// it by, and the step it takes across the window.
type side struct {
	flag string
	dx   int
	dy   int
}

var (
	westward  = side{"-L", -1, 0}
	eastward  = side{"-R", 1, 0}
	northward = side{"-U", 0, -1}
	southward = side{"-D", 0, 1}
)

// undoStep is what puts one change back, and what to call it when it is undone.
type undoStep struct {
	what string
	cmds [][]string
}

// layout is the view of one window's panes as boxes to point at.
type layout struct {
	session string
	window  string
	name    string
	shape   shape
	cursor  int
	undo    []undoStep
	preset  int
	sizing  bool
	placing bool
	before  string
}

func (l *layout) at() box { return l.shape.boxes[l.cursor] }

func (l *layout) focus(id string) {
	for i, b := range l.shape.boxes {
		if b.id == id {
			l.cursor = i
			return
		}
	}
	if l.cursor >= len(l.shape.boxes) {
		l.cursor = len(l.shape.boxes) - 1
	}
}

func (l *layout) focusActive() {
	for i, b := range l.shape.boxes {
		if b.active {
			l.cursor = i
			return
		}
	}
}

func (l *layout) remember(what string, cmds [][]string) {
	l.undo = append(l.undo, undoStep{what: what, cmds: cmds})
}

// neighbour is the pane next to the cursor's on that side: the nearest one
// past its edge that stands across from it rather than diagonally beyond it.
func (l *layout) neighbour(s side) int {
	cur := l.at()
	best, nearest, widest := -1, 0, 0
	for i, b := range l.shape.boxes {
		if i == l.cursor {
			continue
		}
		gap, overlap := 0, 0
		if s.dx != 0 {
			gap = cur.left - b.right
			if s.dx > 0 {
				gap = b.left - cur.right
			}
			overlap = min(cur.bottom, b.bottom) - max(cur.top, b.top) + 1
		} else {
			gap = cur.top - b.bottom
			if s.dy > 0 {
				gap = b.top - cur.bottom
			}
			overlap = min(cur.right, b.right) - max(cur.left, b.left) + 1
		}
		if gap <= 0 || overlap <= 0 {
			continue
		}
		if best < 0 || gap < nearest || (gap == nearest && overlap > widest) {
			best, nearest, widest = i, gap, overlap
		}
	}
	return best
}

func (m *model) openLayout() {
	r := m.current()
	if r.kind == sessionRow {
		m.hint("point at a window or a pane")
		return
	}
	sh, err := m.tmux.shapeOf(r.window.id)
	if err != nil {
		m.fail(err)
		return
	}
	m.layout = &layout{
		session: r.session.id,
		window:  r.window.id,
		name:    r.session.name + " · " + r.window.name,
		shape:   sh,
	}
	if r.kind == paneRow {
		m.layout.focus(r.pane.id)
	} else {
		m.layout.focusActive()
	}
	m.clear()
}

func (m *model) closeLayout() {
	window := m.layout.window
	m.layout = nil
	m.reload(window)
	m.clear()
}

func (m *model) layoutReload(focus string) {
	sh, err := m.tmux.shapeOf(m.layout.window)
	if err != nil {
		m.fail(err)
		return
	}
	if len(sh.boxes) == 0 {
		m.closeLayout()
		return
	}
	m.layout.shape = sh
	m.layout.focus(focus)
}

func (m *model) layoutKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.layout.sizing {
		return m.sizeKey(msg)
	}
	if m.layout.placing {
		return m.placeKey(msg)
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.closeLayout()
	case "h", "left":
		m.point(westward)
	case "j", "down":
		m.point(southward)
	case "k", "up":
		m.point(northward)
	case "l", "right":
		m.point(eastward)
	case "H":
		m.push(westward)
	case "J":
		m.push(southward)
	case "K":
		m.push(northward)
	case "L":
		m.push(eastward)
	case "x":
		m.cutPane()
	case "p":
		m.askPlace()
	case "s":
		m.startSizing()
	case "=":
		m.nextPreset()
	case "z":
		m.zoom()
	case "u":
		m.undoLayout()
	case "enter":
		if err := m.tmux.switchTo(m.client, m.layout.session, m.layout.window, m.layout.at().id); err != nil {
			m.fail(err)
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}

// placeKey takes the side of the pane under the cursor that what is cut lands
// on. Anything but a direction backs out, leaving the clipboard as it was.
func (m *model) placeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.layout.placing = false
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "h", "left":
		m.place(westward)
	case "j", "down":
		m.place(southward)
	case "k", "up":
		m.place(northward)
	case "l", "right":
		m.place(eastward)
	default:
		m.clear()
	}
	return m, nil
}

// cutPane takes the pane under the cursor, or puts it back when it is the one
// already cut, into the same clipboard the tree cuts into.
func (m *model) cutPane() {
	b := m.layout.at()
	if len(m.clip) == 1 && m.clip[0].id == b.id {
		m.clip = nil
		m.clear()
		return
	}
	m.clip = []item{{kind: paneRow, id: b.id, label: b.label()}}
	m.done("cut %s — p puts it beside the pane you point at", b.label())
}

func (m *model) askPlace() {
	if len(m.clip) == 0 {
		m.hint("nothing cut — x cuts the pane under the cursor")
		return
	}
	for _, it := range m.clip {
		if it.kind != paneRow {
			m.hint("a window goes beside a window: paste it in the tree")
			return
		}
	}
	if m.stillZoomed() {
		return
	}
	m.layout.placing = true
	m.clear()
}

// place splits the pane under the cursor and puts what is cut in the half on
// that side, wherever in the tree it was cut from.
func (m *model) place(s side) {
	l := m.layout
	dst := l.at()
	var cmds [][]string
	for _, it := range m.clip {
		if it.id == dst.id {
			m.hint("already there")
			return
		}
		cmd := []string{"join-pane", "-d", "-v"}
		if s.dx != 0 {
			cmd = []string{"join-pane", "-d", "-h"}
		}
		if s.dx < 0 || s.dy < 0 {
			cmd = append(cmd, "-b")
		}
		cmds = append(cmds, append(cmd, "-s", it.id, "-t", dst.id))
	}
	placed, beside, first := describe(m.clip), dst.label(), m.clip[0].id
	if err := m.tmux.apply(cmds); err != nil {
		m.fail(err)
		return
	}
	m.clip = nil
	m.layoutReload(first)
	m.done("put %s beside %s", placed, beside)
}

// sizeKey nudges the pane's borders until esc leaves, so that the keys that
// point at a pane are the ones that resize it.
func (m *model) sizeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q", "s", "enter":
		m.stopSizing()
	case "h", "left":
		m.resize(westward, 1)
	case "j", "down":
		m.resize(southward, 1)
	case "k", "up":
		m.resize(northward, 1)
	case "l", "right":
		m.resize(eastward, 1)
	case "H":
		m.resize(westward, 5)
	case "J":
		m.resize(southward, 5)
	case "K":
		m.resize(northward, 5)
	case "L":
		m.resize(eastward, 5)
	}
	return m, nil
}

func (m *model) point(s side) {
	other := m.layout.neighbour(s)
	if other < 0 {
		m.hint("no pane that way")
		return
	}
	m.layout.cursor = other
	m.clear()
}

func (m *model) push(s side) {
	l := m.layout
	if m.stillZoomed() {
		return
	}
	other := l.neighbour(s)
	if other < 0 {
		m.hint("no pane that way")
		return
	}
	moved, swapped := l.at().id, l.shape.boxes[other].id
	if _, err := m.tmux.run("swap-pane", "-d", "-s", moved, "-t", swapped); err != nil {
		m.fail(err)
		return
	}
	l.remember("the move", [][]string{{"swap-pane", "-d", "-s", moved, "-t", swapped}})
	m.layoutReload(moved)
	m.clear()
}

func (m *model) startSizing() {
	if m.stillZoomed() {
		return
	}
	before, err := m.tmux.layoutOf(m.layout.window)
	if err != nil {
		m.fail(err)
		return
	}
	m.layout.sizing, m.layout.before = true, before
	m.clear()
}

// stopSizing folds a whole run of nudges into one undo step, so that u puts the
// pane back the size it was before the run rather than one nudge back.
func (m *model) stopSizing() {
	l := m.layout
	l.sizing = false
	now, err := m.tmux.layoutOf(l.window)
	if err == nil && now != l.before {
		l.remember("the resize", [][]string{{"select-layout", "-t", l.window, l.before}})
	}
	m.clear()
}

func (m *model) resize(s side, step int) {
	l := m.layout
	if _, err := m.tmux.run("resize-pane", "-t", l.at().id, s.flag, strconv.Itoa(step)); err != nil {
		m.fail(err)
		return
	}
	m.layoutReload(l.at().id)
}

var presets = []string{"even-horizontal", "even-vertical", "main-horizontal", "main-vertical", "tiled"}

func (m *model) nextPreset() {
	l := m.layout
	if m.stillZoomed() {
		return
	}
	before, err := m.tmux.layoutOf(l.window)
	if err != nil {
		m.fail(err)
		return
	}
	name := presets[l.preset%len(presets)]
	l.preset++
	if _, err := m.tmux.run("select-layout", "-t", l.window, name); err != nil {
		m.fail(err)
		return
	}
	l.remember("the layout", [][]string{{"select-layout", "-t", l.window, before}})
	m.layoutReload(l.at().id)
	m.done("%s", name)
}

func (m *model) zoom() {
	l := m.layout
	if _, err := m.tmux.run("resize-pane", "-Z", "-t", l.at().id); err != nil {
		m.fail(err)
		return
	}
	m.layoutReload(l.at().id)
	m.clear()
}

func (m *model) undoLayout() {
	l := m.layout
	if len(l.undo) == 0 {
		m.hint("nothing to undo")
		return
	}
	step := l.undo[len(l.undo)-1]
	l.undo = l.undo[:len(l.undo)-1]
	if err := m.tmux.apply(step.cmds); err != nil {
		m.fail(err)
		return
	}
	m.layoutReload(l.at().id)
	m.done("undid %s", step.what)
}

// stillZoomed reports the one state the layout view cannot work in: tmux gives
// a zoomed pane the whole window, so the others have nowhere to be drawn and
// nothing to be pushed against.
func (m *model) stillZoomed() bool {
	if !m.layout.shape.zoomed {
		return false
	}
	m.hint("zoomed — z shows the whole window again")
	return true
}

const (
	armUp uint8 = 1 << iota
	armRight
	armDown
	armLeft
)

var armRunes = map[uint8]rune{
	armLeft | armRight:                   '─',
	armUp | armDown:                      '│',
	armDown | armRight:                   '┌',
	armDown | armLeft:                    '┐',
	armUp | armRight:                     '└',
	armUp | armLeft:                      '┘',
	armUp | armDown | armRight:           '├',
	armUp | armDown | armLeft:            '┤',
	armLeft | armRight | armDown:         '┬',
	armLeft | armRight | armUp:           '┴',
	armUp | armDown | armLeft | armRight: '┼',
	armUp:                                '│',
	armDown:                              '│',
	armLeft:                              '─',
	armRight:                             '─',
}

// tint is how a cell of the map is painted. Cells are grouped by it a row at a
// time, so a row costs one styled string per run rather than one per cell.
type tint uint8

const (
	tintNone tint = iota
	tintEdge
	tintPick
	tintName
	tintDetail
	tintLive
	tintCut
)

func (t tint) style() lipgloss.Style {
	switch t {
	case tintEdge:
		return guideStyle
	case tintPick:
		return sessionStyle
	case tintName:
		return nameStyle
	case tintDetail:
		return detailStyle
	case tintLive:
		return activeStyle
	case tintCut:
		return cutStyle.Italic(true)
	}
	return plainStyle
}

type canvas struct {
	w, h  int
	runes []rune
	arms  []uint8
	tints []tint
}

func newCanvas(w, h int) *canvas {
	return &canvas{w: w, h: h, runes: make([]rune, w*h), arms: make([]uint8, w*h), tints: make([]tint, w*h)}
}

func (c *canvas) inside(x, y int) bool { return x >= 0 && x < c.w && y >= 0 && y < c.h }

func (c *canvas) arm(x, y int, a uint8, t tint) {
	if !c.inside(x, y) {
		return
	}
	i := y*c.w + x
	c.arms[i] |= a
	c.runes[i] = 0
	c.tints[i] = t
}

func (c *canvas) put(x, y int, text string, t tint) {
	for _, r := range text {
		if c.inside(x, y) {
			i := y*c.w + x
			c.runes[i], c.arms[i], c.tints[i] = r, 0, t
		}
		x++
	}
}

// frame draws the four edges of a box. Corners are set last and merged with
// whatever is already there, so two boxes sharing a line meet in a junction
// rather than one corner overwriting the other.
func (c *canvas) frame(x0, y0, x1, y1 int, t tint) {
	for x := x0 + 1; x < x1; x++ {
		c.arm(x, y0, armLeft|armRight, t)
		c.arm(x, y1, armLeft|armRight, t)
	}
	for y := y0 + 1; y < y1; y++ {
		c.arm(x0, y, armUp|armDown, t)
		c.arm(x1, y, armUp|armDown, t)
	}
	c.arm(x0, y0, armDown|armRight, t)
	c.arm(x1, y0, armDown|armLeft, t)
	c.arm(x0, y1, armUp|armRight, t)
	c.arm(x1, y1, armUp|armLeft, t)
}

func (c *canvas) rows() []string {
	var lines []string
	for y := range c.h {
		var b strings.Builder
		run, at := strings.Builder{}, tintNone
		flush := func() {
			if run.Len() > 0 {
				b.WriteString(at.style().Render(run.String()))
				run.Reset()
			}
		}
		for x := range c.w {
			i := y*c.w + x
			r := c.runes[i]
			if r == 0 {
				r = ' '
				if arms := c.arms[i]; arms != 0 {
					r = armRunes[arms]
				}
			}
			if c.tints[i] != at {
				flush()
				at = c.tints[i]
			}
			run.WriteRune(r)
		}
		flush()
		lines = append(lines, b.String())
	}
	return lines
}

// layoutMap draws the window to scale, one box a pane, keeping the window's own
// proportions so that the map reads as the window does.
func (m *model) layoutMap(width, height int) []string {
	l := m.layout
	boxes := l.shape.boxes
	if l.shape.zoomed {
		// A zoomed pane is given the whole window while the others go on
		// reporting the rectangles underneath it, which would draw over each
		// other; the zoomed one is the only pane really on screen.
		boxes = nil
		for _, b := range l.shape.boxes {
			if b.width() == l.shape.width && b.height() == l.shape.height {
				boxes = append(boxes, b)
			}
		}
	}
	if len(boxes) == 0 || l.shape.width == 0 || l.shape.height == 0 {
		return nil
	}
	scale := min(float64(width)/float64(l.shape.width), float64(height)/float64(l.shape.height))
	mapW, mapH := int(float64(l.shape.width)*scale), int(float64(l.shape.height)*scale)
	mapW, mapH = max(mapW, 3), max(mapH, 3)
	c := newCanvas(mapW, mapH)
	for _, b := range boxes {
		if b.id != l.at().id {
			m.drawBox(c, b, scale, false)
		}
	}
	for _, b := range boxes {
		if b.id == l.at().id {
			m.drawBox(c, b, scale, true)
		}
	}
	pad := strings.Repeat(" ", max((width-mapW)/2, 0))
	var lines []string
	for _, row := range c.rows() {
		lines = append(lines, pad+row)
	}
	return lines
}

// drawBox draws a pane out to the cells tmux keeps for its borders, which the
// pane beside it keeps too, so that two panes meet on one line rather than each
// drawing an edge of its own.
func (m *model) drawBox(c *canvas, b box, scale float64, picked bool) {
	sh := m.layout.shape
	at := func(v int) int { return int(float64(v) * scale) }
	x0, y0 := at(max(b.left-1, 0)), at(max(b.top-1, 0))
	x1, y1 := at(min(b.right+1, sh.width-1)), at(min(b.bottom+1, sh.height-1))
	x1, y1 = min(max(x1, x0+2), c.w-1), min(max(y1, y0+2), c.h-1)
	edge, name := tintEdge, tintName
	if picked {
		edge, name = tintPick, tintPick
	}
	cut := false
	for _, it := range m.clip {
		if it.id == b.id {
			cut, name = true, tintCut
		}
	}
	c.frame(x0, y0, x1, y1, edge)
	inner := x1 - x0 - 1
	label := fmt.Sprintf("%d  %s", b.index, b.label())
	if cut {
		label = "✂ " + label
	}
	if picked {
		label = "▶ " + label
	}
	c.put(x0+1, y0+1, fit(label, inner), name)
	if b.active && lipgloss.Width(fit(label, inner)) < inner {
		c.put(x0+1+lipgloss.Width(fit(label, inner)), y0+1, " ●", tintLive)
	}
	if y1-y0 >= 3 {
		c.put(x0+1, y0+2, fit(fmt.Sprintf("%dx%d", b.width(), b.height()), inner), tintDetail)
	}
}

func (m *model) layoutView() string {
	var b strings.Builder
	b.WriteString(m.layoutHeader())
	b.WriteString("\n\n")
	height := m.viewHeight()
	lines := m.layoutMap(m.width-2, height)
	top := max((height-len(lines))/2, 0)
	for range top {
		b.WriteString("\n")
	}
	for _, line := range lines {
		b.WriteString(" " + line + "\n")
	}
	for i := top + len(lines); i < height; i++ {
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.layoutFooter())
	return b.String()
}

func (m *model) layoutHeader() string {
	l := m.layout
	right := fmt.Sprintf("%s · %dx%d", plural(len(l.shape.boxes), "pane"), l.shape.width, l.shape.height)
	if l.shape.zoomed {
		right = "zoomed · " + right
	}
	if len(m.clip) > 0 {
		right = "✂ " + describe(m.clip) + " cut · " + right
	}
	return sessionStyle.Render(fit(l.name, m.width)) + m.gap(l.name, right) + headerStyle.Render(right)
}

const (
	layoutHelp = "hjkl point   HJKL push   x cut   p place   s size   = layout   z zoom   u undo   esc back"
	sizeHelp   = "h j k l  nudge the border that way   H J K L  by five   esc  done"
	placeHelp  = "h j k l  the side it goes on   any other key  keep it where it is"
)

func (m *model) layoutFooter() string {
	status := ""
	switch m.tone {
	case toneDone:
		status = doneStyle.Render(fit("✓ "+m.status, m.width))
	case toneHint:
		status = hintStyle.Render(fit(m.status, m.width))
	case toneFail:
		status = errorStyle.Render(fit("✗ "+m.status, m.width))
	}
	help := layoutHelp
	if m.layout.sizing {
		help = sizeHelp
		status = sessionStyle.Render(fit("resizing "+m.layout.at().label(), m.width))
	}
	if m.layout.placing {
		help = placeHelp
		status = sessionStyle.Render(fit("which side of "+m.layout.at().label()+"?", m.width))
	}
	return status + "\n" + helpStyle.Render(fit(help, m.width))
}
