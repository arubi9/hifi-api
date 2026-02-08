package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/mrpir/hifi-tui/internal/models"
)

// TrackPopupModel shows track detail.
type TrackPopupModel struct {
	track models.Track
	width int
}

// NewTrackPopup creates a track detail popup.
func NewTrackPopup(track models.Track, w int) TrackPopupModel {
	return TrackPopupModel{track: track, width: w}
}

// Track returns the track.
func (t *TrackPopupModel) Track() models.Track {
	return t.track
}

// View renders the track popup.
func (t *TrackPopupModel) View() string {
	tr := t.track

	title := lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render(tr.Title)
	if tr.Explicit {
		title += " " + StyleExplicit.Render("EXPLICIT")
	}

	label := StyleDetailLabel
	value := StyleDetailValue

	body := ""
	body += label.Render("Artist") + value.Render(tr.Artist) + "\n"
	body += label.Render("Album") + value.Render(tr.Album) + "\n"
	body += label.Render("Duration") + value.Render(models.FormatDuration(tr.Duration)) + "\n"
	if tr.TrackNumber > 0 {
		body += label.Render("Track #") + value.Render(fmt.Sprintf("%d", tr.TrackNumber)) + "\n"
	}
	if tr.Year > 0 {
		body += label.Render("Year") + value.Render(fmt.Sprintf("%d", tr.Year)) + "\n"
	}
	body += label.Render("Quality") + lipgloss.NewStyle().Foreground(ColorPrimary).Render(shortQuality(tr.AudioQuality)) + "\n"
	body += label.Render("ID") + lipgloss.NewStyle().Foreground(ColorSecondary).Render(fmt.Sprintf("%d", tr.ID)) + "\n"

	buttons := lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true).Render("[d] Download") +
		"  " +
		lipgloss.NewStyle().Foreground(ColorSecondary).Render("[Esc] Close")

	boxWidth := 60
	if boxWidth > t.width*90/100 {
		boxWidth = t.width * 90 / 100
	}

	return StyleModalOverlay.
		Width(boxWidth).
		Render(title + "\n\n" + body + "\n" + buttons)
}
