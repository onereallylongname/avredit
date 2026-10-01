package model

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/onereallylongname/avredit/internal/theme"
)

// Picker is a filterable list selector overlay.
type Picker struct {
	title      string
	items      []string
	cursor     int
	filter     string
	filtering  bool  // true when in filter input mode (activated by "/")
	visible    []int // indices into items that match filter
	active     bool
	width      int
	height     int
	theme      *theme.Theme
	separators map[int]bool // indices that are non-selectable category headers
}

// NewPicker creates a picker with the given title and items.
func NewPicker(title string, items []string, th *theme.Theme) Picker {
	visible := make([]int, len(items))
	for i := range items {
		visible[i] = i
	}
	return Picker{
		title:   title,
		items:   items,
		cursor:  0,
		visible: visible,
		active:  true,
		width:   30,
		height:  15,
		theme:   th,
	}
}

// NewCategoryPicker creates a picker with items grouped under category headers.
func NewCategoryPicker(title string, categories []PickerCategory, th *theme.Theme) Picker {
	var items []string
	seps := make(map[int]bool)
	for ci, cat := range categories {
		if len(cat.Items) == 0 {
			continue
		}
		if ci > 0 || len(items) > 0 {
			items = append(items, "") // blank line between categories
			seps[len(items)-1] = true
		}
		items = append(items, "── "+cat.Label+" ──")
		seps[len(items)-1] = true
		sorted := make([]string, len(cat.Items))
		copy(sorted, cat.Items)
		sort.Strings(sorted)
		items = append(items, sorted...)
	}

	visible := make([]int, len(items))
	for i := range items {
		visible[i] = i
	}
	// Start cursor on first selectable item
	cursor := 0
	for i, idx := range visible {
		if !seps[idx] {
			cursor = i
			break
		}
	}
	return Picker{
		title:      title,
		items:      items,
		cursor:     cursor,
		visible:    visible,
		active:     true,
		width:      30,
		height:     15,
		theme:      th,
		separators: seps,
	}
}

// PickerCategory groups items under a label.
type PickerCategory struct {
	Label string
	Items []string
}

// Active returns whether the picker is shown.
func (p *Picker) Active() bool {
	return p.active
}

// Selected returns the currently highlighted item, or empty if none.
func (p *Picker) Selected() string {
	if p.cursor >= 0 && p.cursor < len(p.visible) {
		idx := p.visible[p.cursor]
		if p.separators[idx] {
			return ""
		}
		return p.items[idx]
	}
	return ""
}

// SetSize updates dimensions.
func (p *Picker) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// HandleKey processes a keypress. Returns (selected item, done).
func (p *Picker) HandleKey(msg tea.KeyPressMsg) (string, bool) {
	if p.filtering {
		return p.handleFilterKey(msg)
	}
	switch msg.String() {
	case "esc", "ctrl+c":
		p.active = false
		return "", true
	case "enter":
		sel := p.Selected()
		if sel == "" {
			return "", false // ignore if on separator
		}
		p.active = false
		return sel, true
	case "j", "down":
		p.moveDown()
	case "k", "up":
		p.moveUp()
	case "g":
		p.moveTo(0)
	case "G":
		p.moveTo(len(p.visible) - 1)
	case "/":
		p.filtering = true
		p.filter = ""
	default:
		// Quick single-char jump: typing a letter starts filter mode
		if !isControlKey(msg) {
			text := msg.Text
			if text == "" && msg.Code > 0 && msg.Code < 128 {
				text = string(msg.Code)
			}
			if text != "" {
				p.filter += text
				p.applyFilter()
				p.filtering = true
			}
		}
	}
	return "", false
}

// handleFilterKey handles input while in filter mode.
func (p *Picker) handleFilterKey(msg tea.KeyPressMsg) (string, bool) {
	switch msg.String() {
	case "esc":
		p.filtering = false
		p.filter = ""
		p.applyFilter()
	case "enter":
		sel := p.Selected()
		if sel == "" {
			return "", false
		}
		p.active = false
		return sel, true
	case "backspace":
		if len(p.filter) > 0 {
			p.filter = p.filter[:len(p.filter)-1]
			p.applyFilter()
		} else {
			p.filtering = false
		}
	case "up":
		p.moveUp()
	case "down":
		p.moveDown()
	default:
		if !isControlKey(msg) {
			text := msg.Text
			if text == "" && msg.Code > 0 && msg.Code < 128 {
				text = string(msg.Code)
			}
			if text != "" {
				p.filter += text
				p.applyFilter()
			}
		}
	}
	return "", false
}

// moveDown moves cursor to next selectable item.
func (p *Picker) moveDown() {
	for i := p.cursor + 1; i < len(p.visible); i++ {
		if !p.separators[p.visible[i]] {
			p.cursor = i
			return
		}
	}
}

// moveUp moves cursor to previous selectable item.
func (p *Picker) moveUp() {
	for i := p.cursor - 1; i >= 0; i-- {
		if !p.separators[p.visible[i]] {
			p.cursor = i
			return
		}
	}
}

// moveTo moves to the closest selectable item near target index.
func (p *Picker) moveTo(target int) {
	if target < 0 {
		target = 0
	}
	if target >= len(p.visible) {
		target = len(p.visible) - 1
	}
	// Search forward from target
	for i := target; i < len(p.visible); i++ {
		if !p.separators[p.visible[i]] {
			p.cursor = i
			return
		}
	}
	// Search backward
	for i := target; i >= 0; i-- {
		if !p.separators[p.visible[i]] {
			p.cursor = i
			return
		}
	}
}

// applyFilter updates visible items based on current filter.
func (p *Picker) applyFilter() {
	p.visible = p.visible[:0]
	lower := strings.ToLower(p.filter)
	for i, item := range p.items {
		if p.separators[i] {
			continue // hide separators when filtering
		}
		if lower == "" || strings.Contains(strings.ToLower(item), lower) {
			p.visible = append(p.visible, i)
		}
	}
	// When no filter, include separators for visual grouping
	if lower == "" && len(p.separators) > 0 {
		p.visible = p.visible[:0]
		for i := range p.items {
			p.visible = append(p.visible, i)
		}
	}
	if p.cursor >= len(p.visible) {
		p.cursor = len(p.visible) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	// Ensure cursor is on a selectable item
	if len(p.visible) > 0 && p.separators[p.visible[p.cursor]] {
		p.moveDown()
	}
}

// View renders the picker overlay.
func (p Picker) View() string {
	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(p.theme.Primary).
		Bold(true)
	sb.WriteString(titleStyle.Render(p.title))
	sb.WriteString("\n")

	if p.filter != "" {
		filterStyle := lipgloss.NewStyle().Foreground(p.theme.Warning)
		sb.WriteString(filterStyle.Render("/" + p.filter))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	maxShow := p.height - 4
	if maxShow < 3 {
		maxShow = 3
	}

	// Compute scroll window
	start := 0
	if p.cursor >= maxShow {
		start = p.cursor - maxShow + 1
	}
	end := start + maxShow
	if end > len(p.visible) {
		end = len(p.visible)
	}

	activeStyle := p.theme.TreeNodeActive
	normalStyle := lipgloss.NewStyle().Foreground(p.theme.Fg)
	sepStyle := lipgloss.NewStyle().Foreground(p.theme.Muted).Italic(true)

	for i := start; i < end; i++ {
		idx := p.visible[i]
		item := p.items[idx]
		if p.separators[idx] {
			sb.WriteString(sepStyle.Render("   " + item))
		} else if i == p.cursor {
			cursor := " " + p.theme.Sym.Cursor + " "
			sb.WriteString(activeStyle.Render(cursor + item))
		} else {
			sb.WriteString(normalStyle.Render("   " + item))
		}
		if i < end-1 {
			sb.WriteString("\n")
		}
	}

	if len(p.visible) == 0 {
		dimStyle := lipgloss.NewStyle().Foreground(p.theme.Muted).Italic(true)
		sb.WriteString(dimStyle.Render("  (no matches)"))
	}

	return sb.String()
}
