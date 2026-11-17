package model

import (
	"encoding/json"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/onereallylongname/avredit/internal/config"
	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/schema"
)

func TestEnterOnNamedReferenceJumpsToDefinition(t *testing.T) {
	rawJSON := `{
		"type": "record",
		"name": "Root",
		"namespace": "com.example",
		"fields": [
			{"name": "child", "type": {"type": "record", "name": "Child", "fields": []}},
			{"name": "childRef", "type": "Child"}
		]
	}`
	var raw map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		t.Fatal(err)
	}
	proj, err := projection.Build(raw)
	if err != nil {
		t.Fatal(err)
	}

	var ref, definition *projection.Node
	for _, node := range proj.Nodes {
		if node.Kind == schema.KindNamed {
			ref = node
		}
		if node.Kind == schema.KindRecord && node.Name() == "Child" {
			definition = node
		}
	}
	if ref == nil || definition == nil {
		t.Fatalf("schema nodes not found: reference=%v definition=%v", ref, definition)
	}

	for _, focus := range []FocusPanel{PanelTree, PanelDetails} {
		t.Run(focusName(focus), func(t *testing.T) {
			app := NewApp(proj, "test.avsc", config.DefaultConfig())
			app.tree.JumpToNode(ref.ID)
			app.syncDetailsToTree()
			app.focus = focus

			updated, _ := app.Update(tea.KeyPressMsg{Code: 0x0D})
			result := updated.(App)
			if got := result.tree.SelectedNode(); got == nil || got.ID != definition.ID {
				t.Fatalf("tree selection = %v, want definition %q", got, definition.ID)
			}
			if result.details.node == nil || result.details.node.ID != definition.ID {
				t.Fatalf("details node = %v, want definition %q", result.details.node, definition.ID)
			}
		})
	}
}

func TestEnterOnUnresolvedNamedReferenceShowsError(t *testing.T) {
	rawJSON := `{
		"type": "record",
		"name": "Root",
		"fields": [{"name": "missing", "type": "Missing"}]
	}`
	var raw map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		t.Fatal(err)
	}
	proj, err := projection.Build(raw)
	if err != nil {
		t.Fatal(err)
	}

	var ref *projection.Node
	for _, node := range proj.Nodes {
		if node.Kind == schema.KindNamed {
			ref = node
			break
		}
	}
	if ref == nil {
		t.Fatal("named reference not found")
	}

	app := NewApp(proj, "test.avsc", config.DefaultConfig())
	app.tree.JumpToNode(ref.ID)
	updated, _ := app.Update(tea.KeyPressMsg{Code: 0x0D})
	result := updated.(App)
	if result.flash == "" {
		t.Fatal("expected an error message for the unresolved reference")
	}
	if got := result.tree.SelectedNode(); got == nil || got.ID != ref.ID {
		t.Fatalf("tree selection = %v, want unresolved reference %q", got, ref.ID)
	}
}

func focusName(focus FocusPanel) string {
	if focus == PanelTree {
		return "tree"
	}
	return "details"
}
