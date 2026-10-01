package projection

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/schema"
)

// Projection is the internal tree representation of an Avro schema.
type Projection struct {
	RootID string
	Nodes  map[string]*Node
}

// New creates an empty projection.
func New() *Projection {
	return &Projection{
		Nodes: make(map[string]*Node),
	}
}

// Get retrieves a node by ID or returns nil.
func (p *Projection) Get(id string) *Node {
	return p.Nodes[id]
}

// Register adds a node to the projection and links it to its parent.
func (p *Projection) Register(node *Node) error {
	p.Nodes[node.ID] = node

	if node.ParentID == "" {
		p.RootID = node.ID
	} else {
		parent := p.Nodes[node.ParentID]
		if parent == nil {
			return fmt.Errorf("parent node does not exist: %s", node.ParentID)
		}
		parent.Children = append(parent.Children, node.ID)
	}
	return nil
}

// Build constructs a full projection from a parsed Avro schema (JSON-decoded map).
func Build(raw map[string]any) (*Projection, error) {
	ResetNodeCounter()
	proj := New()

	schemaNode := NewNode(schema.KindSchema, raw, "", []any{})
	if err := proj.Register(schemaNode); err != nil {
		return nil, err
	}

	if err := visitType(proj, raw, schemaNode.ID, []any{}); err != nil {
		return nil, fmt.Errorf("building projection: %w", err)
	}

	if err := Validate(proj); err != nil {
		return nil, err
	}
	return proj, nil
}

func visitType(proj *Projection, typeVal any, parentID string, path []any) error {
	norm, err := NormalizeType(typeVal)
	if err != nil {
		return err
	}

	typeNode := NewNode(norm.Kind, typeVal, parentID, clonePath(path))
	if err := proj.Register(typeNode); err != nil {
		return err
	}

	switch norm.Kind {
	case schema.KindRecord:
		for i, f := range norm.Fields {
			fieldMap, ok := f.(map[string]any)
			if !ok {
				return fmt.Errorf("record field at index %d is not an object", i)
			}
			if err := visitField(proj, fieldMap, typeNode.ID, appendPath(path, "fields", i)); err != nil {
				return err
			}
		}

	case schema.KindArray:
		if norm.Items != nil {
			if err := visitType(proj, norm.Items, typeNode.ID, appendPath(path, "items")); err != nil {
				return err
			}
		}

	case schema.KindMap:
		if norm.Values != nil {
			if err := visitType(proj, norm.Values, typeNode.ID, appendPath(path, "values")); err != nil {
				return err
			}
		}

	case schema.KindUnion:
		for i, branch := range norm.Branches {
			if err := visitType(proj, branch, typeNode.ID, appendPath(path, i)); err != nil {
				return err
			}
		}
	}

	return nil
}

func visitField(proj *Projection, field map[string]any, parentID string, path []any) error {
	fieldNode := NewNode(schema.KindField, field, parentID, clonePath(path))
	if err := proj.Register(fieldNode); err != nil {
		return err
	}

	fieldType := field["type"]
	if fieldType == nil {
		return fmt.Errorf("field %q missing 'type'", field["name"])
	}

	return visitType(proj, fieldType, fieldNode.ID, appendPath(path, "type"))
}

// Path helpers

func clonePath(p []any) []any {
	cp := make([]any, len(p))
	copy(cp, p)
	return cp
}

func appendPath(base []any, segments ...any) []any {
	result := make([]any, len(base), len(base)+len(segments))
	copy(result, base)
	return append(result, segments...)
}

// GetParent returns the parent node of the given node, or nil for root.
func (p *Projection) GetParent(node *Node) *Node {
	if node.ParentID == "" {
		return nil
	}
	return p.Nodes[node.ParentID]
}

// IsInSingleSlot checks if removing/moving this node would leave a single-slot parent empty.
func (p *Projection) IsInSingleSlot(node *Node) bool {
	if node.ParentID == "" {
		return false
	}
	parent := p.Get(node.ParentID)
	if parent == nil {
		return false
	}
	return parent.Kind == schema.KindField ||
		parent.Kind == schema.KindArray ||
		parent.Kind == schema.KindMap
}

// IsDescendant checks if candidateID is a descendant of ancestorID.
func (p *Projection) IsDescendant(ancestorID, candidateID string) bool {
	current := p.Get(candidateID)
	for current != nil && current.ParentID != "" {
		if current.ParentID == ancestorID {
			return true
		}
		current = p.Get(current.ParentID)
	}
	return false
}

// GetSlotsForNode returns the available slots for a node based on its kind.
func GetSlotsForNode(node *Node) []SlotInfo {
	switch node.Kind {
	case schema.KindSchema:
		return []SlotInfo{{Slot: schema.SlotSchemaRoot, Multiple: false}}
	case schema.KindRecord:
		return []SlotInfo{{Slot: schema.SlotRecordFields, Multiple: true}}
	case schema.KindField:
		return []SlotInfo{{Slot: schema.SlotFieldType, Multiple: false}}
	case schema.KindArray:
		return []SlotInfo{{Slot: schema.SlotArrayItems, Multiple: false}}
	case schema.KindMap:
		return []SlotInfo{{Slot: schema.SlotMapValues, Multiple: false}}
	case schema.KindUnion:
		return []SlotInfo{{Slot: schema.SlotUnionBranch, Multiple: true}}
	default:
		return nil
	}
}

// SlotInfo describes a slot's identity and cardinality.
type SlotInfo struct {
	Slot     schema.Slot
	Multiple bool
}

// FindNamedRefs returns all KindNamed nodes whose string value matches refName.
func (p *Projection) FindNamedRefs(refName string) []*Node {
	var refs []*Node
	for _, node := range p.Nodes {
		if node.Kind == schema.KindNamed && node.NativeString() == refName {
			refs = append(refs, node)
		}
	}
	return refs
}

// NamedTypes collects all user-defined named types (record, enum, fixed) from the projection.
// Includes both primary names and aliases for named-type-suggestions.
func (p *Projection) NamedTypes() []string {
	names, aliases := p.NamedTypesAndAliases()
	return append(names, aliases...)
}

// NamedTypesAndAliases returns named type names and aliases as separate lists.
func (p *Projection) NamedTypesAndAliases() (names []string, aliases []string) {
	seenNames := make(map[string]bool)
	seenAliases := make(map[string]bool)

	for _, node := range p.Nodes {
		if node.Kind == schema.KindRecord || node.Kind == schema.KindEnum || node.Kind == schema.KindFixed {
			m := node.NativeMap()
			if m == nil {
				continue
			}
			name, _ := m["name"].(string)
			if name == "" {
				continue
			}
			ns, _ := m["namespace"].(string)
			fullName := name
			if ns != "" {
				fullName = ns + "." + name
			}
			if !seenNames[fullName] {
				seenNames[fullName] = true
				names = append(names, fullName)
			}
			if al, ok := m["aliases"].([]any); ok {
				for _, a := range al {
					if alias, ok := a.(string); ok && alias != "" {
						if !seenAliases[alias] {
							seenAliases[alias] = true
							aliases = append(aliases, alias)
						}
					}
				}
			}
		}
	}
	return names, aliases
}
