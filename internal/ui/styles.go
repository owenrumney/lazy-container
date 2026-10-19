package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	colorAccent = lipgloss.Color("39")
	colorDim    = lipgloss.Color("240")
	colorGreen  = lipgloss.Color("42")
	colorRed    = lipgloss.Color("203")
	colorYellow = lipgloss.Color("221")
	colorWhite  = lipgloss.Color("255")

	borderStyle        = lipgloss.NewStyle().Foreground(colorDim)
	borderFocusedStyle = lipgloss.NewStyle().Foreground(colorAccent)
	titleStyle         = lipgloss.NewStyle().Foreground(colorWhite).Bold(true)
	titleDimStyle      = lipgloss.NewStyle().Foreground(colorDim).Bold(true)
	selectedStyle      = lipgloss.NewStyle().Background(colorAccent).Foreground(lipgloss.Color("231")).Bold(true)
	selectedDimStyle   = lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(colorWhite)
	dimStyle           = lipgloss.NewStyle().Foreground(colorDim)
	greenStyle         = lipgloss.NewStyle().Foreground(colorGreen)
	redStyle           = lipgloss.NewStyle().Foreground(colorRed)
	yellowStyle        = lipgloss.NewStyle().Foreground(colorYellow)
	tabActiveStyle     = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Underline(true)
	errStyle           = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	warnStyle          = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
	okStyle            = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	keyStyle           = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	labelStyle         = lipgloss.NewStyle().Foreground(colorDim).Bold(true)
	jsonKeyStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
)

// box draws a bordered frame of exactly w×h cells with the title in the top edge.
func box(title string, body []string, w, h int, focused bool) string {
	if w < 4 || h < 2 {
		return ""
	}
	border := borderStyle
	ts := titleDimStyle
	if focused {
		border = borderFocusedStyle
		ts = titleStyle
	}
	inner := w - 2
	label := " " + ansi.Truncate(title, inner-3, "…") + " "
	fill := inner - 1 - ansi.StringWidth(label)
	var b strings.Builder
	b.WriteString(border.Render("╭─"))
	b.WriteString(ts.Render(label))
	b.WriteString(border.Render(strings.Repeat("─", fill) + "╮"))
	side := border.Render("│")
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		b.WriteString("\n" + side + pad(line, inner) + side)
	}
	b.WriteString("\n" + border.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return b.String()
}
