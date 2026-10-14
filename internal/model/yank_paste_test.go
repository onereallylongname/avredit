package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/onereallylongname/avredit/internal/config"
	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/theme"
)

// findNodeByName locates a node by its "name" attribute in the projection.
func findNodeByName(proj *projection.Projection, name string) *projection.Node {
	for _, node := range proj.Nodes {
		if node.Name() == name {
			return node
		}
	}
	return nil
}

func TestHandleYankKey_TreePane_YanksFieldName(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width, app.height, app.ready = 120, 40, true
	app.focus = PanelTree
	app.tree.JumpToNode(findNodeByName(proj, "id").ID)

	result, cmd := app.handleYankKey('y', "y")
	updated := result.(App)

	if cmd == nil {
		t.Fatal("expected a non-nil clipboard command")
	}
	if updated.flash != "yanked field name" {
		t.Errorf("flash = %q, want %q", updated.flash, "yanked field name")
	}
}

func TestHandleYankKey_TreePane_IgnoresNonYKey(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width, app.height, app.ready = 120, 40, true
	app.focus = PanelTree
	app.tree.JumpToNode(findNodeByName(proj, "id").ID)

	// "yp"/"ya" have no meaning in the tree pane (no name/value pair there).
	for _, key := range []string{"p", "a"} {
		result, cmd := app.handleYankKey('y', key)
		updated := result.(App)
		if cmd != nil {
			t.Errorf("key %q: expected nil cmd, got non-nil", key)
		}
		if updated.flash != "" {
			t.Errorf("key %q: flash = %q, want empty", key, updated.flash)
		}
	}
}

func TestHandleYankKey_PendingKeyMismatch_NoOp(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width, app.height, app.ready = 120, 40, true

	result, cmd := app.handleYankKey('x', "y")
	updated := result.(App)
	if cmd != nil {
		t.Error("expected nil cmd when pending key isn't 'y'")
	}
	if updated.flash != "" {
		t.Errorf("flash = %q, want empty", updated.flash)
	}
}

func TestHandleYankKey_DetailsPane_YankNameValueAndBoth(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width, app.height, app.ready = 120, 40, true
	app.focus = PanelDetails
	app.details.SetNode(findNodeByName(proj, "id"))
	app.details.cursor = 0 // "Name" attribute, value "id"

	cases := []struct {
		key       string
		wantFlash string
	}{
		{"p", "yanked property name"},
		{"y", "yanked property value"},
		{"a", "yanked property"},
	}
	for _, c := range cases {
		result, cmd := app.handleYankKey('y', c.key)
		updated := result.(App)
		if cmd == nil {
			t.Errorf("key %q: expected non-nil cmd", c.key)
		}
		if updated.flash != c.wantFlash {
			t.Errorf("key %q: flash = %q, want %q", c.key, updated.flash, c.wantFlash)
		}
	}
}

func TestHandleYankKey_DetailsPane_EmptyValueFlashesError(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width, app.height, app.ready = 120, 40, true
	app.focus = PanelDetails
	app.details.SetNode(findNodeByName(proj, "id"))
	app.details.cursor = 2 // "Doc" attribute, empty value for this field

	result, cmd := app.handleYankKey('y', "y")
	updated := result.(App)
	if cmd != nil {
		t.Error("expected nil cmd when there is nothing to yank")
	}
	if updated.flashLevel != FlashError {
		t.Errorf("flashLevel = %v, want FlashError", updated.flashLevel)
	}
}

// TestUpdate_YankSequence_TreePane exercises the full "y"+"y" two-key
// sequence through App.Update, verifying pendingKey is set then resolved.
func TestUpdate_YankSequence_TreePane(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width, app.height, app.ready = 120, 40, true
	app.focus = PanelTree
	app.tree.JumpToNode(findNodeByName(proj, "id").ID)

	result, _ := app.Update(tea.KeyPressMsg{Code: 'y'})
	mid := result.(App)
	if mid.pendingKey != 'y' {
		t.Fatalf("pendingKey = %q, want 'y'", mid.pendingKey)
	}

	result, _ = mid.Update(tea.KeyPressMsg{Code: 'y'})
	final := result.(App)
	if final.pendingKey != 0 {
		t.Errorf("pendingKey = %q, want cleared", final.pendingKey)
	}
	if final.flash != "yanked field name" {
		t.Errorf("flash = %q, want %q", final.flash, "yanked field name")
	}
}

// TestUpdate_YankSequence_UnrelatedKeyCancels verifies a mismatched second
// key cancels the pending sequence without performing a tree navigation
// (the key is consumed, like vim's unrecognized operator+motion).
func TestUpdate_YankSequence_UnrelatedKeyCancels(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width, app.height, app.ready = 120, 40, true
	app.focus = PanelTree

	result, _ := app.Update(tea.KeyPressMsg{Code: 'y'})
	mid := result.(App)

	result, _ = mid.Update(tea.KeyPressMsg{Code: 'j'})
	final := result.(App)
	if final.pendingKey != 0 {
		t.Errorf("pendingKey = %q, want cleared", final.pendingKey)
	}
	if final.flash != "" {
		t.Errorf("flash = %q, want empty (j should not navigate here)", final.flash)
	}
}

func TestEditor_InsertText(t *testing.T) {
	e := NewEditor("hello", EditorStyle{})
	e.cursor = 5 // end
	e.InsertText(" world")
	if e.Value() != "hello world" {
		t.Errorf("Value() = %q, want %q", e.Value(), "hello world")
	}

	e2 := NewEditor("ac", EditorStyle{})
	e2.cursor = 1
	e2.InsertText("b")
	if e2.Value() != "abc" {
		t.Errorf("Value() = %q, want %q", e2.Value(), "abc")
	}

	// Embedded newlines are preserved as literal runes (same as InsertRune('\n')).
	e3 := NewEditor("", EditorStyle{})
	e3.InsertText("line1\nline2")
	if e3.Value() != "line1\nline2" {
		t.Errorf("Value() = %q, want %q", e3.Value(), "line1\nline2")
	}
}

func TestDetailsModel_PasteText(t *testing.T) {
	proj := buildTestProjection(t)
	th := theme.Dark()
	details := NewDetailsModel(proj, th)
	details.SetSize(80, 40)
	details.SetNode(findNodeByName(proj, "id"))

	if !details.BeginEdit() {
		t.Fatal("BeginEdit failed")
	}
	details.PasteText("-pasted")
	if got := details.editor.Value(); got != "id-pasted" {
		t.Errorf("editor value = %q, want %q", got, "id-pasted")
	}
}

func TestDetailsModel_PasteText_NoOpWhenNotEditing(t *testing.T) {
	proj := buildTestProjection(t)
	th := theme.Dark()
	details := NewDetailsModel(proj, th)
	details.SetSize(80, 40)
	details.SetNode(findNodeByName(proj, "id"))

	// Not editing: paste should be a silent no-op (no panic, no mutation).
	details.PasteText("x")
}

func TestInsertRunesAt(t *testing.T) {
	buf, cursor := insertRunesAt([]rune("ac"), 1, "b")
	if string(buf) != "abc" || cursor != 2 {
		t.Errorf("got buf=%q cursor=%d, want buf=%q cursor=2", string(buf), cursor, "abc")
	}

	buf, cursor = insertRunesAt(nil, 0, "hi")
	if string(buf) != "hi" || cursor != 2 {
		t.Errorf("got buf=%q cursor=%d, want buf=%q cursor=2", string(buf), cursor, "hi")
	}
}

func TestPasteText_SearchMode(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width, app.height, app.ready = 120, 40, true
	app.mode = ModeSearch

	app.pasteText("n:id")
	if string(app.searchBuf) != "n:id" {
		t.Errorf("searchBuf = %q, want %q", string(app.searchBuf), "n:id")
	}
	if app.searchCursor != len("n:id") {
		t.Errorf("searchCursor = %d, want %d", app.searchCursor, len("n:id"))
	}
	if len(app.searchResults) == 0 {
		t.Error("expected search to run and find the 'id' field")
	}
}

func TestPasteText_CommandMode(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width, app.height, app.ready = 120, 40, true
	app.mode = ModeCommand

	app.pasteText("export")
	if string(app.cmdBuf) != "export" {
		t.Errorf("cmdBuf = %q, want %q", string(app.cmdBuf), "export")
	}
}

func TestPasteText_NormalMode_NoOp(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width, app.height, app.ready = 120, 40, true
	app.mode = ModeNormal

	app.pasteText("ignored")
	if len(app.searchBuf) != 0 || len(app.cmdBuf) != 0 {
		t.Error("expected no mutation in Normal mode")
	}
}

// TestUpdate_PasteMsg_RoutesToSearchBar verifies bracketed-paste
// (tea.PasteMsg) is routed the same way as the ctrl+v/shift+insert fallback.
func TestUpdate_PasteMsg_RoutesToSearchBar(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width, app.height, app.ready = 120, 40, true
	app.mode = ModeSearch

	result, _ := app.Update(tea.PasteMsg{Content: "pasted"})
	updated := result.(App)
	if string(updated.searchBuf) != "pasted" {
		t.Errorf("searchBuf = %q, want %q", string(updated.searchBuf), "pasted")
	}
}
