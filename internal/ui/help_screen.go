package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// HelpScreenModel shows the help overlay.
type HelpScreenModel struct {
	width  int
	height int
}

// NewHelpScreen creates a help screen.
func NewHelpScreen(w, h int) HelpScreenModel {
	return HelpScreenModel{width: w, height: h}
}

// View renders the help overlay.
func (h HelpScreenModel) View() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorFg).
		MarginBottom(1).
		Render("Keyboard Shortcuts")

	section := func(name string) string {
		return lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(name)
	}
	keyStyle := lipgloss.NewStyle().Foreground(ColorPrimary)
	descStyle := lipgloss.NewStyle().Foreground(ColorFg)

	kv := func(k, desc string) string {
		return "  " + keyStyle.Width(18).Render(k) + descStyle.Render(desc)
	}

	content := title + "\n\n" +
		section("Navigation") + "\n" +
		kv("Tab / Shift+Tab", "Cycle panels") + "\n" +
		kv("Up / Down", "Navigate list") + "\n" +
		kv("Enter", "View details popup") + "\n" +
		kv("Backspace", "Go back") + "\n\n" +

		section("Search") + "\n" +
		kv("/", "Open search") + "\n" +
		kv("Esc", "Close search") + "\n\n" +

		section("Selection") + "\n" +
		kv("Space", "Toggle selection") + "\n" +
		kv("a", "Select all visible") + "\n" +
		kv("A (Shift+a)", "Clear all selections") + "\n\n" +

		section("Actions") + "\n" +
		kv("d", "Download selected") + "\n" +
		kv("m", "Context menu") + "\n" +
		kv("s", "Open settings") + "\n" +
		kv("?", "Toggle this help") + "\n" +
		kv("q", "Quit") + "\n\n" +

		section("In Queue View") + "\n" +
		kv("c", "Cancel selected job") + "\n\n" +

		lipgloss.NewStyle().Foreground(ColorSecondary).Render("Press ? or Esc to close")

	boxWidth := 56
	if mw := h.width * 90 / 100; boxWidth > mw && mw > 0 {
		boxWidth = mw
	}

	return StyleModalOverlay.
		Width(boxWidth).
		Render(content)
}
