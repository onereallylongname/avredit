package model

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Editor is a lightweight inline text editor for single-line values.
type Editor struct {
	value  []rune
	cursor int
	active bool
	width  int
	style  EditorStyle
}

// EditorStyle holds the visual styles for the editor.
type EditorStyle struct {
	Text        lipgloss.Style
	Cursor      lipgloss.Style
	Placeholder lipgloss.Style
}

// NewEditor creates an editor with the given initial value.
func NewEditor(initial string, style EditorStyle) Editor {
	runes := []rune(initial)
	return Editor{
		value:  runes,
		cursor: len(runes),
		active: true,
		width:  40,
		style:  style,
	}
}

// Value returns the current text content.
func (e *Editor) Value() string {
	return string(e.value)
}

// SetValue replaces the editor content and moves cursor to end.
func (e *Editor) SetValue(s string) {
	e.value = []rune(s)
	e.cursor = len(e.value)
}

// InsertRune inserts a single rune at the cursor position.
func (e *Editor) InsertRune(ch rune) {
	e.value = append(e.value[:e.cursor], append([]rune{ch}, e.value[e.cursor:]...)...)
	e.cursor++
}

// InsertText inserts a string (e.g. pasted clipboard text) at the cursor
// position. Embedded newlines are preserved as literal '\n' runes, same as
// a single InsertRune('\n'), so pasted multi-line text works in multiline
// attributes (see RenderWrapped).
func (e *Editor) InsertText(s string) {
	if s == "" {
		return
	}
	runes := []rune(s)
	e.value = append(e.value[:e.cursor:e.cursor], append(runes, e.value[e.cursor:]...)...)
	e.cursor += len(runes)
}

// SetWidth sets the visible width.
func (e *Editor) SetWidth(w int) {
	e.width = w
}

// Active returns whether the editor is in editing state.
func (e *Editor) Active() bool {
	return e.active
}

// Activate enables editing.
func (e *Editor) Activate() {
	e.active = true
}

// Deactivate disables editing.
func (e *Editor) Deactivate() {
	e.active = false
}

// HandleKey processes a keypress. Returns true if the key was consumed.
func (e *Editor) HandleKey(msg tea.KeyPressMsg) bool {
	if !e.active {
		return false
	}

	switch msg.String() {
	case "left":
		if e.cursor > 0 {
			e.cursor--
		}
	case "right":
		if e.cursor < len(e.value) {
			e.cursor++
		}
	case "home", "ctrl+a":
		e.cursor = 0
	case "end", "ctrl+e":
		e.cursor = len(e.value)
	case "backspace":
		if e.cursor > 0 {
			e.value = append(e.value[:e.cursor-1], e.value[e.cursor:]...)
			e.cursor--
		}
	case "delete":
		if e.cursor < len(e.value) {
			e.value = append(e.value[:e.cursor], e.value[e.cursor+1:]...)
		}
	case "ctrl+u":
		e.value = e.value[e.cursor:]
		e.cursor = 0
	case "ctrl+k":
		e.value = e.value[:e.cursor]
	case "ctrl+w":
		// Delete word backward
		if e.cursor > 0 {
			pos := e.cursor - 1
			for pos > 0 && e.value[pos] == ' ' {
				pos--
			}
			for pos > 0 && e.value[pos-1] != ' ' {
				pos--
			}
			e.value = append(e.value[:pos], e.value[e.cursor:]...)
			e.cursor = pos
		}
	default:
		// Insert printable character
		if !isControlKey(msg) {
			var ch rune
			if msg.Text != "" {
				// Use Text field (actual typed character with correct case)
				for _, r := range msg.Text {
					e.value = append(e.value[:e.cursor], append([]rune{r}, e.value[e.cursor:]...)...)
					e.cursor++
				}
				return true
			} else if msg.Code > 0 && msg.Code < 128 {
				ch = msg.Code
			}
			if ch > 0 {
				e.value = append(e.value[:e.cursor], append([]rune{ch}, e.value[e.cursor:]...)...)
				e.cursor++
			}
		}
	}
	return true
}

// View renders the editor content with a visible cursor.
func (e Editor) View() string {
	if !e.active {
		return e.style.Text.Render(string(e.value))
	}

	var sb strings.Builder
	text := e.value
	cursorPos := e.cursor

	// Render text before cursor
	if cursorPos > 0 {
		sb.WriteString(e.style.Text.Render(string(text[:cursorPos])))
	}

	// Render cursor character
	if cursorPos < len(text) {
		ch := text[cursorPos]
		cursorDisplay := string(ch)
		if ch == '\n' {
			cursorDisplay = "↵\n"
		}
		sb.WriteString(e.style.Cursor.Render(cursorDisplay))
		// Render text after cursor
		if cursorPos+1 < len(text) {
			sb.WriteString(e.style.Text.Render(string(text[cursorPos+1:])))
		}
	} else {
		// Cursor at end — show block cursor on space
		sb.WriteString(e.style.Cursor.Render(" "))
	}

	return sb.String()
}

// RenderWrapped renders the editor content word-wrapped to fit within the given
// width, returning one rendered string per visual row plus the index of the row
// that contains the cursor. Existing newlines (from ctrl+enter) are treated as
// hard breaks; long lines are soft-wrapped at word boundaries (falling back to a
// hard character break when a single word exceeds the width). This guarantees the
// cursor is always present on one of the returned rows, so callers can keep it
// visible without truncating or losing content.
func (e Editor) RenderWrapped(width int) (lines []string, cursorRow int) {
	if width < 1 {
		width = 1
	}
	value := e.value
	cursor := e.cursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(value) {
		cursor = len(value)
	}

	// Split into hard lines on real '\n' characters. Ranges are [s, e] and
	// partition [0, len(value)] with no gaps, so every cursor offset maps to
	// exactly one hard line (see loop below).
	type hardLine struct{ s, e int }
	var hardLines []hardLine
	start := 0
	for i, r := range value {
		if r == '\n' {
			hardLines = append(hardLines, hardLine{start, i})
			start = i + 1
		}
	}
	hardLines = append(hardLines, hardLine{start, len(value)})

	for _, hl := range hardLines {
		seg := value[hl.s:hl.e]
		spans := wrapRunes(seg, width)

		localCursor := -1
		hasCursor := cursor >= hl.s && cursor <= hl.e
		if hasCursor {
			localCursor = cursor - hl.s
		}

		for si, sp := range spans {
			spanCursor := -1
			if hasCursor {
				if localCursor >= sp[0] && localCursor < sp[1] {
					spanCursor = localCursor - sp[0]
				} else if si == len(spans)-1 && localCursor == len(seg) {
					// Cursor sits at the very end of this hard line's text.
					spanCursor = localCursor - sp[0]
				}
			}
			if spanCursor >= 0 {
				cursorRow = len(lines)
			}
			lines = append(lines, e.renderCursorSpan(seg[sp[0]:sp[1]], spanCursor))
		}
	}
	return lines, cursorRow
}

// renderCursorSpan renders a single visual line of runes, highlighting the rune
// at localCursor (if >= 0) with the cursor style. localCursor == len(seg) means
// the cursor sits past the last rune (rendered as a trailing block cursor).
func (e Editor) renderCursorSpan(seg []rune, localCursor int) string {
	if !e.active || localCursor < 0 {
		return e.style.Text.Render(string(seg))
	}

	var sb strings.Builder
	if localCursor > 0 {
		sb.WriteString(e.style.Text.Render(string(seg[:localCursor])))
	}
	if localCursor < len(seg) {
		sb.WriteString(e.style.Cursor.Render(string(seg[localCursor])))
		if localCursor+1 < len(seg) {
			sb.WriteString(e.style.Text.Render(string(seg[localCursor+1:])))
		}
	} else {
		sb.WriteString(e.style.Cursor.Render(" "))
	}
	return sb.String()
}

// wrapRunes computes word-wrap spans for a single line of runes (no embedded
// newlines), each span being a [start, end) rune-index pair. Spans partition
// the input exactly (no runes are dropped), preferring to break at spaces;
// a word longer than width is hard-broken. An empty input yields one empty span
// so blank lines are preserved.
func wrapRunes(line []rune, width int) [][2]int {
	if width < 1 {
		width = 1
	}
	n := len(line)
	if n == 0 {
		return [][2]int{{0, 0}}
	}

	var spans [][2]int
	start := 0
	for start < n {
		end := start + width
		if end >= n {
			spans = append(spans, [2]int{start, n})
			break
		}
		breakAt := -1
		for i := end; i > start; i-- {
			if line[i] == ' ' {
				breakAt = i
				break
			}
		}
		if breakAt == -1 {
			// No space to break on; hard break at width.
			spans = append(spans, [2]int{start, end})
			start = end
		} else {
			// Include the space itself on this line so offsets stay exact.
			spans = append(spans, [2]int{start, breakAt + 1})
			start = breakAt + 1
		}
	}
	return spans
}

// isControlKey returns true if the keypress is a non-printable control sequence.
func isControlKey(msg tea.KeyPressMsg) bool {
	s := msg.String()
	if len(s) == 0 {
		return true
	}
	// Known control prefixes
	if strings.HasPrefix(s, "ctrl+") || strings.HasPrefix(s, "alt+") {
		return true
	}
	// Named keys that aren't printable
	switch s {
	case "enter", "tab", "esc", "up", "down", "left", "right",
		"home", "end", "pgup", "pgdown", "insert", "delete",
		"backspace", "f1", "f2", "f3", "f4", "f5", "f6",
		"f7", "f8", "f9", "f10", "f11", "f12":
		return true
	}
	return false
}
