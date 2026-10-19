package ui

import "fmt"

// row is one pre-rendered list entry. plain is used under the selection highlight.
type row struct {
	id     string
	styled string
	plain  string
}

type listPanel struct {
	title  string
	rows   []row
	cursor int
	offset int
}

// setRows replaces the rows, keeping the cursor on the same id where possible.
func (p *listPanel) setRows(rows []row) {
	selected := p.selectedID()
	p.rows = rows
	p.cursor = 0
	for i, r := range rows {
		if r.id == selected {
			p.cursor = i
			break
		}
	}
	p.clamp()
}

func (p listPanel) selectedID() string {
	if p.cursor < 0 || p.cursor >= len(p.rows) {
		return ""
	}
	return p.rows[p.cursor].id
}

func (p *listPanel) move(n int) {
	p.cursor += n
	p.clamp()
}

func (p *listPanel) clamp() {
	p.cursor = max(0, min(p.cursor, len(p.rows)-1))
}

// ensureVisible scrolls the window of h rows so the cursor is on screen.
func (p *listPanel) ensureVisible(h int) {
	p.offset = visibleOffset(p.offset, p.cursor, len(p.rows), h)
}

func (p listPanel) view(w, h int, focused bool, loaded bool) string {
	inner := w - 2
	visible := h - 2
	body := make([]string, 0, visible)
	switch {
	case !loaded:
		body = append(body, dimStyle.Render("loading…"))
	case len(p.rows) == 0:
		body = append(body, dimStyle.Render("none"))
	default:
		offset := visibleOffset(p.offset, p.cursor, len(p.rows), visible)
		for i := offset; i < len(p.rows) && i < offset+visible; i++ {
			line := p.rows[i].styled
			if i == p.cursor {
				style := selectedDimStyle
				if focused {
					style = selectedStyle
				}
				line = style.Render(pad(p.rows[i].plain, inner))
			}
			body = append(body, line)
		}
	}
	title := fmt.Sprintf("%s (%d)", p.title, len(p.rows))
	return box(title, body, w, h, focused)
}

func visibleOffset(offset, cursor, count, h int) int {
	if h <= 0 || count == 0 {
		return 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+h {
		offset = cursor - h + 1
	}
	return max(0, min(offset, max(count-h, 0)))
}
