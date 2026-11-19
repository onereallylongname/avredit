package model

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/onereallylongname/avredit/internal/theme"
)

// KeyBinding describes a single key binding.
type KeyBinding struct {
	Key  string
	Desc string
}

// KeySection groups related keybindings under a heading.
type KeySection struct {
	Title    string
	Bindings []KeyBinding
}

// Keybindings returns all keybinding sections for display in help/CLI.
func Keybindings() []KeySection {
	return []KeySection{
		{
			Title: "Global",
			Bindings: []KeyBinding{
				{"q", "Quit"},
				{"?", "Show help"},
				{"tab", "Cycle panel focus"},
				{"1/2/3", "Focus explorer/tree/details"},
				{"ctrl+e", "Toggle file explorer"},
				{"ctrl+g", "Switch explorer to/from Schema Registry"},
				{"esc", "Cancel / clear highlights"},
				{"u", "Undo"},
				{"ctrl+r", "Redo"},
			},
		},
		{
			Title: "Tree (Normal)",
			Bindings: []KeyBinding{
				{"j/k", "Navigate up/down"},
				{"h/l", "Collapse/expand node"},
				{"g/G", "Jump to top/bottom"},
				{"space", "Toggle expand"},
				{"enter", "Focus details pane"},
				{"a", "Add field"},
				{"d", "Delete node"},
				{"c", "Copy node"},
				{"m", "Move field to record"},
				{"yy", "Yank (copy) field name to clipboard"},
				{"/", "Search"},
			},
		},
		{
			Title: "Explorer",
			Bindings: []KeyBinding{
				{"j/k", "Navigate up/down"},
				{"h/l", "Collapse/expand dir"},
				{"enter/space", "Open file / toggle dir"},
				{"g/G", "Jump to top/bottom"},
				{"backspace/-", "Navigate to parent dir"},
				{".", "Set selected dir as root"},
				{"/", "Search files (recursive)"},
			},
		},
		{
			Title: "Details (Normal)",
			Bindings: []KeyBinding{
				{"j/k", "Navigate attributes"},
				{"enter/e", "Edit attribute"},
				{"a", "Add custom attribute"},
				{"d", "Delete attribute / list item"},
				{"R", "Rename custom attr key"},
				{"yp", "Yank (copy) attribute name"},
				{"yy", "Yank (copy) attribute value"},
				{"ya", "Yank (copy) \"name: value\""},
				{"esc", "Return to tree pane"},
			},
		},
		{
			Title: "Edit Mode",
			Bindings: []KeyBinding{
				{"enter", "Confirm edit"},
				{"esc", "Cancel edit"},
				{"shift+enter", "Newline (multiline fields; needs terminal support)"},
				{"ctrl+j / alt+enter", "Newline (multiline fields; universal fallback)"},
				{"ctrl+v / shift+insert", "Paste from clipboard"},
			},
		},
		{
			Title: "Search",
			Bindings: []KeyBinding{
				{"/", "Search (contextual: files or schema)"},
				{"enter", "Jump to match"},
				{"n/N", "Next/prev match"},
				{"esc", "Close search"},
				{"n:foo", "Filter by name"},
				{"t:foo", "Filter by type"},
				{"p:foo", "Filter by parent"},
				{"ns:foo", "Filter by namespace"},
				{"a:foo", "Filter by alias"},
				{"d:foo", "Filter by doc text"},
				{"ctrl+v / shift+insert", "Paste from clipboard"},
			},
		},
		{
			Title: "Command",
			Bindings: []KeyBinding{
				{":/Ctrl+P", "Open command line"},
				{"↑/↓", "Command history"},
				{":w [file]", "Save (optionally to file)"},
				{":q", "Quit (confirms if unsaved)"},
				{":q!", "Force quit (discard changes)"},
				{":wq", "Save and quit"},
				{":export [file]", "Export .avsc"},
				{":theme <name>", "Switch theme"},
				{":notifications", "Show notification log"},
				{":help", "Show help"},
				{":options", "Show the right-click menu"},
				{"tab", "Autocomplete command"},
				{"ctrl+v / shift+insert", "Paste from clipboard"},
			},
		},
		{
			Title: "Schema Registry (Confluent Cloud / Confluent-compatible)",
			Bindings: []KeyBinding{
				{"ctrl+g", "Switch explorer between Files and Registry"},
				{"j/k/enter/space", "Navigate subjects/versions, fetch a version"},
				{"/", "Search fetched subjects"},
				{":registry check [subject]", "Compatibility check vs latest registered version (opens a report if incompatible)"},
				{":registry report", "Reopen the last compatibility check report"},
				{"s (in report)", "Save the report to disk (timestamped file)"},
			},
		},
	}
}

// KeybindingsText returns a plain-text representation for CLI help output.
func KeybindingsText() string {
	var sb strings.Builder
	for _, sec := range Keybindings() {
		sb.WriteString("  " + sec.Title + ":\n")
		for _, b := range sec.Bindings {
			sb.WriteString("    " + padRight(b.Key, 14) + b.Desc + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// helpKeyWidth is the column width reserved for key names in the help view.
const helpKeyWidth = 14

// HelpView renders a styled help overlay for the TUI, wrapping descriptions
// so no line is wider than width.
func HelpView(th *theme.Theme, width int) string {
	titleStyle := lipgloss.NewStyle().Foreground(th.Primary).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(th.Warning).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(th.Fg)
	dimStyle := lipgloss.NewStyle().Foreground(th.Muted).Italic(true)

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("  Keybindings"))
	sb.WriteString("\n\n")

	for _, sec := range Keybindings() {
		sb.WriteString(dimStyle.Render("  ─ " + sec.Title + " ─"))
		sb.WriteString("\n")
		for _, b := range sec.Bindings {
			descW := width - 2 - helpKeyWidth - 1
			if descW < 10 {
				descW = 10
			}
			descLines := strings.Split(ansi.Wrap(b.Desc, descW, ""), "\n")
			sb.WriteString("  " + keyStyle.Render(padRight(b.Key, helpKeyWidth)) + descStyle.Render(descLines[0]) + "\n")
			for _, extra := range descLines[1:] {
				sb.WriteString(strings.Repeat(" ", 2+helpKeyWidth+1) + descStyle.Render(extra) + "\n")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func padRight(s string, width int) string {
	if len(s) >= width {
		// Still guarantee a separating space so the description doesn't run
		// into a key that exceeds the padding width.
		return s + " "
	}
	return s + strings.Repeat(" ", width-len(s))
}
