package validation

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/schema"
)

// RuleDuplicateFieldNames checks for duplicate field names within each record.
// Avro spec requires unique field names per record. Copy/add operations may
// produce temporary duplicates — this reports them as warnings (advisory, not blocking).
func RuleDuplicateFieldNames(proj *projection.Projection) []Issue {
	var issues []Issue

	for _, node := range proj.Nodes {
		if node.Kind != schema.KindRecord {
			continue
		}

		seen := make(map[string]string) // name → first field node ID
		for _, childID := range node.Children {
			child := proj.Get(childID)
			if child == nil || child.Kind != schema.KindField {
				continue
			}
			name := child.Name()
			if name == "" {
				continue
			}
			if firstID, exists := seen[name]; exists {
				issues = append(issues, Issue{
					Severity: SeverityWarning,
					NodeID:   childID,
					Rule:     "duplicate-field-name",
					Message:  fmt.Sprintf("Duplicate field name %q in record %q (first at %s)", name, node.Name(), firstID),
				})
			} else {
				seen[name] = childID
			}
		}
	}

	return issues
}

// RuleCircularReferences detects circular named-type references via DFS.
// A cycle means a type references itself (directly or transitively), which
// would cause infinite recursion during serialization.
func RuleCircularReferences(proj *projection.Projection) []Issue {
	var issues []Issue

	// Build a graph: named-type name → set of named-type names it contains
	// For each record/enum/fixed, find all KindNamed descendants
	typeGraph := make(map[string][]string) // type name → referenced type names
	typeNodes := make(map[string]string)   // type name → node ID

	for _, node := range proj.Nodes {
		if node.Kind != schema.KindRecord && node.Kind != schema.KindEnum && node.Kind != schema.KindFixed {
			continue
		}
		name := qualifiedName(node)
		if name == "" {
			continue
		}
		typeNodes[name] = node.ID
		refs := collectNamedRefs(proj, node)
		typeGraph[name] = refs
	}

	// DFS cycle detection
	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // fully explored
	)
	colors := make(map[string]int)

	var dfs func(name string, path []string) bool
	dfs = func(name string, path []string) bool {
		colors[name] = gray
		for _, ref := range typeGraph[name] {
			switch colors[ref] {
			case gray:
				// Cycle detected
				cycle := append(path, ref)
				issues = append(issues, Issue{
					Severity: SeverityError,
					NodeID:   typeNodes[name],
					Rule:     "circular-reference",
					Message:  fmt.Sprintf("Circular type reference: %s", formatCycle(cycle, ref)),
				})
				return true
			case white:
				if dfs(ref, append(path, ref)) {
					return true
				}
			}
		}
		colors[name] = black
		return false
	}

	for name := range typeGraph {
		if colors[name] == white {
			dfs(name, []string{name})
		}
	}

	return issues
}

// RuleEmptyRecords reports records with zero fields.
func RuleEmptyRecords(proj *projection.Projection) []Issue {
	var issues []Issue

	for _, node := range proj.Nodes {
		if node.Kind != schema.KindRecord {
			continue
		}
		if len(node.Children) == 0 {
			issues = append(issues, Issue{
				Severity: SeverityInfo,
				NodeID:   node.ID,
				Rule:     "empty-record",
				Message:  fmt.Sprintf("Record %q has no fields", node.Name()),
			})
		}
	}

	return issues
}

// RuleEmptyUnions reports unions with zero branches (structurally invalid).
func RuleEmptyUnions(proj *projection.Projection) []Issue {
	var issues []Issue

	for _, node := range proj.Nodes {
		if node.Kind != schema.KindUnion {
			continue
		}
		if len(node.Children) == 0 {
			issues = append(issues, Issue{
				Severity: SeverityError,
				NodeID:   node.ID,
				Rule:     "empty-union",
				Message:  "Union must have at least one branch",
			})
		}
	}

	return issues
}

// --- Helpers ---

// qualifiedName returns the fully-qualified name (namespace.name) of a named type node.
func qualifiedName(node *projection.Node) string {
	m := node.NativeMap()
	if m == nil {
		return ""
	}
	name, _ := m["name"].(string)
	if name == "" {
		return ""
	}
	ns, _ := m["namespace"].(string)
	if ns != "" {
		return ns + "." + name
	}
	return name
}

// collectNamedRefs finds all KindNamed references within a type's subtree (non-recursive into other named types).
func collectNamedRefs(proj *projection.Projection, root *projection.Node) []string {
	var refs []string
	seen := make(map[string]bool)

	var walk func(nodeID string)
	walk = func(nodeID string) {
		node := proj.Get(nodeID)
		if node == nil {
			return
		}
		if node.Kind == schema.KindNamed {
			ref := node.NativeString()
			if ref != "" && !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
			return
		}
		// Don't recurse into nested named types (they have their own entry in typeGraph)
		if node != root && (node.Kind == schema.KindRecord || node.Kind == schema.KindEnum || node.Kind == schema.KindFixed) {
			return
		}
		for _, childID := range node.Children {
			walk(childID)
		}
	}

	for _, childID := range root.Children {
		walk(childID)
	}
	return refs
}

// formatCycle formats a cycle path for display.
func formatCycle(path []string, target string) string {
	result := ""
	for i, p := range path {
		if i > 0 {
			result += " → "
		}
		result += p
		if p == target && i > 0 {
			break
		}
	}
	return result
}
