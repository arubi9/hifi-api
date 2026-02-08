package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mrpir/hifi-tui/internal/models"
)

// AlbumPopupModel shows album detail with a track list.
type AlbumPopupModel struct {
	album  models.Album
	tracks []models.Track
	table  table.Model
	width  int
	height int
}

// NewAlbumPopup creates an album detail popup.
func NewAlbumPopup(album models.Album, tracks []models.Track, w, h int) AlbumPopupModel {
	cols := []table.Column{
		{Title: "#", Width: 4},
		{Title: "Title", Width: 35},
		{Title: "Duration", Width: 8},
		{Title: "Quality", Width: 10},
	}

	rows := make([]table.Row, len(tracks))
	for i, t := range tracks {
		rows[i] = table.Row{
			fmt.Sprintf("%d", t.TrackNumber),
			truncate(t.Title, 35),
			models.FormatDuration(t.Duration),
			shortQuality(t.AudioQuality),
		}
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(min2(len(tracks)+1, 20)),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorBorder).
		BorderBottom(true).
		Bold(true).
		Foreground(ColorFg).
		Background(ColorBgDarker)
	s.Selected = s.Selected.
		Foreground(ColorFg).
		Background(ColorHighlight)
	t.SetStyles(s)

	return AlbumPopupModel{
		album:  album,
		tracks: tracks,
		table:  t,
		width:  w,
		height: h,
	}
}

// Tracks returns the album's tracks.
func (a *AlbumPopupModel) Tracks() []models.Track {
	return a.tracks
}

// Album returns the album.
func (a *AlbumPopupModel) Album() models.Album {
	return a.album
}

// Update handles album popup events.
func (a *AlbumPopupModel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	a.table, cmd = a.table.Update(msg)
	return cmd
}

// View renders the album popup.
func (a *AlbumPopupModel) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render(a.album.Title) +
		lipgloss.NewStyle().Foreground(ColorSecondary).Render(" by " + a.album.Artist)

	info := ""
	if a.album.Year > 0 {
		info += fmt.Sprintf("%d  ", a.album.Year)
	}
	info += fmt.Sprintf("%d tracks  ", len(a.tracks))
	info += lipgloss.NewStyle().Foreground(ColorPrimary).Render(shortQuality(a.album.AudioQuality))

	infoLine := lipgloss.NewStyle().Foreground(ColorSecondary).Render(info)

	buttons := lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true).Render("[d] Download All") +
		"  " +
		lipgloss.NewStyle().Foreground(ColorSecondary).Render("[Esc] Close")

	boxWidth := 80
	if boxWidth > a.width*90/100 {
		boxWidth = a.width * 90 / 100
	}

	return StyleModalOverlay.
		Width(boxWidth).
		Render(title + "\n" + infoLine + "\n\n" + a.table.View() + "\n\n" + buttons)
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
