package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	browsing mode = iota
	renaming
	namingSession
	confirming
)

// tone tells the footer whether the status line reports something that
// happened, something the key could not do, or a failure tmux reported.
type tone int

const (
	toneQuiet tone = iota
	toneDone
	toneHint
	toneFail
)

type model struct {
	tmux      tmux
	client    string
	here      string
	sessions  []session
	rows      []row
	cursor    int
	top       int
	width     int
	height    int
	marked    map[string]bool
	clip      []item
	doomed    []item
	layout    *layout
	collapsed map[string]bool
	expanded  map[string]bool
	status    string
	tone      tone
	mode      mode
	input     textinput.Model
	help      bool
	// holdsTitles is false on tmux older than 3.5, where a pane cannot be named.
	holdsTitles bool
}

func newModel(t tmux, client string) (*model, error) {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 60
	m := &model{
		tmux:        t,
		client:      client,
		here:        t.sessionOf(client),
		holdsTitles: t.holdsTitles(),
		marked:      map[string]bool{},
		collapsed:   map[string]bool{},
		expanded:    map[string]bool{},
		width:       80,
		height:      24,
		input:       in,
	}
	sessions, err := t.tree()
	if err != nil {
		return nil, err
	}
	m.sessions = sessions
	m.rebuild()
	m.focusHere()
	return m, nil
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) rebuild() {
	rows := []row{}
	for s := range m.sessions {
		sess := &m.sessions[s]
		rows = append(rows, row{kind: sessionRow, session: sess})
		if m.collapsed[sess.id] {
			continue
		}
		for w := range sess.windows {
			win := &sess.windows[w]
			rows = append(rows, row{kind: windowRow, session: sess, window: win})
			if !m.expanded[win.id] {
				continue
			}
			for p := range win.panes {
				rows = append(rows, row{kind: paneRow, session: sess, window: win, pane: &win.panes[p]})
			}
		}
	}
	cut := map[string]bool{}
	for _, it := range m.clip {
		cut[it.id] = true
	}
	for i := range rows {
		id := rows[i].id()
		rows[i].marked = m.marked[id]
		rows[i].cut = cut[id]
	}
	m.rows = rows
	if m.cursor >= len(rows) {
		m.cursor = len(rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *model) reload(focus string) {
	sessions, err := m.tmux.tree()
	if err != nil {
		m.fail(err)
		return
	}
	m.sessions = sessions
	m.forgetGone()
	m.rebuild()
	m.focus(focus)
}

// forgetGone drops marks and cuts for anything tmux no longer has, so that
// closing something does not leave a clipboard tmux will refuse to paste.
func (m *model) forgetGone() {
	live := map[string]bool{}
	for _, sess := range m.sessions {
		live[sess.id] = true
		for _, w := range sess.windows {
			live[w.id] = true
			for _, p := range w.panes {
				live[p.id] = true
			}
		}
	}
	var clip []item
	for _, it := range m.clip {
		if live[it.id] {
			clip = append(clip, it)
		}
	}
	m.clip = clip
	for id := range m.marked {
		if !live[id] {
			delete(m.marked, id)
		}
	}
}

func (m *model) focus(id string) {
	for i, r := range m.rows {
		if r.id() == id {
			m.cursor = i
			return
		}
	}
}

func (m *model) focusHere() {
	for i, r := range m.rows {
		if r.kind == windowRow && r.session.id == m.here && r.window.active {
			m.cursor = i
			return
		}
	}
}

func (m *model) fail(err error) {
	m.status, m.tone = err.Error(), toneFail
}

func (m *model) done(format string, args ...any) {
	m.status, m.tone = fmt.Sprintf(format, args...), toneDone
}

func (m *model) hint(format string, args ...any) {
	m.status, m.tone = fmt.Sprintf(format, args...), toneHint
}

func (m *model) clear() {
	m.status, m.tone = "", toneQuiet
}

func (m *model) current() row { return m.rows[m.cursor] }

// sessionNamed is the id of a session tmux has just been told to make, which
// the model knows only by the name it asked for.
func (m *model) sessionNamed(name string) string {
	for i := range m.sessions {
		if m.sessions[i].name == name {
			return m.sessions[i].id
		}
	}
	return ""
}

func (m *model) sessionAt(id string) *session {
	for i := range m.sessions {
		if m.sessions[i].id == id {
			return &m.sessions[i]
		}
	}
	return nil
}

// hereName is the name of the session the popup was opened from, for the help
// line and the messages that talk about it.
func (m *model) hereName() string {
	if sess := m.sessionAt(m.here); sess != nil {
		return sess.name
	}
	return m.here
}

// marks lists the marked rows, each once. A row another mark already takes with
// it is left out — a window or pane inside a marked session, a pane inside a
// marked window — and a window linked into two sessions is marked in both.
func (m *model) marks() []row {
	var rows []row
	seen := map[string]bool{}
	for _, r := range m.rows {
		if !r.marked || seen[r.id()] {
			continue
		}
		if r.kind != sessionRow && m.marked[r.session.id] {
			continue
		}
		if r.kind == paneRow && m.marked[r.window.id] {
			continue
		}
		seen[r.id()] = true
		rows = append(rows, r)
	}
	return rows
}

// selection is what an action works on: every marked row, or the row under the
// cursor when nothing is marked. A session stands for all of its windows.
func (m *model) selection() []item {
	var items []item
	for _, r := range m.marks() {
		if r.kind == sessionRow {
			items = append(items, m.itemsOfSession(r.session.id)...)
			continue
		}
		items = append(items, itemsOf(r)...)
	}
	if len(items) > 0 {
		return items
	}
	if cursor := m.current(); cursor.kind == sessionRow {
		return m.itemsOfSession(cursor.session.id)
	}
	return itemsOf(m.current())
}

func itemsOf(r row) []item {
	switch r.kind {
	case windowRow:
		return []item{{kind: windowRow, id: r.window.id, label: r.window.name}}
	case paneRow:
		return []item{{kind: paneRow, id: r.pane.id, label: r.label()}}
	}
	return nil
}

func (m *model) itemsOfSession(id string) []item {
	sess := m.sessionAt(id)
	if sess == nil {
		return nil
	}
	var items []item
	for _, w := range sess.windows {
		items = append(items, item{kind: windowRow, id: w.id, label: w.name})
	}
	return items
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.layout != nil {
			return m.layoutKey(msg)
		}
		switch m.mode {
		case browsing:
			return m.browseKey(msg)
		case confirming:
			return m.confirmKey(msg)
		}
		return m.editKey(msg)
	}
	return m, nil
}

func (m *model) editKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.mode = browsing
		m.clear()
		return m, nil
	case tea.KeyEnter:
		name := strings.TrimSpace(m.input.Value())
		mode := m.mode
		m.mode = browsing
		if mode == namingSession {
			if name != "" {
				m.moveToNewSession(name)
			}
			return m, nil
		}
		m.rename(name)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// confirmKey answers the close prompt. Only y goes through with it, so a key
// pressed at the wrong moment never kills a window.
func (m *model) confirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.doomed
	m.mode, m.doomed = browsing, nil
	if key := msg.String(); key != "y" && key != "Y" {
		m.hint("nothing closed")
		return m, nil
	}
	m.kill(items)
	return m, nil
}

func (m *model) browseKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.rows) == 0 {
		return m, tea.Quit
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if len(m.clip) > 0 || len(m.marked) > 0 {
			m.clip, m.marked = nil, map[string]bool{}
			m.rebuild()
			m.clear()
			return m, nil
		}
		return m, tea.Quit
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "ctrl+d":
		m.move(m.viewHeight() / 2)
	case "ctrl+u":
		m.move(-m.viewHeight() / 2)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.rows) - 1
	case "l", "right", "tab":
		m.open()
	case "h", "left":
		m.close()
	case " ":
		m.mark()
	case "x":
		m.cut()
	case "d":
		m.askClose()
	case "p":
		m.paste(false)
	case "P":
		m.paste(true)
	case "J":
		m.shift(true)
	case "K":
		m.shift(false)
	case "M":
		m.merge()
	case "S":
		m.askNewSession()
	case "r":
		m.askRename()
	case "L":
		m.openLayout()
	case "enter":
		r := m.current()
		window, pane := "", ""
		if r.window != nil {
			window = r.window.id
		}
		if r.pane != nil {
			pane = r.pane.id
		}
		if err := m.tmux.switchTo(m.client, r.session.id, window, pane); err != nil {
			m.fail(err)
			return m, nil
		}
		return m, tea.Quit
	case "?":
		m.help = !m.help
	}
	return m, nil
}

func (m *model) move(by int) {
	m.cursor += by
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
}

func (m *model) open() {
	r := m.current()
	switch r.kind {
	case sessionRow:
		delete(m.collapsed, r.session.id)
	case windowRow:
		m.expanded[r.window.id] = true
	}
	m.rebuild()
}

func (m *model) close() {
	r := m.current()
	switch r.kind {
	case sessionRow:
		m.collapsed[r.session.id] = true
	case windowRow:
		if !m.expanded[r.window.id] {
			m.focus(r.session.id)
			return
		}
		delete(m.expanded, r.window.id)
	case paneRow:
		m.focus(r.window.id)
		return
	}
	m.rebuild()
}

func (m *model) mark() {
	id := m.current().id()
	if m.marked[id] {
		delete(m.marked, id)
	} else {
		m.marked[id] = true
	}
	m.rebuild()
	m.move(1)
}

func (m *model) cut() {
	items := m.selection()
	if len(items) == 0 {
		m.hint("nothing to cut")
		return
	}
	m.clip = items
	m.marked = map[string]bool{}
	m.rebuild()
	m.done("cut %s — p pastes after the cursor, P before it", describe(items))
}

func (m *model) paste(before bool) {
	if len(m.clip) == 0 {
		m.hint("nothing cut — x cuts the window or pane under the cursor")
		return
	}
	dst := m.current()
	cmds := pasteCommands(m.clip, dst, before)
	if len(cmds) == 0 {
		m.hint("already there")
		return
	}
	moved := describe(m.clip)
	focus := m.clip[0].id
	if err := m.tmux.apply(cmds); err != nil {
		m.fail(err)
		return
	}
	if dst.window != nil && holdsPane(m.clip) {
		m.expanded[dst.window.id] = true
	}
	m.clip = nil
	m.reload(focus)
	m.done("moved %s to %s", moved, dst.label())
}

func (m *model) shift(down bool) {
	r := m.current()
	switch r.kind {
	case sessionRow:
		m.hint("sessions are listed by name — rename one to move it")
	case windowRow:
		other := m.neighbour(r, down)
		if other == "" {
			m.hint("no window that way")
			return
		}
		if _, err := m.tmux.run("swap-window", "-d", "-s", r.window.id, "-t", other); err != nil {
			m.fail(err)
			return
		}
		m.reload(r.window.id)
		m.clear()
	case paneRow:
		side := "-U"
		if down {
			side = "-D"
		}
		if _, err := m.tmux.run("swap-pane", "-d", side, "-t", r.pane.id); err != nil {
			m.fail(err)
			return
		}
		m.reload(r.pane.id)
		m.clear()
	}
}

func (m *model) neighbour(r row, down bool) string {
	sess := m.sessionAt(r.session.id)
	if sess == nil {
		return ""
	}
	for i, w := range sess.windows {
		if w.id != r.window.id {
			continue
		}
		next := i - 1
		if down {
			next = i + 1
		}
		if next < 0 || next >= len(sess.windows) {
			return ""
		}
		return sess.windows[next].id
	}
	return ""
}

// mergeable is what M folds into here: every marked session, or the one under
// the cursor when none is marked, and never here itself.
func (m *model) mergeable() []*session {
	var sessions []*session
	marked := false
	for _, r := range m.marks() {
		if r.kind != sessionRow {
			continue
		}
		marked = true
		if r.session.id != m.here {
			sessions = append(sessions, r.session)
		}
	}
	if marked {
		return sessions
	}
	if r := m.current(); r.session.id != m.here {
		return []*session{r.session}
	}
	return nil
}

func (m *model) merge() {
	sessions := m.mergeable()
	if len(sessions) == 0 {
		m.hint("%s is the session you came from", m.hereName())
		return
	}
	var items []item
	windows, what := 0, sessions[0].name
	for _, sess := range sessions {
		items = append(items, m.itemsOfSession(sess.id)...)
		windows += len(sess.windows)
	}
	if len(sessions) > 1 {
		what = plural(len(sessions), "session")
	}
	if err := m.tmux.apply(appendCommands(items, m.here)); err != nil {
		m.fail(err)
		return
	}
	m.marked = map[string]bool{}
	m.reload(m.here)
	m.done("merged %s of %s into %s", plural(windows, "window"), what, m.hereName())
}

// closing is what d acts on: every marked row, or the row under the cursor when
// nothing is marked. A session stands for itself here rather than for its
// windows, which tmux closes with it.
func (m *model) closing() []item {
	var items []item
	for _, r := range m.marks() {
		items = append(items, item{kind: r.kind, id: r.id(), label: r.label()})
	}
	if len(items) > 0 {
		return items
	}
	r := m.current()
	return []item{{kind: r.kind, id: r.id(), label: r.label()}}
}

func (m *model) askClose() {
	items := m.closing()
	for _, it := range items {
		if it.kind == sessionRow && it.id == m.here {
			m.hint("%s is the session you came from", m.hereName())
			return
		}
	}
	m.doomed = items
	m.mode = confirming
	m.clear()
}

func (m *model) kill(items []item) {
	focus := m.survivor(items)
	if err := m.tmux.apply(killCommands(items)); err != nil {
		m.fail(err)
		m.reload(focus)
		return
	}
	m.marked = map[string]bool{}
	m.reload(focus)
	m.done("closed %s", describe(items))
}

// survivor names the row to leave the cursor on once these are gone: the first
// row below it that outlives them, or else the nearest one above.
func (m *model) survivor(items []item) string {
	dead := map[string]bool{}
	for _, it := range items {
		dead[it.id] = true
	}
	lives := func(r row) bool {
		if dead[r.id()] || dead[r.session.id] {
			return false
		}
		return r.window == nil || !dead[r.window.id]
	}
	for i := m.cursor + 1; i < len(m.rows); i++ {
		if lives(m.rows[i]) {
			return m.rows[i].id()
		}
	}
	for i := m.cursor - 1; i >= 0; i-- {
		if lives(m.rows[i]) {
			return m.rows[i].id()
		}
	}
	return ""
}

func (m *model) askNewSession() {
	if len(m.clip) == 0 {
		m.clip = m.selection()
		m.marked = map[string]bool{}
		m.rebuild()
	}
	if len(m.clip) == 0 {
		m.hint("nothing to move")
		return
	}
	m.mode = namingSession
	m.input.SetValue("")
	m.input.Focus()
}

func (m *model) moveToNewSession(name string) {
	clip := m.clip
	if err := m.tmux.newSession(name, clip); err != nil {
		m.fail(err)
		return
	}
	m.clip = nil
	m.reload(m.sessionNamed(name))
	m.done("moved %s into a new session %s", describe(clip), name)
}

func (m *model) askRename() {
	r := m.current()
	if r.kind == paneRow && !m.holdsTitles {
		m.hint("naming a pane needs tmux 3.5")
		return
	}
	name := r.label()
	if r.kind == paneRow {
		name = r.pane.title
	}
	m.mode = renaming
	m.input.SetValue(name)
	m.input.CursorEnd()
	m.input.Focus()
}

// rename gives the row the typed name. A pane is named by its title, which an
// empty name clears, leaving the pane to read as whatever runs in it; a session
// or a window keeps the name it has rather than being left nameless.
func (m *model) rename(name string) {
	r := m.current()
	if name == "" && r.kind != paneRow {
		return
	}
	target, err := r.id(), error(nil)
	switch r.kind {
	case sessionRow:
		_, err = m.tmux.run("rename-session", "-t", r.session.id, name)
	case windowRow:
		_, err = m.tmux.run("rename-window", "-t", r.window.id, name)
	default:
		err = m.tmux.namePane(r.pane.id, name)
	}
	if err != nil {
		m.fail(err)
		return
	}
	m.reload(target)
	if name == "" {
		m.done("cleared the name of the pane")
		return
	}
	m.done("renamed to %s", name)
}

// holdsPane says whether pasting will land a pane inside a window, which the
// window has to be unfolded to show.
func holdsPane(items []item) bool {
	for _, it := range items {
		if it.kind == paneRow {
			return true
		}
	}
	return false
}

func describe(items []item) string {
	if len(items) == 1 {
		return items[0].label
	}
	sessions, windows, panes := 0, 0, 0
	for _, it := range items {
		switch it.kind {
		case sessionRow:
			sessions++
		case paneRow:
			panes++
		default:
			windows++
		}
	}
	var parts []string
	if sessions > 0 {
		parts = append(parts, plural(sessions, "session"))
	}
	if windows > 0 {
		parts = append(parts, plural(windows, "window"))
	}
	if panes > 0 {
		parts = append(parts, plural(panes, "pane"))
	}
	if len(parts) < 3 {
		return strings.Join(parts, " and ")
	}
	return parts[0] + ", " + parts[1] + " and " + parts[2]
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
