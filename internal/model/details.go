package model

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/onereallylongname/avredit/internal/projection"
	"github.com/onereallylongname/avredit/internal/schema"
	"github.com/onereallylongname/avredit/internal/theme"
)

// AttrKind describes the editing behavior of an attribute row.
type AttrKind int

const (
	AttrText      AttrKind = iota // simple text input
	AttrSelect                    // dropdown picker
	AttrMultiline                 // multiline text (ctrl+enter for newlines)
	AttrListItem                  // individual list item (editable, deletable)
	AttrListAdd                   // add row for a list
	AttrReadonly                  // non-editable display
)

// EditableAttr represents a single row in the details panel.
type EditableAttr struct {
	Key       string // display label
	Value     string // current value
	Scope     string // "native"
	NativeKey string // key in the native map
	Kind      AttrKind
	Options   []string // for AttrSelect
	ListKey   string   // for AttrListItem/AttrListAdd: parent list key
	ListIndex int      // for AttrListItem: index in the list
}

// IsEditable returns true if this attribute supports editing.
func (a *EditableAttr) IsEditable() bool {
	return a.Kind != AttrReadonly
}

// DetailsModel renders and edits attributes of the selected node.
type DetailsModel struct {
	proj   *projection.Projection
	theme  *theme.Theme
	node   *projection.Node
	width  int
	height int

	// Edit state
	attrs          []EditableAttr
	cursor         int
	editing        bool
	editingKey     bool // true when editing a custom attr key (rename)
	editor         Editor
	oldValue       string
	customStartIdx int // index where custom attrs begin in attrs slice
}

// NewDetailsModel creates an empty details panel.
func NewDetailsModel(proj *projection.Projection, th *theme.Theme) DetailsModel {
	dm := DetailsModel{proj: proj, theme: th}
	if proj != nil && proj.RootID != "" {
		if root, ok := proj.Nodes[proj.RootID]; ok {
			dm.node = root
			dm.rebuildAttrs()
		}
	}
	return dm
}

// SetNode updates the displayed node and rebuilds the attribute list.
func (d *DetailsModel) SetNode(node *projection.Node) {
	d.node = node
	d.editing = false
	d.editingKey = false
	d.rebuildAttrs()
	d.cursor = 0
}

// SetSize updates panel dimensions.
func (d *DetailsModel) SetSize(w, h int) {
	d.width = w
	d.height = h
}

// Editing returns true if an attribute is being edited.
func (d *DetailsModel) Editing() bool {
	return d.editing
}

// EditingKey returns true if a custom attr key (rename) is being edited.
func (d *DetailsModel) EditingKey() bool {
	return d.editingKey
}

// CursorAttr returns the attribute under the cursor.
func (d *DetailsModel) CursorAttr() *EditableAttr {
	if d.cursor >= 0 && d.cursor < len(d.attrs) {
		return &d.attrs[d.cursor]
	}
	return nil
}

// BeginEdit starts inline editing of the value under cursor.
func (d *DetailsModel) BeginEdit() bool {
	attr := d.CursorAttr()
	if attr == nil || !attr.IsEditable() {
		return false
	}
	d.editing = true
	d.editingKey = false
	d.oldValue = attr.Value
	style := EditorStyle{
		Text:   d.theme.DetailValue,
		Cursor: lipgloss.NewStyle().Background(d.theme.Primary).Foreground(lipgloss.Color("#1a1b26")),
	}
	d.editor = NewEditor(attr.Value, style)
	d.editor.SetWidth(d.width - 6)
	return true
}

// BeginEditKey starts inline editing of the key name (for custom attr rename).
func (d *DetailsModel) BeginEditKey() bool {
	attr := d.CursorAttr()
	if attr == nil || !d.IsCustomAttr() {
		return false
	}
	d.editing = true
	d.editingKey = true
	d.oldValue = attr.NativeKey
	style := EditorStyle{
		Text:   d.theme.DetailKey,
		Cursor: lipgloss.NewStyle().Background(d.theme.Primary).Foreground(lipgloss.Color("#1a1b26")),
	}
	d.editor = NewEditor(attr.NativeKey, style)
	d.editor.SetWidth(d.width - 6)
	return true
}

// CommitEdit finalises the edit, returning the attr, new value, and whether it changed.
func (d *DetailsModel) CommitEdit() (*EditableAttr, string, bool) {
	if !d.editing {
		return nil, "", false
	}
	d.editing = false
	newVal := d.editor.Value()
	attr := d.CursorAttr()
	if attr == nil {
		d.editingKey = false
		return nil, "", false
	}
	changed := newVal != d.oldValue
	if !d.editingKey {
		attr.Value = newVal
	}
	return attr, newVal, changed
}

// CancelEdit discards the edit.
func (d *DetailsModel) CancelEdit() {
	d.editing = false
	d.editingKey = false
}

// InsertNewline inserts a newline in the editor buffer (for multiline fields).
func (d *DetailsModel) InsertNewline() {
	if d.editing {
		d.editor.InsertRune('\n')
	}
}

// PasteText inserts s at the cursor of the active inline editor (used for
// both value edits and custom-attribute-key rename edits).
func (d *DetailsModel) PasteText(s string) {
	if d.editing {
		d.editor.InsertText(s)
	}
}

// IsCustomAttr returns true if the cursor is on a custom (non-standard) attribute.
func (d *DetailsModel) IsCustomAttr() bool {
	return d.cursor >= d.customStartIdx && d.customStartIdx < len(d.attrs)
}

// CursorNativeKey returns the native key of the attribute under cursor.
func (d *DetailsModel) CursorNativeKey() string {
	attr := d.CursorAttr()
	if attr == nil {
		return ""
	}
	return attr.NativeKey
}

// HandleKey processes keypresses in the details panel (navigation only).
func (d *DetailsModel) HandleKey(msg tea.KeyPressMsg) (handled bool) {
	if d.editing {
		switch msg.String() {
		case "enter":
			return true // caller should CommitEdit
		case "esc":
			d.CancelEdit()
			return true
		default:
			d.editor.HandleKey(msg)
			return true
		}
	}

	switch msg.String() {
	case "j", "down":
		if d.cursor < len(d.attrs)-1 {
			d.cursor++
		}
		return true
	case "k", "up":
		if d.cursor > 0 {
			d.cursor--
		}
		return true
	}
	return false
}

// rebuildAttrs reconstructs the attribute list from the node.
func (d *DetailsModel) rebuildAttrs() {
	d.attrs = nil
	if d.node == nil {
		return
	}
	d.attrs = append(d.attrs, d.collectStandardAttributes()...)
	d.customStartIdx = len(d.attrs)
	d.attrs = append(d.attrs, d.collectCustomAttributes()...)
}

// wrapWidth returns the usable content width for wrapping text, matching the
// inner width the bordered panel actually renders (panel width minus the two
// border columns), so wrapped lines never get silently truncated/ellipsized.
func (d DetailsModel) wrapWidth() int {
	w := d.width - 2
	if w < 1 {
		w = 1
	}
	return w
}

// View renders the details panel content (without border).
func (d DetailsModel) View() string {
	if d.node == nil {
		return "  No node selected"
	}

	var lines []string
	addLine := func(s string) { lines = append(lines, s) }

	wrapW := d.wrapWidth()

	// Kind badge
	kindLabel := fmt.Sprintf(" %s ", strings.ToUpper(string(d.node.Kind)))
	addLine(lipgloss.NewStyle().Foreground(d.theme.Dim).Italic(true).Render(kindLabel))
	addLine("")

	ks := d.theme.DetailKey
	vs := d.theme.DetailValue
	es := lipgloss.NewStyle().Foreground(d.theme.Muted).Italic(true)
	cs := lipgloss.NewStyle().Foreground(d.theme.Primary).Bold(true)

	// focusLine tracks the rendered row that should stay visible: either the
	// row under the navigation cursor, or (while editing) the row containing
	// the text-edit cursor itself.
	focusLine := -1

	// renderIndentedBlock appends the wrapped rows of `wrapped`, prefixing the
	// first row with `prefix` and aligning continuation rows beneath it. It
	// returns the absolute line index of the row containing the edit cursor.
	renderIndentedBlock := func(prefix string, wrapped []string, cursorRow int) int {
		indent := strings.Repeat(" ", lipgloss.Width(prefix))
		startIdx := len(lines)
		for i, wl := range wrapped {
			if i == 0 {
				addLine(prefix + wl)
			} else {
				addLine(indent + wl)
			}
		}
		return startIdx + cursorRow
	}

	for i, attr := range d.attrs {
		// Custom section header
		if i == d.customStartIdx && d.customStartIdx < len(d.attrs) {
			addLine("")
			addLine(d.theme.DetailTitle.Render(" Custom Attributes "))
			addLine("")
		}

		cur := i == d.cursor
		editing := d.editing && cur
		pfx := "  "
		if cur {
			pfx = cs.Render(d.theme.Sym.Cursor) + " "
		}
		if cur && focusLine < 0 {
			focusLine = len(lines)
		}

		switch attr.Kind {
		case AttrReadonly:
			label := ks.Render(pfx + attr.Key + ":")
			if attr.Value == "" {
				addLine(label)
			} else {
				addLine(label + " " + vs.Render(attr.Value))
			}

		case AttrSelect:
			label := ks.Render(pfx + attr.Key + ":")
			if attr.Value == "" {
				addLine(label + " " + es.Render("— "+d.theme.Sym.DetailExpand))
			} else {
				addLine(label + " " + vs.Render(attr.Value+" "+d.theme.Sym.DetailExpand))
			}

		case AttrListItem:
			indent := "    "
			if cur {
				indent = "  " + cs.Render(d.theme.Sym.Cursor) + " "
			}
			if editing {
				wrapped, cursorRow := d.editor.RenderWrapped(wrapW - lipgloss.Width(indent))
				focusLine = renderIndentedBlock(indent, wrapped, cursorRow)
			} else {
				addLine(indent + vs.Render("• "+attr.Value))
			}

		case AttrListAdd:
			indent := "    "
			if cur {
				indent = "  " + cs.Render(d.theme.Sym.Cursor) + " "
			}
			if editing {
				wrapped, cursorRow := d.editor.RenderWrapped(wrapW - lipgloss.Width(indent))
				focusLine = renderIndentedBlock(indent, wrapped, cursorRow)
			} else {
				addLine(indent + es.Render("+ add "+attr.ListKey))
			}

		default: // AttrText, AttrMultiline
			label := ks.Render(pfx + attr.Key + ":")
			if editing && d.editingKey {
				// Key rename is always short; keep single-line rendering.
				addLine(pfx + d.editor.View() + ": " + vs.Render(attr.Value))
				focusLine = len(lines) - 1
			} else if editing {
				prefix := label + " "
				wrapped, cursorRow := d.editor.RenderWrapped(wrapW - lipgloss.Width(prefix))
				focusLine = renderIndentedBlock(prefix, wrapped, cursorRow)
			} else if attr.Value == "" {
				addLine(label + " " + es.Render("—"))
			} else {
				prefix := label + " "
				wrapped := wrapPlainText(attr.Value, wrapW-lipgloss.Width(prefix))
				indent := strings.Repeat(" ", lipgloss.Width(prefix))
				for wi, wl := range wrapped {
					if wi == 0 {
						addLine(prefix + vs.Render(wl))
					} else {
						addLine(indent + vs.Render(wl))
					}
				}
			}
		}
	}

	return strings.Join(clipToCursor(lines, focusLine, d.height), "\n")
}

// wrapPlainText word-wraps s to fit within width, treating existing newlines
// as hard breaks (used to render static, non-editing text such as the Doc
// field without truncating or overflowing the panel).
func wrapPlainText(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	runes := []rune(s)
	start := 0
	var out []string
	emit := func(seg []rune) {
		for _, sp := range wrapRunes(seg, width) {
			out = append(out, string(seg[sp[0]:sp[1]]))
		}
	}
	for i, r := range runes {
		if r == '\n' {
			emit(runes[start:i])
			start = i + 1
		}
	}
	emit(runes[start:])
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// clipToCursor returns the window of lines that fits within the panel's
// available height, adjusted so that focusLine (if set) stays visible. If the
// content already fits, or height is unknown, all lines are returned
// unchanged so callers without a fixed size behave exactly as before.
func clipToCursor(lines []string, focusLine, panelHeight int) []string {
	innerH := panelHeight - 2 // border top/bottom
	if innerH <= 0 || len(lines) <= innerH {
		return lines
	}
	offset := 0
	if focusLine >= 0 {
		if focusLine >= innerH {
			offset = focusLine - innerH + 1
		}
		maxOffset := len(lines) - innerH
		if offset > maxOffset {
			offset = maxOffset
		}
		if offset < 0 {
			offset = 0
		}
	}
	end := offset + innerH
	if end > len(lines) {
		end = len(lines)
	}
	return lines[offset:end]
}

// collectStandardAttributes builds the standard attribute rows for the node kind.
func (d DetailsModel) collectStandardAttributes() []EditableAttr {
	node := d.node
	m := node.NativeMap()

	gs := func(key string) string {
		if m == nil {
			return ""
		}
		s, _ := m[key].(string)
		return s
	}

	fa := func(key string) string {
		if m == nil {
			return ""
		}
		v, ok := m[key]
		if !ok || v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}

	var attrs []EditableAttr

	// Helper: expand a list field into header + items + add row
	addList := func(label, nativeKey string) {
		attrs = append(attrs, EditableAttr{Key: label, Kind: AttrReadonly, Scope: "native", NativeKey: nativeKey})
		if m != nil {
			if sl, ok := m[nativeKey].([]any); ok {
				for i, v := range sl {
					s := fmt.Sprintf("%v", v)
					attrs = append(attrs, EditableAttr{
						Key: s, Value: s, Scope: "native", NativeKey: nativeKey,
						Kind: AttrListItem, ListKey: nativeKey, ListIndex: i,
					})
				}
			}
		}
		attrs = append(attrs, EditableAttr{Kind: AttrListAdd, Scope: "native", NativeKey: nativeKey, ListKey: nativeKey})
	}

	switch node.Kind {
	case schema.KindSchema, schema.KindRecord:
		attrs = append(attrs,
			EditableAttr{Key: "Name", Value: gs("name"), Scope: "native", NativeKey: "name", Kind: AttrText},
			EditableAttr{Key: "Namespace", Value: gs("namespace"), Scope: "native", NativeKey: "namespace", Kind: AttrText},
			EditableAttr{Key: "Doc", Value: gs("doc"), Scope: "native", NativeKey: "doc", Kind: AttrMultiline},
		)
		addList("Aliases", "aliases")

	case schema.KindField:
		typeVal := d.fieldTypeLabel()
		allTypes := append(schema.PrimitiveTypes, schema.ComplexTypes...)
		allTypes = append(allTypes, d.proj.NamedTypes()...) // named-type-suggestions
		attrs = append(attrs,
			EditableAttr{Key: "Name", Value: gs("name"), Scope: "native", NativeKey: "name", Kind: AttrText},
			EditableAttr{Key: "Type", Value: typeVal, Scope: "native", NativeKey: "__type__", Kind: AttrSelect,
				Options: allTypes},
			EditableAttr{Key: "Doc", Value: gs("doc"), Scope: "native", NativeKey: "doc", Kind: AttrMultiline},
			EditableAttr{Key: "Default", Value: fa("default"), Scope: "native", NativeKey: "default", Kind: AttrText},
			EditableAttr{Key: "Order", Value: gs("order"), Scope: "native", NativeKey: "order", Kind: AttrSelect,
				Options: []string{"(none)", "ascending", "descending", "ignore"}},
		)
		addList("Aliases", "aliases")

	case schema.KindEnum:
		attrs = append(attrs,
			EditableAttr{Key: "Name", Value: gs("name"), Scope: "native", NativeKey: "name", Kind: AttrText},
			EditableAttr{Key: "Namespace", Value: gs("namespace"), Scope: "native", NativeKey: "namespace", Kind: AttrText},
			EditableAttr{Key: "Doc", Value: gs("doc"), Scope: "native", NativeKey: "doc", Kind: AttrMultiline},
		)
		addList("Symbols", "symbols")
		// Default: select from current symbols
		symbolOpts := []string{"(none)"}
		if m != nil {
			if sl, ok := m["symbols"].([]any); ok {
				for _, v := range sl {
					if s, ok := v.(string); ok {
						symbolOpts = append(symbolOpts, s)
					}
				}
			}
		}
		attrs = append(attrs,
			EditableAttr{Key: "Default", Value: fa("default"), Scope: "native", NativeKey: "default", Kind: AttrSelect,
				Options: symbolOpts},
		)
		addList("Aliases", "aliases")

	case schema.KindFixed:
		attrs = append(attrs,
			EditableAttr{Key: "Name", Value: gs("name"), Scope: "native", NativeKey: "name", Kind: AttrText},
			EditableAttr{Key: "Namespace", Value: gs("namespace"), Scope: "native", NativeKey: "namespace", Kind: AttrText},
			EditableAttr{Key: "Size", Value: fa("size"), Scope: "native", NativeKey: "size", Kind: AttrText},
			EditableAttr{Key: "Doc", Value: gs("doc"), Scope: "native", NativeKey: "doc", Kind: AttrMultiline},
		)
		addList("Aliases", "aliases")

	case schema.KindPrimitive:
		// Determine base type regardless of string or map form
		baseType := ""
		if m != nil {
			baseType, _ = m["type"].(string)
		} else if s := node.NativeString(); s != "" {
			baseType = s
		}

		// Show LogicalType picker if the base type supports it
		if logOpts, ok := schema.LogicalTypes[baseType]; ok {
			opts := []string{"(none)"}
			for _, lt := range logOpts {
				if lt != "" {
					opts = append(opts, lt)
				}
			}
			attrs = append(attrs, EditableAttr{
				Key: "LogicalType", Value: gs("logicalType"), Scope: "native", NativeKey: "logicalType",
				Kind: AttrSelect, Options: opts,
			})
		}

		// Show Precision/Scale only when logicalType is "decimal"
		currentLT := gs("logicalType")
		if currentLT == "decimal" || currentLT == "big-decimal" {
			attrs = append(attrs,
				EditableAttr{Key: "Precision", Value: fa("precision"), Scope: "native", NativeKey: "precision", Kind: AttrText},
				EditableAttr{Key: "Scale", Value: fa("scale"), Scope: "native", NativeKey: "scale", Kind: AttrText},
			)
		}

		// If no map and no logical type options, just show the type as readonly
		if m == nil && baseType != "" {
			if _, ok := schema.LogicalTypes[baseType]; !ok {
				attrs = append(attrs, EditableAttr{Key: "Type", Value: baseType, Kind: AttrReadonly})
			}
		}

	case schema.KindArray:
		allTypes := append(schema.PrimitiveTypes, schema.ComplexTypes...)
		allTypes = append(allTypes, d.proj.NamedTypes()...) // named-type-suggestions
		attrs = append(attrs,
			EditableAttr{Key: "Items Type", Value: d.fieldTypeLabel(), Scope: "native", NativeKey: "__type__", Kind: AttrSelect,
				Options: allTypes},
			EditableAttr{Key: "Default", Value: fa("default"), Scope: "native", NativeKey: "default", Kind: AttrText},
		)

	case schema.KindMap:
		allTypes := append(schema.PrimitiveTypes, schema.ComplexTypes...)
		allTypes = append(allTypes, d.proj.NamedTypes()...) // named-type-suggestions
		attrs = append(attrs,
			EditableAttr{Key: "Values Type", Value: d.fieldTypeLabel(), Scope: "native", NativeKey: "__type__", Kind: AttrSelect,
				Options: allTypes},
			EditableAttr{Key: "Default", Value: fa("default"), Scope: "native", NativeKey: "default", Kind: AttrText},
		)

	case schema.KindNamed:
		if s := node.NativeString(); s != "" {
			attrs = append(attrs, EditableAttr{Key: "Reference", Value: s, Kind: AttrReadonly})
		}
	}

	return attrs
}

// collectCustomAttributes returns non-standard attributes from the native map.
func (d DetailsModel) collectCustomAttributes() []EditableAttr {
	m := d.node.NativeMap()
	if m == nil {
		return nil
	}

	standardKeys := buildStandardSet(d.node.Kind)
	var attrs []EditableAttr

	keys := make([]string, 0, len(m))
	for k := range m {
		if !standardKeys[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, k := range keys {
		attrs = append(attrs, EditableAttr{
			Key: k, Value: fmt.Sprintf("%v", m[k]),
			Scope: "native", NativeKey: k,
			Kind: AttrText,
		})
	}
	return attrs
}

// buildStandardSet creates a lookup set for standard keys of a node kind.
func buildStandardSet(kind schema.NodeKind) map[string]bool {
	std := schema.StandardNativeKeys[kind]
	set := make(map[string]bool, len(std))
	for _, k := range std {
		set[k] = true
	}
	return set
}

// fieldTypeLabel derives the type label for a field node from its child.
func (d DetailsModel) fieldTypeLabel() string {
	if d.node == nil || len(d.node.Children) == 0 {
		return "?"
	}
	child := d.proj.Nodes[d.node.Children[0]]
	if child == nil {
		return "?"
	}
	return typeNodeLabel(d.proj, child)
}

// typeNodeLabel returns a displayable type name for a type node.
func typeNodeLabel(proj *projection.Projection, node *projection.Node) string {
	switch node.Kind {
	case schema.KindPrimitive:
		if s := node.NativeString(); s != "" {
			return s
		}
		if m := node.NativeMap(); m != nil {
			if tp, ok := m["type"].(string); ok {
				return tp
			}
		}
		return "primitive"
	case schema.KindRecord:
		if name := node.Name(); name != "" {
			return name
		}
		return "record"
	case schema.KindEnum:
		if name := node.Name(); name != "" {
			return name
		}
		return "enum"
	case schema.KindFixed:
		return "fixed"
	case schema.KindArray:
		return "array"
	case schema.KindMap:
		return "map"
	case schema.KindUnion:
		parts := make([]string, 0, len(node.Children))
		for _, cid := range node.Children {
			c := proj.Nodes[cid]
			if c != nil {
				parts = append(parts, typeNodeLabel(proj, c))
			}
		}
		return strings.Join(parts, "|")
	case schema.KindNamed:
		if s := node.NativeString(); s != "" {
			return s
		}
		return "named"
	default:
		return "?"
	}
}
