// Package ui is the Bubble Tea front end.
package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/owenrumney/container-tui/internal/container"
)

type panelID int

const (
	panelContainers panelID = iota
	panelImages
	panelVolumes
	panelNetworks
	panelCount
)

type detailTab int

const (
	tabLogs detailTab = iota
	tabStats
	tabConfig
	tabShell
	tabCount
)

type confirmAction struct {
	prompt string
	verb   string
	fn     func(context.Context) error
}

type layout struct {
	left, right int
	main        int
	status      int
	panels      [panelCount]int
}

// Model is the root Bubble Tea model.
type Model struct {
	client *container.Client
	width  int
	height int
	ready  bool
	loaded bool

	focus         panelID
	tab           detailTab
	detailFocused bool
	panels        [panelCount]listPanel
	viewport      viewport.Model
	lastDetailKey string

	containers []container.Container
	images     []container.Image
	volumes    []container.Volume
	networks   []container.Network
	df         container.DiskUsage
	system     container.SystemStatus
	refreshing bool

	stats         map[string]statSample
	history       map[string]statHistory
	statsInFlight bool
	statsErr      error

	config    string
	configFor string

	logs        []string
	logsFor     string
	logsRunning bool
	logLines    <-chan string
	logCancel   context.CancelFunc
	logSeq      int
	logWrap     bool
	mouseOff    bool
	detailSeq   int

	shell    *shellSession
	shellFor string
	shellSeq int
	shellErr error

	confirm   *confirmAction
	help      bool
	busy      string
	flashText string
	flashErr  bool
	flashID   int
}

var tabNames = [tabCount]string{"Logs", "Stats", "Config", "Shell"}

// New builds the initial model. The first snapshot is requested from Init.
func New(client *container.Client) Model {
	m := Model{
		client:     client,
		stats:      map[string]statSample{},
		history:    map[string]statHistory{},
		viewport:   viewport.New(0, 0),
		refreshing: true,
	}
	m.panels[panelContainers].title = "Containers"
	m.panels[panelImages].title = "Images"
	m.panels[panelVolumes].title = "Volumes"
	m.panels[panelNetworks].title = "Networks"
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchSnapshot(m.client), tick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.resize()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if t, ok := m.tabAt(msg.X, msg.Y); ok {
				return m, m.selectTab(t)
			}
			if p, row, ok := m.panelAt(msg.X, msg.Y); ok {
				return m, m.clickPanel(p, row)
			}
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	case tickMsg:
		cmds := []tea.Cmd{tick(), m.refreshNow()}
		if m.statsVisible() {
			cmds = append(cmds, m.fetchStats())
		}
		return m, tea.Batch(cmds...)
	case snapshotMsg:
		return m, m.applySnapshot(msg)
	case statsMsg:
		m.statsInFlight = false
		m.statsErr = msg.err
		if msg.err == nil {
			m.applyStats(msg)
		}
		if m.statsVisible() {
			m.renderDetail()
		}
		return m, nil
	case configMsg:
		if msg.key != m.configFor {
			return m, nil
		}
		m.config = msg.body
		if msg.err != nil {
			m.config = errStyle.Render(msg.err.Error())
		}
		m.renderDetail()
		return m, nil
	case logStartedMsg:
		if msg.seq != m.logSeq {
			return m, nil
		}
		if msg.err != nil {
			m.logs = append(m.logs, errStyle.Render(msg.err.Error()))
			m.renderDetail()
			return m, nil
		}
		m.logLines = msg.lines
		return m, waitLogs(msg.lines, msg.seq)
	case logLinesMsg:
		if msg.seq != m.logSeq {
			return m, nil
		}
		m.logs = append(m.logs, msg.lines...)
		if len(m.logs) > maxLogLines {
			m.logs = m.logs[len(m.logs)-maxLogLines:]
		}
		if m.logsVisible() {
			m.renderDetail()
		}
		if msg.done {
			return m, nil
		}
		return m, waitLogs(m.logLines, msg.seq)
	case shellOutputMsg:
		if msg.seq != m.shellSeq || m.shell == nil {
			return m, nil
		}
		if m.shellVisible() {
			m.renderDetail()
		}
		if msg.done {
			return m, nil
		}
		return m, waitShell(m.shell, msg.seq)
	case selectionSettledMsg:
		if msg.seq != m.detailSeq {
			return m, nil
		}
		return m, m.loadDetail()
	case actionDoneMsg:
		m.busy = ""
		flash := m.flash("done: "+msg.verb, false)
		if msg.err != nil {
			flash = m.flash(msg.err.Error(), true)
		}
		return m, tea.Batch(flash, m.refreshNow())
	case execDoneMsg:
		if msg.err != nil {
			return m, m.flash("shell: "+msg.err.Error(), true)
		}
		return m, m.refreshNow()
	case clearFlashMsg:
		if msg.id == m.flashID {
			m.flashText = ""
		}
		return m, nil
	}
	return m, nil
}

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	l := m.layout()
	left := make([]string, 0, panelCount+1)
	left = append(left, m.statusView(l.left, l.status))
	for i := range panelCount {
		focused := m.focus == i && !m.detailFocused
		left = append(left, m.panels[i].view(l.left, l.panels[i], focused, m.loaded))
	}
	right := box(m.detailTitle(), strings.Split(m.viewport.View(), "\n"), l.right, l.main, m.detailFocused)
	main := lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(left, "\n"), right)
	if m.help {
		main = lipgloss.Place(m.width, l.main, lipgloss.Center, lipgloss.Center, helpView())
	}
	return main + "\n" + m.bottomBar()
}

func (m Model) layout() layout {
	l := layout{main: max(m.height-1, 3), status: 5}
	l.left = max(32, m.width/3)
	if l.left > m.width-20 {
		l.left = max(m.width/2, 10)
	}
	l.right = max(m.width-l.left, 0)
	avail := l.main - l.status
	collapsed := 0
	for i := range panelCount {
		if i != m.focus {
			l.panels[i] = max(1, min(len(m.panels[i].rows), 3)) + 2
			collapsed += l.panels[i]
		}
	}
	l.panels[m.focus] = max(avail-collapsed, 3)
	return l
}

func (m *Model) resize() {
	l := m.layout()
	m.viewport.Width = max(l.right-2, 0)
	m.viewport.Height = max(l.main-2, 0)
	if m.shell != nil {
		m.shell.resize(m.shellSize())
	}
	m.rebuildRows()
	m.ensureVisible()
	m.renderDetail()
}

func (m *Model) rebuildRows() {
	width := m.layout().left - 2
	m.panels[panelContainers].setRows(containerRows(m.containers, width))
	m.panels[panelImages].setRows(imageRows(m.images, width))
	m.panels[panelVolumes].setRows(volumeRows(m.volumes, width))
	m.panels[panelNetworks].setRows(networkRows(m.networks, width))
}

func (m *Model) ensureVisible() {
	l := m.layout()
	for i := range panelCount {
		m.panels[i].ensureVisible(l.panels[i] - 2)
	}
}

func (m *Model) applySnapshot(msg snapshotMsg) tea.Cmd {
	m.refreshing = false
	if msg.err != nil {
		m.loaded = true
		m.system = msg.system
		return m.flash(msg.err.Error(), true)
	}
	before := m.detailKey()
	wasRunning := m.logsRunning
	m.loaded = true
	sort.Slice(msg.containers, func(i, j int) bool { return msg.containers[i].ID < msg.containers[j].ID })
	sort.Slice(msg.images, func(i, j int) bool { return msg.images[i].Configuration.Name < msg.images[j].Configuration.Name })
	sort.Slice(msg.volumes, func(i, j int) bool { return msg.volumes[i].Configuration.Name < msg.volumes[j].Configuration.Name })
	sort.Slice(msg.networks, func(i, j int) bool { return msg.networks[i].Configuration.Name < msg.networks[j].Configuration.Name })
	m.containers, m.images, m.volumes, m.networks = msg.containers, msg.images, msg.volumes, msg.networks
	m.df, m.system = msg.df, msg.system
	m.rebuildRows()
	m.ensureVisible()

	if before != m.detailKey() {
		return m.loadDetail()
	}
	if c, ok := m.selectedContainer(); ok && m.logsVisible() && m.logsFor == c.ID && c.Running() != wasRunning {
		cmd := m.startLogs(c)
		m.renderDetail()
		return cmd
	}
	if m.statsVisible() {
		m.renderDetail()
	}
	return nil
}

func (m *Model) applyStats(msg statsMsg) {
	seen := make(map[string]bool, len(msg.items))
	for _, s := range msg.items {
		seen[s.ID] = true
		sample := statSample{stats: s, at: msg.at}
		if prev, ok := m.stats[s.ID]; ok {
			elapsed := msg.at.Sub(prev.at)
			sample = statSample{
				stats:    s,
				at:       msg.at,
				cpuPct:   cpuPercent(prev.stats, s, elapsed),
				netRate:  byteRate(prev.stats.NetworkRxBytes+prev.stats.NetworkTxBytes, s.NetworkRxBytes+s.NetworkTxBytes, elapsed),
				diskRate: byteRate(prev.stats.BlockReadBytes+prev.stats.BlockWriteBytes, s.BlockReadBytes+s.BlockWriteBytes, elapsed),
				hasCPU:   true,
			}
			m.history[s.ID] = m.history[s.ID].push(sample.cpuPct, memPercent(s), sample.netRate, sample.diskRate)
		}
		m.stats[s.ID] = sample
	}
	for id := range m.stats {
		if !seen[id] {
			delete(m.stats, id)
			delete(m.history, id)
		}
	}
}

func (m *Model) refreshNow() tea.Cmd {
	if m.refreshing {
		return nil
	}
	m.refreshing = true
	return fetchSnapshot(m.client)
}

func (m *Model) fetchStats() tea.Cmd {
	if m.statsInFlight {
		return nil
	}
	m.statsInFlight = true
	return fetchStats(m.client)
}

func (m *Model) flash(text string, isErr bool) tea.Cmd {
	m.flashID++
	m.flashText = text
	m.flashErr = isErr
	return clearFlash(m.flashID)
}

func (m Model) statsVisible() bool {
	_, ok := m.selectedContainer()
	return ok && m.focus == panelContainers && m.tab == tabStats
}

func (m Model) logsVisible() bool {
	return m.focus == panelContainers && m.tab == tabLogs
}

func (m Model) selectedContainer() (container.Container, bool) {
	id := m.panels[panelContainers].selectedID()
	for _, c := range m.containers {
		if c.ID == id {
			return c, true
		}
	}
	return container.Container{}, false
}

func (m Model) selectedImage() (container.Image, bool) {
	id := m.panels[panelImages].selectedID()
	for _, img := range m.images {
		if img.Configuration.Name == id {
			return img, true
		}
	}
	return container.Image{}, false
}

// detailKey identifies what the detail pane is showing so changes can be detected.
func (m Model) detailKey() string {
	key := fmt.Sprintf("%d:%s", m.focus, m.panels[m.focus].selectedID())
	if m.focus == panelContainers {
		key += fmt.Sprintf(":%d", m.tab)
	}
	return key
}

func (m *Model) selectionChanged() tea.Cmd {
	m.detailSeq++
	return settle(m.detailSeq)
}

// loadDetail starts whatever fetch the current selection and tab need.
func (m *Model) loadDetail() tea.Cmd {
	var cmd tea.Cmd
	if !m.logsVisible() {
		m.stopLogs()
	}
	if m.shellFor != "" && m.shellFor != m.panels[panelContainers].selectedID() {
		m.stopShell()
	}
	client := m.client
	id := m.panels[m.focus].selectedID()
	switch {
	case id == "":
	case m.focus == panelContainers:
		c, _ := m.selectedContainer()
		switch m.tab {
		case tabLogs:
			if m.logsFor != c.ID || m.logsRunning != c.Running() {
				cmd = m.startLogs(c)
			}
		case tabStats:
			cmd = m.fetchStats()
		case tabConfig:
			cmd = m.loadConfig("container:"+id, func(ctx context.Context) (string, error) { return client.InspectContainer(ctx, id) })
		case tabShell:
			if m.shell == nil && m.shellErr == nil {
				cmd = m.startShell(c)
			}
		}
	case m.focus == panelImages:
		cmd = m.loadConfig("image:"+id, func(ctx context.Context) (string, error) { return client.InspectImage(ctx, id) })
	case m.focus == panelVolumes:
		cmd = m.loadConfig("volume:"+id, func(ctx context.Context) (string, error) { return client.InspectVolume(ctx, id) })
	case m.focus == panelNetworks:
		cmd = m.loadConfig("network:"+id, func(ctx context.Context) (string, error) { return client.InspectNetwork(ctx, id) })
	}
	m.renderDetail()
	return cmd
}

func (m *Model) loadConfig(key string, inspect func(context.Context) (string, error)) tea.Cmd {
	if m.configFor == key {
		return nil
	}
	m.configFor = key
	m.config = ""
	return fetchConfig(key, inspect)
}

func (m *Model) startLogs(c container.Container) tea.Cmd {
	m.stopLogs()
	m.logSeq++
	m.logs = nil
	m.logsFor = c.ID
	m.logsRunning = c.Running()
	ctx, cancel := context.WithCancel(context.Background())
	m.logCancel = cancel
	return startLogs(ctx, m.client, c.ID, m.logSeq)
}

func (m *Model) stopLogs() {
	if m.logCancel != nil {
		m.logCancel()
		m.logCancel = nil
	}
	m.logSeq++
	m.logsFor = ""
	m.logLines = nil
}

// shellSize leaves the bottom pane row for renderShell's status line.
func (m Model) shellSize() (int, int) {
	return max(m.viewport.Width, 1), max(m.viewport.Height-1, 1)
}

func (m Model) shellVisible() bool {
	return m.focus == panelContainers && m.tab == tabShell
}

// shellFocused reports whether key presses belong to the shell rather than the UI.
func (m Model) shellFocused() bool {
	if !m.detailFocused || !m.shellVisible() || m.shell == nil {
		return false
	}
	done, _ := m.shell.status()
	return !done
}

func (m *Model) startShell(c container.Container) tea.Cmd {
	m.stopShell()
	if m.client == nil || !c.Running() {
		return nil
	}
	w, h := m.shellSize()
	s, err := newShellSession(m.client.ExecCommand(c.ID, "/bin/sh", "-c", shellProbe), w, h)
	if err != nil {
		m.shellErr = err
		return nil
	}
	m.shellSeq++
	m.shell = s
	m.shellFor = c.ID
	return waitShell(s, m.shellSeq)
}

func (m *Model) stopShell() {
	if m.shell != nil {
		m.shell.close()
		m.shell = nil
	}
	m.shellFor = ""
	m.shellErr = nil
	m.shellSeq++
}

func (m *Model) shutdown() {
	m.stopLogs()
	m.stopShell()
}

// renderDetail pushes the current selection's content into the viewport.
func (m *Model) renderDetail() {
	width := m.viewport.Width
	content := dimStyle.Render("nothing selected")
	if m.panels[m.focus].selectedID() != "" {
		switch {
		case m.focus != panelContainers || m.tab == tabConfig:
			content = renderConfig(m.config, width)
		case m.tab == tabLogs:
			content = renderLogs(m.logs, width, m.logWrap)
		case m.tab == tabShell:
			c, _ := m.selectedContainer()
			content = renderShell(m.shell, m.shellErr, c, width, m.viewport.Height)
		case m.tab == tabStats:
			c, _ := m.selectedContainer()
			var sample *statSample
			if s, ok := m.stats[c.ID]; ok {
				sample = &s
			}
			content = renderStats(c, sample, m.history[c.ID], width, m.viewport.Height, time.Now())
			if m.statsErr != nil {
				content += "\n\n" + errStyle.Render(m.statsErr.Error())
			}
		}
	}
	key := m.detailKey()
	atBottom := m.viewport.AtBottom()
	m.viewport.SetContent(content)
	switch {
	case key != m.lastDetailKey:
		m.lastDetailKey = key
		if m.logsVisible() {
			m.viewport.GotoBottom()
		} else {
			m.viewport.GotoTop()
		}
	case m.logsVisible() && atBottom:
		m.viewport.GotoBottom()
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.shellFocused() {
		return m.handleShellKey(msg)
	}
	key := msg.String()
	if key == "ctrl+c" {
		m.shutdown()
		return m, tea.Quit
	}
	if m.confirm != nil {
		c := m.confirm
		m.confirm = nil
		if key == "y" || key == "Y" || key == "enter" {
			m.busy = c.verb
			return m, runAction(c.verb, c.fn)
		}
		return m, nil
	}
	if m.help {
		m.help = false
		return m, nil
	}
	if m.detailFocused {
		return m.handleDetailKey(msg)
	}
	switch key {
	case "q":
		m.shutdown()
		return m, tea.Quit
	case "?":
		m.help = true
		return m, nil
	case "tab", "right", "l":
		return m, m.setFocus((m.focus + 1) % panelCount)
	case "shift+tab", "left", "h":
		return m, m.setFocus((m.focus + panelCount - 1) % panelCount)
	case "1", "2", "3", "4":
		return m, m.setFocus(panelID(key[0] - '1'))
	case "R", "ctrl+r":
		return m, m.refreshNow()
	case "j", "down":
		return m, m.moveCursor(1)
	case "k", "up":
		return m, m.moveCursor(-1)
	case "g", "home":
		return m, m.moveCursor(-len(m.panels[m.focus].rows))
	case "G", "end":
		return m, m.moveCursor(len(m.panels[m.focus].rows))
	case "pgdown", "ctrl+d":
		return m, m.moveCursor(m.pageSize())
	case "pgup", "ctrl+u":
		return m, m.moveCursor(-m.pageSize())
	case "enter":
		if m.panels[m.focus].selectedID() == "" {
			return m, nil
		}
		m.detailFocused = true
		return m, m.restartShellIfDone()
	case "[":
		return m, m.switchTab(-1)
	case "]":
		return m, m.switchTab(1)
	case "w":
		return m.toggleLogWrap()
	case "v":
		return m.toggleMouse()
	}
	return m.handleAction(key)
}

// handleShellKey forwards every press to the PTY, so the shell owns ctrl+c and q.
func (m Model) handleShellKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == shellDetachKey {
		m.detailFocused = false
		return m, nil
	}
	m.shell.write(keyBytes(msg))
	return m, nil
}

// restartShellIfDone relaunches an exited shell so enter brings it back.
func (m *Model) restartShellIfDone() tea.Cmd {
	if !m.shellVisible() {
		return nil
	}
	done := true
	if m.shell != nil {
		done, _ = m.shell.status()
	}
	if !done {
		return nil
	}
	c, ok := m.selectedContainer()
	if !ok {
		return nil
	}
	cmd := m.startShell(c)
	m.renderDetail()
	return cmd
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if cmd := m.restartShellIfDone(); cmd != nil {
			return m, cmd
		}
		m.detailFocused = false
		return m, nil
	case "esc", "tab", "shift+tab":
		m.detailFocused = false
		return m, nil
	case "q":
		m.shutdown()
		return m, tea.Quit
	case "[":
		return m, m.switchTab(-1)
	case "]":
		return m, m.switchTab(1)
	case "g":
		m.viewport.GotoTop()
		return m, nil
	case "G":
		m.viewport.GotoBottom()
		return m, nil
	case "w":
		return m.toggleLogWrap()
	case "v":
		return m.toggleMouse()
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// toggleMouse releases the mouse so the terminal can select text, since mouse
// reporting takes precedence over the terminal's own selection.
func (m Model) toggleMouse() (tea.Model, tea.Cmd) {
	m.mouseOff = !m.mouseOff
	if m.mouseOff {
		return m, tea.Batch(tea.DisableMouse, m.flash("mouse released — select text as usual, v to re-enable", false))
	}
	return m, tea.Batch(tea.EnableMouseCellMotion, m.flash("mouse enabled", false))
}

func (m Model) toggleLogWrap() (tea.Model, tea.Cmd) {
	if !m.logsVisible() {
		return m, nil
	}
	m.logWrap = !m.logWrap
	m.renderDetail()
	return m, nil
}

func (m Model) pageSize() int {
	return max(m.layout().panels[m.focus]-2, 1)
}

func (m *Model) moveCursor(n int) tea.Cmd {
	p := &m.panels[m.focus]
	before := p.selectedID()
	p.move(n)
	p.ensureVisible(m.layout().panels[m.focus] - 2)
	if p.selectedID() == before {
		return nil
	}
	return m.selectionChanged()
}

func (m *Model) setFocus(f panelID) tea.Cmd {
	if f == m.focus {
		return nil
	}
	m.focus = f
	m.detailFocused = false
	m.ensureVisible()
	return m.selectionChanged()
}

func (m *Model) switchTab(n int) tea.Cmd {
	if m.focus != panelContainers {
		return nil
	}
	return m.selectTab((m.tab + detailTab(n) + tabCount) % tabCount)
}

func (m *Model) selectTab(t detailTab) tea.Cmd {
	if m.tab == t {
		return nil
	}
	m.tab = t
	return m.loadDetail()
}

// panelAt maps a click in the left column to a panel, and to the row under the
// pointer. A row of -1 means the click landed on the panel's border or title.
func (m Model) panelAt(x, y int) (panelID, int, bool) {
	l := m.layout()
	if x < 0 || x >= l.left || y < l.status {
		return 0, -1, false
	}
	top := l.status
	for i := range panelCount {
		h := l.panels[i]
		if y >= top+h {
			top += h
			continue
		}
		p := m.panels[i]
		visible := h - 2
		j := y - top - 1 // the title row sits above the first body row
		if j < 0 || j >= visible || !m.loaded {
			return panelID(i), -1, true
		}
		idx := visibleOffset(p.offset, p.cursor, len(p.rows), visible) + j
		if idx >= len(p.rows) {
			return panelID(i), -1, true
		}
		return panelID(i), idx, true
	}
	return 0, -1, false
}

// clickPanel focuses the panel and moves its cursor to the clicked row.
func (m *Model) clickPanel(p panelID, row int) tea.Cmd {
	cmds := []tea.Cmd{m.setFocus(p)}
	if row >= 0 && row != m.panels[p].cursor {
		m.panels[p].cursor = row
		m.panels[p].ensureVisible(m.layout().panels[p] - 2)
		cmds = append(cmds, m.selectionChanged())
	}
	return tea.Batch(cmds...)
}

// tabAt maps a click to a tab heading in the detail pane's title row, using the
// same offsets box() and detailTitle() lay the title out with.
func (m Model) tabAt(x, y int) (detailTab, bool) {
	name := m.panels[panelContainers].selectedID()
	if y != 0 || m.focus != panelContainers || name == "" {
		return 0, false
	}
	l := m.layout()
	origin := l.left + 3       // box corner, border, then the label's leading space
	limit := max(l.right-5, 0) // box() truncates the title to this
	off := ansi.StringWidth(name) + 3
	for i, t := range tabNames {
		w := ansi.StringWidth(t)
		if off+w > limit {
			return 0, false
		}
		if x >= origin+off && x < origin+off+w {
			return detailTab(i), true
		}
		off += w + 3 // the " │ " separator
	}
	return 0, false
}

func (m Model) handleAction(key string) (tea.Model, tea.Cmd) {
	client := m.client
	switch m.focus {
	case panelContainers:
		return m.containerAction(key)
	case panelImages:
		switch key {
		case "p":
			m.confirm = &confirmAction{prompt: "Remove dangling images?", verb: "prune images", fn: func(ctx context.Context) error { return client.PruneImages(ctx, false) }}
		case "P":
			m.confirm = &confirmAction{prompt: "Remove ALL unused images?", verb: "prune all images", fn: func(ctx context.Context) error { return client.PruneImages(ctx, true) }}
		case "d":
			if img, ok := m.selectedImage(); ok {
				name := img.Configuration.Name
				m.confirm = &confirmAction{prompt: "Delete image " + container.ShortImageName(name) + "?", verb: "delete " + container.ShortImageName(name), fn: func(ctx context.Context) error { return client.DeleteImage(ctx, name) }}
			}
		}
	case panelVolumes:
		name := m.panels[panelVolumes].selectedID()
		switch {
		case key == "p":
			m.confirm = &confirmAction{prompt: "Remove unused volumes?", verb: "prune volumes", fn: client.PruneVolumes}
		case key == "d" && name != "":
			m.confirm = &confirmAction{prompt: "Delete volume " + name + "?", verb: "delete " + name, fn: func(ctx context.Context) error { return client.DeleteVolume(ctx, name) }}
		}
	case panelNetworks:
		name := m.panels[panelNetworks].selectedID()
		switch {
		case key == "p":
			m.confirm = &confirmAction{prompt: "Remove unused networks?", verb: "prune networks", fn: client.PruneNetworks}
		case key == "d" && name != "":
			m.confirm = &confirmAction{prompt: "Delete network " + name + "?", verb: "delete " + name, fn: func(ctx context.Context) error { return client.DeleteNetwork(ctx, name) }}
		}
	}
	return m, nil
}

func (m Model) containerAction(key string) (tea.Model, tea.Cmd) {
	client := m.client
	if key == "p" {
		m.confirm = &confirmAction{prompt: "Remove all stopped containers?", verb: "prune containers", fn: client.PruneContainers}
		return m, nil
	}
	c, ok := m.selectedContainer()
	if !ok {
		return m, nil
	}
	id := c.ID
	running := c.Running()
	switch key {
	case "s":
		if !running {
			return m, m.flash(id+" is not running", true)
		}
		return m, m.action("stop "+id, func(ctx context.Context) error { return client.StopContainer(ctx, id) })
	case "S":
		if running {
			return m, m.flash(id+" is already running", true)
		}
		return m, m.action("start "+id, func(ctx context.Context) error { return client.StartContainer(ctx, id) })
	case "r":
		return m, m.action("restart "+id, func(ctx context.Context) error {
			if running {
				if err := client.StopContainer(ctx, id); err != nil {
					return err
				}
			}
			return client.StartContainer(ctx, id)
		})
	case "K":
		if !running {
			return m, m.flash(id+" is not running", true)
		}
		return m, m.action("kill "+id, func(ctx context.Context) error { return client.KillContainer(ctx, id) })
	case "d":
		prompt := "Delete container " + id + "?"
		if running {
			prompt = "Force delete RUNNING container " + id + "?"
		}
		m.confirm = &confirmAction{prompt: prompt, verb: "delete " + id, fn: func(ctx context.Context) error { return client.DeleteContainer(ctx, id, running) }}
		return m, nil
	case "e":
		if !running {
			return m, m.flash(id+" is not running", true)
		}
		cmd := client.ExecCommand(id, "/bin/sh", "-c", shellProbe)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return execDoneMsg{err: err} })
	}
	return m, nil
}

func (m *Model) action(verb string, fn func(context.Context) error) tea.Cmd {
	m.busy = verb
	return runAction(verb, fn)
}

func (m Model) statusView(w, h int) string {
	var line1 string
	if m.system.Running {
		line1 = greenStyle.Render("●") + " running" + dimStyle.Render("  container "+m.system.Version)
	} else {
		line1 = redStyle.Render("●") + " stopped" + dimStyle.Render("  run: container system start")
	}
	df := m.df
	line2 := dimStyle.Render(fmt.Sprintf("images %s · %s reclaimable", humanBytes(df.Images.SizeInBytes), humanBytes(df.Images.Reclaimable)))
	line3 := dimStyle.Render(fmt.Sprintf("containers %s · volumes %s", humanBytes(df.Containers.SizeInBytes), humanBytes(df.Volumes.SizeInBytes)))
	if !m.loaded {
		line1 = dimStyle.Render("loading…")
		line2, line3 = "", ""
	}
	return box("Status", []string{line1, line2, line3}, w, h, false)
}

func (m Model) detailTitle() string {
	name := m.panels[m.focus].selectedID()
	if name == "" {
		return "Detail"
	}
	if m.focus != panelContainers {
		return name + " · " + tabActiveStyle.Render("Config")
	}
	tabs := make([]string, 0, tabCount)
	for i, t := range tabNames {
		if detailTab(i) == m.tab {
			tabs = append(tabs, tabActiveStyle.Render(t))
		} else {
			tabs = append(tabs, dimStyle.Render(t))
		}
	}
	return name + " · " + strings.Join(tabs, dimStyle.Render(" │ "))
}

func (m Model) bottomBar() string {
	var text string
	switch {
	case m.confirm != nil:
		text = warnStyle.Render(m.confirm.prompt + "  [y/n]")
	case m.busy != "":
		text = keyStyle.Render("⟳ " + m.busy + "…")
	case m.flashText != "" && m.flashErr:
		text = errStyle.Render("✗ " + m.flashText)
	case m.flashText != "":
		text = okStyle.Render("✓ " + m.flashText)
	default:
		text = renderHints(m.hints())
	}
	return pad(" "+text, m.width)
}
