package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var sparkChars = []rune("▁▂▃▄▅▆▇█")

// column describes one cell of a list row. Width 0 means "take the remaining space".
type column struct {
	text  string
	width int
	style lipgloss.Style
	right bool
}

func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	val := float64(n) / float64(div)
	suffix := "KMGTPE"[exp]
	if val >= 100 {
		return fmt.Sprintf("%.0f %cB", val, suffix)
	}
	return fmt.Sprintf("%.1f %cB", val, suffix)
}

func humanAgo(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	}
}

func humanDuration(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd %dh", int(d.Hours()/24), int(d.Hours())%24)
	}
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// pad truncates or right-pads s to exactly w cells.
func pad(s string, w int) string {
	s = truncate(s, w)
	if gap := w - ansi.StringWidth(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

func padLeft(s string, w int) string {
	s = truncate(s, w)
	if gap := w - ansi.StringWidth(s); gap > 0 {
		s = strings.Repeat(" ", gap) + s
	}
	return s
}

// formatRow lays out columns into a line of exactly width cells.
// With styled=false every column is rendered as plain text so a highlight can cover the row.
func formatRow(width int, cols []column, styled bool) string {
	fixed := 0
	flex := 0
	for _, c := range cols {
		if c.width == 0 {
			flex++
		} else {
			fixed += c.width
		}
	}
	gaps := len(cols) - 1
	remaining := max(width-fixed-gaps, 0)
	parts := make([]string, 0, len(cols))
	for _, c := range cols {
		w := c.width
		if w == 0 {
			w = remaining / max(flex, 1)
		}
		cell := pad(c.text, w)
		if c.right {
			cell = padLeft(c.text, w)
		}
		if styled {
			cell = c.style.Render(cell)
		}
		parts = append(parts, cell)
	}
	return pad(strings.Join(parts, " "), width)
}

// chart draws values as block columns h rows tall, newest on the right.
// peak is the top of the y axis; anything above it clips.
func chart(values []float64, peak float64, w, h int, style lipgloss.Style) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	if len(values) > w {
		values = values[len(values)-w:]
	}
	if peak <= 0 {
		peak = 1
	}
	gap := strings.Repeat(" ", w-len(values))
	rows := make([]string, h)
	for r := range rows {
		var b strings.Builder
		b.WriteString(gap)
		for _, v := range values {
			eighths := int(max(0, min(v/peak, 1)) * float64(h*8))
			if v > 0 && eighths == 0 {
				eighths = 1 // keep a trickle visible
			}
			switch cell := eighths - (h-1-r)*8; {
			case cell <= 0:
				b.WriteRune(' ')
			case cell >= 8:
				b.WriteRune('█')
			default:
				b.WriteRune(sparkChars[cell-1])
			}
		}
		rows[r] = style.Render(b.String())
	}
	return rows
}

func peakOf(values []float64) float64 {
	p := 0.0
	for _, v := range values {
		p = max(p, v)
	}
	return p
}

// fracStyle colours a 0..1 utilisation: green, amber over 70%, red over 90%.
func fracStyle(fraction float64) lipgloss.Style {
	switch {
	case fraction > 0.9:
		return redStyle
	case fraction > 0.7:
		return yellowStyle
	default:
		return greenStyle
	}
}

func stateStyle(state string) lipgloss.Style {
	switch state {
	case "running":
		return greenStyle
	case "stopped", "exited":
		return redStyle
	default:
		return yellowStyle
	}
}
