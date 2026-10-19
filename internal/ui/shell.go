package ui

import (
	"os"
	"os/exec"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
)

// shellSession runs a container shell on a PTY and keeps a virtual terminal of
// its output, so the detail pane can draw it without leaving the TUI.
type shellSession struct {
	term   *vt.SafeEmulator
	ptmx   *os.File
	cmd    *exec.Cmd
	redraw chan struct{}

	mu      sync.Mutex
	closed  bool
	done    bool
	exitErr error
}

// shellDetachKey returns focus to the TUI. Shells rarely bind it, unlike esc.
const shellDetachKey = "ctrl+]"

func newShellSession(cmd *exec.Cmd, w, h int) (*shellSession, error) {
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	if err != nil {
		return nil, err
	}
	s := &shellSession{
		term:   vt.NewSafeEmulator(w, h),
		ptmx:   ptmx,
		cmd:    cmd,
		redraw: make(chan struct{}, 1),
	}
	go s.pump()
	return s, nil
}

// pump feeds PTY output to the terminal and nudges the UI, coalescing bursts
// into a single pending redraw.
func (s *shellSession) pump() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			_, _ = s.term.Write(buf[:n])
			select {
			case s.redraw <- struct{}{}:
			default:
			}
		}
		if err != nil {
			s.finish(s.cmd.Wait())
			close(s.redraw)
			return
		}
	}
}

func (s *shellSession) finish(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
	if !s.closed {
		s.exitErr = err
	}
}

func (s *shellSession) status() (done bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done, s.exitErr
}

func (s *shellSession) write(b []byte) {
	if len(b) == 0 {
		return
	}
	if done, _ := s.status(); done {
		return
	}
	_, _ = s.ptmx.Write(b)
}

func (s *shellSession) resize(w, h int) {
	if w <= 0 || h <= 0 || (s.term.Width() == w && s.term.Height() == h) {
		return
	}
	_ = pty.Setsize(s.ptmx, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	s.term.Resize(w, h)
}

func (s *shellSession) render() string {
	return s.term.Render()
}

func (s *shellSession) close() {
	s.mu.Lock()
	already := s.closed
	s.closed = true
	s.mu.Unlock()
	if already {
		return
	}
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.ptmx.Close()
}

// waitShell blocks until the shell redraws or exits.
func waitShell(s *shellSession, seq int) tea.Cmd {
	return func() tea.Msg {
		_, ok := <-s.redraw
		return shellOutputMsg{seq: seq, done: !ok}
	}
}

// keyBytes encodes a Bubble Tea key press as the bytes a terminal would send.
// Control keys below 32 share their value with the byte, so they fall through.
func keyBytes(msg tea.KeyMsg) []byte {
	var b []byte
	switch msg.Type {
	case tea.KeyRunes:
		b = []byte(string(msg.Runes))
	case tea.KeySpace:
		b = []byte{' '}
	case tea.KeyBackspace:
		b = []byte{0x7f}
	case tea.KeyUp:
		b = []byte("\x1b[A")
	case tea.KeyDown:
		b = []byte("\x1b[B")
	case tea.KeyRight:
		b = []byte("\x1b[C")
	case tea.KeyLeft:
		b = []byte("\x1b[D")
	case tea.KeyHome:
		b = []byte("\x1b[H")
	case tea.KeyEnd:
		b = []byte("\x1b[F")
	case tea.KeyPgUp:
		b = []byte("\x1b[5~")
	case tea.KeyPgDown:
		b = []byte("\x1b[6~")
	case tea.KeyDelete:
		b = []byte("\x1b[3~")
	case tea.KeyInsert:
		b = []byte("\x1b[2~")
	case tea.KeyShiftTab:
		b = []byte("\x1b[Z")
	default:
		if msg.Type < 0 || msg.Type > 31 {
			return nil
		}
		b = []byte{byte(msg.Type)}
	}
	if msg.Alt {
		b = append([]byte{0x1b}, b...)
	}
	return b
}
