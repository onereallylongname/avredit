package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/onereallylongname/avredit/internal/command"
	"github.com/onereallylongname/avredit/internal/config"
	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/schema"
	"github.com/onereallylongname/avredit/internal/theme"
)

// stripANSI removes ANSI escape sequences for test assertions.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func buildTestProjection(t *testing.T) *projection.Projection {
	t.Helper()
	projection.ResetNodeCounter()
	raw := map[string]any{
		"type":      "record",
		"name":      "User",
		"namespace": "com.example",
		"fields": []any{
			map[string]any{
				"name": "id",
				"type": "int",
			},
			map[string]any{
				"name":     "email",
				"type":     "string",
				"x-pii":    true,
				"x-source": "registration",
			},
		},
	}
	proj, err := projection.Build(raw)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return proj
}

func TestTreeModel_Flatten(t *testing.T) {
	proj := buildTestProjection(t)
	th := theme.Dark()
	tree := NewTreeModel(proj, th)

	// Root is expanded + its first child (record), so we should see:
	// schema → record → field(id) → field(email)
	// At minimum the schema and record should be visible
	if len(tree.lines) < 4 {
		t.Fatalf("expected at least 4 visible lines, got %d", len(tree.lines))
	}

	// First line should be schema
	if tree.lines[0].Node.Kind != "schema" {
		t.Errorf("first line kind = %q, want schema", tree.lines[0].Node.Kind)
	}
}

func TestTreeModel_Navigation(t *testing.T) {
	proj := buildTestProjection(t)
	th := theme.Dark()
	tree := NewTreeModel(proj, th)
	tree.SetSize(60, 20)

	// Start at cursor 0
	if tree.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", tree.cursor)
	}

	// Move down
	tree = tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	if tree.cursor != 1 {
		t.Errorf("after j: cursor = %d, want 1", tree.cursor)
	}

	// Move down again
	tree = tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	if tree.cursor != 2 {
		t.Errorf("after 2x j: cursor = %d, want 2", tree.cursor)
	}

	// Move up
	tree = tree.HandleKey(tea.KeyPressMsg{Code: 'k'})
	if tree.cursor != 1 {
		t.Errorf("after k: cursor = %d, want 1", tree.cursor)
	}

	// Jump to top
	tree = tree.HandleKey(tea.KeyPressMsg{Code: 'G'})
	if tree.cursor != len(tree.lines)-1 {
		t.Errorf("after G: cursor = %d, want %d", tree.cursor, len(tree.lines)-1)
	}

	tree = tree.HandleKey(tea.KeyPressMsg{Code: 'g'})
	if tree.cursor != 0 {
		t.Errorf("after g: cursor = %d, want 0", tree.cursor)
	}
}

func TestTreeModel_ExpandCollapse(t *testing.T) {
	proj := buildTestProjection(t)
	th := theme.Dark()
	tree := NewTreeModel(proj, th)
	tree.SetSize(60, 20)

	initialLines := len(tree.lines)

	// Collapse the record (line 1 is the record in expanded state)
	tree.cursor = 1
	tree = tree.HandleKey(tea.KeyPressMsg{Code: 'h'})

	if len(tree.lines) >= initialLines {
		t.Errorf("after collapse: %d lines, expected fewer than %d", len(tree.lines), initialLines)
	}

	// Re-expand
	tree = tree.HandleKey(tea.KeyPressMsg{Code: 'l'})
	if len(tree.lines) != initialLines {
		t.Errorf("after re-expand: %d lines, want %d", len(tree.lines), initialLines)
	}
}

func TestDetailsModel_CustomAttributes(t *testing.T) {
	proj := buildTestProjection(t)
	th := theme.Dark()
	details := NewDetailsModel(proj, th)
	details.SetSize(80, 40)

	// Find the email field node (has x-pii and x-source custom attributes)
	var emailNode *projection.Node
	for _, node := range proj.Nodes {
		if node.Name() == "email" {
			emailNode = node
			break
		}
	}
	if emailNode == nil {
		t.Fatal("email node not found")
	}

	details.SetNode(emailNode)
	view := stripANSI(details.View())

	// Verify custom attributes section appears
	if !strings.Contains(view, "Custom Attributes") {
		t.Errorf("expected 'Custom Attributes' section in details view, got:\n%s", view)
	}
	if !strings.Contains(view, "x-pii") {
		t.Errorf("expected 'x-pii' custom attribute in view, got:\n%s", view)
	}
	if !strings.Contains(view, "x-source") {
		t.Errorf("expected 'x-source' custom attribute in view, got:\n%s", view)
	}
}

func TestDetailsModel_StandardAttributes(t *testing.T) {
	proj := buildTestProjection(t)
	th := theme.Dark()
	details := NewDetailsModel(proj, th)
	details.SetSize(80, 40)

	// Set to root node
	root := proj.Nodes[proj.RootID]
	details.SetNode(root)
	view := stripANSI(details.View())

	// Kind is now a muted title badge
	if !strings.Contains(view, "SCHEMA") {
		t.Errorf("expected 'SCHEMA' kind badge in view, got:\n%s", view)
	}
	// Name attribute should be present
	if !strings.Contains(view, "Name") {
		t.Errorf("expected 'Name' attribute in view, got:\n%s", view)
	}
	// ID, Path, Parent should NOT be shown
	if strings.Contains(view, "ID:") {
		t.Error("ID should not be displayed in details")
	}
	if strings.Contains(view, "Parent:") {
		t.Error("Parent should not be displayed in details")
	}
}

func TestStatusBarModel_View(t *testing.T) {
	th := theme.Dark()
	sb := NewStatusBarModel(th, "test.avsc")
	sb.SetWidth(80)
	sb.SetMode(ModeNormal)
	sb.SetStats("5 fields, depth 3")

	view := sb.View()
	if !strings.Contains(view, "NORMAL") {
		t.Error("expected 'NORMAL' mode in status bar")
	}
	if !strings.Contains(view, "test.avsc") {
		t.Error("expected file name in status bar")
	}
}

func TestStatusBarModel_NodePath(t *testing.T) {
	th := theme.Dark()
	sb := NewStatusBarModel(th, "test.avsc")
	sb.SetWidth(120)
	sb.SetMode(ModeNormal)
	sb.SetNodePath("fields › 0 › type")

	view := stripANSI(sb.View())
	if !strings.Contains(view, "fields") {
		t.Errorf("expected node path in status bar, got:\n%s", view)
	}
}

func TestApp_EscapeFromModes(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Enter search mode
	app.mode = ModeSearch
	if app.mode != ModeSearch {
		t.Fatal("expected search mode")
	}

	// Escape should return to Normal
	result, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated := result.(App)
	if updated.mode != ModeNormal {
		t.Errorf("escape from Search: mode = %v, want Normal", updated.mode)
	}

	// Enter command mode, ctrl+c should return to Normal
	updated.mode = ModeCommand
	result, _ = updated.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	updated = result.(App)
	if updated.mode != ModeNormal {
		t.Errorf("ctrl+c from Command: mode = %v, want Normal", updated.mode)
	}
}

// --- Phase 3 Tests ---

func TestApp_UndoRedo(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Move to the "id" field node (cursor 2 in tree, after schema and record)
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()

	sel := app.tree.SelectedNode()
	if sel == nil || sel.Name() != "id" {
		t.Fatalf("expected id field selected, got %v", sel)
	}

	// Rename the field via UpdateAttribute
	originalName := sel.Name()
	cmd, err := command.UpdateAttribute(proj, command.UpdateAttributeParams{
		NodeID:   sel.ID,
		Scope:    "native",
		Key:      "name",
		NewValue: "user_id",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = app.history.Execute(cmd)

	if sel.Name() != "user_id" {
		t.Errorf("after do: name = %q, want user_id", sel.Name())
	}

	// Undo
	result, _ := app.Update(tea.KeyPressMsg{Code: 'u'})
	app = result.(App)
	if sel.Name() != originalName {
		t.Errorf("after undo: name = %q, want %q", sel.Name(), originalName)
	}

	// Redo
	result, _ = app.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	app = result.(App)
	if sel.Name() != "user_id" {
		t.Errorf("after redo: name = %q, want user_id", sel.Name())
	}
}

func TestApp_ActionCopy(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Navigate to "id" field
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()

	sel := app.tree.SelectedNode()
	if sel == nil || sel.Name() != "id" {
		t.Fatalf("expected id field, got %v", sel)
	}

	// Count fields before
	parent := proj.GetParent(sel)
	fieldsBefore := len(parent.Children)

	// Copy
	result, _ := app.Update(tea.KeyPressMsg{Code: 'c'})
	app = result.(App)

	fieldsAfter := len(parent.Children)
	if fieldsAfter != fieldsBefore+1 {
		t.Errorf("after copy: %d children, want %d", fieldsAfter, fieldsBefore+1)
	}

	// Undo should restore
	result, _ = app.Update(tea.KeyPressMsg{Code: 'u'})
	app = result.(App)
	if len(parent.Children) != fieldsBefore {
		t.Errorf("after undo: %d children, want %d", len(parent.Children), fieldsBefore)
	}
}

func TestApp_ActionDelete(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Navigate to "id" field
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()

	sel := app.tree.SelectedNode()
	if sel == nil || sel.Name() != "id" {
		t.Fatalf("expected id field, got %v", sel)
	}

	parent := proj.GetParent(sel)
	fieldsBefore := len(parent.Children)

	// Press 'd' to delete — should delete immediately (no confirm)
	result, _ := app.Update(tea.KeyPressMsg{Code: 'd'})
	app = result.(App)
	if app.overlay != OverlayNone {
		t.Fatal("overlay should not be shown on delete")
	}
	if len(parent.Children) != fieldsBefore-1 {
		t.Errorf("after delete: %d children, want %d", len(parent.Children), fieldsBefore-1)
	}

	// Undo
	result, _ = app.Update(tea.KeyPressMsg{Code: 'u'})
	app = result.(App)
	if len(parent.Children) != fieldsBefore {
		t.Errorf("after undo: %d children, want %d", len(parent.Children), fieldsBefore)
	}
}

func TestApp_EditMode(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Navigate to "id" field
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()

	// Enter focuses the details pane (stays Normal)
	result, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = result.(App)
	if app.mode != ModeNormal {
		t.Errorf("expected Normal mode after first enter, got %v", app.mode)
	}
	if app.focus != PanelDetails {
		t.Errorf("expected details focus, got %v", app.focus)
	}

	// Enter again in details starts edit mode
	result, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = result.(App)
	if app.mode != ModeEdit {
		t.Errorf("expected Edit mode after second enter, got %v", app.mode)
	}
	if app.focus != PanelDetails {
		t.Errorf("expected details focus, got %v", app.focus)
	}

	// Esc returns to Normal
	result, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	app = result.(App)
	if app.mode != ModeNormal {
		t.Errorf("expected Normal mode after esc, got %v", app.mode)
	}
}

func TestDetailsModel_EditAttribute(t *testing.T) {
	proj := buildTestProjection(t)
	th := theme.Dark()
	details := NewDetailsModel(proj, th)
	details.SetSize(80, 40)

	// Find the "id" field
	var idNode *projection.Node
	for _, node := range proj.Nodes {
		if node.Name() == "id" {
			idNode = node
			break
		}
	}
	if idNode == nil {
		t.Fatal("id node not found")
	}

	details.SetNode(idNode)

	// Cursor should be at 0 (Name attribute)
	attr := details.CursorAttr()
	if attr == nil {
		t.Fatal("no cursor attr")
	}
	if attr.Key != "Name" {
		t.Errorf("cursor attr key = %q, want 'Name'", attr.Key)
	}

	// Begin edit
	ok := details.BeginEdit()
	if !ok {
		t.Fatal("BeginEdit returned false")
	}
	if !details.Editing() {
		t.Fatal("expected editing state")
	}

	// Cancel
	details.CancelEdit()
	if details.Editing() {
		t.Fatal("expected not editing after cancel")
	}
}

func TestEditor_BasicOperations(t *testing.T) {
	style := EditorStyle{}
	e := NewEditor("hello", style)

	if e.Value() != "hello" {
		t.Errorf("initial value = %q", e.Value())
	}

	// Cursor at end (5), type a character
	e.HandleKey(tea.KeyPressMsg{Code: '!'})
	if e.Value() != "hello!" {
		t.Errorf("after append = %q", e.Value())
	}

	// Move left and insert
	e.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	e.HandleKey(tea.KeyPressMsg{Code: 'X'})
	// "hello!" cursor was at 6. After left cursor=5. Insert 'X' at pos 5: "helloX!"
	if e.Value() != "helloX!" {
		t.Errorf("after insert mid = %q, want 'helloX!'", e.Value())
	}

	// Backspace
	e.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if e.Value() != "hello!" {
		t.Errorf("after backspace = %q, want 'hello!'", e.Value())
	}
}

func TestPicker_BasicNavigation(t *testing.T) {
	th := theme.Dark()
	items := []string{"int", "string", "boolean", "record", "array"}
	p := NewPicker("Select type", items, th)

	// Initial cursor at 0
	if p.Selected() != "int" {
		t.Errorf("initial selection = %q, want 'int'", p.Selected())
	}

	// Move down
	p.HandleKey(tea.KeyPressMsg{Code: 'j'})
	if p.Selected() != "string" {
		t.Errorf("after j = %q, want 'string'", p.Selected())
	}

	// Filter with 'r'
	p.HandleKey(tea.KeyPressMsg{Code: 'r'})
	// Filter = "jr", should match "string" only? Actually filter is just "r" 
	// because j navigated. Wait - in picker, 'j' is handled as navigation not as filter char.
	// So filter is now "r", let's check visible
	if p.filter != "r" {
		// Actually 'j' moves cursor, doesn't set filter. Let me re-check.
		// After j: cursor moves. Then HandleKey('r') => not a control key, so it adds to filter.
		// But wait - 'j' was handled as navigation in the switch. Let me trace:
		// First call: HandleKey('j') => case "j", "down": cursor++ => cursor=1, filter=""
		// Second call: HandleKey('r') => default case: filter += "r" => filter="r"
		// So filter is "r", and visible should be items containing "r": "string", "record", "array"
		t.Logf("filter = %q, visible count = %d", p.filter, len(p.visible))
	}

	// Select with enter
	sel, done := p.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !done {
		t.Error("expected done after enter")
	}
	if sel == "" {
		t.Error("expected non-empty selection")
	}
}

func TestConfirm_YesNo(t *testing.T) {
	th := theme.Dark()
	c := NewConfirm("Delete node?", th)

	if !c.Active() {
		t.Fatal("expected active")
	}

	// Press 'n' — cancel
	confirmed, done := c.HandleKey(tea.KeyPressMsg{Code: 'n'})
	if !done || confirmed {
		t.Error("expected done=true, confirmed=false for 'n'")
	}

	// New confirm, press 'y'
	c = NewConfirm("Delete?", th)
	confirmed, done = c.HandleKey(tea.KeyPressMsg{Code: 'y'})
	if !done || !confirmed {
		t.Error("expected done=true, confirmed=true for 'y'")
	}
}

// --- Action Undo/Redo Tests ---

// snapshotProj serializes projection to JSON for comparison.
func snapshotProj(t *testing.T, proj *projection.Projection) string {
	t.Helper()
	data, err := projection.GenerateAvro(proj)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	return string(b)
}

// buildMoveTestProjection creates a schema with two records (User has a nested Address).
func buildMoveTestProjection(t *testing.T) *projection.Projection {
	t.Helper()
	projection.ResetNodeCounter()
	raw := map[string]any{
		"type":      "record",
		"name":      "User",
		"namespace": "com.example",
		"fields": []any{
			map[string]any{"name": "id", "type": "int"},
			map[string]any{"name": "name", "type": "string"},
			map[string]any{
				"name": "address",
				"type": map[string]any{
					"type": "record",
					"name": "Address",
					"fields": []any{
						map[string]any{"name": "street", "type": "string"},
						map[string]any{"name": "city", "type": "string"},
					},
				},
			},
		},
	}
	proj, err := projection.Build(raw)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return proj
}

func TestAction_AddField_UndoRedo(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Navigate to a field node (line 2 = id field)
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()

	original := snapshotProj(t, proj)

	// Press 'a' to trigger AddField → opens picker
	result, _ := app.Update(tea.KeyPressMsg{Code: 'a'})
	app = result.(App)
	if app.overlay != OverlayPicker {
		t.Fatal("expected picker overlay after 'a'")
	}

	// Select "string" from picker
	result, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = result.(App)

	afterAdd := snapshotProj(t, proj)
	if original == afterAdd {
		t.Fatal("snapshot should differ after add")
	}

	// Undo
	result, _ = app.Update(tea.KeyPressMsg{Code: 'u'})
	app = result.(App)
	afterUndo := snapshotProj(t, proj)
	if afterUndo != original {
		t.Errorf("after undo should match original\ngot:\n%s\nwant:\n%s", afterUndo, original)
	}

	// Redo
	result, _ = app.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	app = result.(App)
	afterRedo := snapshotProj(t, proj)
	if afterRedo != afterAdd {
		t.Errorf("after redo should match post-add\ngot:\n%s\nwant:\n%s", afterRedo, afterAdd)
	}
}

func TestAction_Delete_UndoRedo(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Navigate to email field (line 3)
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()

	sel := app.tree.SelectedNode()
	if sel == nil || sel.Name() != "email" {
		t.Fatalf("expected email selected, got %v", sel)
	}

	original := snapshotProj(t, proj)

	// Press 'd' → deletes immediately
	result, _ := app.Update(tea.KeyPressMsg{Code: 'd'})
	app = result.(App)

	afterDelete := snapshotProj(t, proj)
	if original == afterDelete {
		t.Fatal("snapshot should differ after delete")
	}

	// Undo
	result, _ = app.Update(tea.KeyPressMsg{Code: 'u'})
	app = result.(App)
	afterUndo := snapshotProj(t, proj)
	if afterUndo != original {
		t.Errorf("after undo should match original\ngot:\n%s\nwant:\n%s", afterUndo, original)
	}

	// Redo
	result, _ = app.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	app = result.(App)
	afterRedo := snapshotProj(t, proj)
	if afterRedo != afterDelete {
		t.Errorf("after redo should match post-delete\ngot:\n%s\nwant:\n%s", afterRedo, afterDelete)
	}
}

func TestAction_Copy_UndoRedo(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Navigate to id field (line 2)
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()

	original := snapshotProj(t, proj)

	// Press 'c' to copy
	result, _ := app.Update(tea.KeyPressMsg{Code: 'c'})
	app = result.(App)

	afterCopy := snapshotProj(t, proj)
	if original == afterCopy {
		t.Fatal("snapshot should differ after copy")
	}

	// Undo
	result, _ = app.Update(tea.KeyPressMsg{Code: 'u'})
	app = result.(App)
	afterUndo := snapshotProj(t, proj)
	if afterUndo != original {
		t.Errorf("after undo should match original\ngot:\n%s\nwant:\n%s", afterUndo, original)
	}

	// Redo
	result, _ = app.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	app = result.(App)
	afterRedo := snapshotProj(t, proj)
	if afterRedo != afterCopy {
		t.Errorf("after redo should match post-copy\ngot:\n%s\nwant:\n%s", afterRedo, afterCopy)
	}
}

func TestApp_SaveClearsDirty(t *testing.T) {
	proj := buildTestProjection(t)
	// Use temp dir to avoid leaving files in the source tree
	tmpFile := filepath.Join(t.TempDir(), "test.avsc")
	app := NewApp(proj, tmpFile, config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Make a change to set dirty
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()
	result, _ := app.Update(tea.KeyPressMsg{Code: 'd'})
	app = result.(App)
	if !app.dirty {
		t.Fatal("expected dirty=true after delete")
	}

	// Save with :w
	app.mode = ModeCommand
	app.cmdBuf = []rune("w")
	result, _ = app.Update(tea.KeyPressMsg{Code: 0x0D}) // enter
	app = result.(App)

	if app.dirty {
		t.Fatal("expected dirty=false after :w save")
	}
	if app.mode != ModeNormal {
		t.Fatalf("expected ModeNormal after command, got %d", app.mode)
	}

	// Verify file was actually written
	if _, err := os.Stat(tmpFile); err != nil {
		t.Fatalf("expected file to exist at %s: %v", tmpFile, err)
	}

	// Now q should NOT trigger confirmation
	result, cmd := app.Update(tea.KeyPressMsg{Code: 'q'})
	app = result.(App)
	if app.overlay == OverlayConfirm {
		t.Fatal("quit should not prompt after save")
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit command")
	}
}

func TestApp_SaveAsWritesToExplorerRoot(t *testing.T) {
	proj := buildTestProjection(t)
	tmpDir := t.TempDir()
	origFile := filepath.Join(tmpDir, "original.avsc")
	app := NewApp(proj, origFile, config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Verify explorer root is the temp dir
	if app.explorer.RootDir() != tmpDir {
		t.Fatalf("expected explorer root=%s, got=%s", tmpDir, app.explorer.RootDir())
	}

	// Make a change
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()
	result, _ := app.Update(tea.KeyPressMsg{Code: 'd'})
	app = result.(App)

	// :w newfile.avsc — should save to tmpDir/newfile.avsc
	app.mode = ModeCommand
	app.cmdBuf = []rune("w newfile.avsc")
	result, _ = app.Update(tea.KeyPressMsg{Code: 0x0D})
	app = result.(App)

	expectedPath := filepath.Join(tmpDir, "newfile.avsc")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected file at %s but got error: %v", expectedPath, err)
	}
	if app.filePath != expectedPath {
		t.Fatalf("expected filePath=%s, got=%s", expectedPath, app.filePath)
	}
	if app.dirty {
		t.Fatal("expected dirty=false after save-as")
	}
}

func TestAction_Move_UndoRedo(t *testing.T) {
	proj := buildMoveTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Find the "name" field and "Address" record
	var nameFieldID, addressRecordID string
	for id, n := range proj.Nodes {
		if n.Kind == "field" && n.Name() == "name" {
			nameFieldID = id
		}
		if n.Kind == "record" && n.Name() == "Address" {
			addressRecordID = id
		}
	}
	if nameFieldID == "" || addressRecordID == "" {
		t.Fatal("couldn't find name field or Address record")
	}

	// Navigate tree cursor to the name field
	app.tree.JumpToNode(nameFieldID)
	app.syncDetailsToTree()

	original := snapshotProj(t, proj)

	// Execute move directly (since picker interaction is complex)
	cmd, err := command.MoveNode(proj, command.MoveNodeParams{
		NodeID:   nameFieldID,
		TargetID: addressRecordID,
		Index:    -1,
	})
	if err != nil {
		t.Fatalf("MoveNode: %v", err)
	}
	_ = app.history.Execute(cmd)
	app.dirty = true

	afterMove := snapshotProj(t, proj)
	if original == afterMove {
		t.Fatal("snapshot should differ after move")
	}

	// Verify the field is now in Address
	addressNode := proj.Get(addressRecordID)
	found := false
	for _, childID := range addressNode.Children {
		if childID == nameFieldID {
			found = true
			break
		}
	}
	if !found {
		t.Error("name field should be in Address children after move")
	}

	// Undo
	_ = app.history.Undo()
	afterUndo := snapshotProj(t, proj)
	if afterUndo != original {
		t.Errorf("after undo should match original\ngot:\n%s\nwant:\n%s", afterUndo, original)
	}

	// Redo
	_ = app.history.Redo()
	afterRedo := snapshotProj(t, proj)
	if afterRedo != afterMove {
		t.Errorf("after redo should match post-move\ngot:\n%s\nwant:\n%s", afterRedo, afterMove)
	}
}

func buildArrayTestProjection(t *testing.T) *projection.Projection {
	t.Helper()
	projection.ResetNodeCounter()
	raw := map[string]any{
		"type":      "record",
		"name":      "Order",
		"namespace": "com.example",
		"fields": []any{
			map[string]any{
				"name": "id",
				"type": "int",
			},
			map[string]any{
				"name": "tags",
				"type": map[string]any{"type": "array", "items": "string"},
			},
			map[string]any{
				"name": "metadata",
				"type": map[string]any{"type": "map", "values": "int"},
			},
		},
	}
	proj, err := projection.Build(raw)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return proj
}

func TestApp_ArrayItemsTypeChange_FullFlow(t *testing.T) {
	proj := buildArrayTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Find the array node for "tags" field
	var arrayNode *projection.Node
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindArray {
			arrayNode = n
			break
		}
	}
	if arrayNode == nil {
		t.Fatal("array node not found")
	}

	// Navigate tree to array node
	app.tree.JumpToNode(arrayNode.ID)
	app.syncDetailsToTree()

	// Verify we're on the array node
	sel := app.tree.SelectedNode()
	if sel == nil || sel.Kind != schema.KindArray {
		t.Fatalf("expected array node selected, got %v", sel)
	}

	// Verify details shows "Items Type" and it's the first attr
	if len(app.details.attrs) == 0 {
		t.Fatal("expected details attrs for array, got none")
	}
	firstAttr := app.details.attrs[0]
	if firstAttr.Key != "Items Type" {
		t.Fatalf("expected first attr key = 'Items Type', got %q", firstAttr.Key)
	}
	if firstAttr.Value != "string" {
		t.Fatalf("expected initial items type 'string', got %q", firstAttr.Value)
	}

	// Switch focus to details
	app.focus = PanelDetails
	app.details.cursor = 0

	// Press enter to begin edit on "Items Type" (triggers picker with ActionReplace)
	result, _ := app.Update(tea.KeyPressMsg{Code: 0x0D}) // enter
	app = result.(App)

	if app.overlay != OverlayPicker {
		t.Fatalf("expected OverlayPicker, got %d", app.overlay)
	}
	if app.overlayAction != ActionReplace {
		t.Fatalf("expected ActionReplace, got %d", app.overlayAction)
	}

	// Simulate picker interaction via Update (just like real user)
	// Use filter to find "int" in the categorized picker
	pickerItems := app.picker.items
	found := false
	for _, item := range pickerItems {
		if item == "int" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("'int' not found in picker items")
	}

	// Type "int" to filter, then select
	for _, ch := range "int" {
		result, _ = app.Update(tea.KeyPressMsg{Code: ch})
		app = result.(App)
	}

	// Verify picker shows "int" selected
	if app.picker.Selected() != "int" {
		t.Fatalf("expected picker selected = 'int', got %q", app.picker.Selected())
	}

	// Press enter to confirm selection (this goes through updateOverlay)
	result, _ = app.Update(tea.KeyPressMsg{Code: 0x0D})
	app = result.(App)

	// Verify overlay is closed
	if app.overlay != OverlayNone {
		t.Fatalf("expected overlay closed, got %d", app.overlay)
	}

	// Verify the items type was changed
	// The array node should now have a child that is KindPrimitive with native="int"
	if len(arrayNode.Children) == 0 {
		t.Fatal("array node has no children after replace")
	}
	newChild := proj.Nodes[arrayNode.Children[0]]
	if newChild == nil {
		t.Fatal("new child node not found in projection")
	}
	if newChild.Kind != schema.KindPrimitive {
		t.Fatalf("expected new child kind = KindPrimitive, got %s", newChild.Kind)
	}
	if newChild.NativeString() != "int" {
		t.Fatalf("expected new child native = 'int', got %q", newChild.NativeString())
	}

	// Verify details was refreshed
	if len(app.details.attrs) > 0 && app.details.attrs[0].Key == "Items Type" {
		if app.details.attrs[0].Value != "int" {
			t.Fatalf("expected details to show items type 'int', got %q", app.details.attrs[0].Value)
		}
	}

	// Verify dirty flag
	if !app.dirty {
		t.Fatal("expected dirty=true after replace")
	}
}

func TestApp_MapValuesTypeChange_FullFlow(t *testing.T) {
	proj := buildArrayTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Find the map node for "metadata" field
	var mapNode *projection.Node
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindMap {
			mapNode = n
			break
		}
	}
	if mapNode == nil {
		t.Fatal("map node not found")
	}

	// Navigate tree to map node
	app.tree.JumpToNode(mapNode.ID)
	app.syncDetailsToTree()

	// Verify we're on the map node
	sel := app.tree.SelectedNode()
	if sel == nil || sel.Kind != schema.KindMap {
		t.Fatalf("expected map node selected, got %v", sel)
	}

	// Verify details shows "Values Type"
	if len(app.details.attrs) == 0 {
		t.Fatal("expected details attrs for map, got none")
	}
	firstAttr := app.details.attrs[0]
	if firstAttr.Key != "Values Type" {
		t.Fatalf("expected first attr key = 'Values Type', got %q", firstAttr.Key)
	}
	if firstAttr.Value != "int" {
		t.Fatalf("expected initial values type 'int', got %q", firstAttr.Value)
	}

	// Switch focus to details
	app.focus = PanelDetails
	app.details.cursor = 0

	// Press enter to begin edit (triggers picker with ActionReplace)
	result, _ := app.Update(tea.KeyPressMsg{Code: 0x0D})
	app = result.(App)

	if app.overlay != OverlayPicker {
		t.Fatalf("expected OverlayPicker, got %d", app.overlay)
	}
	if app.overlayAction != ActionReplace {
		t.Fatalf("expected ActionReplace, got %d", app.overlayAction)
	}

	// Simulate picker selecting "long"
	result, _ = app.handlePickerResult("long")
	app = result.(App)

	// Verify the values type was changed
	if len(mapNode.Children) == 0 {
		t.Fatal("map node has no children after replace")
	}
	newChild := proj.Nodes[mapNode.Children[0]]
	if newChild == nil {
		t.Fatal("new child node not found in projection")
	}
	if newChild.Kind != schema.KindPrimitive {
		t.Fatalf("expected new child kind = KindPrimitive, got %s", newChild.Kind)
	}
	if newChild.NativeString() != "long" {
		t.Fatalf("expected new child native = 'long', got %q", newChild.NativeString())
	}

	if !app.dirty {
		t.Fatal("expected dirty=true after replace")
	}
}

func TestApp_NewArrayField_ChangeItemsType(t *testing.T) {
	// This tests the exact user bug: add a new field of type "array", then
	// try to change its items type from the details panel.
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Navigate to the record node (line 1)
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()
	sel := app.tree.SelectedNode()
	if sel == nil || sel.Kind != schema.KindRecord {
		t.Fatalf("expected record selected, got %v", sel)
	}

	// Add a new field of type "array" via the picker flow
	app.overlayAction = ActionAddField
	result, _ := app.handlePickerResult("array")
	app = result.(App)

	// Find the newly created array node
	var arrayNode *projection.Node
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindArray {
			arrayNode = n
			break
		}
	}
	if arrayNode == nil {
		t.Fatal("array node not found after adding array field")
	}

	// The array should have a child (items type) node
	if len(arrayNode.Children) == 0 {
		t.Fatal("newly created array node has no items child — this is the bug")
	}
	itemsChild := proj.Nodes[arrayNode.Children[0]]
	if itemsChild == nil {
		t.Fatal("items child node not in projection")
	}
	if itemsChild.Kind != schema.KindPrimitive {
		t.Fatalf("expected items child to be primitive, got %s", itemsChild.Kind)
	}

	// Navigate to the array node
	app.tree.JumpToNode(arrayNode.ID)
	app.syncDetailsToTree()

	// Verify details shows "Items Type"
	if len(app.details.attrs) == 0 || app.details.attrs[0].Key != "Items Type" {
		t.Fatal("expected details to show 'Items Type' for array node")
	}

	// Now try to change items type to "int" (the bug: this used to fail silently)
	app.focus = PanelDetails
	app.details.cursor = 0
	result, _ = app.Update(tea.KeyPressMsg{Code: 0x0D})
	app = result.(App)

	if app.overlay != OverlayPicker {
		t.Fatalf("expected picker overlay, got %d", app.overlay)
	}

	// Select "int" through the picker
	result, _ = app.handlePickerResult("int")
	app = result.(App)

	// Verify the items type changed to "int"
	if len(arrayNode.Children) == 0 {
		t.Fatal("array has no children after replace")
	}
	newChild := proj.Nodes[arrayNode.Children[0]]
	if newChild == nil {
		t.Fatal("new child not found")
	}
	if newChild.NativeString() != "int" {
		t.Fatalf("expected items type 'int', got %q", newChild.NativeString())
	}
}

func TestApp_ReplaceFieldTypeToArray_HasChild(t *testing.T) {
	// When replacing a field's type to "array", the new array should have an items child.
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Navigate to id field (line 2)
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()

	sel := app.tree.SelectedNode()
	if sel == nil || sel.Name() != "id" {
		t.Fatalf("expected id field, got %v", sel)
	}

	// Replace type to "array" via ActionReplace
	app.overlayAction = ActionReplace
	result, _ := app.handlePickerResult("array")
	app = result.(App)

	// The field should now have a child that is KindArray
	if len(sel.Children) == 0 {
		t.Fatal("field has no children after replace to array")
	}
	arrayChild := proj.Nodes[sel.Children[0]]
	if arrayChild == nil || arrayChild.Kind != schema.KindArray {
		t.Fatalf("expected array child, got %v", arrayChild)
	}

	// The array child should have its own items child (primitive "string")
	if len(arrayChild.Children) == 0 {
		t.Fatal("array node has no items child after replace — bug not fixed")
	}
	itemsChild := proj.Nodes[arrayChild.Children[0]]
	if itemsChild == nil || itemsChild.Kind != schema.KindPrimitive {
		t.Fatalf("expected primitive items child, got %v", itemsChild)
	}
	if itemsChild.NativeString() != "string" {
		t.Fatalf("expected items type 'string', got %q", itemsChild.NativeString())
	}
}

func TestAction_ReplaceType_UndoRedo(t *testing.T) {
	proj := buildTestProjection(t)
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	// Navigate to id field (line 2)
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.tree = app.tree.HandleKey(tea.KeyPressMsg{Code: 'j'})
	app.syncDetailsToTree()

	sel := app.tree.SelectedNode()
	if sel == nil || sel.Name() != "id" {
		t.Fatalf("expected id selected, got %v", sel)
	}

	original := snapshotProj(t, proj)

	// Execute replace type via command directly (type field → string)
	newNode := projection.NewNodeDetached(schema.KindPrimitive, "string", []any{})
	cloneResult := &projection.CloneResult{Root: newNode, Nodes: []*projection.Node{newNode}}
	cmd, err := command.ReplaceType(proj, command.ReplaceTypeParams{
		ParentID:   sel.ID,
		NewSubtree: cloneResult,
	})
	if err != nil {
		t.Fatalf("ReplaceType: %v", err)
	}
	_ = app.history.Execute(cmd)
	app.dirty = true

	afterReplace := snapshotProj(t, proj)
	if original == afterReplace {
		t.Fatal("snapshot should differ after replace")
	}

	// Undo
	_ = app.history.Undo()
	afterUndo := snapshotProj(t, proj)
	if afterUndo != original {
		t.Errorf("after undo should match original\ngot:\n%s\nwant:\n%s", afterUndo, original)
	}

	// Redo
	_ = app.history.Redo()
	afterRedo := snapshotProj(t, proj)
	if afterRedo != afterReplace {
		t.Errorf("after redo should match post-replace\ngot:\n%s\nwant:\n%s", afterRedo, afterReplace)
	}
}

// TestApp_RunSearch_DocPrefix is a regression test for a bug where the tree
// search (runSearch) hardcoded every query into search.Filters{Name: query},
// ignoring search.ParseQuery entirely. That meant prefix filters like "d:"
// (doc), "t:" (type), "p:" (parent), "ns:" (namespace), and "a:" (alias) were
// silently treated as name substrings and never matched anything real.
func TestApp_RunSearch_DocPrefix(t *testing.T) {
	projection.ResetNodeCounter()
	raw := map[string]any{
		"type":      "record",
		"name":      "User",
		"namespace": "com.example",
		"fields": []any{
			map[string]any{
				"name": "id",
				"type": "int",
			},
			map[string]any{
				"name": "email",
				"doc":  "the user's primary contact email address for test notifications",
				"type": "string",
			},
		},
	}
	proj, err := projection.Build(raw)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.width = 120
	app.height = 40
	app.ready = true

	app.searchBuf = []rune("d:test")
	app.runSearch()

	if len(app.searchResults) == 0 {
		t.Fatal("expected d:test to match the doc field on the 'email' field, got no results")
	}
	found := false
	for _, r := range app.searchResults {
		if r.Node.Name() == "email" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected search results to include the 'email' field, got %+v", app.searchResults)
	}

	// Plain free-text search should still work as before (matches by name).
	app.searchBuf = []rune("email")
	app.runSearch()
	if len(app.searchResults) == 0 {
		t.Fatal("expected plain free-text search for 'email' to match the field by name")
	}
}
