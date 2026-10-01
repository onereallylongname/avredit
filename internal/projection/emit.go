package projection

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/schema"
)

// GenerateAvro reconstructs a valid Avro schema from the projection.
// Walks the tree from root, emitting each node according to its kind.
// Structural keys (fields, items, values, type) are rebuilt from children;
// all other native map keys are preserved as-is from the original parse.
func GenerateAvro(proj *Projection) (any, error) {
	root := proj.Get(proj.RootID)
	if root == nil {
		return nil, fmt.Errorf("projection has no root")
	}
	return emitNode(proj, root)
}

func emitNode(proj *Projection, node *Node) (any, error) {
	switch node.Kind {
	case schema.KindSchema:
		return emitSchema(proj, node)
	case schema.KindRecord:
		return emitRecord(proj, node)
	case schema.KindField:
		return emitField(proj, node)
	case schema.KindArray:
		return emitArray(proj, node)
	case schema.KindMap:
		return emitMap(proj, node)
	case schema.KindUnion:
		return emitUnion(proj, node)
	case schema.KindEnum:
		return emitLeaf(node), nil
	case schema.KindFixed:
		return emitLeaf(node), nil
	case schema.KindPrimitive, schema.KindNamed:
		return emitPrimitive(node), nil
	default:
		return nil, fmt.Errorf("unknown node kind: %s", node.Kind)
	}
}

func emitSchema(proj *Projection, node *Node) (any, error) {
	if len(node.Children) == 0 {
		return nil, fmt.Errorf("schema node has no children")
	}
	typeChild := proj.Get(node.Children[0])
	if typeChild == nil {
		return nil, fmt.Errorf("schema type child not found")
	}
	return emitNode(proj, typeChild)
}

func emitRecord(proj *Projection, node *Node) (any, error) {
	result := copyNativeMap(node)

	fields := make([]any, 0, len(node.Children))
	for _, childID := range node.Children {
		child := proj.Get(childID)
		if child == nil {
			return nil, fmt.Errorf("record field child %s not found", childID)
		}
		fieldVal, err := emitNode(proj, child)
		if err != nil {
			return nil, err
		}
		fields = append(fields, fieldVal)
	}
	result["fields"] = fields
	return result, nil
}

func emitField(proj *Projection, node *Node) (any, error) {
	result := copyNativeMap(node)

	if len(node.Children) == 0 {
		return nil, fmt.Errorf("field node %s has no type child", node.ID)
	}
	typeChild := proj.Get(node.Children[0])
	if typeChild == nil {
		return nil, fmt.Errorf("field type child not found")
	}
	typeVal, err := emitNode(proj, typeChild)
	if err != nil {
		return nil, err
	}
	result["type"] = typeVal
	return result, nil
}

func emitArray(proj *Projection, node *Node) (any, error) {
	result := copyNativeMap(node)

	if len(node.Children) == 0 {
		return nil, fmt.Errorf("array node %s has no items child", node.ID)
	}
	itemsChild := proj.Get(node.Children[0])
	if itemsChild == nil {
		return nil, fmt.Errorf("array items child not found")
	}
	itemsVal, err := emitNode(proj, itemsChild)
	if err != nil {
		return nil, err
	}
	result["items"] = itemsVal
	return result, nil
}

func emitMap(proj *Projection, node *Node) (any, error) {
	result := copyNativeMap(node)

	if len(node.Children) == 0 {
		return nil, fmt.Errorf("map node %s has no values child", node.ID)
	}
	valuesChild := proj.Get(node.Children[0])
	if valuesChild == nil {
		return nil, fmt.Errorf("map values child not found")
	}
	valuesVal, err := emitNode(proj, valuesChild)
	if err != nil {
		return nil, err
	}
	result["values"] = valuesVal
	return result, nil
}

func emitUnion(proj *Projection, node *Node) (any, error) {
	branches := make([]any, 0, len(node.Children))
	for _, childID := range node.Children {
		child := proj.Get(childID)
		if child == nil {
			return nil, fmt.Errorf("union branch child %s not found", childID)
		}
		branchVal, err := emitNode(proj, child)
		if err != nil {
			return nil, err
		}
		branches = append(branches, branchVal)
	}
	return branches, nil
}

func emitLeaf(node *Node) any {
	// Enum and fixed are emitted exactly as stored in native.
	return node.Attrs.Native
}

func emitPrimitive(node *Node) any {
	// Primitives are emitted exactly as stored — preserving original format.
	// Could be a plain string ("int") or object ({"type":"int","logicalType":"date"}).
	return node.Attrs.Native
}

// copyNativeMap creates a shallow copy of the node's native map, excluding
// keys that are reconstructed from children (fields, items, values, type for field).
// This is critical: structural keys are rebuilt by walking child nodes, while
// all other attributes (name, doc, namespace, etc.) are preserved verbatim.
func copyNativeMap(node *Node) map[string]any {
	source := node.NativeMap()
	if source == nil {
		return map[string]any{}
	}

	result := make(map[string]any, len(source))
	for k, v := range source {
		// Skip keys that are rebuilt from children
		switch node.Kind {
		case schema.KindRecord:
			if k == "fields" {
				continue
			}
		case schema.KindField:
			if k == "type" {
				continue
			}
		case schema.KindArray:
			if k == "items" {
				continue
			}
		case schema.KindMap:
			if k == "values" {
				continue
			}
		}
		result[k] = v
	}
	return result
}
