package command

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/schema"
)

// ReplaceTypeParams defines parameters for the ReplaceType command.
type ReplaceTypeParams struct {
	ParentID   string // field, array, or map node
	NewSubtree *projection.CloneResult
}

// ReplaceType creates a command that atomically swaps the type child of a single-slot parent.
// If the parent has no existing child (e.g., newly created array/map), it simply adds the new subtree.
func ReplaceType(proj *projection.Projection, params ReplaceTypeParams) (*Command, error) {
	parent := proj.Get(params.ParentID)
	if parent == nil {
		return nil, fmt.Errorf("parent not found: %s", params.ParentID)
	}

	if parent.Kind != schema.KindField && parent.Kind != schema.KindArray && parent.Kind != schema.KindMap {
		return nil, fmt.Errorf("replaceType only works on field/array/map, got: %s", parent.Kind)
	}

	newRoot := params.NewSubtree.Root
	newNodes := params.NewSubtree.Nodes

	// Handle childless parent (newly created array/map without items/values child)
	if len(parent.Children) == 0 {
		return &Command{
			DoFn: func() error {
				for _, n := range newNodes {
					proj.Nodes[n.ID] = n
				}
				parent.Children = []string{newRoot.ID}
				newRoot.ParentID = params.ParentID
				return nil
			},
			UndoFn: func() error {
				for _, n := range newNodes {
					delete(proj.Nodes, n.ID)
				}
				parent.Children = parent.Children[:0]
				return nil
			},
			Desc: fmt.Sprintf("Set type on %s %s", parent.Kind, params.ParentID),
		}, nil
	}

	oldTypeID := parent.Children[0]
	oldTypeNode := proj.Get(oldTypeID)
	if oldTypeNode == nil {
		return nil, fmt.Errorf("old type node not found: %s", oldTypeID)
	}

	// Capture old subtree for undo
	oldSubtree := collectSubtree(proj, oldTypeNode)

	return &Command{
		DoFn: func() error {
			// Remove old subtree
			for _, n := range oldSubtree {
				delete(proj.Nodes, n.ID)
			}
			parent.Children = parent.Children[:0]

			// Insert new subtree
			for _, n := range newNodes {
				proj.Nodes[n.ID] = n
			}
			parent.Children = []string{newRoot.ID}
			newRoot.ParentID = params.ParentID
			return nil
		},
		UndoFn: func() error {
			// Remove new subtree
			for _, n := range newNodes {
				delete(proj.Nodes, n.ID)
			}
			parent.Children = parent.Children[:0]

			// Restore old subtree
			for _, n := range oldSubtree {
				proj.Nodes[n.ID] = n
			}
			parent.Children = []string{oldTypeID}
			oldTypeNode.ParentID = params.ParentID
			return nil
		},
		Desc: fmt.Sprintf("Replace type on %s %s", parent.Kind, params.ParentID),
	}, nil
}
