package ui

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitFor polls until cond holds, so the PTY reader goroutine can catch up.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the shell")
}

func TestShellSessionRoundTrip(t *testing.T) {
	s, err := newShellSession(exec.Command("sh"), 40, 6)
	require.NoError(t, err)
	t.Cleanup(s.close)

	s.write(keyBytes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("echo marker-ok")}))
	s.write(keyBytes(tea.KeyMsg{Type: tea.KeyEnter}))
	waitFor(t, func() bool { return strings.Contains(ansi.Strip(s.render()), "marker-ok") })

	rows := strings.Split(s.render(), "\n")
	assert.Len(t, rows, 6)

	s.resize(60, 10)
	assert.Equal(t, 60, s.term.Width())
	assert.Equal(t, 10, s.term.Height())

	s.write(keyBytes(tea.KeyMsg{Type: tea.KeyCtrlD}))
	waitFor(t, func() bool { done, _ := s.status(); return done })
	done, exitErr := s.status()
	assert.True(t, done)
	assert.NoError(t, exitErr)
	// writes after exit are dropped rather than failing
	s.write([]byte("ignored"))
}

func TestKeyBytes(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyMsg
		want string
	}{
		{"runes", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ls")}, "ls"},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, "\r"},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, "\t"},
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}, "\x03"},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, "\x1b"},
		{"backspace", tea.KeyMsg{Type: tea.KeyBackspace}, "\x7f"},
		{"space", tea.KeyMsg{Type: tea.KeySpace}, " "},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, "\x1b[A"},
		{"delete", tea.KeyMsg{Type: tea.KeyDelete}, "\x1b[3~"},
		{"alt+b", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b"), Alt: true}, "\x1bb"},
		{"unmapped", tea.KeyMsg{Type: tea.KeyF5}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(keyBytes(tt.msg)))
		})
	}
}
