package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mrpir/hifi-tui/internal/models"
)

// ContextMenuModel shows a context menu for an item.
type ContextMenuModel struct {
	options  []menuOption
	cursor   int
	width    int
	itemType string
}

type menuOption struct {
	Label  string
	Action string
}

// NewContextMenu creates a context menu based on item type.
func NewContextMenu(mode models.ViewMode) ContextMenuModel {
	var opts []menuOption
	var itemType string

	switch mode {
	case models.ViewTracks:
		itemType = "Track"
		opts = []menuOption{
			{"Download", "download"},
			{"View Details", "view_details"},
			{"Show in Explorer", "show_explorer"},
		}
	case models.ViewAlbums:
		itemType = "Album"
		opts = []menuOption{
			{"Download All Tracks", "download"},
			{"View Tracks", "view_tracks"},
			{"Show in Explorer", "show_explorer"},
		}
	case models.ViewArtists:
		itemType = "Artist"
		opts = []menuOption{
			{"Download All Albums", "download"},
			{"View Albums", "view_albums"},
		}
	case models.ViewQueue:
		itemType = "Download"
		opts = []menuOption{
			{"Show in Explorer", "show_explorer"},
		}
	}

	return ContextMenuModel{
		options:  opts,
		width:    44,
		itemType: itemType,
	}
}

// Up moves the cursor up.
func (m *ContextMenuModel) Up() {
	if m.cursor > 0 {
		m.cursor--
	}
}

// Down moves the cursor down.
func (m *ContextMenuModel) Down() {
	if m.cursor < len(m.options)-1 {
		m.cursor++
	}
}

// Selected returns the action of the selected option.
func (m *ContextMenuModel) Selected() string {
	if m.cursor >= 0 && m.cursor < len(m.options) {
		return m.options[m.cursor].Action
	}
	return ""
}

// View renders the context menu.
func (m *ContextMenuModel) View() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder).
		Width(m.width - 4).
		Render(m.itemType + " Actions")

	optionStyle := lipgloss.NewStyle().
		Foreground(ColorFg).
		PaddingLeft(2)

	selectedStyle := lipgloss.NewStyle().
		Foreground(ColorFg).
		Background(ColorHighlight).
		PaddingLeft(2).
		Width(m.width - 4)

	body := ""
	for i, opt := range m.options {
		if i == m.cursor {
			body += selectedStyle.Render(opt.Label) + "\n"
		} else {
			body += optionStyle.Render(opt.Label) + "\n"
		}
	}

	footer := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Render("\nEnter select  Esc close")

	return StyleModalOverlay.
		Width(m.width).
		Render(title + "\n" + body + footer)
}
