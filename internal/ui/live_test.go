//go:build integration

package ui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/owenrumney/container-tui/internal/container"
)

// TestLiveFrames drives the model against the real container CLI without a terminal
// and prints each frame. Run with: go test ./internal/ui -tags integration -run TestLive -v
func TestLiveFrames(t *testing.T) {
	client, err := container.New()
	require.NoError(t, err)

	m := New(client)
	m = step(t, m, tea.WindowSizeMsg{Width: 140, Height: 36})
	m, cmd := run(t, m, fetchSnapshot(client))
	require.True(t, m.loaded)
	frame(t, "after snapshot", m)

	// The snapshot starts the log stream for the first container; collect the first batch.
	m, cmd = run(t, m, cmd)
	m, _ = run(t, m, cmd)
	frame(t, "logs", m)

	// Stats tab needs two samples for a CPU reading.
	m, cmd = run(t, m, m.switchTab(1))
	m, _ = run(t, m, cmd)
	time.Sleep(1500 * time.Millisecond)
	m, _ = run(t, m, m.fetchStats())
	frame(t, "stats", m)

	// Config tab.
	m, cmd = run(t, m, m.switchTab(1))
	m, _ = run(t, m, cmd)
	frame(t, "config", m)

	// Images panel config.
	m.setFocus(panelImages)
	m, cmd = run(t, m, m.loadDetail())
	m, _ = run(t, m, cmd)
	frame(t, "images", m)

	m.help = true
	frame(t, "help", m)
	m.stopLogs()
}

func step(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

// run executes a command synchronously and feeds its message back into the model.
func run(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return m, nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var last tea.Cmd
		for _, c := range batch {
			m, last = run(t, m, c)
		}
		return m, last
	}
	next, out := m.Update(msg)
	return next.(Model), out
}

func frame(t *testing.T, name string, m Model) {
	t.Helper()
	fmt.Printf("\n===== %s =====\n%s\n", name, ansi.Strip(m.View()))
}
