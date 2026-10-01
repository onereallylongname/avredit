package command

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/projection"
)

// CopyNodeParams defines parameters for the CopyNode command.
type CopyNodeParams struct {
	SourceID string
	TargetID string
	Index    int // -1 for append
}

// CopyNode creates a command that duplicates a node subtree into a target.
func CopyNode(proj *projection.Projection, params CopyNodeParams) (*Command, error) {
	source := proj.Get(params.SourceID)
	if source == nil {
		return nil, fmt.Errorf("source not found: %s", params.SourceID)
	}

	// Cannot copy type nodes (they are structural, not user-facing)
	if proj.IsInSingleSlot(source) {
		return nil, fmt.Errorf("cannot copy a type node (use the parent field instead)")
	}

	cloned, err := projection.CloneSubtree(proj, params.SourceID)
	if err != nil {
		return nil, fmt.Errorf("cloning subtree: %w", err)
	}

	return CreateNode(proj, CreateNodeParams{
		NewSubtree: cloned,
		TargetID:   params.TargetID,
		Index:      params.Index,
	})
}
