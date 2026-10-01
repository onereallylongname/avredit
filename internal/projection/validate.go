package projection

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/schema"
)

// Validate checks structural integrity of a projection.
func Validate(proj *Projection) error {
	if proj.RootID == "" {
		return fmt.Errorf("projection has no root")
	}

	for _, node := range proj.Nodes {
		if node.Kind == "" {
			return fmt.Errorf("node %s missing kind", node.ID)
		}
		if node.ID == "" {
			return fmt.Errorf("node missing id")
		}

		switch node.Kind {
		case schema.KindField:
			if len(node.Children) != 1 {
				return fmt.Errorf("field %s must have exactly one type child, has %d", node.ID, len(node.Children))
			}
		case schema.KindUnion:
			if len(node.Children) == 0 {
				return fmt.Errorf("union %s must have at least one branch", node.ID)
			}
		case schema.KindArray:
			if len(node.Children) != 1 {
				return fmt.Errorf("array %s must have exactly one items child, has %d", node.ID, len(node.Children))
			}
		case schema.KindMap:
			if len(node.Children) != 1 {
				return fmt.Errorf("map %s must have exactly one values child, has %d", node.ID, len(node.Children))
			}
		}
	}

	return validatePathsDefined(proj)
}

func validatePathsDefined(proj *Projection) error {
	for _, node := range proj.Nodes {
		if node.Path == nil {
			return fmt.Errorf("node %s has nil path", node.ID)
		}
	}
	return nil
}

// RebuildPaths recomputes the JSON path for every node in the projection.
func RebuildPaths(proj *Projection) error {
	root := proj.Get(proj.RootID)
	if root == nil {
		return fmt.Errorf("projection has no root")
	}
	rebuildVisit(proj, root, []any{})
	return nil
}

func rebuildVisit(proj *Projection, node *Node, path []any) {
	node.Path = clonePath(path)

	switch node.Kind {
	case schema.KindSchema:
		if len(node.Children) > 0 {
			child := proj.Get(node.Children[0])
			if child != nil {
				rebuildVisit(proj, child, path)
			}
		}

	case schema.KindRecord:
		for i, childID := range node.Children {
			child := proj.Get(childID)
			if child != nil {
				rebuildVisit(proj, child, appendPath(path, "fields", i))
			}
		}

	case schema.KindField:
		if len(node.Children) > 0 {
			child := proj.Get(node.Children[0])
			if child != nil {
				rebuildVisit(proj, child, appendPath(path, "type"))
			}
		}

	case schema.KindArray:
		if len(node.Children) > 0 {
			child := proj.Get(node.Children[0])
			if child != nil {
				rebuildVisit(proj, child, appendPath(path, "items"))
			}
		}

	case schema.KindMap:
		if len(node.Children) > 0 {
			child := proj.Get(node.Children[0])
			if child != nil {
				rebuildVisit(proj, child, appendPath(path, "values"))
			}
		}

	case schema.KindUnion:
		for i, childID := range node.Children {
			child := proj.Get(childID)
			if child != nil {
				rebuildVisit(proj, child, appendPath(path, i))
			}
		}
	}
}

// errNodeNotFound creates a standard error for missing nodes.
func errNodeNotFound(id string) error {
	return fmt.Errorf("node not found: %s", id)
}
