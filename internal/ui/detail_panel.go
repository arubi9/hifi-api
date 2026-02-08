package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/mrpir/hifi-tui/internal/download"
	"github.com/mrpir/hifi-tui/internal/models"
)

// DetailPanelModel shows details about the selected item.
type DetailPanelModel struct {
	content string
	height  int
}

// NewDetailPanel creates a new detail panel.
func NewDetailPanel() DetailPanelModel {
	return DetailPanelModel{content: noSelection()}
}

// SetHeight sets the panel height.
func (d *DetailPanelModel) SetHeight(h int) {
	d.height = h
}

// Clear resets to no selection.
func (d *DetailPanelModel) Clear() {
	d.content = noSelection()
}

// ShowTrack displays track details.
func (d *DetailPanelModel) ShowTrack(t models.Track) {
	title := StyleDetailTitle.Render(t.Title)
	body := ""
	body += detailRow("Artist", t.Artist)
	body += detailRow("Album", t.Album)
	body += detailRow("Duration", models.FormatDuration(t.Duration))
	body += detailRow("Quality", shortQuality(t.AudioQuality))
	if t.Year > 0 {
		body += detailRow("Year", fmt.Sprintf("%d", t.Year))
	}
	if t.TrackNumber > 0 {
		body += detailRow("Track #", fmt.Sprintf("%d", t.TrackNumber))
	}
	if t.Explicit {
		body += "\n" + StyleExplicit.Render("EXPLICIT")
	}

	actions := "\n" + lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("d") +
		lipgloss.NewStyle().Foreground(ColorSecondary).Render(" Download  ") +
		lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("space") +
		lipgloss.NewStyle().Foreground(ColorSecondary).Render(" Select")

	d.content = title + "\n" + body + actions
}

// ShowAlbum displays album details.
func (d *DetailPanelModel) ShowAlbum(a models.Album) {
	title := StyleDetailTitle.Render(a.Title)
	body := ""
	body += detailRow("Artist", a.Artist)
	body += detailRow("Tracks", fmt.Sprintf("%d", a.NumTracks))
	if a.Year > 0 {
		body += detailRow("Year", fmt.Sprintf("%d", a.Year))
	}
	body += detailRow("Quality", shortQuality(a.AudioQuality))

	actions := "\n" + lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("enter") +
		lipgloss.NewStyle().Foreground(ColorSecondary).Render(" View tracks  ") +
		lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("d") +
		lipgloss.NewStyle().Foreground(ColorSecondary).Render(" Download all")

	d.content = title + "\n" + body + actions
}

// ShowArtist displays artist details.
func (d *DetailPanelModel) ShowArtist(a models.Artist) {
	title := StyleDetailTitle.Render(a.Name)
	body := detailRow("Popularity", fmt.Sprintf("%d", a.Popularity))

	actions := "\n" + lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("enter") +
		lipgloss.NewStyle().Foreground(ColorSecondary).Render(" View albums  ") +
		lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("d") +
		lipgloss.NewStyle().Foreground(ColorSecondary).Render(" Download all")

	d.content = title + "\n" + body + actions
}

// ShowDownload displays download item details with progress bar.
func (d *DetailPanelModel) ShowDownload(item models.DownloadItem) {
	title := StyleDetailTitle.Render(item.Name)
	body := ""
	body += detailRow("Artist", item.Artist)
	body += detailRow("Type", item.ItemType)

	statusStyle := lipgloss.NewStyle().Foreground(StatusColor(item.Status))
	body += StyleDetailLabel.Render("Status") + statusStyle.Render(item.Status) + "\n"

	if item.SpeedStr != "" {
		body += detailRow("Speed", item.SpeedStr)
	}

	if item.Total > 0 {
		pct := float64(item.Progress) / float64(item.Total) * 100
		body += detailRow("Progress", fmt.Sprintf("%d/%d", item.Progress, item.Total))
		body += ProgressBar(20, pct) + fmt.Sprintf(" %.0f%%", pct) + "\n"
	}

	if item.Failed > 0 {
		body += StyleDetailLabel.Render("Failed") +
			lipgloss.NewStyle().Foreground(ColorError).Render(fmt.Sprintf("%d", item.Failed)) + "\n"
	}

	actions := "\n" + lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("c") +
		lipgloss.NewStyle().Foreground(ColorSecondary).Render(" Cancel job")

	d.content = title + "\n" + body + actions
}

// ShowDownloadProgress shows live download progress.
func (d *DetailPanelModel) ShowDownloadProgress(snap download.Snapshot) {
	title := StyleDetailTitle.Render("Downloading")
	body := ""
	if snap.TrackTitle != "" {
		body += detailRow("Track", snap.TrackTitle)
	}
	if snap.TrackArtist != "" {
		body += detailRow("Artist", snap.TrackArtist)
	}
	body += detailRow("Status", snap.Status)
	if snap.SpeedStr != "" {
		body += detailRow("Speed", snap.SpeedStr)
	}
	if snap.ETAStr != "" {
		body += detailRow("ETA", snap.ETAStr)
	}
	body += detailRow("Tracks", fmt.Sprintf("%d/%d", snap.TracksDone, snap.TracksTotal))
	if snap.TracksFailed > 0 {
		body += StyleDetailLabel.Render("Failed") +
			lipgloss.NewStyle().Foreground(ColorError).Render(fmt.Sprintf("%d", snap.TracksFailed)) + "\n"
	}
	body += ProgressBar(20, snap.Percent) + fmt.Sprintf(" %.0f%%", snap.Percent) + "\n"

	d.content = title + "\n" + body
}

// View renders the detail panel.
func (d *DetailPanelModel) View() string {
	return StyleDetailPanel.Height(d.height).Render(d.content)
}

func detailRow(label, value string) string {
	if value == "" {
		value = "-"
	}
	return StyleDetailLabel.Render(label) + StyleDetailValue.Render(value) + "\n"
}

func noSelection() string {
	return lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Italic(true).
		Render("No item selected")
}
