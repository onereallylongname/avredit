package command

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/projection"
)

// MoveNodeParams defines parameters for the MoveNode command.
type MoveNodeParams struct {
	NodeID   string
	TargetID string
	Index    int // -1 for append
}

// MoveNode creates a command that moves a node from its current parent to a new target.
func MoveNode(proj *projection.Projection, params MoveNodeParams) (*Command, error) {
	node := proj.Get(params.NodeID)
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", params.NodeID)
	}

	if proj.IsInSingleSlot(node) {
		return nil, fmt.Errorf("cannot move node %s: it is in a single-slot position (use ReplaceType)", params.NodeID)
	}

	oldParent := proj.Get(node.ParentID)
	if oldParent == nil {
		return nil, fmt.Errorf("old parent not found for %s", params.NodeID)
	}

	target := proj.Get(params.TargetID)
	if target == nil {
		return nil, fmt.Errorf("target node not found: %s", params.TargetID)
	}

	oldIndex := indexOf(oldParent.Children, params.NodeID)
	if oldIndex < 0 {
		return nil, fmt.Errorf("invariant: node %s not found in parent children", params.NodeID)
	}

	// Compute insertion index
	sameParent := oldParent.ID == target.ID
	insertIdx := params.Index
	if insertIdx < 0 {
		if sameParent {
			insertIdx = len(target.Children) - 1
		} else {
			insertIdx = len(target.Children)
		}
	}

	return &Command{
		DoFn: func() error {
			// Detach from old parent
			oldParent.Children = append(oldParent.Children[:oldIndex], oldParent.Children[oldIndex+1:]...)
			// Attach to new parent
			children := target.Children
			if insertIdx > len(children) {
				insertIdx = len(children)
			}
			target.Children = append(children[:insertIdx], append([]string{params.NodeID}, children[insertIdx:]...)...)
			node.ParentID = params.TargetID
			return nil
		},
		UndoFn: func() error {
			// Detach from new parent
			idx := indexOf(target.Children, params.NodeID)
			if idx >= 0 {
				target.Children = append(target.Children[:idx], target.Children[idx+1:]...)
			}
			// Restore to old parent at old position
			children := oldParent.Children
			oldParent.Children = append(children[:oldIndex], append([]string{params.NodeID}, children[oldIndex:]...)...)
			node.ParentID = oldParent.ID
			return nil
		},
		Desc: fmt.Sprintf("Move node %s", params.NodeID),
	}, nil
}
