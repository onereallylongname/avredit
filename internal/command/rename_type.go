package command

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/projection"
)

// RenameNamedTypeParams defines parameters for renaming a named type and propagating.
type RenameNamedTypeParams struct {
	NodeID  string // the record/enum/fixed node
	OldName string // previous name (or alias)
	NewName string // new name (or alias)
}

// RenameNamedType renames a type's "name" attribute and updates all KindNamed references.
func RenameNamedType(proj *projection.Projection, params RenameNamedTypeParams) (*Command, error) {
	node := proj.Get(params.NodeID)
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", params.NodeID)
	}
	m := node.NativeMap()
	if m == nil {
		return nil, fmt.Errorf("node %s has no native map", params.NodeID)
	}

	// Find refs by short name
	refs := proj.FindNamedRefs(params.OldName)

	// Also find refs by fully-qualified name (namespace.name)
	ns, _ := m["namespace"].(string)
	var qualifiedRefs []*projection.Node
	if ns != "" {
		oldFQ := ns + "." + params.OldName
		qualifiedRefs = proj.FindNamedRefs(oldFQ)
	}
	newFQ := ""
	if ns != "" {
		newFQ = ns + "." + params.NewName
	}

	return &Command{
		DoFn: func() error {
			m["name"] = params.NewName
			for _, ref := range refs {
				ref.Attrs.Native = params.NewName
			}
			for _, ref := range qualifiedRefs {
				ref.Attrs.Native = newFQ
			}
			return nil
		},
		UndoFn: func() error {
			m["name"] = params.OldName
			for _, ref := range refs {
				ref.Attrs.Native = params.OldName
			}
			oldFQ := ns + "." + params.OldName
			for _, ref := range qualifiedRefs {
				ref.Attrs.Native = oldFQ
			}
			return nil
		},
		Desc: fmt.Sprintf("Rename type %s → %s", params.OldName, params.NewName),
	}, nil
}

// RenameAliasParams defines parameters for renaming an alias and propagating.
type RenameAliasParams struct {
	NodeID   string // the record/enum/fixed node
	OldAlias string
	NewAlias string
}

// RenameAlias renames an alias in the aliases list and updates all KindNamed references.
func RenameAlias(proj *projection.Projection, params RenameAliasParams) (*Command, error) {
	node := proj.Get(params.NodeID)
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", params.NodeID)
	}
	m := node.NativeMap()
	if m == nil {
		return nil, fmt.Errorf("node %s has no native map", params.NodeID)
	}

	refs := proj.FindNamedRefs(params.OldAlias)

	return &Command{
		DoFn: func() error {
			if aliases, ok := m["aliases"].([]any); ok {
				for i, a := range aliases {
					if s, ok := a.(string); ok && s == params.OldAlias {
						aliases[i] = params.NewAlias
						break
					}
				}
			}
			for _, ref := range refs {
				ref.Attrs.Native = params.NewAlias
			}
			return nil
		},
		UndoFn: func() error {
			if aliases, ok := m["aliases"].([]any); ok {
				for i, a := range aliases {
					if s, ok := a.(string); ok && s == params.NewAlias {
						aliases[i] = params.OldAlias
						break
					}
				}
			}
			for _, ref := range refs {
				ref.Attrs.Native = params.OldAlias
			}
			return nil
		},
		Desc: fmt.Sprintf("Rename alias %s → %s", params.OldAlias, params.NewAlias),
	}, nil
}
