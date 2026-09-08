package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	accent = lipgloss.Color("212")
	blue   = lipgloss.Color("110")
	green  = lipgloss.Color("114")
	yellow = lipgloss.Color("179")
	red    = lipgloss.Color("203")
	// The terminal palette gives colour 8, which stays readable on a light
	// background; a fixed 256 grey does not.
	grey     = lipgloss.Color("8")
	cursorBg = lipgloss.Color("239")

	plainStyle   = lipgloss.NewStyle()
	sessionStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	guideStyle   = lipgloss.NewStyle().Foreground(grey)
	indexStyle   = lipgloss.NewStyle().Foreground(blue)
	nameStyle    = plainStyle
	commandStyle = lipgloss.NewStyle().Foreground(grey)
	detailStyle  = lipgloss.NewStyle().Foreground(grey)
	activeStyle  = lipgloss.NewStyle().Foreground(green)
	markStyle    = lipgloss.NewStyle().Foreground(green)
	cutStyle     = lipgloss.NewStyle().Foreground(yellow)
	headerStyle  = lipgloss.NewStyle().Foreground(grey)
	helpStyle    = lipgloss.NewStyle().Foreground(grey)
	doneStyle    = lipgloss.NewStyle().Foreground(green)
	hintStyle    = lipgloss.NewStyle().Foreground(yellow)
	errorStyle   = lipgloss.NewStyle().Foreground(red)
)

type cell struct {
	text  string
	style lipgloss.Style
}

var home, _ = os.UserHomeDir()

func shortPath(p string) string {
	if home != "" && p == home {
		return "~"
	}
	if home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

// fitTail trims from the front, so a path that does not fit keeps the end that
// tells it apart rather than the root every path shares.
func fitTail(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width("…"+string(r)) > width {
		r = r[1:]
	}
	return "…" + string(r)
}

func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)+"…") > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func (m *model) chromeHeight() int {
	if m.help && m.layout == nil {
		return 13
	}
	return 6
}

func (m *model) viewHeight() int {
	h := m.height - m.chromeHeight()
	if h < 3 {
		return 3
	}
	return h
}

func (m *model) scroll(height int) {
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+height {
		m.top = m.cursor - height + 1
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m *model) View() string {
	if m.layout != nil {
		return m.layoutView()
	}
	if len(m.rows) == 0 {
		return "no tmux sessions\n"
	}
	height := m.viewHeight()
	m.scroll(height)
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")
	shown := 0
	for i := m.top; i < len(m.rows) && shown < height; i, shown = i+1, shown+1 {
		b.WriteString(m.line(m.rows[i], i == m.cursor))
		b.WriteString("\n")
	}
	for ; shown < height; shown++ {
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.footer())
	return b.String()
}

func (m *model) header() string {
	windows, panes := 0, 0
	for _, s := range m.sessions {
		windows += len(s.windows)
		for _, w := range s.windows {
			panes += len(w.panes)
		}
	}
	left := fmt.Sprintf("%s  %s  %s",
		plural(len(m.sessions), "session"), plural(windows, "window"), plural(panes, "pane"))
	right, style := "", markStyle
	if len(m.clip) > 0 {
		right, style = "✂ "+describe(m.clip)+" cut", cutStyle
	} else if n := len(m.marks()); n > 0 {
		right = fmt.Sprintf("✓ %d marked", n)
	}
	return headerStyle.Render(fit(left, m.width)) + m.gap(left, right) + style.Render(right)
}

func (m *model) gap(left, right string) string {
	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if pad < 1 {
		return " "
	}
	return strings.Repeat(" ", pad)
}

func (m *model) line(r row, selected bool) string {
	pointer := cell{"  ", plainStyle}
	if selected {
		pointer = cell{"▶ ", sessionStyle}
	}
	flag := cell{"  ", plainStyle}
	switch {
	case r.cut:
		flag = cell{"✂ ", cutStyle}
	case r.marked:
		flag = cell{"✓ ", markStyle}
	}
	cells, right := m.cells(r)
	width := m.width - 1
	right = fitTail(right, width*2/5)
	body, used := renderCells(append([]cell{pointer, flag}, cells...), width-lipgloss.Width(right)-1, selected)
	pad := width - used - lipgloss.Width(right)
	if pad < 0 {
		pad = 0
	}
	return body +
		paint(plainStyle, selected).Render(strings.Repeat(" ", pad)) +
		paint(detailStyle, selected).Render(right)
}

func renderCells(cells []cell, width int, selected bool) (string, int) {
	var b strings.Builder
	used := 0
	for _, c := range cells {
		if used >= width {
			break
		}
		text := fit(c.text, width-used)
		b.WriteString(paint(c.style, selected).Render(text))
		used += lipgloss.Width(text)
	}
	return b.String(), used
}

func paint(style lipgloss.Style, selected bool) lipgloss.Style {
	if selected {
		return style.Background(cursorBg)
	}
	return style
}

// cells lays a row out in columns the kinds share: the guide and fold column,
// then the index, then the name. A pane's branch mark stands in the fold column
// of its window, so indexes and names line up down the whole tree.
func (m *model) cells(r row) ([]cell, string) {
	switch r.kind {
	case sessionRow:
		fold := "▾ "
		if m.collapsed[r.session.id] {
			fold = "▸ "
		}
		cells := []cell{{fold, guideStyle}, {r.session.name, sessionStyle}}
		if r.session.attached {
			cells = append(cells, cell{" ●", activeStyle})
		}
		right := plural(len(r.session.windows), "window")
		if r.session.id == m.here {
			right = "here · " + right
		}
		return cells, right

	case windowRow:
		cells := []cell{
			{"│ ", guideStyle},
			{fold(len(r.window.panes), m.expanded[r.window.id]), guideStyle},
			{" ", plainStyle},
			{fmt.Sprintf("%-2d", r.window.index), indexStyle},
			{" ", plainStyle},
			{r.window.name, textStyle(r, nameStyle)},
		}
		if r.window.active {
			cells = append(cells, cell{" ●", activeStyle})
		}
		return cells, shortPath(r.window.panes[0].path)

	default:
		branch := " ├"
		if last := r.window.panes[len(r.window.panes)-1]; last.id == r.pane.id {
			branch = " └"
		}
		name := commandStyle
		if r.pane.title != "" {
			name = nameStyle
		}
		cells := []cell{
			{"│ ", guideStyle},
			{branch, guideStyle},
			{" ", plainStyle},
			{fmt.Sprintf("%-2d", r.pane.index), indexStyle},
			{" ", plainStyle},
			{r.label(), textStyle(r, name)},
		}
		if r.pane.title != "" {
			cells = append(cells, cell{"  " + r.pane.command, detailStyle})
		}
		return cells, shortPath(r.pane.path)
	}
}

func fold(panes int, open bool) string {
	if panes < 2 && !open {
		return "  "
	}
	count := strconv.Itoa(panes)
	if panes > 9 {
		count = "+"
	}
	if open {
		return "▾" + count
	}
	return "▸" + count
}

func textStyle(r row, style lipgloss.Style) lipgloss.Style {
	if r.cut {
		return cutStyle.Italic(true)
	}
	return style
}

func (m *model) footer() string {
	if m.mode == confirming {
		return sessionStyle.Render("close "+describe(m.doomed)+"?") + "\n" +
			m.helpLine("y  close    any other key  keep it")
	}
	if m.mode != browsing {
		prompt := "new session name"
		if m.mode == renaming {
			prompt = "rename to"
		}
		return sessionStyle.Render(prompt+": ") + m.input.View() + "\n" +
			m.helpLine("enter  confirm    esc  cancel")
	}
	status := ""
	switch m.tone {
	case toneDone:
		status = doneStyle.Render(fit("✓ "+m.status, m.width))
	case toneHint:
		status = hintStyle.Render(fit(m.status, m.width))
	case toneFail:
		status = errorStyle.Render(fit("✗ "+m.status, m.width))
	}
	if m.help {
		return status + "\n\n" + m.helpLine(keyHelp(m.hereName()))
	}
	return status + "\n" + m.helpLine(shortHelp)
}

// helpLine is the bottom line of the screen: the help it offers, with the
// version in the corner. Help given as a block of lines keeps the version on
// the last of them, which is the only line the corner belongs to.
func (m *model) helpLine(help string) string {
	lines := strings.Split(help, "\n")
	last := len(lines) - 1
	lines[last] = fit(lines[last], m.width-lipgloss.Width(version)-2)
	corner := helpStyle.Render(lines[last]) + m.gap(lines[last], version) + helpStyle.Render(version)
	if last == 0 {
		return corner
	}
	return helpStyle.Render(strings.Join(lines[:last], "\n")) + "\n" + corner
}

const shortHelp = "j k move   x cut   d close   r rename   L layout   p P paste   enter go   ? keys"

func keyHelp(here string) string {
	return strings.Join([]string{
		"  j k        move the cursor        x      cut the window, pane or whole session",
		"  h l        fold, unfold           p P    paste after, before the cursor",
		"  J K        move it up, down       S      move what is cut into a new session",
		"  g G        first, last            r      rename the window, pane or session",
		"  space      mark a row or session  d      close it, once y confirms",
		"  enter      go there and close     L      lay out the window's panes",
		"  q esc      quit; esc drops marks  M      merge marked sessions into " + here,
	}, "\n")
}
