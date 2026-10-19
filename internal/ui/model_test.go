package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/owenrumney/container-tui/internal/container"
)

func fixture[T any](t *testing.T, name string) T {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "container", "testdata", name))
	require.NoError(t, err)
	var v T
	require.NoError(t, json.Unmarshal(data, &v))
	return v
}

func testSnapshot(t *testing.T) snapshotMsg {
	t.Helper()
	return snapshotMsg{
		containers: fixture[[]container.Container](t, "containers.json"),
		images:     fixture[[]container.Image](t, "images.json"),
		volumes:    fixture[[]container.Volume](t, "volumes.json"),
		networks:   fixture[[]container.Network](t, "networks.json"),
		df:         fixture[container.DiskUsage](t, "df.json"),
		system:     container.SystemStatus{Running: true, Version: "1.3.1"},
	}
}

func sized(t *testing.T, w, h int) Model {
	t.Helper()
	m := New(nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = next.(Model)
	next, _ = m.Update(testSnapshot(t))
	return next.(Model)
}

func key(m Model, k string) Model {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	switch k {
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	}
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestLayoutFillsScreen(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {200, 60}} {
		m := sized(t, size[0], size[1])
		l := m.layout()
		assert.Equal(t, size[0], l.left+l.right)
		total := l.status
		for _, h := range l.panels {
			total += h
		}
		assert.Equal(t, l.main, total, "left column height must match main area")
		assert.Equal(t, size[1]-1, l.main)
		assert.GreaterOrEqual(t, l.panels[m.focus], l.panels[panelImages], "focused panel is the largest")
	}
}

func TestViewDimensions(t *testing.T) {
	m := sized(t, 120, 40)
	lines := strings.Split(m.View(), "\n")
	require.Len(t, lines, 40)
	for i, l := range lines {
		assert.Equal(t, 120, ansi.StringWidth(l), "line %d width", i)
	}
	plain := ansi.Strip(m.View())
	assert.Contains(t, plain, "Containers (1)")
	assert.Contains(t, plain, "Images (2)")
	assert.Contains(t, plain, "buildkit")
	assert.Contains(t, plain, "container 1.3.1")
	assert.Contains(t, plain, "Logs")
}

func TestFocusAndTabs(t *testing.T) {
	m := sized(t, 120, 40)
	assert.Equal(t, panelContainers, m.focus)
	m = key(m, "tab")
	assert.Equal(t, panelImages, m.focus)
	m = key(m, "4")
	assert.Equal(t, panelNetworks, m.focus)
	m = key(m, "tab")
	assert.Equal(t, panelContainers, m.focus)

	m = key(m, "]")
	assert.Equal(t, tabStats, m.tab)
	m = key(m, "[")
	m = key(m, "[")
	assert.Equal(t, tabShell, m.tab) // wraps past Logs to the last tab
	m = key(m, "[")
	assert.Equal(t, tabConfig, m.tab)

	m = key(m, "enter")
	assert.True(t, m.detailFocused)
	m = key(m, "esc")
	assert.False(t, m.detailFocused)
}

func TestSelectionSurvivesRefresh(t *testing.T) {
	m := sized(t, 120, 40)
	m = key(m, "tab")
	m = key(m, "j")
	assert.Equal(t, 1, m.panels[panelImages].cursor)
	selected := m.panels[panelImages].selectedID()

	snap := testSnapshot(t)
	snap.images[0], snap.images[1] = snap.images[1], snap.images[0]
	next, _ := m.Update(snap)
	m = next.(Model)
	assert.Equal(t, selected, m.panels[panelImages].selectedID())
}

func TestConfirmFlow(t *testing.T) {
	m := sized(t, 120, 40)
	m = key(m, "d")
	require.NotNil(t, m.confirm)
	assert.Contains(t, m.confirm.prompt, "RUNNING container buildkit")
	assert.Contains(t, ansi.Strip(m.View()), "[y/n]")

	m = key(m, "n")
	assert.Nil(t, m.confirm)
	assert.Empty(t, m.busy)
}

func TestHintsFollowState(t *testing.T) {
	m := sized(t, 120, 40)
	bar := ansi.Strip(m.bottomBar())
	assert.Contains(t, bar, "s stop")
	assert.NotContains(t, bar, "S start")

	m.containers[0].Status.State = "stopped"
	bar = ansi.Strip(m.bottomBar())
	assert.Contains(t, bar, "S start")
	assert.NotContains(t, bar, "e shell")
}

func TestLogWrapToggle(t *testing.T) {
	long := strings.Repeat("x", 200)
	assert.Equal(t, 1, strings.Count(renderLogs([]string{long}, 40, false), "\n")+1)
	assert.Equal(t, 5, strings.Count(renderLogs([]string{long}, 40, true), "\n")+1)

	m := sized(t, 120, 40)
	assert.False(t, m.logWrap)
	assert.Contains(t, ansi.Strip(m.bottomBar()), "w wrap")
	m = key(m, "w")
	assert.True(t, m.logWrap)
	assert.Contains(t, ansi.Strip(m.bottomBar()), "w no wrap")
}

func TestSnapshotErrorKeepsData(t *testing.T) {
	m := sized(t, 120, 40)
	next, _ := m.Update(snapshotMsg{err: assert.AnError})
	m = next.(Model)
	assert.Len(t, m.containers, 1)
	assert.True(t, m.flashErr)
	assert.Contains(t, ansi.Strip(m.bottomBar()), assert.AnError.Error())
}

func TestStatsCPU(t *testing.T) {
	m := sized(t, 120, 40)
	now := time.Now()
	first := statsMsg{items: []container.Stats{{ID: "buildkit", CPUUsageUsec: 1_000_000, NetworkRxBytes: 1000, BlockReadBytes: 500}}, at: now}
	second := statsMsg{items: []container.Stats{{ID: "buildkit", CPUUsageUsec: 2_000_000, NetworkRxBytes: 5000, BlockReadBytes: 2500}}, at: now.Add(2 * time.Second)}

	m.applyStats(first)
	assert.False(t, m.stats["buildkit"].hasCPU)
	m.applyStats(second)
	assert.True(t, m.stats["buildkit"].hasCPU)
	assert.InDelta(t, 50.0, m.stats["buildkit"].cpuPct, 0.01)
	assert.InDelta(t, 2000.0, m.stats["buildkit"].netRate, 0.01)
	assert.InDelta(t, 1000.0, m.stats["buildkit"].diskRate, 0.01)
	assert.Len(t, m.history["buildkit"].cpu, 1)
	assert.Len(t, m.history["buildkit"].net, 1)

	m.applyStats(statsMsg{at: now.Add(4 * time.Second)})
	assert.Empty(t, m.stats)
}

func TestByteRateIgnoresCounterReset(t *testing.T) {
	assert.Zero(t, byteRate(5000, 100, time.Second))
	assert.Zero(t, byteRate(100, 5000, 0))
	assert.InDelta(t, 1000.0, byteRate(0, 2000, 2*time.Second), 0.01)
}

func TestChart(t *testing.T) {
	rows := chart([]float64{0, 25, 50, 100}, 100, 4, 2, dimStyle)
	require.Len(t, rows, 2)
	for _, r := range rows {
		assert.Equal(t, 4, ansi.StringWidth(ansi.Strip(r)))
	}
	assert.Equal(t, "   █", ansi.Strip(rows[0])) // only 100 reaches the top row
	assert.Equal(t, " ▄██", ansi.Strip(rows[1])) // 25 is half a cell, 50 fills one

	// a value below one eighth still draws, so idle traffic is visible
	assert.Equal(t, "▁", ansi.Strip(chart([]float64{0.001}, 100, 1, 1, dimStyle)[0]))
	// fewer values than the width right-align against the newest sample
	assert.Equal(t, "  █", ansi.Strip(chart([]float64{100}, 100, 3, 1, dimStyle)[0]))
}

func TestPanelWindowing(t *testing.T) {
	tests := []struct {
		name                  string
		offset, cursor, count int
		h, want               int
	}{
		{"cursor above window", 5, 2, 20, 4, 2},
		{"cursor below window", 0, 9, 20, 4, 6},
		{"cursor inside window", 3, 5, 20, 4, 3},
		{"short list", 0, 1, 2, 10, 0},
		{"empty", 0, 0, 0, 10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, visibleOffset(tt.offset, tt.cursor, tt.count, tt.h))
		})
	}
}

func TestHumanBytes(t *testing.T) {
	tests := map[int64]string{
		0:           "0 B",
		999:         "999 B",
		4184689:     "4.2 MB",
		2147483648:  "2.1 GB",
		21591785472: "21.6 GB",
		287400000:   "287 MB",
	}
	for n, want := range tests {
		assert.Equal(t, want, humanBytes(n))
	}
}

func TestHumanAgo(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, "just now", humanAgo(now.Add(-30*time.Second), now))
	assert.Equal(t, "5m ago", humanAgo(now.Add(-5*time.Minute), now))
	assert.Equal(t, "17h ago", humanAgo(now.Add(-17*time.Hour), now))
	assert.Equal(t, "3d ago", humanAgo(now.Add(-72*time.Hour), now))
	assert.Empty(t, humanAgo(time.Time{}, now))
}

func TestFormatRowWidth(t *testing.T) {
	cols := []column{{text: "●", width: 1}, {text: "a-very-long-container-name", width: 8}, {text: "running", width: 7}, {text: "image"}}
	for _, w := range []int{20, 40, 80} {
		assert.Equal(t, w, ansi.StringWidth(formatRow(w, cols, true)))
		assert.Equal(t, w, ansi.StringWidth(formatRow(w, cols, false)))
	}
}

func TestBoxDimensions(t *testing.T) {
	out := box("Title", []string{"one", "two"}, 30, 6, true)
	lines := strings.Split(out, "\n")
	require.Len(t, lines, 6)
	for _, l := range lines {
		assert.Equal(t, 30, ansi.StringWidth(l))
	}
	assert.Contains(t, ansi.Strip(lines[0]), "Title")
}

func TestStatsColumns(t *testing.T) {
	assert.Equal(t, 1, statsColumns(79))
	assert.Equal(t, 2, statsColumns(80))
	assert.Equal(t, 2, statsColumns(139))
	assert.Equal(t, 4, statsColumns(140))
}

func TestStatsGridLayout(t *testing.T) {
	charts := make([]statChart, 4)
	for i := range charts {
		charts[i] = statChart{name: "M", value: "1%", values: []float64{10, 90}, peak: 100, style: dimStyle}
	}
	for _, tt := range []struct {
		width, wantRows int
	}{
		{70, 4},  // one column, four stacked
		{100, 2}, // 2x2
		{160, 1}, // 4x1
	} {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			out := statsGrid(charts, tt.width, 40)
			cols := statsColumns(tt.width)
			chartH := (len(out) / tt.wantRows) - 1
			assert.Positive(t, chartH)
			assert.Len(t, out, tt.wantRows*(chartH+1))
			assert.Equal(t, 4/cols, tt.wantRows)
			for _, l := range out {
				assert.LessOrEqual(t, ansi.StringWidth(ansi.Strip(l)), tt.width)
			}
		})
	}
}

func TestStatsGridShrinksToOneRow(t *testing.T) {
	charts := []statChart{{name: "M", values: []float64{1}, peak: 1, style: dimStyle}}
	out := statsGrid(charts, 70, 4) // no vertical room left after the text
	assert.Len(t, out, 2)           // header plus a single sparkline row
}

func clickAt(m Model, x, y int) Model {
	next, _ := m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	return next.(Model)
}

// tabColumn finds a heading in the rendered title row, so the test checks the
// real pixels rather than repeating tabAt's arithmetic. The row is full of
// box-drawing runes, so it counts columns rather than bytes.
func tabColumn(t *testing.T, m Model, name string) int {
	t.Helper()
	row := []rune(ansi.Strip(strings.Split(m.View(), "\n")[0]))
	for i := range row {
		if (row[i] == '│' || row[i] == '·') && string(row[i+1:min(i+2+len(name), len(row))]) == " "+name {
			return i + 2
		}
	}
	t.Fatalf("tab %q not in title row %q", name, string(row))
	return 0
}

func TestClickSelectsTab(t *testing.T) {
	m := sized(t, 120, 40)
	require.Equal(t, tabLogs, m.tab)

	for _, tt := range []struct {
		name string
		want detailTab
	}{{"Stats", tabStats}, {"Config", tabConfig}, {"Shell", tabShell}, {"Logs", tabLogs}} {
		t.Run(tt.name, func(t *testing.T) {
			x := tabColumn(t, m, tt.name)
			got, ok := m.tabAt(x, 0)
			require.True(t, ok, "no tab at x=%d", x)
			assert.Equal(t, tt.want, got)
			// both edges of the heading hit, the cell before it does not
			_, ok = m.tabAt(x+len(tt.name)-1, 0)
			assert.True(t, ok)
			_, ok = m.tabAt(x-1, 0)
			assert.False(t, ok)
		})
	}

	m = clickAt(m, tabColumn(t, m, "Config"), 0)
	assert.Equal(t, tabConfig, m.tab)
}

func TestClickIgnoresNonTabs(t *testing.T) {
	m := sized(t, 120, 40)
	_, ok := m.tabAt(tabColumn(t, m, "Stats"), 1) // body row, not the title
	assert.False(t, ok)
	_, ok = m.tabAt(2, 0) // over the left column
	assert.False(t, ok)

	m = key(m, "tab") // images panel has no tabs
	_, ok = m.tabAt(60, 0)
	assert.False(t, ok)
}

func TestClickFallsThroughToViewport(t *testing.T) {
	m := sized(t, 120, 40)
	m.viewport.SetContent(strings.Repeat("line\n", 200))
	m.viewport.GotoTop()
	before := m.viewport.YOffset
	m = clickAt(m, 60, 20)
	assert.Equal(t, tabLogs, m.tab)
	assert.Equal(t, before, m.viewport.YOffset)
}

func TestClickIgnoresTruncatedTabs(t *testing.T) {
	wide := sized(t, 120, 40)
	shell := tabColumn(t, wide, "Shell")

	narrow := sized(t, 60, 40) // title no longer fits every heading
	row := ansi.Strip(strings.Split(narrow.View(), "\n")[0])
	require.NotContains(t, row, "Shell")
	_, ok := narrow.tabAt(shell, 0)
	assert.False(t, ok)
}

func TestMouseToggle(t *testing.T) {
	m := sized(t, 120, 40)
	assert.False(t, m.mouseOff)
	assert.Contains(t, ansi.Strip(renderHints(m.hints())), "v select text")

	m = key(m, "v")
	assert.True(t, m.mouseOff)
	// the flash confirms the change, the hint is the standing reminder
	assert.Contains(t, ansi.Strip(m.bottomBar()), "mouse released")
	assert.Contains(t, ansi.Strip(renderHints(m.hints())), "v mouse on")

	m = key(m, "v")
	assert.False(t, m.mouseOff)
	assert.Contains(t, ansi.Strip(renderHints(m.hints())), "v select text")
}

func TestMouseToggleFromDetailFocus(t *testing.T) {
	m := key(sized(t, 120, 40), "enter")
	require.True(t, m.detailFocused)
	m = key(m, "v")
	assert.True(t, m.mouseOff)
	assert.True(t, m.detailFocused) // toggling does not move focus
}

// screenRow finds the rendered row containing text, so the test uses the real
// layout instead of repeating panelAt's arithmetic.
func screenRow(t *testing.T, m Model, text string) int {
	t.Helper()
	for i, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(ansi.Strip(l), text) {
			return i
		}
	}
	t.Fatalf("%q is not on screen", text)
	return 0
}

func TestClickSelectsPanelRow(t *testing.T) {
	m := sized(t, 120, 40)
	require.Equal(t, panelContainers, m.focus)

	m = clickAt(m, 2, screenRow(t, m, "golang"))
	assert.Equal(t, panelImages, m.focus)
	assert.Equal(t, "docker.io/library/golang:1.26-bookworm", m.panels[panelImages].selectedID())

	m = clickAt(m, 2, screenRow(t, m, "alpine"))
	assert.Equal(t, panelImages, m.focus)
	assert.Equal(t, "docker.io/library/alpine:latest", m.panels[panelImages].selectedID())

	m = clickAt(m, 2, screenRow(t, m, "ctui-probe"))
	assert.Equal(t, panelVolumes, m.focus)
	assert.Equal(t, "ctui-probe", m.panels[panelVolumes].selectedID())
}

func TestClickPanelTitleFocusesOnly(t *testing.T) {
	m := sized(t, 120, 40)
	m = clickAt(m, 2, screenRow(t, m, "golang")) // cursor off row 0
	require.Equal(t, 1, m.panels[panelImages].cursor)

	m = clickAt(m, 2, screenRow(t, m, "Containers ("))
	assert.Equal(t, panelContainers, m.focus)
	assert.Equal(t, 1, m.panels[panelImages].cursor) // title click leaves cursors alone
}

func TestClickLeavesDetailFocus(t *testing.T) {
	m := key(sized(t, 120, 40), "enter")
	require.True(t, m.detailFocused)
	m = clickAt(m, 2, screenRow(t, m, "golang"))
	assert.False(t, m.detailFocused)
}

func TestClickOutsidePanels(t *testing.T) {
	m := sized(t, 120, 40)
	_, _, ok := m.panelAt(2, screenRow(t, m, "container 1.3.1")) // status box
	assert.False(t, ok)
	_, _, ok = m.panelAt(m.layout().left, screenRow(t, m, "golang")) // detail column
	assert.False(t, ok)
	_, _, ok = m.panelAt(2, 39) // bottom bar
	assert.False(t, ok)
}

func TestClickRespectsScrollOffset(t *testing.T) {
	m := sized(t, 120, 40)
	rows := make([]row, 50)
	for i := range rows {
		id := fmt.Sprintf("c%02d", i)
		rows[i] = row{id: id, styled: id, plain: id}
	}
	m.panels[panelContainers].rows = rows
	m.panels[panelContainers].cursor = 45
	m.ensureVisible()
	require.Positive(t, m.panels[panelContainers].offset, "panel should be scrolled")

	m = clickAt(m, 2, screenRow(t, m, "c38"))
	assert.Equal(t, "c38", m.panels[panelContainers].selectedID())
}
