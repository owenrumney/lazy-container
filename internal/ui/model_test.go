package ui

import (
	"encoding/json"
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
	first := statsMsg{items: []container.Stats{{ID: "buildkit", CPUUsageUsec: 1_000_000}}, at: now}
	second := statsMsg{items: []container.Stats{{ID: "buildkit", CPUUsageUsec: 2_000_000}}, at: now.Add(2 * time.Second)}

	m.applyStats(first)
	assert.False(t, m.stats["buildkit"].hasCPU)
	m.applyStats(second)
	assert.True(t, m.stats["buildkit"].hasCPU)
	assert.InDelta(t, 50.0, m.stats["buildkit"].cpuPct, 0.01)
	assert.Len(t, m.cpuHistory["buildkit"], 1)

	m.applyStats(statsMsg{at: now.Add(4 * time.Second)})
	assert.Empty(t, m.stats)
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
