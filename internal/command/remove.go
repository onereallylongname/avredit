package command

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/projection"
)

// RemoveNodeParams defines parameters for the RemoveNode command.
type RemoveNodeParams struct {
	NodeID string
}

// RemoveNode creates a command that removes a node and its subtree.
func RemoveNode(proj *projection.Projection, params RemoveNodeParams) (*Command, error) {
	node := proj.Get(params.NodeID)
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", params.NodeID)
	}

	if proj.IsInSingleSlot(node) {
		return nil, fmt.Errorf("cannot remove node %s: it is the sole type child (use ReplaceType instead)", params.NodeID)
	}

	parent := proj.Get(node.ParentID)
	if parent == nil {
		return nil, fmt.Errorf("parent not found for node %s", params.NodeID)
	}

	index := indexOf(parent.Children, params.NodeID)
	if index < 0 {
		return nil, fmt.Errorf("invariant violation: node %s not in parent children", params.NodeID)
	}

	// Capture entire subtree for undo
	subtree := collectSubtree(proj, node)

	return &Command{
		DoFn: func() error {
			// Detach from parent
			parent.Children = append(parent.Children[:index], parent.Children[index+1:]...)
			// Remove subtree from map
			for _, n := range subtree {
				delete(proj.Nodes, n.ID)
			}
			return nil
		},
		UndoFn: func() error {
			// Re-register subtree
			for _, n := range subtree {
				proj.Nodes[n.ID] = n
			}
			// Reattach at same position
			children := parent.Children
			parent.Children = append(children[:index], append([]string{params.NodeID}, children[index:]...)...)
			node.ParentID = parent.ID
			return nil
		},
		Desc: fmt.Sprintf("Remove node %s", params.NodeID),
	}, nil
}

// collectSubtree gathers a node and all its descendants.
func collectSubtree(proj *projection.Projection, root *projection.Node) []*projection.Node {
	var nodes []*projection.Node
	var visit func(n *projection.Node)
	visit = func(n *projection.Node) {
		nodes = append(nodes, n)
		for _, childID := range n.Children {
			child := proj.Get(childID)
			if child != nil {
				visit(child)
			}
		}
	}
	visit(root)
	return nodes
}
