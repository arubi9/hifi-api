package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// StatusBarModel shows the bottom status bar.
type StatusBarModel struct {
	viewName string
	info     string
	width    int
}

// NewStatusBar creates a new status bar.
func NewStatusBar() StatusBarModel {
	return StatusBarModel{viewName: "Tracks"}
}

// SetWidth sets the status bar width.
func (s *StatusBarModel) SetWidth(w int) {
	s.width = w
}

// Update sets the view name and info text.
func (s *StatusBarModel) UpdateView(viewName, info string) {
	s.viewName = viewName
	s.info = info
}

// View renders the status bar.
func (s *StatusBarModel) View() string {
	left := StyleStatusViewName.Render(s.viewName)
	if s.info != "" {
		left += lipgloss.NewStyle().Foreground(ColorBorderLt).Render(" | ") +
			lipgloss.NewStyle().Foreground(ColorSecondary).Render(s.info)
	}

	hints := StyleStatusHints.Render("/ Search  d Download  s Settings  ? Help  q Quit")

	// Calculate available space
	leftWidth := lipgloss.Width(left)
	hintsWidth := lipgloss.Width(hints)
	gap := s.width - leftWidth - hintsWidth - 2
	if gap < 1 {
		gap = 1
	}
	spacer := lipgloss.NewStyle().Width(gap).Render("")

	return StyleStatusBar.Width(s.width).Render(left + spacer + hints)
}
