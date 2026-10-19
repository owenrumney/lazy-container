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

func sparkline(values []float64, width int) string {
	if width <= 0 {
		return ""
	}
	if len(values) > width {
		values = values[len(values)-width:]
	}
	peak := 1.0
	for _, v := range values {
		peak = max(peak, v)
	}
	var b strings.Builder
	for i := 0; i < width-len(values); i++ {
		b.WriteRune(' ')
	}
	for _, v := range values {
		idx := int(v / peak * float64(len(sparkChars)-1))
		b.WriteRune(sparkChars[max(0, min(idx, len(sparkChars)-1))])
	}
	return b.String()
}

func bar(fraction float64, width int) string {
	fraction = max(0, min(fraction, 1))
	filled := int(fraction*float64(width) + 0.5)
	style := greenStyle
	switch {
	case fraction > 0.9:
		style = redStyle
	case fraction > 0.7:
		style = yellowStyle
	}
	return style.Render(strings.Repeat("█", filled)) + dimStyle.Render(strings.Repeat("░", width-filled))
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
