package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/mrpir/hifi-tui/internal/models"
)

// NavPanelModel is the left navigation panel.
type NavPanelModel struct {
	active      models.ViewMode
	height      int
	trackCount  int
	albumCount  int
	artistCount int
	queueCount  int
}

// NewNavPanel creates a new navigation panel.
func NewNavPanel() NavPanelModel {
	return NavPanelModel{active: models.ViewTracks}
}

// SetActive sets the active nav mode.
func (n *NavPanelModel) SetActive(mode models.ViewMode) {
	n.active = mode
}

// SetHeight sets the available height.
func (n *NavPanelModel) SetHeight(h int) {
	n.height = h
}

// SetCounts updates the result counts.
func (n *NavPanelModel) SetCounts(tracks, albums, artists int) {
	n.trackCount = tracks
	n.albumCount = albums
	n.artistCount = artists
}

// SetQueueCount updates the queue count.
func (n *NavPanelModel) SetQueueCount(count int) {
	n.queueCount = count
}

// View renders the navigation panel.
func (n *NavPanelModel) View() string {
	s := StyleNavLabel.Render("  BROWSE") + "\n"
	s += n.navItem("Tracks", n.trackCount, models.ViewTracks) + "\n"
	s += n.navItem("Albums", n.albumCount, models.ViewAlbums) + "\n"
	s += n.navItem("Artists", n.artistCount, models.ViewArtists) + "\n"
	s += "\n"

	// Separator
	s += lipgloss.NewStyle().
		Foreground(ColorBorder).
		Width(20).
		Render("────────────────────") + "\n"
	s += "\n"

	s += StyleNavLabel.Render("  LIBRARY") + "\n"
	s += n.navItem("Downloads", n.queueCount, models.ViewQueue) + "\n"

	return StyleNavPanel.Height(n.height).Render(s)
}

func (n *NavPanelModel) navItem(label string, count int, mode models.ViewMode) string {
	countStr := ""
	if count > 0 {
		countStr = fmt.Sprintf(" (%d)", count)
	}

	if n.active == mode {
		return StyleNavBtnActive.Render(
			lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(label+countStr),
		)
	}
	return StyleNavBtn.Render(label + countStr)
}
