package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SearchBarModel handles the search input toggle.
type SearchBarModel struct {
	input   textinput.Model
	visible bool
	width   int
}

// NewSearchBar creates a new search bar.
func NewSearchBar() SearchBarModel {
	ti := textinput.New()
	ti.Placeholder = "Type to search..."
	ti.CharLimit = 200
	ti.PromptStyle = StyleSearchPrefix
	ti.Prompt = " / "
	return SearchBarModel{input: ti}
}

// SetWidth updates the search bar width.
func (s *SearchBarModel) SetWidth(w int) {
	s.width = w
	s.input.Width = w - 8
}

// Visible returns whether the search bar is showing.
func (s *SearchBarModel) Visible() bool {
	return s.visible
}

// Toggle shows/hides the search bar.
func (s *SearchBarModel) Toggle() {
	s.visible = !s.visible
	if s.visible {
		s.input.SetValue("")
		s.input.Focus()
	} else {
		s.input.Blur()
	}
}

// Show makes the search bar visible and focused.
func (s *SearchBarModel) Show() {
	s.visible = true
	s.input.SetValue("")
	s.input.Focus()
}

// Hide makes the search bar invisible.
func (s *SearchBarModel) Hide() {
	s.visible = false
	s.input.Blur()
}

// Value returns the current input text.
func (s *SearchBarModel) Value() string {
	return s.input.Value()
}

// SetValue sets the input text.
func (s *SearchBarModel) SetValue(v string) {
	s.input.SetValue(v)
}

// Focused returns whether the input is focused.
func (s *SearchBarModel) Focused() bool {
	return s.input.Focused()
}

// Focus gives focus to the input.
func (s *SearchBarModel) Focus() {
	s.input.Focus()
}

// Blur removes focus from the input.
func (s *SearchBarModel) Blur() {
	s.input.Blur()
}

// Update handles input events.
func (s *SearchBarModel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return cmd
}

// View renders the search bar.
func (s *SearchBarModel) View() string {
	if !s.visible {
		return ""
	}
	return StyleSearchBar.Width(s.width).Render(s.input.View())
}

// SubmitMsg is sent when the user presses Enter in the search bar.
type SubmitMsg struct {
	Query string
}

// CheckSubmit returns a SubmitMsg command if Enter was pressed.
func (s *SearchBarModel) CheckSubmit(msg tea.KeyMsg) tea.Cmd {
	if msg.String() == "enter" && s.visible && s.input.Focused() {
		query := s.input.Value()
		if query != "" {
			s.Hide()
			return func() tea.Msg {
				return SubmitMsg{Query: query}
			}
		}
	}
	return nil
}

// SearchBarHeight returns the height consumed by the search bar.
func (s *SearchBarModel) Height() int {
	if !s.visible {
		return 0
	}
	return 3
}

// InputView returns just the raw input view without the container style.
func (s *SearchBarModel) InputView() string {
	return s.input.View()
}

// RenderWithStyle renders the search bar with given style.
func (s *SearchBarModel) RenderWithStyle(style lipgloss.Style) string {
	if !s.visible {
		return ""
	}
	return style.Render(s.input.View())
}
