package command

import (
	"fmt"

	"github.com/onereallylongname/avredit/internal/projection"
)

// UpdateAttributeParams defines parameters for the UpdateAttribute command.
type UpdateAttributeParams struct {
	NodeID   string
	Scope    string // "native" or "custom"
	Key      string
	NewValue any // nil means delete the key
}

// UpdateAttribute creates a command that changes a single attribute on a node.
// If NewValue is nil, the key is deleted from the map.
func UpdateAttribute(proj *projection.Projection, params UpdateAttributeParams) (*Command, error) {
	node := proj.Get(params.NodeID)
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", params.NodeID)
	}

	var oldValue any
	var existed bool

	switch params.Scope {
	case "native":
		m := node.NativeMap()
		if m == nil {
			return nil, fmt.Errorf("node %s native attrs is not a map", params.NodeID)
		}
		oldValue, existed = m[params.Key]
	case "custom":
		oldValue, existed = node.Attrs.Custom[params.Key]
	default:
		return nil, fmt.Errorf("invalid scope: %s (must be 'native' or 'custom')", params.Scope)
	}

	return &Command{
		DoFn: func() error {
			switch params.Scope {
			case "native":
				m := node.NativeMap()
				if params.NewValue == nil {
					delete(m, params.Key)
				} else {
					m[params.Key] = params.NewValue
				}
			case "custom":
				if params.NewValue == nil {
					delete(node.Attrs.Custom, params.Key)
				} else {
					node.Attrs.Custom[params.Key] = params.NewValue
				}
			}
			return nil
		},
		UndoFn: func() error {
			switch params.Scope {
			case "native":
				m := node.NativeMap()
				if existed {
					m[params.Key] = oldValue
				} else {
					delete(m, params.Key)
				}
			case "custom":
				if existed {
					node.Attrs.Custom[params.Key] = oldValue
				} else {
					delete(node.Attrs.Custom, params.Key)
				}
			}
			return nil
		},
		Desc: fmt.Sprintf("Update %s.%s", params.Scope, params.Key),
	}, nil
}

// DeleteAttributeParams defines parameters for the DeleteAttribute command.
type DeleteAttributeParams struct {
	NodeID string
	Scope  string // "native" or "custom"
	Key    string
}

// DeleteAttribute creates a command that removes an attribute from a node.
func DeleteAttribute(proj *projection.Projection, params DeleteAttributeParams) (*Command, error) {
	node := proj.Get(params.NodeID)
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", params.NodeID)
	}

	switch params.Scope {
	case "native":
		m := node.NativeMap()
		if m == nil {
			return nil, fmt.Errorf("node %s native attrs is not a map", params.NodeID)
		}
		oldValue, existed := m[params.Key]
		if !existed {
			return nil, fmt.Errorf("native attribute %q not found on node %s", params.Key, params.NodeID)
		}
		return &Command{
			DoFn: func() error {
				delete(node.NativeMap(), params.Key)
				return nil
			},
			UndoFn: func() error {
				node.NativeMap()[params.Key] = oldValue
				return nil
			},
			Desc: fmt.Sprintf("Delete native.%s", params.Key),
		}, nil

	case "custom":
		oldValue, existed := node.Attrs.Custom[params.Key]
		if !existed {
			return nil, fmt.Errorf("custom attribute %q not found on node %s", params.Key, params.NodeID)
		}
		return &Command{
			DoFn: func() error {
				delete(node.Attrs.Custom, params.Key)
				return nil
			},
			UndoFn: func() error {
				node.Attrs.Custom[params.Key] = oldValue
				return nil
			},
			Desc: fmt.Sprintf("Delete custom.%s", params.Key),
		}, nil

	default:
		return nil, fmt.Errorf("invalid scope: %s", params.Scope)
	}
}

// RenameAttributeParams defines parameters for renaming a native map key.
type RenameAttributeParams struct {
	NodeID string
	OldKey string
	NewKey string
}

// RenameAttribute atomically renames a key in the native map, preserving its value.
func RenameAttribute(proj *projection.Projection, params RenameAttributeParams) (*Command, error) {
	node := proj.Get(params.NodeID)
	if node == nil {
		return nil, fmt.Errorf("node not found: %s", params.NodeID)
	}
	m := node.NativeMap()
	if m == nil {
		return nil, fmt.Errorf("node %s has no native map", params.NodeID)
	}
	val, ok := m[params.OldKey]
	if !ok {
		return nil, fmt.Errorf("key %q not found", params.OldKey)
	}

	return &Command{
		DoFn: func() error {
			m := node.NativeMap()
			delete(m, params.OldKey)
			m[params.NewKey] = val
			return nil
		},
		UndoFn: func() error {
			m := node.NativeMap()
			delete(m, params.NewKey)
			m[params.OldKey] = val
			return nil
		},
		Desc: fmt.Sprintf("Rename %s → %s", params.OldKey, params.NewKey),
	}, nil
}
