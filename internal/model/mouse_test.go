package model

import (
	"strings"

	"charm.land/lipgloss/v2"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/onereallylongname/avredit/internal/config"
	"github.com/onereallylongname/avredit/internal/projection"
)

func newMouseTestApp(t *testing.T) App {
	t.Helper()
	projection.ResetNodeCounter()
	proj, err := projection.Build(map[string]any{
		"type": "record", "name": "User",
		"fields": []any{
			map[string]any{"name": "a", "type": "string"},
			map[string]any{"name": "b", "type": "string"},
			map[string]any{"name": "c", "type": "string"},
			map[string]any{"name": "d", "type": "string"},
			map[string]any{"name": "e", "type": "string"},
		},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	m, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m.(App)
}

func TestMouse_WheelMovesTreeCursor(t *testing.T) {
	app := newMouseTestApp(t)
	m, _ := app.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	if got := m.(App).tree.cursor; got != wheelStep {
		t.Errorf("cursor = %d, want %d", got, wheelStep)
	}
}

func TestMouse_ClickSelectsThenToggles(t *testing.T) {
	app := newMouseTestApp(t)
	// Row 0 is the root record (screen y=1); it is expanded by default.
	m, _ := app.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	app = m.(App)
	if app.tree.cursor != 2 || app.focus != PanelTree {
		t.Fatalf("cursor=%d focus=%v, want 2/tree", app.tree.cursor, app.focus)
	}

	lines := len(app.tree.lines)
	app.lastClick = mouseClick{}
	m, _ = app.Update(tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft})
	app = m.(App)
	app.lastClick = mouseClick{}
	m, _ = app.Update(tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft})
	if got := len(m.(App).tree.lines); got >= lines {
		t.Errorf("clicking the selected record should collapse it: %d lines, was %d", got, lines)
	}
}

func TestMouse_RightClickOpensMenuAndRunsItem(t *testing.T) {
	app := newMouseTestApp(t)
	m, _ := app.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseRight})
	app = m.(App)
	if app.overlay != OverlayPicker || app.overlayAction != ActionContextMenu {
		t.Fatalf("expected context menu overlay, got %v/%v", app.overlay, app.overlayAction)
	}
	app.overlay = OverlayNone
	m, _ = app.runContextMenuItem("Command line  (:)")
	if m.(App).mode != ModeCommand {
		t.Error("menu item should enter command mode")
	}
}

func TestThemePicker_CancelRestoresTheme(t *testing.T) {
	app := newMouseTestApp(t)
	orig := app.theme
	app.picker = NewPicker("Select theme", app.themeReg.Names(), app.theme)
	app.overlay = OverlayPicker
	app.overlayAction = ActionSelectTheme

	m, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	app = m.(App)
	if app.theme == orig && len(app.themeReg.Names()) > 1 {
		t.Error("moving through the picker should preview another theme")
	}
	m, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.(App).theme != orig {
		t.Error("cancelling should restore the original theme")
	}
}

func TestHelp_ScrollStopsAtBottomAndBoxSizeConstant(t *testing.T) {
	app := newMouseTestApp(t)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	app = m.(App)
	app.overlay = OverlayHelp

	for i := 0; i < 500; i++ {
		m, _ = app.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		app = m.(App)
	}
	if max := app.helpMaxScroll(); app.helpScroll != max {
		t.Fatalf("helpScroll = %d, want clamped to %d", app.helpScroll, max)
	}
	m, _ = app.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if m.(App).helpScroll != app.helpMaxScroll()-1 {
		t.Error("a single k should scroll up immediately")
	}

	height := func(a App) (int, int) {
		out := a.renderOverlay()
		return len(strings.Split(out, "\n")), lipgloss.Width(out)
	}
	top := app
	top.helpScroll = 0
	h1, w1 := height(top)
	h2, w2 := height(app)
	if h1 != h2 || w1 != w2 {
		t.Errorf("overlay size changed while scrolling: %dx%d vs %dx%d", w1, h1, w2, h2)
	}
	for _, l := range strings.Split(top.renderOverlay(), "\n") {
		if lipgloss.Width(l) > 80 {
			t.Errorf("line wider than window: %q", l)
		}
	}
}

func TestMouse_ClickMenuItemRunsIt(t *testing.T) {
	app := newMouseTestApp(t)
	m, _ := app.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseRight})
	app = m.(App)

	row := -1
	for y, l := range strings.Split(app.renderOverlay(), "\n") {
		if strings.Contains(l, "Search  (/)") {
			row = y
		}
	}
	if row < 0 {
		t.Fatal("menu item not rendered")
	}
	m, _ = app.Update(tea.MouseClickMsg{X: app.width / 2, Y: row, Button: tea.MouseLeft})
	app = m.(App)
	if app.mode != ModeSearch || app.overlay != OverlayNone {
		t.Errorf("mode=%v overlay=%v, want search with menu closed", app.mode, app.overlay)
	}

	app = newMouseTestApp(t)
	m, _ = app.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseRight})
	m, _ = m.(App).Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	if m.(App).overlay != OverlayNone {
		t.Error("click outside the menu should close it")
	}
}

func TestCommands_HelpAndOptions(t *testing.T) {
	app := newMouseTestApp(t)
	m, _ := app.executeCommand("help")
	if m.(App).overlay != OverlayHelp {
		t.Error(":help should open the help overlay")
	}
	m, _ = app.executeCommand("options")
	if got := m.(App); got.overlay != OverlayPicker || got.overlayAction != ActionContextMenu {
		t.Error(":options should open the context menu")
	}
}
