package validation

import (
	"testing"

	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/schema"
)

// --- Test helpers ---

func buildProjection(t *testing.T, raw map[string]any) *projection.Projection {
	t.Helper()
	proj, err := projection.Build(raw)
	if err != nil {
		t.Fatalf("failed to build projection: %v", err)
	}
	return proj
}

func mustHaveIssues(t *testing.T, issues []Issue, expectedRule string, expectedCount int) {
	t.Helper()
	count := 0
	for _, iss := range issues {
		if iss.Rule == expectedRule {
			count++
		}
	}
	if count != expectedCount {
		t.Errorf("expected %d issues for rule %q, got %d (total issues: %v)", expectedCount, expectedRule, count, issues)
	}
}

func mustHaveNoIssues(t *testing.T, issues []Issue, rule string) {
	t.Helper()
	for _, iss := range issues {
		if iss.Rule == rule {
			t.Errorf("unexpected issue for rule %q: %s", rule, iss.Message)
		}
	}
}

// --- Duplicate field names ---

func TestRuleDuplicateFieldNames_NoDuplicates(t *testing.T) {
	raw := map[string]any{
		"type":      "record",
		"name":      "User",
		"namespace": "com.example",
		"fields": []any{
			map[string]any{"name": "id", "type": "int"},
			map[string]any{"name": "name", "type": "string"},
			map[string]any{"name": "email", "type": "string"},
		},
	}
	proj := buildProjection(t, raw)
	issues := RuleDuplicateFieldNames(proj)
	mustHaveNoIssues(t, issues, "duplicate-field-name")
}

func TestRuleDuplicateFieldNames_WithDuplicates(t *testing.T) {
	proj := buildProjectionWithDuplicateFields(t)
	issues := RuleDuplicateFieldNames(proj)
	mustHaveIssues(t, issues, "duplicate-field-name", 1)

	// Verify severity is Warning (advisory, not blocking)
	for _, iss := range issues {
		if iss.Rule == "duplicate-field-name" && iss.Severity != SeverityWarning {
			t.Errorf("duplicate-field-name should be Warning severity, got %v", iss.Severity)
		}
	}
}

func TestRuleDuplicateFieldNames_NestedRecords(t *testing.T) {
	// Same field name in different records is fine
	raw := map[string]any{
		"type":      "record",
		"name":      "Outer",
		"namespace": "com.example",
		"fields": []any{
			map[string]any{"name": "id", "type": "int"},
			map[string]any{
				"name": "inner",
				"type": map[string]any{
					"type": "record",
					"name": "Inner",
					"fields": []any{
						map[string]any{"name": "id", "type": "int"},
					},
				},
			},
		},
	}
	proj := buildProjection(t, raw)
	issues := RuleDuplicateFieldNames(proj)
	mustHaveNoIssues(t, issues, "duplicate-field-name")
}

// --- Circular references ---

func TestRuleCircularReferences_NoCycle(t *testing.T) {
	raw := map[string]any{
		"type":      "record",
		"name":      "Order",
		"namespace": "com.example",
		"fields": []any{
			map[string]any{"name": "id", "type": "int"},
			map[string]any{"name": "name", "type": "string"},
		},
	}
	proj := buildProjection(t, raw)
	issues := RuleCircularReferences(proj)
	mustHaveNoIssues(t, issues, "circular-reference")
}

func TestRuleCircularReferences_SelfReference(t *testing.T) {
	// A record that references itself (valid in Avro for recursive structures)
	// This is actually valid in Avro — records can reference themselves by name.
	// Our rule detects it as structural concern but Avro allows it.
	// For now, let's just verify the rule runs without panic on named refs.
	raw := map[string]any{
		"type":      "record",
		"name":      "TreeNode",
		"namespace": "com.example",
		"fields": []any{
			map[string]any{"name": "value", "type": "string"},
			map[string]any{
				"name": "children",
				"type": map[string]any{
					"type":  "array",
					"items": "com.example.TreeNode",
				},
			},
		},
	}
	proj := buildProjection(t, raw)
	issues := RuleCircularReferences(proj)
	// Self-reference IS a cycle in our graph — this is an intentional detection
	mustHaveIssues(t, issues, "circular-reference", 1)
}

// --- Empty records ---

func TestRuleEmptyRecords_WithFields(t *testing.T) {
	raw := map[string]any{
		"type": "record",
		"name": "User",
		"fields": []any{
			map[string]any{"name": "id", "type": "int"},
		},
	}
	proj := buildProjection(t, raw)
	issues := RuleEmptyRecords(proj)
	mustHaveNoIssues(t, issues, "empty-record")
}

func TestRuleEmptyRecords_Empty(t *testing.T) {
	raw := map[string]any{
		"type":   "record",
		"name":   "Empty",
		"fields": []any{},
	}
	proj := buildProjection(t, raw)
	issues := RuleEmptyRecords(proj)
	mustHaveIssues(t, issues, "empty-record", 1)

	// Verify severity is Info (advisory)
	for _, iss := range issues {
		if iss.Rule == "empty-record" && iss.Severity != SeverityInfo {
			t.Errorf("empty-record should be Info severity, got %v", iss.Severity)
		}
	}
}

// --- Empty unions ---

func TestRuleEmptyUnions_Valid(t *testing.T) {
	raw := map[string]any{
		"type": "record",
		"name": "WithUnion",
		"fields": []any{
			map[string]any{"name": "value", "type": []any{"null", "string"}},
		},
	}
	proj := buildProjection(t, raw)
	issues := RuleEmptyUnions(proj)
	mustHaveNoIssues(t, issues, "empty-union")
}

// --- Analyzer integration ---

func TestAnalyzer_NilProjection(t *testing.T) {
	a := NewAnalyzer()
	issues := a.Analyze(nil)
	if len(issues) != 0 {
		t.Errorf("expected no issues for nil projection, got %d", len(issues))
	}
}

func TestAnalyzer_RunsAllRules(t *testing.T) {
	a := NewAnalyzer()
	raw := map[string]any{
		"type": "record",
		"name": "Valid",
		"fields": []any{
			map[string]any{"name": "id", "type": "int"},
		},
	}
	proj := buildProjection(t, raw)
	issues := a.Analyze(proj)
	// Valid schema should have no issues
	if len(issues) != 0 {
		t.Errorf("expected no issues for valid schema, got %d: %v", len(issues), issues)
	}
}

func TestAnalyzer_DisableRule(t *testing.T) {
	a := NewAnalyzer()
	proj := buildProjectionWithDuplicateFields(t)

	// With rule enabled
	issues := a.Analyze(proj)
	mustHaveIssues(t, issues, "duplicate-field-name", 1)

	// Disable and re-run
	a.SetEnabled("duplicate-field-name", false)
	issues = a.Analyze(proj)
	mustHaveNoIssues(t, issues, "duplicate-field-name")

	// Re-enable
	a.SetEnabled("duplicate-field-name", true)
	issues = a.Analyze(proj)
	mustHaveIssues(t, issues, "duplicate-field-name", 1)
}

func TestAnalyzer_RuleIDs(t *testing.T) {
	a := NewAnalyzer()
	ids := a.RuleIDs()
	expected := []string{"duplicate-field-name", "circular-reference", "empty-record", "empty-union"}
	if len(ids) != len(expected) {
		t.Fatalf("expected %d rule IDs, got %d", len(expected), len(ids))
	}
	for i, id := range ids {
		if id != expected[i] {
			t.Errorf("rule %d: expected %q, got %q", i, expected[i], id)
		}
	}
}

// --- Helper utilities ---

func TestErrorCount(t *testing.T) {
	issues := []Issue{
		{Severity: SeverityError},
		{Severity: SeverityWarning},
		{Severity: SeverityError},
		{Severity: SeverityInfo},
	}
	if ErrorCount(issues) != 2 {
		t.Errorf("expected 2 errors, got %d", ErrorCount(issues))
	}
}

func TestWarningCount(t *testing.T) {
	issues := []Issue{
		{Severity: SeverityError},
		{Severity: SeverityWarning},
		{Severity: SeverityWarning},
		{Severity: SeverityInfo},
	}
	if WarningCount(issues) != 2 {
		t.Errorf("expected 2 warnings, got %d", WarningCount(issues))
	}
}

func TestIssuesForNode(t *testing.T) {
	issues := []Issue{
		{NodeID: "n1", Rule: "a"},
		{NodeID: "n2", Rule: "b"},
		{NodeID: "n1", Rule: "c"},
	}
	filtered := IssuesForNode(issues, "n1")
	if len(filtered) != 2 {
		t.Errorf("expected 2 issues for n1, got %d", len(filtered))
	}
}

// --- Fixtures ---

// buildProjectionWithDuplicateFields creates a projection where a record has two fields
// with the same name (simulating copy operation outcome).
func buildProjectionWithDuplicateFields(t *testing.T) *projection.Projection {
	t.Helper()
	projection.ResetNodeCounter()
	proj := projection.New()

	// Build manually to simulate post-copy state with duplicate names
	schemaNode := projection.NewNode(schema.KindSchema, map[string]any{
		"type": "record", "name": "User", "fields": []any{},
	}, "", []any{})
	proj.Nodes[schemaNode.ID] = schemaNode
	proj.RootID = schemaNode.ID

	record := projection.NewNode(schema.KindRecord, map[string]any{
		"type": "record", "name": "User", "fields": []any{},
	}, schemaNode.ID, []any{})
	proj.Nodes[record.ID] = record
	schemaNode.Children = append(schemaNode.Children, record.ID)

	field1 := projection.NewNode(schema.KindField, map[string]any{
		"name": "email", "type": "string",
	}, record.ID, []any{"fields", 0})
	proj.Nodes[field1.ID] = field1
	record.Children = append(record.Children, field1.ID)

	type1 := projection.NewNode(schema.KindPrimitive, "string", field1.ID, []any{"fields", 0, "type"})
	proj.Nodes[type1.ID] = type1
	field1.Children = append(field1.Children, type1.ID)

	// Duplicate field (simulates copy)
	field2 := projection.NewNode(schema.KindField, map[string]any{
		"name": "email", "type": "string",
	}, record.ID, []any{"fields", 1})
	proj.Nodes[field2.ID] = field2
	record.Children = append(record.Children, field2.ID)

	type2 := projection.NewNode(schema.KindPrimitive, "string", field2.ID, []any{"fields", 1, "type"})
	proj.Nodes[type2.ID] = type2
	field2.Children = append(field2.Children, type2.ID)

	return proj
}
