package projection

import "github.com/onereallylongname/avredit/internal/schema"

// Stats holds computed statistics about a schema.
type Stats struct {
	FieldCount int
	MaxDepth   int
}

// CalculateStats computes field count and maximum tree depth.
func CalculateStats(proj *Projection) Stats {
	if proj.RootID == "" {
		return Stats{}
	}

	var stats Stats
	statsVisit(proj, proj.RootID, 0, &stats)
	return stats
}

func statsVisit(proj *Projection, nodeID string, depth int, stats *Stats) {
	node := proj.Get(nodeID)
	if node == nil {
		return
	}

	if depth > stats.MaxDepth {
		stats.MaxDepth = depth
	}

	if node.Kind == schema.KindField {
		stats.FieldCount++
	}

	for _, childID := range node.Children {
		statsVisit(proj, childID, depth+1, stats)
	}
}
