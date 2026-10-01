package projection_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/schema"
)

// --- Test Helpers ---

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

func mustBuildFromFile(t *testing.T, path string) *projection.Projection {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading sample file %s: %v", path, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parsing sample file %s: %v", path, err)
	}
	proj, err := projection.Build(raw)
	if err != nil {
		t.Fatalf("Build from file %s failed: %v", path, err)
	}
	return proj
}

func samplesDir() string {
	// Tests run from package dir (tui/internal/projection); samples are at repo root
	return filepath.Join("..", "..", "..", "samples")
}

func nodeCount(proj *projection.Projection) int {
	return len(proj.Nodes)
}

// --- Inline Test Schemas ---

const simpleRecord = `{
	"type": "record",
	"name": "User",
	"namespace": "com.example",
	"fields": [
		{"name": "id", "type": "int"},
		{"name": "name", "type": "string"}
	]
}`

const nestedRecord = `{
	"type": "record",
	"name": "Order",
	"namespace": "com.shop",
	"fields": [
		{"name": "orderId", "type": "string"},
		{"name": "item", "type": {
			"type": "record",
			"name": "Item",
			"fields": [
				{"name": "sku", "type": "string"},
				{"name": "qty", "type": "int"}
			]
		}}
	]
}`

const unionSchema = `{
	"type": "record",
	"name": "Event",
	"fields": [
		{"name": "payload", "type": ["null", "string", "int"]}
	]
}`

const arraySchema = `{
	"type": "record",
	"name": "Collection",
	"fields": [
		{"name": "items", "type": {"type": "array", "items": "string"}}
	]
}`

const mapSchema = `{
	"type": "record",
	"name": "Config",
	"fields": [
		{"name": "settings", "type": {"type": "map", "values": "string"}}
	]
}`

const logicalTypeSchema = `{
	"type": "record",
	"name": "Timestamped",
	"fields": [
		{"name": "createdAt", "type": {"type": "long", "logicalType": "timestamp-millis"}}
	]
}`

// --- Build Tests ---

func TestBuild_SimpleRecord(t *testing.T) {
	proj := mustBuild(t, simpleRecord)

	// schema + record + 2*(field + type) = 6
	if got := nodeCount(proj); got != 6 {
		t.Errorf("expected 6 nodes, got %d", got)
	}

	root := proj.Get(proj.RootID)
	if root == nil {
		t.Fatal("root is nil")
	}
	if root.Kind != schema.KindSchema {
		t.Errorf("root kind = %s, want schema", root.Kind)
	}
}

func TestBuild_NestedRecord(t *testing.T) {
	proj := mustBuild(t, nestedRecord)

	// schema, record:Order, field:orderId, prim:string,
	// field:item, record:Item, field:sku, prim:string, field:qty, prim:int = 10
	if got := nodeCount(proj); got != 10 {
		t.Errorf("expected 10 nodes, got %d", got)
	}
}

func TestBuild_Union(t *testing.T) {
	proj := mustBuild(t, unionSchema)

	// schema, record, field, union, null, string, int = 7
	if got := nodeCount(proj); got != 7 {
		t.Errorf("expected 7 nodes, got %d", got)
	}

	var unionNode *projection.Node
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindUnion {
			unionNode = n
			break
		}
	}
	if unionNode == nil {
		t.Fatal("union node not found")
	}
	if len(unionNode.Children) != 3 {
		t.Errorf("union branches = %d, want 3", len(unionNode.Children))
	}
}

func TestBuild_Array(t *testing.T) {
	proj := mustBuild(t, arraySchema)
	// schema, record, field, array, prim:string = 5
	if got := nodeCount(proj); got != 5 {
		t.Errorf("expected 5 nodes, got %d", got)
	}
}

func TestBuild_Map(t *testing.T) {
	proj := mustBuild(t, mapSchema)
	// schema, record, field, map, prim:string = 5
	if got := nodeCount(proj); got != 5 {
		t.Errorf("expected 5 nodes, got %d", got)
	}
}

func TestBuild_LogicalType(t *testing.T) {
	proj := mustBuild(t, logicalTypeSchema)
	// schema, record, field, primitive(logicalType) = 4
	if got := nodeCount(proj); got != 4 {
		t.Errorf("expected 4 nodes, got %d", got)
	}
}

// --- Sample File Tests ---

func TestBuild_SampleUserEvent(t *testing.T) {
	path := filepath.Join(samplesDir(), "user-event.avsc")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample file not found, skipping")
	}
	proj := mustBuildFromFile(t, path)

	stats := projection.CalculateStats(proj)
	if stats.FieldCount < 10 {
		t.Errorf("user-event should have many fields, got %d", stats.FieldCount)
	}
}

func TestBuild_SampleEcommerce(t *testing.T) {
	path := filepath.Join(samplesDir(), "ecommerce-order.avsc")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample file not found, skipping")
	}
	proj := mustBuildFromFile(t, path)

	if nodeCount(proj) < 5 {
		t.Errorf("ecommerce schema should have many nodes, got %d", nodeCount(proj))
	}
}

func TestRoundtrip_AllSamples(t *testing.T) {
	dir := samplesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skip("samples dir not accessible, skipping")
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".avsc" {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			assertRoundtripBytes(t, data)
		})
	}
}

// --- Emit / Roundtrip Tests ---

func TestEmit_Roundtrip_SimpleRecord(t *testing.T) {
	assertRoundtrip(t, simpleRecord)
}

func TestEmit_Roundtrip_NestedRecord(t *testing.T) {
	assertRoundtrip(t, nestedRecord)
}

func TestEmit_Roundtrip_Union(t *testing.T) {
	assertRoundtrip(t, unionSchema)
}

func TestEmit_Roundtrip_Array(t *testing.T) {
	assertRoundtrip(t, arraySchema)
}

func TestEmit_Roundtrip_Map(t *testing.T) {
	assertRoundtrip(t, mapSchema)
}

func TestEmit_Roundtrip_LogicalType(t *testing.T) {
	assertRoundtrip(t, logicalTypeSchema)
}

func assertRoundtrip(t *testing.T, jsonStr string) {
	t.Helper()
	assertRoundtripBytes(t, []byte(jsonStr))
}

func assertRoundtripBytes(t *testing.T, data []byte) {
	t.Helper()

	var original any
	if err := json.Unmarshal(data, &original); err != nil {
		t.Fatalf("parse original: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse raw: %v", err)
	}

	proj, err := projection.Build(raw)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	emitted, err := projection.GenerateAvro(proj)
	if err != nil {
		t.Fatalf("GenerateAvro: %v", err)
	}

	origJSON, _ := json.Marshal(original)
	emitJSON, _ := json.Marshal(emitted)

	if string(origJSON) != string(emitJSON) {
		t.Errorf("roundtrip mismatch:\noriginal: %s\nemitted:  %s", truncate(origJSON, 200), truncate(emitJSON, 200))
	}
}

func truncate(b []byte, maxLen int) string {
	if len(b) <= maxLen {
		return string(b)
	}
	return string(b[:maxLen]) + "..."
}

// --- Stats Tests ---

func TestStats_SimpleRecord(t *testing.T) {
	proj := mustBuild(t, simpleRecord)
	stats := projection.CalculateStats(proj)
	if stats.FieldCount != 2 {
		t.Errorf("FieldCount = %d, want 2", stats.FieldCount)
	}
}

func TestStats_NestedRecord(t *testing.T) {
	proj := mustBuild(t, nestedRecord)
	stats := projection.CalculateStats(proj)
	if stats.FieldCount != 4 {
		t.Errorf("FieldCount = %d, want 4", stats.FieldCount)
	}
}

// --- Clone Tests ---

func TestCloneSubtree(t *testing.T) {
	proj := mustBuild(t, simpleRecord)

	var fieldID string
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindField {
			fieldID = n.ID
			break
		}
	}

	result, err := projection.CloneSubtree(proj, fieldID)
	if err != nil {
		t.Fatalf("CloneSubtree: %v", err)
	}

	if result.Root.ID == fieldID {
		t.Error("cloned root has same ID as source")
	}
	if len(result.Nodes) != 2 {
		t.Errorf("clone has %d nodes, want 2", len(result.Nodes))
	}
}

// --- Validate Tests ---

func TestValidate_ValidProjection(t *testing.T) {
	proj := mustBuild(t, simpleRecord)
	if err := projection.Validate(proj); err != nil {
		t.Errorf("valid projection returned error: %v", err)
	}
}

// --- RebuildPaths ---

func TestRebuildPaths(t *testing.T) {
	proj := mustBuild(t, simpleRecord)

	for _, n := range proj.Nodes {
		n.Path = nil
	}

	if err := projection.RebuildPaths(proj); err != nil {
		t.Fatalf("RebuildPaths: %v", err)
	}

	for id, n := range proj.Nodes {
		if n.Path == nil {
			t.Errorf("node %s still has nil path after rebuild", id)
		}
	}
}

// --- NamedTypes ---

func TestNamedTypes(t *testing.T) {
	proj := mustBuild(t, nestedRecord)
	names := proj.NamedTypes()

	if len(names) < 2 {
		t.Errorf("expected at least 2 named types, got %d: %v", len(names), names)
	}
}

// --- IsInSingleSlot ---

func TestIsInSingleSlot(t *testing.T) {
	proj := mustBuild(t, simpleRecord)

	for _, n := range proj.Nodes {
		if n.Kind == schema.KindPrimitive {
			if !proj.IsInSingleSlot(n) {
				t.Errorf("primitive %s should be in single slot", n.ID)
			}
		}
		if n.Kind == schema.KindField {
			if proj.IsInSingleSlot(n) {
				t.Errorf("field %s should NOT be in single slot", n.ID)
			}
		}
	}
}

// --- IsDescendant ---

func TestIsDescendant(t *testing.T) {
	proj := mustBuild(t, nestedRecord)

	var orderID, itemID string
	for _, n := range proj.Nodes {
		if n.Kind == schema.KindRecord {
			if n.Name() == "Order" {
				orderID = n.ID
			} else if n.Name() == "Item" {
				itemID = n.ID
			}
		}
	}

	if orderID == "" || itemID == "" {
		t.Fatal("could not find Order and Item records")
	}

	if !proj.IsDescendant(orderID, itemID) {
		t.Error("Item should be descendant of Order")
	}
	if proj.IsDescendant(itemID, orderID) {
		t.Error("Order should NOT be descendant of Item")
	}
}
