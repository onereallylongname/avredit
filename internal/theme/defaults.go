package theme

import (
	"charm.land/lipgloss/v2"
)

// Dark returns the default dark theme inspired by lazygit's dark palette.
func Dark() *Theme {
	fg := lipgloss.Color("#c0caf5")
	bg := lipgloss.Color("#1a1b26")
	dim := lipgloss.Color("#888ea8")
	muted := lipgloss.Color("#626a8a")

	primary := lipgloss.Color("#7aa2f7")
	secondary := lipgloss.Color("#bb9af7")
	success := lipgloss.Color("#9ece6a")
	warning := lipgloss.Color("#e0af68")
	errColor := lipgloss.Color("#f7768e")

	borderStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(muted)

	focusedBorderStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(primary)

	return &Theme{
		Name: "dark",
		Sym:  DefaultSymbols,

		Fg:    fg,
		Bg:    bg,
		Dim:   dim,
		Muted: muted,

		Primary:   primary,
		Secondary: secondary,
		Success:   success,
		Warning:   warning,
		Error:     errColor,

		TreeBorder:    borderStyle,
		DetailBorder:  borderStyle,
		FocusedBorder: focusedBorderStyle,

		TreeNode: lipgloss.NewStyle().Foreground(fg),
		TreeNodeActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1a1b26")).
			Background(primary).
			Bold(true),
		TreeIndent:   lipgloss.NewStyle().Foreground(dim),
		TreeExpander: lipgloss.NewStyle().Foreground(dim),

		KindRecord: lipgloss.NewStyle().Foreground(primary).Bold(true),
		KindField:  lipgloss.NewStyle().Foreground(success),
		KindEnum:   lipgloss.NewStyle().Foreground(secondary),
		KindArray:  lipgloss.NewStyle().Foreground(warning),
		KindMap:    lipgloss.NewStyle().Foreground(lipgloss.Color("#73daca")),
		KindUnion:  lipgloss.NewStyle().Foreground(lipgloss.Color("#ff9e64")),
		KindFixed:  lipgloss.NewStyle().Foreground(lipgloss.Color("#2ac3de")),
		KindPrim:   lipgloss.NewStyle().Foreground(dim),
		KindNamed:  lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7")),

		DetailKey:   lipgloss.NewStyle().Foreground(primary).Bold(true),
		DetailValue: lipgloss.NewStyle().Foreground(fg),
		DetailTitle: lipgloss.NewStyle().Foreground(secondary).Bold(true).Underline(true),

		StatusBar: lipgloss.NewStyle().
			Foreground(fg).
			Background(lipgloss.Color("#24283b")),
		StatusMode: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1a1b26")).
			Bold(true).
			Padding(0, 1),
		StatusFile:  lipgloss.NewStyle().Foreground(fg).Padding(0, 1),
		StatusStats: lipgloss.NewStyle().Foreground(dim).Padding(0, 1),
		StatusHelp:  lipgloss.NewStyle().Foreground(muted).Padding(0, 1),

		StatusNormal:  lipgloss.NewStyle().Background(primary).Foreground(lipgloss.Color("#1a1b26")).Bold(true).Padding(0, 1),
		StatusEdit:    lipgloss.NewStyle().Background(success).Foreground(lipgloss.Color("#1a1b26")).Bold(true).Padding(0, 1),
		StatusSearch:  lipgloss.NewStyle().Background(warning).Foreground(lipgloss.Color("#1a1b26")).Bold(true).Padding(0, 1),
		StatusCommand: lipgloss.NewStyle().Background(secondary).Foreground(lipgloss.Color("#1a1b26")).Bold(true).Padding(0, 1),
	}
}
