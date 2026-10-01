// Package theme defines the visual theme system for avredit.
// Themes are collections of lipgloss styles used by all UI components.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Symbols holds configurable Unicode characters used throughout the UI.
// All fields are optional in theme JSON — omitted values use defaults.
type Symbols struct {
	Expanded     string // Tree expanded node (default: "▼")
	Collapsed    string // Tree collapsed node (default: "▶")
	Leaf         string // Tree leaf node (default: " ")
	NamedRef     string // Named type reference badge (default: "→")
	Cursor       string // Active item cursor in pickers/details (default: "▸")
	DetailExpand string // Details expanded section (default: "▾")
	Error        string // Error severity icon (default: "✗")
	Warning      string // Warning severity icon (default: "⚠")
	Success      string // Success/info icon (default: "✓")
	ScrollUp     string // Scroll up hint (default: "↑more")
	ScrollDown   string // Scroll down hint (default: "↓more")
}

// DefaultSymbols provides the standard symbol set.
var DefaultSymbols = Symbols{
	Expanded:     "▼",
	Collapsed:    "▶",
	Leaf:         " ",
	NamedRef:     "→",
	Cursor:       "▸",
	DetailExpand: "▾",
	Error:        "✗",
	Warning:      "⚠",
	Success:      "✓",
	ScrollUp:     "↑more",
	ScrollDown:   "↓more",
}

// Theme holds all lipgloss styles used across the UI.
type Theme struct {
	Name string

	// Configurable symbols
	Sym Symbols

	// Base colors
	Fg    color.Color
	Bg    color.Color
	Dim   color.Color
	Muted color.Color

	// Accent colors
	Primary   color.Color
	Secondary color.Color
	Success   color.Color
	Warning   color.Color
	Error     color.Color

	// Panel styles
	TreeBorder    lipgloss.Style
	DetailBorder  lipgloss.Style
	FocusedBorder lipgloss.Style

	// Tree elements
	TreeNode       lipgloss.Style
	TreeNodeActive lipgloss.Style
	TreeIndent     lipgloss.Style
	TreeExpander   lipgloss.Style

	// Node kind badges
	KindRecord lipgloss.Style
	KindField  lipgloss.Style
	KindEnum   lipgloss.Style
	KindArray  lipgloss.Style
	KindMap    lipgloss.Style
	KindUnion  lipgloss.Style
	KindFixed  lipgloss.Style
	KindPrim   lipgloss.Style
	KindNamed  lipgloss.Style

	// Details panel
	DetailKey   lipgloss.Style
	DetailValue lipgloss.Style
	DetailTitle lipgloss.Style

	// Status bar
	StatusBar     lipgloss.Style
	StatusMode    lipgloss.Style
	StatusFile    lipgloss.Style
	StatusStats   lipgloss.Style
	StatusHelp    lipgloss.Style
	StatusNormal  lipgloss.Style
	StatusEdit    lipgloss.Style
	StatusSearch  lipgloss.Style
	StatusCommand lipgloss.Style
}
