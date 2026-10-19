package ui

import (
	"context"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/owenrumney/container-tui/internal/container"
)

const (
	refreshInterval = 2 * time.Second
	settleDelay     = 120 * time.Millisecond
	flashDuration   = 4 * time.Second
	logTail         = 500
	logBatch        = 500
	maxLogLines     = 5000
)

type snapshotMsg struct {
	containers []container.Container
	images     []container.Image
	volumes    []container.Volume
	networks   []container.Network
	df         container.DiskUsage
	system     container.SystemStatus
	err        error
}

type statsMsg struct {
	items []container.Stats
	at    time.Time
	err   error
}

type configMsg struct {
	key  string
	body string
	err  error
}

type logStartedMsg struct {
	seq   int
	lines <-chan string
	err   error
}

type logLinesMsg struct {
	seq   int
	lines []string
	done  bool
}

type shellOutputMsg struct {
	seq  int
	done bool
}

type actionDoneMsg struct {
	verb string
	err  error
}

type execDoneMsg struct{ err error }

type selectionSettledMsg struct{ seq int }

type clearFlashMsg struct{ id int }

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func settle(seq int) tea.Cmd {
	return tea.Tick(settleDelay, func(time.Time) tea.Msg { return selectionSettledMsg{seq: seq} })
}

func clearFlash(id int) tea.Cmd {
	return tea.Tick(flashDuration, func(time.Time) tea.Msg { return clearFlashMsg{id: id} })
}

// fetchSnapshot lists every resource in parallel and returns one message.
// The first error wins but partial results are still delivered.
func fetchSnapshot(c *container.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var (
			msg snapshotMsg
			mu  sync.Mutex
			wg  sync.WaitGroup
		)
		fail := func(err error) {
			mu.Lock()
			defer mu.Unlock()
			if msg.err == nil {
				msg.err = err
			}
		}
		run := func(f func() error) {
			wg.Go(func() {
				if err := f(); err != nil {
					fail(err)
				}
			})
		}
		run(func() (err error) { msg.containers, err = c.ListContainers(ctx); return err })
		run(func() (err error) { msg.images, err = c.ListImages(ctx); return err })
		run(func() (err error) { msg.volumes, err = c.ListVolumes(ctx); return err })
		run(func() (err error) { msg.networks, err = c.ListNetworks(ctx); return err })
		run(func() (err error) { msg.df, err = c.DiskUsage(ctx); return err })
		run(func() (err error) { msg.system, err = c.SystemStatus(ctx); return err })
		wg.Wait()
		return msg
	}
}

func fetchStats(c *container.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		items, err := c.Stats(ctx)
		return statsMsg{items: items, at: time.Now(), err: err}
	}
}

func fetchConfig(key string, inspect func(context.Context) (string, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		body, err := inspect(ctx)
		return configMsg{key: key, body: body, err: err}
	}
}

func startLogs(ctx context.Context, c *container.Client, id string, seq int) tea.Cmd {
	return func() tea.Msg {
		lines, err := c.Logs(ctx, id, logTail)
		return logStartedMsg{seq: seq, lines: lines, err: err}
	}
}

// waitLogs blocks for one line then drains whatever else is ready, so bursts arrive as one message.
func waitLogs(lines <-chan string, seq int) tea.Cmd {
	return func() tea.Msg {
		first, ok := <-lines
		if !ok {
			return logLinesMsg{seq: seq, done: true}
		}
		batch := []string{first}
		for len(batch) < logBatch {
			select {
			case line, ok := <-lines:
				if !ok {
					return logLinesMsg{seq: seq, lines: batch, done: true}
				}
				batch = append(batch, line)
			default:
				return logLinesMsg{seq: seq, lines: batch}
			}
		}
		return logLinesMsg{seq: seq, lines: batch}
	}
}

func runAction(verb string, fn func(context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return actionDoneMsg{verb: verb, err: fn(ctx)}
	}
}
