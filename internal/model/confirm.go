package model

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/onereallylongname/avredit/internal/theme"
)

// Confirm is a simple yes/no confirmation dialog.
type Confirm struct {
	message string
	active  bool
	theme   *theme.Theme
}

// NewConfirm creates a confirmation dialog.
func NewConfirm(message string, th *theme.Theme) Confirm {
	return Confirm{
		message: message,
		active:  true,
		theme:   th,
	}
}

// Active returns whether the dialog is shown.
func (c *Confirm) Active() bool {
	return c.active
}

// HandleKey processes a keypress. Returns (confirmed, done).
func (c *Confirm) HandleKey(msg tea.KeyPressMsg) (bool, bool) {
	switch msg.String() {
	case "y", "Y":
		c.active = false
		return true, true
	case "n", "N", "esc", "ctrl+c":
		c.active = false
		return false, true
	}
	return false, false
}

// View renders the confirmation dialog.
func (c Confirm) View() string {
	msgStyle := lipgloss.NewStyle().
		Foreground(c.theme.Warning).
		Bold(true)
	hintStyle := lipgloss.NewStyle().
		Foreground(c.theme.Dim)

	return msgStyle.Render(c.message) + "\n" +
		hintStyle.Render("  [y]es / [n]o")
}
