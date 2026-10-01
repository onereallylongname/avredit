// Package search implements the query engine for finding nodes in a projection.
//
// Supports prefix filters (n:name, t:type, p:parent, ns:namespace, a:alias, d:doc)
// and free-text fuzzy matching. All active filters use AND logic.
// Results are scored by weighted relevance and sorted descending.
//
// See docs/ARCHITECTURE.md "Search Engine" for scoring weights.
package search

import (
	"sort"
	"strings"

	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/schema"
)

// Filters defines the search criteria for querying nodes.
type Filters struct {
	Name      string // n: prefix
	Type      string // t: prefix
	Parent    string // p: prefix
	Namespace string // ns: prefix
	Alias     string // a: prefix
	Doc       string // d: prefix
	Text      string // free-text (no prefix)
}

// IsEmpty returns true if no filter is active.
func (f Filters) IsEmpty() bool {
	return f.Name == "" && f.Type == "" && f.Parent == "" && f.Namespace == "" && f.Alias == "" && f.Doc == "" && f.Text == ""
}

// Result represents a single search match.
type Result struct {
	NodeID string
	Node   *projection.Node
	Score  float64
}

// excluded kinds that aren't meaningful search targets
var excludedKinds = map[schema.NodeKind]bool{
	schema.KindPrimitive: true,
	schema.KindNamed:     true,
	schema.KindSchema:    true,
	schema.KindUnion:     true,
	schema.KindArray:     true,
	schema.KindMap:       true,
}

// QueryNodes searches the projection for nodes matching the given filters.
// All specified filters must match (AND logic). Results are sorted by score descending.
func QueryNodes(proj *projection.Projection, filters Filters) []Result {
	if filters.IsEmpty() {
		return nil
	}

	activeFilters := gatherActiveFilters(filters)
	if len(activeFilters) == 0 {
		return nil
	}

	var results []Result

	for id, node := range proj.Nodes {
		if excludedKinds[node.Kind] {
			continue
		}
		score := scoreNode(proj, node, filters, activeFilters)
		if score > 0 {
			results = append(results, Result{NodeID: id, Node: node, Score: score})
		}
	}

	// Remove records that have a descendant also in results
	matchedIDs := make(map[string]bool, len(results))
	for _, r := range results {
		matchedIDs[r.NodeID] = true
	}

	filtered := results[:0]
	for _, r := range results {
		if r.Node.Kind == schema.KindRecord && hasDescendantInSet(proj, r.Node, matchedIDs) {
			continue
		}
		filtered = append(filtered, r)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Score > filtered[j].Score
	})

	return filtered
}

// ParseQuery parses a search string into structured filters.
// Supports prefixes: "n:", "t:", "p:", "ns:", "a:", "d:"
func ParseQuery(input string) Filters {
	input = strings.TrimSpace(input)
	if input == "" {
		return Filters{}
	}

	switch {
	case strings.HasPrefix(input, "n:"):
		return Filters{Name: strings.TrimSpace(input[2:])}
	case strings.HasPrefix(input, "t:"):
		return Filters{Type: strings.TrimSpace(input[2:])}
	case strings.HasPrefix(input, "p:"):
		return Filters{Parent: strings.TrimSpace(input[2:])}
	case strings.HasPrefix(input, "ns:"):
		return Filters{Namespace: strings.TrimSpace(input[3:])}
	case strings.HasPrefix(input, "a:"):
		return Filters{Alias: strings.TrimSpace(input[2:])}
	case strings.HasPrefix(input, "d:"):
		return Filters{Doc: strings.TrimSpace(input[2:])}
	default:
		return Filters{Text: input}
	}
}

type activeFilter struct {
	key   string
	value string
}

func gatherActiveFilters(f Filters) []activeFilter {
	var result []activeFilter
	if f.Name != "" {
		result = append(result, activeFilter{"name", f.Name})
	}
	if f.Type != "" {
		result = append(result, activeFilter{"type", f.Type})
	}
	if f.Parent != "" {
		result = append(result, activeFilter{"parent", f.Parent})
	}
	if f.Namespace != "" {
		result = append(result, activeFilter{"namespace", f.Namespace})
	}
	if f.Alias != "" {
		result = append(result, activeFilter{"alias", f.Alias})
	}
	if f.Doc != "" {
		result = append(result, activeFilter{"doc", f.Doc})
	}
	if f.Text != "" {
		result = append(result, activeFilter{"text", f.Text})
	}
	return result
}

func scoreNode(proj *projection.Projection, node *projection.Node, _ Filters, active []activeFilter) float64 {
	props := getSearchableProps(proj, node)
	var total float64
	matched := 0

	for _, af := range active {
		switch af.key {
		case "text":
			best := max4(
				FuzzyScore(props.name, af.value)*3,
				TypeComponentScore(props.typeName, af.value)*2,
				FuzzyScore(props.parent, af.value),
				FuzzyScore(props.namespace, af.value),
			)
			if aliasScore := FuzzyScore(props.aliases, af.value) * 2.5; aliasScore > best {
				best = aliasScore
			}
			if docScore := DocScore(props.doc, af.value) * 1.5; docScore > best {
				best = docScore
			}
			if best > 0 {
				total += best
				matched++
			}
		case "type":
			score := TypeComponentScore(props.typeName, af.value)
			if score > 0 {
				total += score * 2
				matched++
			}
		case "name":
			score := FuzzyScore(props.name, af.value)
			if score > 0 {
				total += score * 3
				matched++
			}
		case "parent":
			score := FuzzyScore(props.parent, af.value)
			if score > 0 {
				total += score * 2
				matched++
			}
		case "namespace":
			score := FuzzyScore(props.namespace, af.value)
			if score > 0 {
				total += score * 2
				matched++
			}
		case "alias":
			score := FuzzyScore(props.aliases, af.value)
			if score > 0 {
				total += score * 3
				matched++
			}
		case "doc":
			score := DocScore(props.doc, af.value)
			if score > 0 {
				total += score * 2
				matched++
			}
		}
	}

	// AND logic: all active filters must match
	if matched != len(active) {
		return 0
	}
	return total
}

type searchProps struct {
	name      string
	typeName  string
	parent    string
	namespace string
	aliases   string
	doc       string
}

func getSearchableProps(proj *projection.Projection, node *projection.Node) searchProps {
	props := searchProps{}

	props.name = node.Name()
	props.namespace = node.Namespace()
	props.typeName = getNodeTypeLabel(proj, node)
	props.parent = getParentRecordName(proj, node)
	props.doc = node.Doc()
	if al := node.Aliases(); len(al) > 0 {
		props.aliases = strings.Join(al, " ")
	}

	return props
}

func getNodeTypeLabel(proj *projection.Projection, node *projection.Node) string {
	switch node.Kind {
	case schema.KindField:
		if len(node.Children) > 0 {
			typeChild := proj.Get(node.Children[0])
			if typeChild != nil {
				return getNodeTypeLabel(proj, typeChild)
			}
		}
		return ""
	case schema.KindPrimitive, schema.KindNamed:
		if s := node.NativeString(); s != "" {
			return s
		}
		m := node.NativeMap()
		if m != nil {
			typeName, _ := m["type"].(string)
			logicalType, _ := m["logicalType"].(string)
			if logicalType != "" {
				return typeName + "," + logicalType
			}
			return typeName
		}
		return string(node.Kind)
	case schema.KindRecord, schema.KindEnum, schema.KindFixed:
		return string(node.Kind)
	case schema.KindUnion:
		parts := []string{"union"}
		for _, childID := range node.Children {
			child := proj.Get(childID)
			if child != nil {
				parts = append(parts, getNodeTypeLabel(proj, child))
			}
		}
		return strings.Join(parts, ",")
	default:
		return string(node.Kind)
	}
}

func getParentRecordName(proj *projection.Projection, node *projection.Node) string {
	current := node
	for current.ParentID != "" {
		parent := proj.Get(current.ParentID)
		if parent == nil {
			return ""
		}
		if parent.Kind == schema.KindRecord {
			return parent.Name()
		}
		current = parent
	}
	return ""
}

func hasDescendantInSet(proj *projection.Projection, node *projection.Node, idSet map[string]bool) bool {
	for _, childID := range node.Children {
		if idSet[childID] {
			return true
		}
		child := proj.Get(childID)
		if child != nil && hasDescendantInSet(proj, child, idSet) {
			return true
		}
	}
	return false
}

func max4(a, b, c, d float64) float64 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	if d > m {
		m = d
	}
	return m
}
