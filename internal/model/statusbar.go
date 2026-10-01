package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/onereallylongname/avredit/internal/theme"
)

// StatusBarModel renders the bottom status bar.
type StatusBarModel struct {
	theme     *theme.Theme
	mode      AppMode
	filePath  string
	nodePath  string
	stats     string
	width     int
	undoCount int
	redoCount int
}

// NewStatusBarModel creates a status bar.
func NewStatusBarModel(th *theme.Theme, filePath string) StatusBarModel {
	return StatusBarModel{
		theme:    th,
		filePath: filePath,
		mode:     ModeNormal,
	}
}

// SetMode updates the displayed mode.
func (s *StatusBarModel) SetMode(mode AppMode) {
	s.mode = mode
}

// SetStats updates the schema statistics text.
func (s *StatusBarModel) SetStats(stats string) {
	s.stats = stats
}

// SetNodePath updates the displayed node path.
func (s *StatusBarModel) SetNodePath(path string) {
	s.nodePath = path
}

// SetWidth updates the bar width.
func (s *StatusBarModel) SetWidth(w int) {
	s.width = w
}

// SetFile updates the displayed file path.
func (s *StatusBarModel) SetFile(path string) {
	s.filePath = path
}

// SetUndo updates the undo/redo stack counts.
func (s *StatusBarModel) SetUndo(undoCount, redoCount int) {
	s.undoCount = undoCount
	s.redoCount = redoCount
}

// View renders the status bar with responsive segment sizing.
// Layout: [MODE] [file] [node path]          [undo] [stats] [help]
// Budget allocation: file gets 40%, node path gets 60%.
// File truncates from left ("…filename.avsc").
// Path truncates from left keeping deepest segments ("…› parent › child").
// Note: path separator is " › " (unicode), matching formatNodePath() in app.go.
func (s StatusBarModel) View() string {
	// Mode badge (always shown)
	var modeStyle lipgloss.Style
	switch s.mode {
	case ModeNormal:
		modeStyle = s.theme.StatusNormal
	case ModeEdit:
		modeStyle = s.theme.StatusEdit
	case ModeSearch:
		modeStyle = s.theme.StatusSearch
	case ModeCommand:
		modeStyle = s.theme.StatusCommand
	}
	modeBadge := modeStyle.Render(s.mode.String())
	modeW := lipgloss.Width(modeBadge)

	// Build right side (fixed elements)
	var rightParts []string
	if s.undoCount > 0 || s.redoCount > 0 {
		rightParts = append(rightParts, s.theme.StatusStats.Render(fmt.Sprintf("[↶%d ↷%d]", s.undoCount, s.redoCount)))
	}
	if s.stats != "" {
		rightParts = append(rightParts, s.theme.StatusStats.Render(s.stats))
	}
	rightParts = append(rightParts, s.theme.StatusHelp.Render("q:quit ?:help"))
	right := strings.Join(rightParts, " ")
	rightW := lipgloss.Width(right)

	// Budget for file + path combined
	budget := s.width - modeW - rightW - 4 // spaces between

	// Truncate file and path to share the budget equally
	file := s.filePath
	path := s.nodePath

	if budget < 10 {
		// Ultra narrow: drop help, recalculate
		if len(rightParts) > 1 {
			rightParts = rightParts[:len(rightParts)-1]
			right = strings.Join(rightParts, " ")
			rightW = lipgloss.Width(right)
			budget = s.width - modeW - rightW - 4
		}
	}

	// Allocate budget: file gets up to 40%, path gets the rest
	fileBudget := budget * 2 / 5
	if fileBudget < 8 {
		fileBudget = 8
	}
	pathBudget := budget - fileBudget - 1

	// Truncate file from left (show end)
	if len(file) > fileBudget {
		file = "…" + file[len(file)-fileBudget+1:]
	}

	// Truncate path from left (show deepest/rightmost segments)
	var pathStr string
	if path != "" && pathBudget > 4 {
		if len(path) <= pathBudget {
			pathStr = path
		} else {
			parts := strings.Split(path, " › ")
			pathStr = ""
			for i := len(parts) - 1; i >= 0; i-- {
				candidate := strings.Join(parts[i:], " › ")
				if i > 0 {
					candidate = "…› " + candidate
				}
				if len(candidate) <= pathBudget {
					pathStr = candidate
				} else {
					break
				}
			}
			if pathStr == "" && len(parts) > 0 {
				last := parts[len(parts)-1]
				if len(last)+3 <= pathBudget {
					pathStr = "…› " + last
				} else {
					pathStr = last[:pathBudget-1] + "…"
				}
			}
		}
	}

	// Compose left section
	left := modeBadge + " " + s.theme.StatusFile.Render(file)
	if pathStr != "" {
		left += " " + s.theme.StatusHelp.Render(pathStr)
	}

	// Fill gap
	leftWidth := lipgloss.Width(left)
	gap := s.width - leftWidth - rightW
	if gap < 1 {
		gap = 1
	}
	bar := left + strings.Repeat(" ", gap) + right

	return s.theme.StatusBar.Width(s.width).Render(bar)
}
