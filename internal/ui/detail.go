package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/owenrumney/container-tui/internal/container"
)

type statSample struct {
	stats    container.Stats
	at       time.Time
	cpuPct   float64
	netRate  float64 // bytes/sec, rx+tx
	diskRate float64 // bytes/sec, read+write
	hasCPU   bool
}

// statHistory is the rolling window behind the stats graphs. cpu and mem are
// percentages; net and disk are bytes/sec.
type statHistory struct {
	cpu, mem, net, disk []float64
}

const maxHistory = 60

var jsonKeyRe = regexp.MustCompile(`^(\s*)"([^"]+)"(\s*:)`)

func (h statHistory) push(cpu, mem, net, disk float64) statHistory {
	return statHistory{
		cpu:  appendCapped(h.cpu, cpu),
		mem:  appendCapped(h.mem, mem),
		net:  appendCapped(h.net, net),
		disk: appendCapped(h.disk, disk),
	}
}

func appendCapped(s []float64, v float64) []float64 {
	s = append(s, v)
	if len(s) > maxHistory {
		s = s[len(s)-maxHistory:]
	}
	return s
}

// cpuPercent returns CPU use between two samples, where 100 means one full core.
func cpuPercent(prev, cur container.Stats, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	delta := float64(cur.CPUUsageUsec - prev.CPUUsageUsec)
	return max(0, delta/float64(elapsed.Microseconds())*100)
}

// byteRate converts two readings of a cumulative counter to bytes/sec.
// A counter that went backwards means a reset, so report nothing.
func byteRate(prev, cur int64, elapsed time.Duration) float64 {
	if elapsed <= 0 || cur < prev {
		return 0
	}
	return float64(cur-prev) / elapsed.Seconds()
}

func rateText(bytesPerSec float64) string {
	return humanBytes(int64(bytesPerSec)) + "/s"
}

func memPercent(s container.Stats) float64 {
	if s.MemoryLimitBytes <= 0 {
		return 0
	}
	return float64(s.MemoryUsageBytes) / float64(s.MemoryLimitBytes) * 100
}

// renderLogs keeps one entry per row unless wrap is on, so a long JSON line
// cannot fill the pane.
func renderLogs(lines []string, width int, wrap bool) string {
	if len(lines) == 0 {
		return dimStyle.Render("no output yet")
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if wrap {
			out = append(out, ansi.Hardwrap(l, width, true))
		} else {
			out = append(out, truncate(l, width))
		}
	}
	return strings.Join(out, "\n")
}

// renderShell draws the shell's screen with a status line pinned to the bottom
// row, which is why the session is sized one row short of the pane.
func renderShell(s *shellSession, startErr error, c container.Container, width, height int) string {
	note := func(msg string) string {
		return strings.Join(append(make([]string, max(height-1, 0)), dimStyle.Render(truncate(msg, width))), "\n")
	}
	switch {
	case !c.Running():
		return note("container is not running")
	case startErr != nil:
		return note(errStyle.Render("shell: " + startErr.Error()))
	case s == nil:
		return note("starting shell…")
	}

	rows := strings.Split(s.render(), "\n")
	for i, r := range rows {
		rows[i] = truncate(r, width)
	}
	for len(rows) < max(height-1, 0) {
		rows = append(rows, "")
	}
	status := dimStyle.Render(shellDetachKey + " to detach")
	if done, err := s.status(); done {
		status = dimStyle.Render("shell exited")
		if err != nil {
			status = dimStyle.Render("shell exited: " + err.Error())
		}
		status += dimStyle.Render(" · enter to restart")
	}
	return strings.Join(append(rows[:max(height-1, 0)], truncate(status, width)), "\n")
}

func renderConfig(body string, width int) string {
	if body == "" {
		return dimStyle.Render("loading…")
	}
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if m := jsonKeyRe.FindStringSubmatchIndex(l); m != nil {
			l = l[:m[2]] + jsonKeyStyle.Render(l[m[2]:m[5]]) + l[m[5]:]
		}
		lines[i] = ansi.Hardwrap(l, width, true)
	}
	return strings.Join(lines, "\n")
}

// statChart is one labelled graph in the stats grid.
type statChart struct {
	name   string
	value  string
	detail string
	values []float64
	peak   float64
	style  lipgloss.Style
}

const (
	gridGap = 3
	// statsTextRows is everything on the stats page that is not a graph.
	statsTextRows = 16
)

// statsColumns fits as many graphs side by side as stay readable.
func statsColumns(width int) int {
	switch {
	case width >= 140:
		return 4
	case width >= 80:
		return 2
	default:
		return 1
	}
}

// statsGrid lays the graphs out in 1, 2 or 4 columns and gives the rows that
// frees up back to their height.
func statsGrid(charts []statChart, width, height int) []string {
	cols := min(statsColumns(width), len(charts))
	rows := (len(charts) + cols - 1) / cols
	cellW := (width - gridGap*(cols-1)) / cols
	chartH := min(max((height-statsTextRows-rows)/rows, 1), 16)

	out := make([]string, 0, rows*(chartH+1))
	for r := range rows {
		cells := make([][]string, 0, cols)
		for i := r * cols; i < min((r+1)*cols, len(charts)); i++ {
			cells = append(cells, charts[i].render(cellW, chartH, cols == 1))
		}
		out = append(out, joinCells(cells, cellW)...)
	}
	return out
}

// render returns the header line followed by the graph. A single column keeps
// the graph indented under the label, lining up with the text below it.
func (sc statChart) render(w, h int, indent bool) []string {
	chartW, prefix, name := w, "", sc.name+"  "
	if indent {
		chartW, prefix, name = min(60, max(w-10, 0)), strings.Repeat(" ", 8), pad(sc.name, 8)
	}
	cell := []string{truncate(labelStyle.Render(name)+sc.value+dimStyle.Render(sc.detail), w)}
	for _, r := range chart(sc.values, sc.peak, chartW, h, sc.style) {
		cell = append(cell, prefix+r)
	}
	return cell
}

func joinCells(cells [][]string, w int) []string {
	if len(cells) == 0 {
		return nil
	}
	out := make([]string, len(cells[0]))
	for i := range out {
		parts := make([]string, len(cells))
		for c, cell := range cells {
			parts[c] = pad(cell[i], w)
		}
		out[i] = strings.Join(parts, strings.Repeat(" ", gridGap))
	}
	return out
}

func renderStats(c container.Container, sample *statSample, history statHistory, width, height int, now time.Time) string {
	var b strings.Builder
	label := func(s string) string { return labelStyle.Render(pad(s, 8)) }
	line := func(s string) { b.WriteString(truncate(s, width) + "\n") }

	state := stateStyle(c.Status.State).Render(c.Status.State)
	up := ""
	if c.Running() && c.Status.StartedDate != nil {
		up = dimStyle.Render("  up " + humanDuration(now.Sub(*c.Status.StartedDate)))
	}
	line(titleStyle.Render(c.ID) + "  " + state + up)
	b.WriteString("\n")

	switch {
	case !c.Running():
		line(dimStyle.Render("container is not running"))
	case sample == nil:
		line(dimStyle.Render("collecting stats…"))
	default:
		s := sample.stats
		cpu := dimStyle.Render("sampling…")
		if sample.hasCPU {
			cpu = fmt.Sprintf("%.1f%%", sample.cpuPct)
		}
		cpuPeak := max(100, peakOf(history.cpu))
		frac := memPercent(s) / 100
		netPeak := peakOf(history.net)
		diskPeak := peakOf(history.disk)

		// detail is trimmed first when a cell is narrow, so value and top survive.
		for _, r := range statsGrid([]statChart{{
			name:   "CPU",
			value:  cpu,
			detail: fmt.Sprintf(" · top %.0f%% · %d cpus", cpuPeak, c.Configuration.Resources.CPUs),
			values: history.cpu, peak: cpuPeak, style: fracStyle(sample.cpuPct / 100),
		}, {
			name:   "Memory",
			value:  fmt.Sprintf("%.0f%%", frac*100),
			detail: fmt.Sprintf(" · %s of %s", humanBytes(s.MemoryUsageBytes), humanBytes(s.MemoryLimitBytes)),
			values: history.mem, peak: 100, style: fracStyle(frac),
		}, {
			name:   "Net",
			value:  rateText(sample.netRate),
			detail: fmt.Sprintf(" · top %s · ↓%s ↑%s", rateText(netPeak), humanBytes(s.NetworkRxBytes), humanBytes(s.NetworkTxBytes)),
			values: history.net, peak: netPeak, style: keyStyle,
		}, {
			name:   "Disk",
			value:  rateText(sample.diskRate),
			detail: fmt.Sprintf(" · top %s · r %s w %s", rateText(diskPeak), humanBytes(s.BlockReadBytes), humanBytes(s.BlockWriteBytes)),
			values: history.disk, peak: diskPeak, style: keyStyle,
		}}, width, height) {
			line(r)
		}

		line(label("PIDs") + fmt.Sprintf("%d", s.NumProcesses))
	}

	b.WriteString("\n")
	cfg := c.Configuration
	line(label("Image") + c.Image())
	line(label("Created") + cfg.CreationDate.Local().Format("2006-01-02 15:04") + dimStyle.Render("  "+humanAgo(cfg.CreationDate, now)))
	line(label("Limits") + fmt.Sprintf("%d cpus · %s memory", cfg.Resources.CPUs, humanBytes(cfg.Resources.MemoryInBytes)))
	line(label("Arch") + cfg.Platform.OS + "/" + cfg.Platform.Architecture)
	for _, n := range c.Status.Networks {
		line(label("Iface") + fmt.Sprintf("%s  %s  gw %s", n.Network, n.IPv4Address, n.IPv4Gateway))
	}
	for _, p := range cfg.PublishedPorts {
		line(label("Port") + fmt.Sprintf("%s:%d → %d/%s", p.HostAddress, p.HostPort, p.ContainerPort, p.Proto))
	}
	for _, mnt := range cfg.Mounts {
		src := mnt.Source
		if src == "" {
			src = dimStyle.Render("(tmpfs)")
		}
		line(label("Mount") + mnt.Destination + dimStyle.Render("  ← "+src))
	}
	if len(cfg.Labels) > 0 {
		b.WriteString("\n")
		for k, v := range cfg.Labels {
			line(label("Label") + dimStyle.Render(k+"=") + v)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
