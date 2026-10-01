package command

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/projection"
)

// CreateNodeParams defines parameters for the CreateNode command.
type CreateNodeParams struct {
	NewSubtree *projection.CloneResult
	TargetID   string
	Index      int // -1 for append
}

// CreateNode creates a command that inserts a new subtree into the projection.
func CreateNode(proj *projection.Projection, params CreateNodeParams) (*Command, error) {
	target := proj.Get(params.TargetID)
	if target == nil {
		return nil, fmt.Errorf("target node not found: %s", params.TargetID)
	}

	root := params.NewSubtree.Root
	nodes := params.NewSubtree.Nodes

	insertIdx := params.Index
	if insertIdx < 0 {
		insertIdx = len(target.Children)
	}

	return &Command{
		DoFn: func() error {
			// Register all nodes
			for _, n := range nodes {
				proj.Nodes[n.ID] = n
			}
			// Attach root to target
			root.ParentID = params.TargetID
			children := target.Children
			if insertIdx > len(children) {
				insertIdx = len(children)
			}
			// Insert at index
			target.Children = append(children[:insertIdx], append([]string{root.ID}, children[insertIdx:]...)...)
			return nil
		},
		UndoFn: func() error {
			// Detach root
			idx := indexOf(target.Children, root.ID)
			if idx >= 0 {
				target.Children = append(target.Children[:idx], target.Children[idx+1:]...)
			}
			// Remove all nodes
			for _, n := range nodes {
				delete(proj.Nodes, n.ID)
			}
			return nil
		},
		Desc: "Create node",
	}, nil
}

func indexOf(slice []string, val string) int {
	for i, s := range slice {
		if s == val {
			return i
		}
	}
	return -1
}
