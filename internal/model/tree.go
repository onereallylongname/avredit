package model

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/schema"
	"github.com/onereallylongname/avredit/internal/theme"
)

// TreeLine represents a single visible line in the flattened tree.
type TreeLine struct {
	Node     *projection.Node
	Depth    int
	Expanded bool
	HasChild bool
}

// TreeModel manages the tree panel: cursor, expand/collapse, scroll, render.
type TreeModel struct {
	proj  *projection.Projection
	theme *theme.Theme

	lines    []TreeLine
	cursor   int
	offset   int // scroll offset (first visible line index)
	width    int
	height   int
	expanded map[string]bool // node ID → expanded state

	// Search highlighting
	matchedIDs map[string]bool
}

// NewTreeModel creates a tree model from a projection.
func NewTreeModel(proj *projection.Projection, th *theme.Theme) TreeModel {
	tm := TreeModel{
		proj:     proj,
		theme:    th,
		expanded: make(map[string]bool),
	}
	// Expand root and first-level by default
	tm.expandDefaults()
	tm.rebuildLines()
	return tm
}

// expandDefaults expands the schema root and its immediate type child.
func (t *TreeModel) expandDefaults() {
	root := t.proj.Nodes[t.proj.RootID]
	if root == nil {
		return
	}
	t.expanded[root.ID] = true
	for _, childID := range root.Children {
		t.expanded[childID] = true
	}
}

// rebuildLines flattens the tree according to current expand state.
func (t *TreeModel) rebuildLines() {
	t.lines = nil
	root := t.proj.Nodes[t.proj.RootID]
	if root == nil {
		return
	}
	t.flattenNode(root, 0)
}

// flattenNode recursively adds visible nodes to lines.
func (t *TreeModel) flattenNode(node *projection.Node, depth int) {
	hasChildren := len(node.Children) > 0
	expanded := t.expanded[node.ID]

	t.lines = append(t.lines, TreeLine{
		Node:     node,
		Depth:    depth,
		Expanded: expanded,
		HasChild: hasChildren,
	})

	if expanded && hasChildren {
		for _, childID := range node.Children {
			childNode := t.proj.Nodes[childID]
			if childNode != nil {
				t.flattenNode(childNode, depth+1)
			}
		}
	}
}

// HandleKey processes a keypress and returns updated model.
func (t TreeModel) HandleKey(msg tea.KeyPressMsg) TreeModel {
	switch msg.String() {
	case "j", "down":
		t.moveCursor(1)
	case "k", "up":
		t.moveCursor(-1)
	case "l", "right":
		t.expandCurrent()
	case "h", "left":
		t.collapseCurrent()
	case "space":
		t.toggleCurrent()
	case "enter":
		t.expandCurrent()
	case "g":
		t.cursor = 0
		t.offset = 0
	case "G":
		if len(t.lines) > 0 {
			t.cursor = len(t.lines) - 1
			t.ensureCursorVisible()
		}
	}
	return t
}

// moveCursor moves the cursor by delta lines (clamped).
func (t *TreeModel) moveCursor(delta int) {
	t.cursor += delta
	if t.cursor < 0 {
		t.cursor = 0
	}
	if t.cursor >= len(t.lines) {
		t.cursor = len(t.lines) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.ensureCursorVisible()
}

// ensureCursorVisible adjusts scroll offset so cursor is within view.
func (t *TreeModel) ensureCursorVisible() {
	viewHeight := t.viewableHeight()
	if viewHeight <= 0 {
		return
	}
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+viewHeight {
		t.offset = t.cursor - viewHeight + 1
	}
}

// viewableHeight returns the number of visible lines (accounting for borders).
func (t *TreeModel) viewableHeight() int {
	// Border takes 2 rows (top + bottom)
	h := t.height - 2
	if h < 1 {
		return 1
	}
	return h
}

// expandCurrent expands the node under cursor.
func (t *TreeModel) expandCurrent() {
	if t.cursor >= 0 && t.cursor < len(t.lines) {
		line := t.lines[t.cursor]
		if line.HasChild && !t.expanded[line.Node.ID] {
			t.expanded[line.Node.ID] = true
			t.rebuildLines()
		}
	}
}

// collapseCurrent collapses the node under cursor, or moves to parent.
func (t *TreeModel) collapseCurrent() {
	if t.cursor < 0 || t.cursor >= len(t.lines) {
		return
	}
	line := t.lines[t.cursor]

	if line.HasChild && t.expanded[line.Node.ID] {
		// Collapse this node
		t.expanded[line.Node.ID] = false
		t.rebuildLines()
	} else if line.Node.ParentID != "" {
		// Move cursor to parent
		for i, l := range t.lines {
			if l.Node.ID == line.Node.ParentID {
				t.cursor = i
				t.ensureCursorVisible()
				break
			}
		}
	}
}

// toggleCurrent toggles expand/collapse for the node under cursor.
func (t *TreeModel) toggleCurrent() {
	if t.cursor < 0 || t.cursor >= len(t.lines) {
		return
	}
	line := t.lines[t.cursor]
	if !line.HasChild {
		return
	}
	t.expanded[line.Node.ID] = !t.expanded[line.Node.ID]
	t.rebuildLines()
}

// SelectedNode returns the node currently under the cursor.
func (t *TreeModel) SelectedNode() *projection.Node {
	if t.cursor >= 0 && t.cursor < len(t.lines) {
		return t.lines[t.cursor].Node
	}
	return nil
}

// JumpToNode expands ancestors and moves cursor to the given node ID.
func (t *TreeModel) JumpToNode(nodeID string) {
	// Expand all ancestors of the target node
	node := t.proj.Nodes[nodeID]
	if node == nil {
		return
	}
	// Walk up to root expanding parents
	current := node
	for current.ParentID != "" {
		t.expanded[current.ParentID] = true
		parent := t.proj.Nodes[current.ParentID]
		if parent == nil {
			break
		}
		current = parent
	}
	t.rebuildLines()

	// Find the line with this node
	for i, line := range t.lines {
		if line.Node.ID == nodeID {
			t.cursor = i
			t.ensureCursorVisible()
			return
		}
	}
}

// SetSize updates the panel dimensions.
func (t *TreeModel) SetSize(w, h int) {
	t.width = w
	t.height = h
}

// View renders the tree panel content (without border).
func (t TreeModel) View() string {
	if len(t.lines) == 0 {
		return "  (empty schema)"
	}

	viewH := t.viewableHeight()
	var sb strings.Builder

	end := t.offset + viewH
	if end > len(t.lines) {
		end = len(t.lines)
	}

	// Available width for content (minus border padding)
	contentWidth := t.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	for i := t.offset; i < end; i++ {
		line := t.lines[i]
		rendered := t.renderLine(line, i == t.cursor, contentWidth)
		sb.WriteString(rendered)
		if i < end-1 {
			sb.WriteString("\n")
		}
	}

	// Pad remaining lines
	rendered := end - t.offset
	for i := rendered; i < viewH; i++ {
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderLine renders a single tree line with type-based coloring.
func (t TreeModel) renderLine(line TreeLine, active bool, maxWidth int) string {
	// Indentation
	indent := strings.Repeat("  ", line.Depth)

	// Expander icon
	var expander string
	if line.HasChild {
		if line.Expanded {
			expander = t.theme.Sym.Expanded
		} else {
			expander = t.theme.Sym.Collapsed
		}
	} else {
		expander = t.theme.Sym.Leaf
	}

	// Node label
	label := nodeLabel(line.Node)

	// Kind badge — for fields, show their type; otherwise show kind letter
	badge := t.nodeBadge(line.Node)

	// Compose the line: prefix + label + (gap) + badge
	prefix := indent + expander + " "

	if active {
		content := fmt.Sprintf("%s%s  %s", prefix, label, badge)
		if len(content) > maxWidth {
			content = content[:maxWidth-1] + "…"
		}
		if len(content) < maxWidth {
			content += strings.Repeat(" ", maxWidth-len(content))
		}
		return t.theme.TreeNodeActive.Render(content)
	}

	// Inactive: badge colored by type kind, label in default color
	isMatch := t.matchedIDs[line.Node.ID]

	var labelStyled, prefixStyled string
	if isMatch {
		matchStyle := t.theme.TreeNode.Underline(true).Bold(true)
		labelStyled = matchStyle.Render(label)
		prefixStyled = t.theme.TreeNode.Render(prefix)
	} else {
		labelStyled = t.theme.TreeNode.Render(label)
		prefixStyled = t.theme.TreeNode.Render(prefix)
	}
	badgeStyled := t.badgeStyle(line.Node).Render(badge)

	rendered := prefixStyled + labelStyled + " " + badgeStyled

	// Pad to width
	renderedWidth := lipgloss.Width(rendered)
	if renderedWidth < maxWidth {
		rendered += strings.Repeat(" ", maxWidth-renderedWidth)
	}

	return rendered
}

// badgeStyle returns the style for the badge based on the node's type.
// For fields, style by child type kind; otherwise by node kind.
func (t TreeModel) badgeStyle(node *projection.Node) lipgloss.Style {
	if node.Kind == schema.KindField && len(node.Children) > 0 {
		child := t.proj.Nodes[node.Children[0]]
		if child != nil {
			return t.kindStyle(child.Kind)
		}
	}
	return t.kindStyle(node.Kind)
}

// nodeBadge returns the badge text for a node.
// For fields, it shows the type (e.g., "int", "null|str"); otherwise the kind letter.
func (t TreeModel) nodeBadge(node *projection.Node) string {
	if node.Kind == schema.KindField {
		return t.fieldTypeBadge(node)
	}
	return t.kindBadge(node.Kind)
}

// fieldTypeBadge derives a short type label from a field's child type node(s).
func (t TreeModel) fieldTypeBadge(field *projection.Node) string {
	if len(field.Children) == 0 {
		return "?"
	}
	child := t.proj.Nodes[field.Children[0]]
	if child == nil {
		return "?"
	}
	return t.typeLabel(child)
}

// typeLabel returns a short displayable type name for a type node.
func (t TreeModel) typeLabel(node *projection.Node) string {
	switch node.Kind {
	case schema.KindPrimitive:
		if m := node.NativeMap(); m != nil {
			lt, _ := m["logicalType"].(string)
			if lt != "" {
				return lt
			}
			if tp, ok := m["type"].(string); ok {
				return shortType(tp)
			}
		}
		if s := node.NativeString(); s != "" {
			return shortType(s)
		}
		return "·"
	case schema.KindRecord:
		return "rec"
	case schema.KindEnum:
		return "enum"
	case schema.KindFixed:
		return "fix"
	case schema.KindArray:
		if len(node.Children) > 0 {
			child := t.proj.Nodes[node.Children[0]]
			if child != nil {
				return "[" + t.typeLabel(child) + "]"
			}
		}
		return "[]"
	case schema.KindMap:
		if len(node.Children) > 0 {
			child := t.proj.Nodes[node.Children[0]]
			if child != nil {
				return "{" + t.typeLabel(child) + "}"
			}
		}
		return "{}"
	case schema.KindNamed:
		s := node.NativeString()
		if s != "" {
			// Show last segment of qualified name
			parts := strings.Split(s, ".")
			name := parts[len(parts)-1]
			if len(name) > 6 {
				return name[:6]
			}
			return name
		}
		return t.theme.Sym.NamedRef
	case schema.KindUnion:
		return t.unionBadge(node)
	default:
		return "?"
	}
}

// unionBadge builds a parenthesized list of union branch types.
func (t TreeModel) unionBadge(union *projection.Node) string {
	if len(union.Children) == 0 {
		return "()"
	}
	parts := make([]string, 0, len(union.Children))
	for _, cid := range union.Children {
		child := t.proj.Nodes[cid]
		if child == nil {
			continue
		}
		parts = append(parts, t.typeLabel(child))
	}
	result := "(" + strings.Join(parts, "|") + ")"
	if len(result) > 14 {
		return result[:13] + "…)"
	}
	return result
}

// shortType abbreviates common Avro type names for badge display.
func shortType(s string) string {
	switch s {
	case "boolean":
		return "bool"
	case "string":
		return "str"
	default:
		return s
	}
}

// kindStyle returns the lipgloss style for a given node kind.
func (t TreeModel) kindStyle(kind schema.NodeKind) lipgloss.Style {
	switch kind {
	case schema.KindRecord:
		return t.theme.KindRecord
	case schema.KindField:
		return t.theme.KindField
	case schema.KindEnum:
		return t.theme.KindEnum
	case schema.KindArray:
		return t.theme.KindArray
	case schema.KindMap:
		return t.theme.KindMap
	case schema.KindUnion:
		return t.theme.KindUnion
	case schema.KindFixed:
		return t.theme.KindFixed
	case schema.KindPrimitive:
		return t.theme.KindPrim
	case schema.KindNamed:
		return t.theme.KindNamed
	default:
		return t.theme.TreeNode
	}
}

// nodeLabel returns a display label for a node.
func nodeLabel(node *projection.Node) string {
	name := node.Name()
	if name != "" {
		return name
	}
	// For named refs, show the reference name
	if node.Kind == schema.KindNamed {
		if s := node.NativeString(); s != "" {
			parts := strings.Split(s, ".")
			return parts[len(parts)-1]
		}
	}
	// For primitives, show the type string
	if node.Kind == schema.KindPrimitive {
		if s := node.NativeString(); s != "" {
			return s
		}
	}
	return string(node.Kind)
}

// kindBadge returns a short kind indicator.
func (t TreeModel) kindBadge(kind schema.NodeKind) string {
	switch kind {
	case schema.KindSchema:
		return "S"
	case schema.KindRecord:
		return "R"
	case schema.KindField:
		return "F"
	case schema.KindEnum:
		return "E"
	case schema.KindArray:
		return "[]"
	case schema.KindMap:
		return "{}"
	case schema.KindUnion:
		return "()"
	case schema.KindFixed:
		return "X"
	case schema.KindPrimitive:
		return "·"
	case schema.KindNamed:
		return t.theme.Sym.NamedRef
	default:
		return "?"
	}
}
