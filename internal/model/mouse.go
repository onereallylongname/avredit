package model

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	doubleClickWindow = 400 * time.Millisecond
	wheelStep         = 3
)

// mouseClick remembers the last left click to detect double clicks.
type mouseClick struct {
	x, y int
	at   time.Time
}

// contextMenuItem is a right-click menu entry mapped to the key it triggers.
type contextMenuItem struct {
	label string
	key   string
}

var globalMenuItems = []contextMenuItem{
	{"Command line  (:)", ":"},
	{"Search  (/)", "/"},
	{"Help  (?)", "?"},
	{"Save  (ctrl+s)", "ctrl+s"},
}

var treeMenuItems = []contextMenuItem{
	{"Add field  (a)", "a"},
	{"Delete  (d)", "d"},
	{"Copy  (c)", "c"},
	{"Move  (m)", "m"},
	{"Undo  (u)", "u"},
}

// panelAt maps a screen position to the panel under it and the row index
// inside that panel's body (-1 on the border).
func (a App) panelAt(x, y int) (FocusPanel, int) {
	row := y - 1
	if y < 1 || y > a.height-2 {
		row = -1
	}
	if a.explorer.Visible() {
		if x < a.explorer.width {
			return PanelExplorer, row
		}
		x -= a.explorer.width
	}
	if x < a.tree.width {
		return PanelTree, row
	}
	return PanelDetails, row
}

// updateMouse handles wheel and click events for the three panels.
func (a App) updateMouse(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.overlay == OverlayPicker {
		if c, ok := msg.(tea.MouseClickMsg); ok {
			return a.clickPicker(c)
		}
		if w, ok := msg.(tea.MouseWheelMsg); ok {
			key := tea.KeyPressMsg{Code: tea.KeyDown}
			if w.Button == tea.MouseWheelUp {
				key = tea.KeyPressMsg{Code: tea.KeyUp}
			}
			return a.updateOverlay(key)
		}
		return a, nil
	}
	if a.overlay != OverlayNone || a.mode != ModeNormal {
		return a, nil
	}
	a.flash = ""

	switch m := msg.(type) {
	case tea.MouseWheelMsg:
		panel, _ := a.panelAt(m.X, m.Y)
		delta := wheelStep
		if m.Button == tea.MouseWheelUp {
			delta = -wheelStep
		} else if m.Button != tea.MouseWheelDown {
			return a, nil
		}
		return a.scrollPanel(panel, delta)
	case tea.MouseClickMsg:
		panel, row := a.panelAt(m.X, m.Y)
		switch m.Button {
		case tea.MouseLeft:
			return a.leftClick(panel, row, m.X, m.Y)
		case tea.MouseRight:
			return a.rightClick(panel, row)
		}
	}
	return a, nil
}

func (a App) scrollPanel(panel FocusPanel, delta int) (tea.Model, tea.Cmd) {
	switch panel {
	case PanelTree:
		a.tree.moveCursor(delta)
		a.syncDetailsToTree()
	case PanelExplorer:
		a.explorer.moveCursor(delta)
	case PanelDetails:
		key := tea.KeyPressMsg{Code: 'j', Text: "j"}
		if delta < 0 {
			key = tea.KeyPressMsg{Code: 'k', Text: "k"}
		}
		for i := 0; i < wheelStep; i++ {
			a.details.HandleKey(key)
		}
	}
	return a, nil
}

// selectRow focuses panel and moves its cursor to row, reporting whether the
// row was already selected and whether it exists.
func (a *App) selectRow(panel FocusPanel, row int) (already, ok bool) {
	a.focus = panel
	switch panel {
	case PanelTree:
		idx := a.tree.offset + row
		if row < 0 || idx >= len(a.tree.lines) {
			return false, false
		}
		already = idx == a.tree.cursor
		a.tree.cursor = idx
		a.syncDetailsToTree()
		return already, true
	case PanelExplorer:
		idx := a.explorer.offset + row
		if row < 0 || idx >= len(a.explorer.entries) {
			return false, false
		}
		already = idx == a.explorer.cursor
		a.explorer.cursor = idx
		return already, true
	}
	return false, false
}

func (a App) leftClick(panel FocusPanel, row, x, y int) (tea.Model, tea.Cmd) {
	now := time.Now()
	double := a.lastClick.x == x && a.lastClick.y == y && now.Sub(a.lastClick.at) <= doubleClickWindow
	a.lastClick = mouseClick{x: x, y: y, at: now}

	already, ok := a.selectRow(panel, row)
	if !ok {
		a.focus = panel
		return a, nil
	}
	switch panel {
	case PanelTree:
		switch {
		case double:
			a.tree.expandCurrent()
		case already:
			a.tree.toggleCurrent()
		}
	case PanelExplorer:
		if double || already {
			return a.updateExplorer(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
	}
	return a, nil
}

func (a App) rightClick(panel FocusPanel, row int) (tea.Model, tea.Cmd) {
	if panel == PanelTree {
		a.selectRow(panel, row)
	}
	return a.openContextMenu(panel), nil
}

// openContextMenu shows the menu for panel, using the current selection.
func (a App) openContextMenu(panel FocusPanel) App {
	items := globalMenuItems
	if panel == PanelTree {
		items = append(append([]contextMenuItem{}, treeMenuItems...), globalMenuItems...)
	}
	a.focus = panel
	labels := make([]string, len(items))
	for i, it := range items {
		labels[i] = it.label
	}
	a.contextMenu = items
	a.picker = NewPicker("Menu", labels, a.theme)
	a.overlay = OverlayPicker
	a.overlayAction = ActionContextMenu
	return a
}

// clickPicker selects the picker row under a left click; a click outside the
// box cancels the picker.
func (a App) clickPicker(m tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if m.Button != tea.MouseLeft {
		return a, nil
	}
	cancel := tea.KeyPressMsg{Code: tea.KeyEscape}
	lines := strings.Split(a.renderOverlay(), "\n")
	if m.Y < 0 || m.Y >= len(lines) {
		return a.updateOverlay(cancel)
	}
	line := ansi.Strip(lines[m.Y])
	left, right := strings.Index(line, "│"), strings.LastIndex(line, "│")
	if left < 0 || left == right {
		return a.updateOverlay(cancel)
	}
	if m.X < ansi.StringWidth(line[:left]) || m.X > ansi.StringWidth(line[:right]) {
		return a.updateOverlay(cancel)
	}

	best, bestIdx := "", -1
	for i, idx := range a.picker.visible {
		item := a.picker.items[idx]
		if a.picker.separators[idx] || strings.TrimSpace(item) == "" {
			continue
		}
		if strings.Contains(line, item) && len(item) > len(best) {
			best, bestIdx = item, i
		}
	}
	if bestIdx < 0 {
		return a, nil
	}
	a.picker.cursor = bestIdx
	return a.updateOverlay(tea.KeyPressMsg{Code: tea.KeyEnter})
}

// runContextMenuItem replays the key bound to the chosen menu entry.
func (a App) runContextMenuItem(label string) (tea.Model, tea.Cmd) {
	for _, it := range a.contextMenu {
		if it.label == label {
			return a.Update(keyMsgFor(it.key))
		}
	}
	return a, nil
}

func keyMsgFor(key string) tea.KeyPressMsg {
	if key == "ctrl+s" {
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	}
	r := []rune(key)[0]
	return tea.KeyPressMsg{Code: r, Text: key}
}
