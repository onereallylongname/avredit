// Package projection implements the internal tree model for Avro schemas.
//
// Architecture: The projection is a flat map of Node objects (keyed by sequential string IDs)
// linked through parent/child ID references. This design enables O(1) lookups, simple
// tree rewiring for mutations, and straightforward serialization.
//
// Build flow: .avsc JSON → NormalizeType (classify) → visitType (recurse) → Register (link)
// Emit flow:  Projection → emitNode (recurse by kind) → reconstruct JSON → marshal
//
// See docs/ARCHITECTURE.md for full design details.
package projection

import (
	"sync/atomic"

	"github.com/onereallylongname/avredit/internal/schema"
)

// nodeCounter provides unique sequential IDs.
var nodeCounter atomic.Int64

// nextNodeID returns the next unique node identifier.
func nextNodeID() string {
	n := nodeCounter.Add(1)
	return "n" + itoa(n)
}

// ResetNodeCounter resets the counter (for testing).
func ResetNodeCounter() {
	nodeCounter.Store(0)
}

// itoa converts int64 to string without importing strconv.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte(n%10) + '0'
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// Node represents a single element in the projection tree.
// Each node has a unique sequential ID (e.g., "n1", "n2"), a kind describing its
// Avro role, and links to parent/children via string IDs.
//
// Key invariant: KindNamed nodes store their reference as a plain string in Attrs.Native
// (accessed via NativeString). All other complex types store a map[string]any (NativeMap).
type Node struct {
	ID       string
	Kind     schema.NodeKind
	ParentID string // empty for root
	Children []string
	Path     []any // JSON path segments (strings and ints)
	Attrs    Attributes
}

// Attributes holds both standard Avro attributes and custom metadata.
// Native stores the raw JSON value as-is from the schema (preserving format for emit).
// Custom holds user-defined key-value pairs that don't appear in standard Avro.
type Attributes struct {
	Native any            // the raw Avro value (string, map, slice)
	Custom map[string]any // custom metadata (e.g., __id)
}

// NewNode creates a new node with a fresh ID.
func NewNode(kind schema.NodeKind, native any, parentID string, path []any) *Node {
	return &Node{
		ID:       nextNodeID(),
		Kind:     kind,
		ParentID: parentID,
		Children: make([]string, 0),
		Path:     path,
		Attrs: Attributes{
			Native: native,
			Custom: map[string]any{},
		},
	}
}

// NewNodeDetached creates a new node with a fresh ID without registering it.
// Used for building subtrees before insertion.
func NewNodeDetached(kind schema.NodeKind, native any, path []any) *Node {
	return &Node{
		ID:       nextNodeID(),
		Kind:     kind,
		ParentID: "",
		Children: make([]string, 0),
		Path:     path,
		Attrs: Attributes{
			Native: native,
			Custom: map[string]any{},
		},
	}
}

// NativeMap returns the native attributes as a map, or nil if it's not a map.
func (n *Node) NativeMap() map[string]any {
	m, _ := n.Attrs.Native.(map[string]any)
	return m
}

// NativeString returns the native value as a string, or empty.
func (n *Node) NativeString() string {
	s, _ := n.Attrs.Native.(string)
	return s
}

// Name returns the "name" field from native attributes, or empty.
func (n *Node) Name() string {
	if m := n.NativeMap(); m != nil {
		name, _ := m["name"].(string)
		return name
	}
	return ""
}

// Namespace returns the "namespace" field from native attributes, or empty.
func (n *Node) Namespace() string {
	if m := n.NativeMap(); m != nil {
		ns, _ := m["namespace"].(string)
		return ns
	}
	return ""
}

// Doc returns the "doc" field from native attributes, or empty.
func (n *Node) Doc() string {
	if m := n.NativeMap(); m != nil {
		doc, _ := m["doc"].(string)
		return doc
	}
	return ""
}

// Aliases returns the "aliases" list from native attributes, or nil.
func (n *Node) Aliases() []string {
	m := n.NativeMap()
	if m == nil {
		return nil
	}
	raw, ok := m["aliases"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
