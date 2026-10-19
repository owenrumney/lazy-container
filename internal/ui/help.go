package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/owenrumney/container-tui/internal/container"
)

type hint struct{ key, label string }

func renderHints(hs []hint) string {
	parts := make([]string, 0, len(hs))
	for _, h := range hs {
		parts = append(parts, keyStyle.Render(h.key)+" "+dimStyle.Render(h.label))
	}
	return strings.Join(parts, dimStyle.Render(" · "))
}

func (m Model) hints() []hint {
	if m.detailFocused {
		hs := []hint{{"j/k", "scroll"}, {"g/G", "top/bottom"}, {"esc", "back"}}
		if m.focus == panelContainers {
			hs = append(hs, hint{"[ ]", "tab"})
		}
		return append(hs, hint{"q", "quit"})
	}
	hs := []hint{{"↑↓", "move"}, {"tab", "panel"}, {"enter", "detail"}}
	switch m.focus {
	case panelContainers:
		hs = append(hs, hint{"[ ]", "tab"})
		if c, ok := m.selectedContainer(); ok {
			if c.Running() {
				hs = append(hs, hint{"s", "stop"}, hint{"r", "restart"}, hint{"K", "kill"}, hint{"e", "shell"})
			} else {
				hs = append(hs, hint{"S", "start"})
			}
			hs = append(hs, hint{"d", "delete"})
		}
		hs = append(hs, hint{"p", "prune"})
	case panelImages:
		hs = append(hs, hint{"d", "delete"}, hint{"p", "prune"}, hint{"P", "prune all"})
	case panelVolumes, panelNetworks:
		hs = append(hs, hint{"d", "delete"}, hint{"p", "prune"})
	}
	return append(hs, hint{"?", "help"}, hint{"q", "quit"})
}

func helpView() string {
	section := func(title string, rows ...[2]string) string {
		var b strings.Builder
		b.WriteString(titleStyle.Render(title) + "\n")
		for _, r := range rows {
			b.WriteString("  " + keyStyle.Render(pad(r[0], 12)) + dimStyle.Render(r[1]) + "\n")
		}
		return b.String()
	}
	body := strings.Join([]string{
		section("Navigation",
			[2]string{"↑/k  ↓/j", "move selection"},
			[2]string{"g / G", "first / last"},
			[2]string{"tab  1-4", "switch panel"},
			[2]string{"[ / ]", "previous / next detail tab"},
			[2]string{"enter / esc", "focus / leave detail (scroll with j/k, pgup/pgdn)"},
			[2]string{"R", "refresh now"},
		),
		section("Containers",
			[2]string{"s / S", "stop / start"},
			[2]string{"r", "restart"},
			[2]string{"K", "kill"},
			[2]string{"d", "delete (force if running)"},
			[2]string{"e", "open a shell"},
			[2]string{"p", "prune stopped containers"},
		),
		section("Images",
			[2]string{"d", "delete"},
			[2]string{"p / P", "prune dangling / all unused"},
		),
		section("Volumes & Networks",
			[2]string{"d", "delete"},
			[2]string{"p", "prune unused"},
		),
		section("General",
			[2]string{"?", "toggle this help"},
			[2]string{"q", "quit"},
		),
	}, "\n")
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(0, 2).
		Render(strings.TrimRight(body, "\n"))
}

// shellProbe prefers bash when the image has it and falls back to sh.
const shellProbe = "command -v bash >/dev/null 2>&1 && exec bash || exec sh"

func containerRows(items []container.Container, width int) []row {
	nameW := 4
	for _, c := range items {
		nameW = max(nameW, len(c.ID))
	}
	nameW = min(nameW, max(width/3, 4))
	rows := make([]row, 0, len(items))
	for _, c := range items {
		cols := []column{
			{text: "●", width: 1, style: stateStyle(c.Status.State)},
			{text: c.ID, width: nameW},
			{text: c.Status.State, width: 7, style: stateStyle(c.Status.State)},
			{text: c.Image(), style: dimStyle},
		}
		rows = append(rows, row{id: c.ID, styled: formatRow(width, cols, true), plain: formatRow(width, cols, false)})
	}
	return rows
}

func imageRows(items []container.Image, width int) []row {
	tagW := 6
	for _, img := range items {
		tagW = max(tagW, len(img.Tag()))
	}
	tagW = min(tagW, 16)
	rows := make([]row, 0, len(items))
	for _, img := range items {
		cols := []column{
			{text: img.Name()},
			{text: img.Tag(), width: tagW, style: dimStyle},
			{text: humanBytes(img.Size()), width: 9, right: true, style: dimStyle},
		}
		rows = append(rows, row{id: img.Configuration.Name, styled: formatRow(width, cols, true), plain: formatRow(width, cols, false)})
	}
	return rows
}

func volumeRows(items []container.Volume, width int) []row {
	rows := make([]row, 0, len(items))
	for _, v := range items {
		cols := []column{
			{text: v.Configuration.Name},
			{text: v.Configuration.Driver, width: 5, style: dimStyle},
			{text: humanBytes(v.Configuration.SizeInBytes), width: 9, right: true, style: dimStyle},
		}
		rows = append(rows, row{id: v.Configuration.Name, styled: formatRow(width, cols, true), plain: formatRow(width, cols, false)})
	}
	return rows
}

func networkRows(items []container.Network, width int) []row {
	rows := make([]row, 0, len(items))
	for _, n := range items {
		cols := []column{
			{text: n.Configuration.Name},
			{text: n.Configuration.Mode, width: 4, style: dimStyle},
			{text: n.Status.IPv4Subnet, width: 18, style: dimStyle},
		}
		rows = append(rows, row{id: n.Configuration.Name, styled: formatRow(width, cols, true), plain: formatRow(width, cols, false)})
	}
	return rows
}
