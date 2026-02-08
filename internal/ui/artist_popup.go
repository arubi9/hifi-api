package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mrpir/hifi-tui/internal/models"
)

// ArtistPopupModel shows artist detail with an album list.
type ArtistPopupModel struct {
	artist models.Artist
	albums []models.Album
	table  table.Model
	width  int
	height int
}

// NewArtistPopup creates an artist detail popup.
func NewArtistPopup(artist models.Artist, albums []models.Album, w, h int) ArtistPopupModel {
	cols := []table.Column{
		{Title: "Title", Width: 30},
		{Title: "Year", Width: 6},
		{Title: "Tracks", Width: 7},
		{Title: "Quality", Width: 10},
	}

	rows := make([]table.Row, len(albums))
	for i, a := range albums {
		year := ""
		if a.Year > 0 {
			year = fmt.Sprintf("%d", a.Year)
		}
		rows[i] = table.Row{
			truncate(a.Title, 30),
			year,
			fmt.Sprintf("%d", a.NumTracks),
			shortQuality(a.AudioQuality),
		}
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(min2(len(albums)+1, 20)),
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

	return ArtistPopupModel{
		artist: artist,
		albums: albums,
		table:  t,
		width:  w,
		height: h,
	}
}

// Artist returns the artist.
func (a *ArtistPopupModel) Artist() models.Artist {
	return a.artist
}

// Albums returns the artist's albums.
func (a *ArtistPopupModel) Albums() []models.Album {
	return a.albums
}

// Update handles artist popup events.
func (a *ArtistPopupModel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	a.table, cmd = a.table.Update(msg)
	return cmd
}

// View renders the artist popup.
func (a *ArtistPopupModel) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render(a.artist.Name)
	info := fmt.Sprintf("%d albums  Popularity: %d", len(a.albums), a.artist.Popularity)
	infoLine := lipgloss.NewStyle().Foreground(ColorSecondary).Render(info)

	buttons := lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true).Render("[d] Download All Albums") +
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
