package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/owenrumney/container-tui/internal/container"
)

type statSample struct {
	stats  container.Stats
	at     time.Time
	cpuPct float64
	hasCPU bool
}

const maxHistory = 60

var jsonKeyRe = regexp.MustCompile(`^(\s*)"([^"]+)"(\s*:)`)

// cpuPercent returns CPU use between two samples, where 100 means one full core.
func cpuPercent(prev, cur container.Stats, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	delta := float64(cur.CPUUsageUsec - prev.CPUUsageUsec)
	return max(0, delta/float64(elapsed.Microseconds())*100)
}

func renderLogs(lines []string, width int) string {
	if len(lines) == 0 {
		return dimStyle.Render("no output yet")
	}
	wrapped := make([]string, 0, len(lines))
	for _, l := range lines {
		wrapped = append(wrapped, ansi.Hardwrap(l, width, true))
	}
	return strings.Join(wrapped, "\n")
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

func renderStats(c container.Container, sample *statSample, history []float64, width int, now time.Time) string {
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
			cpu = fmt.Sprintf("%5.1f%%", sample.cpuPct)
		}
		cpus := fmt.Sprintf("  of %d cpus", c.Configuration.Resources.CPUs)
		sparkW := min(40, max(width-30, 0))
		line(label("CPU") + cpu + dimStyle.Render(cpus) + "  " + keyStyle.Render(sparkline(history, sparkW)))

		frac := 0.0
		if s.MemoryLimitBytes > 0 {
			frac = float64(s.MemoryUsageBytes) / float64(s.MemoryLimitBytes)
		}
		line(label("Memory") + fmt.Sprintf("%s / %s  %.0f%%", humanBytes(s.MemoryUsageBytes), humanBytes(s.MemoryLimitBytes), frac*100))
		line(strings.Repeat(" ", 8) + bar(frac, min(40, max(width-10, 0))))
		line(label("Net") + fmt.Sprintf("↓ %s  ↑ %s", humanBytes(s.NetworkRxBytes), humanBytes(s.NetworkTxBytes)))
		line(label("Block") + fmt.Sprintf("read %s  write %s", humanBytes(s.BlockReadBytes), humanBytes(s.BlockWriteBytes)))
		line(label("PIDs") + fmt.Sprintf("%d", s.NumProcesses))
	}

	b.WriteString("\n")
	cfg := c.Configuration
	line(label("Image") + c.Image())
	line(label("Created") + cfg.CreationDate.Local().Format("2006-01-02 15:04") + dimStyle.Render("  "+humanAgo(cfg.CreationDate, now)))
	line(label("Limits") + fmt.Sprintf("%d cpus · %s memory", cfg.Resources.CPUs, humanBytes(cfg.Resources.MemoryInBytes)))
	line(label("Arch") + cfg.Platform.OS + "/" + cfg.Platform.Architecture)
	for _, n := range c.Status.Networks {
		line(label("Net") + fmt.Sprintf("%s  %s  gw %s", n.Network, n.IPv4Address, n.IPv4Gateway))
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
