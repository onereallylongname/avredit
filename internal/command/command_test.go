package command_test

import (
	"encoding/json"
	"testing"

	"github.com/onereallylongname/avredit/internal/command"
	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/schema"
)

func mustBuild(t *testing.T, jsonStr string) *projection.Projection {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		t.Fatalf("invalid test JSON: %v", err)
	}
	proj, err := projection.Build(raw)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	return proj
}

const testSchema = `{
	"type": "record",
	"name": "User",
	"namespace": "com.example",
	"fields": [
		{"name": "id", "type": "int"},
		{"name": "name", "type": "string"},
		{"name": "email", "type": "string"}
	]
}`

// --- History Tests ---

func TestHistory_ExecuteAndUndo(t *testing.T) {
	h := command.NewHistory(100)

	counter := 0
	cmd := &command.Command{
		DoFn:   func() error { counter++; return nil },
		UndoFn: func() error { counter--; return nil },
		Desc:   "increment",
	}

	if err := h.Execute(cmd); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if counter != 1 {
		t.Errorf("counter = %d, want 1", counter)
	}
	if !h.CanUndo() {
		t.Error("should be able to undo")
	}

	if err := h.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if counter != 0 {
		t.Errorf("counter = %d after undo, want 0", counter)
	}
	if !h.CanRedo() {
		t.Error("should be able to redo")
	}

	if err := h.Redo(); err != nil {
		t.Fatalf("Redo: %v", err)
	}
	if counter != 1 {
		t.Errorf("counter = %d after redo, want 1", counter)
	}
}

func TestHistory_RedoClearedOnNewAction(t *testing.T) {
	h := command.NewHistory(100)

	cmd1 := &command.Command{DoFn: func() error { return nil }, UndoFn: func() error { return nil }, Desc: "a"}
	cmd2 := &command.Command{DoFn: func() error { return nil }, UndoFn: func() error { return nil }, Desc: "b"}

	h.Execute(cmd1)
	h.Undo()

	if !h.CanRedo() {
		t.Fatal("should be able to redo before new action")
	}

	h.Execute(cmd2)

	if h.CanRedo() {
		t.Error("redo should be cleared after new action")
	}
}

func TestHistory_Limit(t *testing.T) {
	h := command.NewHistory(3)

	for i := 0; i < 5; i++ {
		h.Execute(&command.Command{DoFn: func() error { return nil }, UndoFn: func() error { return nil }, Desc: "x"})
	}

	if h.UndoCount() != 3 {
		t.Errorf("undo stack = %d, want 3 (limit)", h.UndoCount())
	}
}

// --- UpdateAttribute Tests ---

func TestUpdateAttribute_Native(t *testing.T) {
	proj := mustBuild(t, testSchema)

	// Find the record node
	var recordID string
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindRecord && n.Name() == "User" {
			recordID = n.ID
			break
		}
	}

	cmd, err := command.UpdateAttribute(proj, command.UpdateAttributeParams{
		NodeID:   recordID,
		Scope:    "native",
		Key:      "doc",
		NewValue: "A user record",
	})
	if err != nil {
		t.Fatalf("UpdateAttribute: %v", err)
	}

	if err := cmd.Do(); err != nil {
		t.Fatalf("Do: %v", err)
	}

	node := proj.Get(recordID)
	m := node.NativeMap()
	if m["doc"] != "A user record" {
		t.Errorf("doc = %v, want 'A user record'", m["doc"])
	}

	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if m["doc"] != nil {
		t.Errorf("doc after undo = %v, want nil", m["doc"])
	}
}

// --- RemoveNode Tests ---

func TestRemoveNode(t *testing.T) {
	proj := mustBuild(t, testSchema)
	initialCount := len(proj.Nodes)

	// Find a field to remove
	var fieldID string
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindField && n.Name() == "email" {
			fieldID = n.ID
			break
		}
	}

	cmd, err := command.RemoveNode(proj, command.RemoveNodeParams{NodeID: fieldID})
	if err != nil {
		t.Fatalf("RemoveNode: %v", err)
	}

	if err := cmd.Do(); err != nil {
		t.Fatalf("Do: %v", err)
	}

	// Should have fewer nodes (field + its type child)
	if len(proj.Nodes) != initialCount-2 {
		t.Errorf("nodes after remove = %d, want %d", len(proj.Nodes), initialCount-2)
	}

	// Undo should restore
	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if len(proj.Nodes) != initialCount {
		t.Errorf("nodes after undo = %d, want %d", len(proj.Nodes), initialCount)
	}
}

func TestRemoveNode_SingleSlot_Rejected(t *testing.T) {
	proj := mustBuild(t, testSchema)

	// Try to remove a primitive type node (in single slot)
	var primID string
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindPrimitive {
			primID = n.ID
			break
		}
	}

	_, err := command.RemoveNode(proj, command.RemoveNodeParams{NodeID: primID})
	if err == nil {
		t.Error("expected error when removing single-slot type node")
	}
}

// --- MoveNode Tests ---

func TestMoveNode(t *testing.T) {
	proj := mustBuild(t, testSchema)

	// Find fields and the record
	var fields []*projection.Node
	var recordID string
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindField {
			fields = append(fields, n)
		}
		if n.Kind == schema.KindRecord {
			recordID = n.ID
		}
	}

	if len(fields) < 2 {
		t.Fatal("need at least 2 fields for move test")
	}

	record := proj.Get(recordID)
	originalOrder := append([]string{}, record.Children...)

	// Move last field to index 0
	lastField := fields[len(fields)-1]
	cmd, err := command.MoveNode(proj, command.MoveNodeParams{
		NodeID:   lastField.ID,
		TargetID: recordID,
		Index:    0,
	})
	if err != nil {
		t.Fatalf("MoveNode: %v", err)
	}

	if err := cmd.Do(); err != nil {
		t.Fatalf("Do: %v", err)
	}

	// First child should now be the moved field
	if record.Children[0] != lastField.ID {
		t.Errorf("first child = %s, want %s", record.Children[0], lastField.ID)
	}

	// Undo
	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	for i, id := range originalOrder {
		if record.Children[i] != id {
			t.Errorf("child[%d] after undo = %s, want %s", i, record.Children[i], id)
		}
	}
}

// --- CopyNode Tests ---

func TestCopyNode(t *testing.T) {
	proj := mustBuild(t, testSchema)
	initialCount := len(proj.Nodes)

	var fieldID, recordID string
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindField && fieldID == "" {
			fieldID = n.ID
		}
		if n.Kind == schema.KindRecord {
			recordID = n.ID
		}
	}

	cmd, err := command.CopyNode(proj, command.CopyNodeParams{
		SourceID: fieldID,
		TargetID: recordID,
		Index:    -1,
	})
	if err != nil {
		t.Fatalf("CopyNode: %v", err)
	}

	if err := cmd.Do(); err != nil {
		t.Fatalf("Do: %v", err)
	}

	// Should have 2 more nodes (cloned field + type)
	if len(proj.Nodes) != initialCount+2 {
		t.Errorf("nodes after copy = %d, want %d", len(proj.Nodes), initialCount+2)
	}

	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if len(proj.Nodes) != initialCount {
		t.Errorf("nodes after undo = %d, want %d", len(proj.Nodes), initialCount)
	}
}

// --- ReplaceType Tests ---

func TestReplaceType(t *testing.T) {
	proj := mustBuild(t, testSchema)

	// Find a field to replace its type
	var fieldID string
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindField && n.Name() == "id" {
			fieldID = n.ID
			break
		}
	}

	field := proj.Get(fieldID)
	oldTypeID := field.Children[0]

	// Build new type subtree (a string instead of int)
	newNode := projection.NewNode(schema.KindPrimitive, "string", "", []any{})
	newSubtree := &projection.CloneResult{
		Root:  newNode,
		Nodes: []*projection.Node{newNode},
	}

	cmd, err := command.ReplaceType(proj, command.ReplaceTypeParams{
		ParentID:   fieldID,
		NewSubtree: newSubtree,
	})
	if err != nil {
		t.Fatalf("ReplaceType: %v", err)
	}

	if err := cmd.Do(); err != nil {
		t.Fatalf("Do: %v", err)
	}

	// Field should now have a different type child
	if field.Children[0] == oldTypeID {
		t.Error("type child should have changed")
	}

	newTypeNode := proj.Get(field.Children[0])
	if newTypeNode.NativeString() != "string" {
		t.Errorf("new type = %v, want 'string'", newTypeNode.Attrs.Native)
	}

	// Undo
	if err := cmd.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	if field.Children[0] != oldTypeID {
		t.Error("type child should be restored after undo")
	}
}
