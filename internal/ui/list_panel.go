package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mrpir/hifi-tui/internal/models"
)

// ListPanelModel displays a table of items.
type ListPanelModel struct {
	table       table.Model
	mode        models.ViewMode
	width       int
	height      int
	items       interface{} // stores the current items for lookup
	selectedIDs map[string]bool
	emptyMsg    string
}

// NewListPanel creates a new list panel.
func NewListPanel() ListPanelModel {
	t := table.New(
		table.WithFocused(true),
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
		Background(ColorHighlight).
		Bold(false)
	t.SetStyles(s)

	return ListPanelModel{
		table:       t,
		mode:        models.ViewTracks,
		selectedIDs: make(map[string]bool),
		emptyMsg:    "Press / to search",
	}
}

// SetSize sets the table dimensions.
func (l *ListPanelModel) SetSize(w, h int) {
	l.width = w
	l.height = h
	l.table.SetWidth(w)
	l.table.SetHeight(h - 2) // account for header
}

// SetSelectedIDs updates which items are selected.
func (l *ListPanelModel) SetSelectedIDs(ids map[string]bool) {
	l.selectedIDs = ids
}

// SetTracks populates the table with track data.
func (l *ListPanelModel) SetTracks(tracks []models.Track) {
	l.mode = models.ViewTracks
	l.items = tracks
	if len(tracks) == 0 {
		l.emptyMsg = "No tracks found. Press / to search."
		l.table.SetRows(nil)
		return
	}

	cols := []table.Column{
		{Title: " ", Width: 2},
		{Title: "Title", Width: l.colWidth(35)},
		{Title: "Artist", Width: l.colWidth(20)},
		{Title: "Album", Width: l.colWidth(20)},
		{Title: "Time", Width: 6},
		{Title: "Quality", Width: 10},
	}
	l.table.SetColumns(cols)

	rows := make([]table.Row, len(tracks))
	for i, t := range tracks {
		sel := " "
		if l.selectedIDs[fmt.Sprintf("%d", t.ID)] {
			sel = "*"
		}
		rows[i] = table.Row{
			sel,
			truncate(t.Title, l.colWidth(35)),
			truncate(t.Artist, l.colWidth(20)),
			truncate(t.Album, l.colWidth(20)),
			models.FormatDuration(t.Duration),
			shortQuality(t.AudioQuality),
		}
	}
	l.table.SetRows(rows)
}

// SetAlbums populates the table with album data.
func (l *ListPanelModel) SetAlbums(albums []models.Album) {
	l.mode = models.ViewAlbums
	l.items = albums
	if len(albums) == 0 {
		l.emptyMsg = "No albums found. Press / to search."
		l.table.SetRows(nil)
		return
	}

	cols := []table.Column{
		{Title: " ", Width: 2},
		{Title: "Title", Width: l.colWidth(30)},
		{Title: "Artist", Width: l.colWidth(20)},
		{Title: "Tracks", Width: 7},
		{Title: "Year", Width: 6},
		{Title: "Quality", Width: 10},
	}
	l.table.SetColumns(cols)

	rows := make([]table.Row, len(albums))
	for i, a := range albums {
		sel := " "
		if l.selectedIDs[fmt.Sprintf("%d", a.ID)] {
			sel = "*"
		}
		year := ""
		if a.Year > 0 {
			year = fmt.Sprintf("%d", a.Year)
		}
		rows[i] = table.Row{
			sel,
			truncate(a.Title, l.colWidth(30)),
			truncate(a.Artist, l.colWidth(20)),
			fmt.Sprintf("%d", a.NumTracks),
			year,
			shortQuality(a.AudioQuality),
		}
	}
	l.table.SetRows(rows)
}

// SetArtists populates the table with artist data.
func (l *ListPanelModel) SetArtists(artists []models.Artist) {
	l.mode = models.ViewArtists
	l.items = artists
	if len(artists) == 0 {
		l.emptyMsg = "No artists found. Press / to search."
		l.table.SetRows(nil)
		return
	}

	cols := []table.Column{
		{Title: " ", Width: 2},
		{Title: "Name", Width: l.colWidth(40)},
		{Title: "Popularity", Width: 12},
	}
	l.table.SetColumns(cols)

	rows := make([]table.Row, len(artists))
	for i, a := range artists {
		sel := " "
		if l.selectedIDs[fmt.Sprintf("%d", a.ID)] {
			sel = "*"
		}
		rows[i] = table.Row{
			sel,
			truncate(a.Name, l.colWidth(40)),
			fmt.Sprintf("%d", a.Popularity),
		}
	}
	l.table.SetRows(rows)
}

// SetQueue populates the table with download queue items.
func (l *ListPanelModel) SetQueue(items []models.DownloadItem) {
	l.mode = models.ViewQueue
	l.items = items
	if len(items) == 0 {
		l.emptyMsg = "No downloads. Select items and press d."
		l.table.SetRows(nil)
		return
	}

	cols := []table.Column{
		{Title: " ", Width: 2},
		{Title: "Name", Width: l.colWidth(25)},
		{Title: "Artist", Width: l.colWidth(15)},
		{Title: "Type", Width: 8},
		{Title: "Status", Width: 12},
		{Title: "Speed", Width: 10},
		{Title: "Progress", Width: 14},
	}
	l.table.SetColumns(cols)

	rows := make([]table.Row, len(items))
	for i, item := range items {
		sel := " "
		if l.selectedIDs[item.JobID] {
			sel = "*"
		}
		progress := ""
		if item.Total > 0 {
			pct := float64(item.Progress) / float64(item.Total) * 100
			progress = fmt.Sprintf("%d/%d (%.0f%%)", item.Progress, item.Total, pct)
		}
		rows[i] = table.Row{
			sel,
			truncate(item.Name, l.colWidth(25)),
			truncate(item.Artist, l.colWidth(15)),
			item.ItemType,
			item.Status,
			item.SpeedStr,
			progress,
		}
	}
	l.table.SetRows(rows)
}

// CursorIndex returns the currently selected row index.
func (l *ListPanelModel) CursorIndex() int {
	return l.table.Cursor()
}

// Mode returns the current view mode.
func (l *ListPanelModel) Mode() models.ViewMode {
	return l.mode
}

// Items returns the current items.
func (l *ListPanelModel) Items() interface{} {
	return l.items
}

// HasItems returns whether the table has any rows.
func (l *ListPanelModel) HasItems() bool {
	return len(l.table.Rows()) > 0
}

// Focus gives focus to the table.
func (l *ListPanelModel) Focus() {
	l.table.Focus()
}

// Blur removes focus from the table.
func (l *ListPanelModel) Blur() {
	l.table.Blur()
}

// Focused returns whether the table has focus.
func (l *ListPanelModel) Focused() bool {
	return l.table.Focused()
}

// Update handles table events.
func (l *ListPanelModel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.table, cmd = l.table.Update(msg)
	return cmd
}

// View renders the list panel.
func (l *ListPanelModel) View() string {
	header := StyleListHeader.Render(l.mode.String())

	if !l.HasItems() {
		empty := lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Width(l.width).
			Align(lipgloss.Center).
			Render(l.emptyMsg)
		return lipgloss.JoinVertical(lipgloss.Left, header, empty)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, l.table.View())
}

func (l *ListPanelModel) colWidth(preferred int) int {
	// Scale column widths proportionally to available space
	totalFixed := 2 + 6 + 10 // selection + time + quality (track mode)
	available := l.width - totalFixed - 6 // padding/borders
	if available < preferred {
		return max2(available, 8)
	}
	return preferred
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "~"
}

func shortQuality(q string) string {
	switch q {
	case "HI_RES_LOSSLESS":
		return "HiRes"
	case "LOSSLESS":
		return "Lossless"
	case "HIGH":
		return "High"
	case "LOW":
		return "Low"
	default:
		return q
	}
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// RefreshSelection re-renders the selection marks in column 0.
func (l *ListPanelModel) RefreshSelection() {
	rows := l.table.Rows()
	if len(rows) == 0 {
		return
	}

	newRows := make([]table.Row, len(rows))
	for i, row := range rows {
		newRow := make(table.Row, len(row))
		copy(newRow, row)

		id := l.getItemID(i)
		if l.selectedIDs[id] {
			newRow[0] = "*"
		} else {
			newRow[0] = " "
		}
		newRows[i] = newRow
	}
	l.table.SetRows(newRows)
}

// getItemID returns the ID string for item at index.
func (l *ListPanelModel) getItemID(idx int) string {
	switch items := l.items.(type) {
	case []models.Track:
		if idx < len(items) {
			return fmt.Sprintf("%d", items[idx].ID)
		}
	case []models.Album:
		if idx < len(items) {
			return fmt.Sprintf("%d", items[idx].ID)
		}
	case []models.Artist:
		if idx < len(items) {
			return fmt.Sprintf("%d", items[idx].ID)
		}
	case []models.DownloadItem:
		if idx < len(items) {
			return items[idx].JobID
		}
	}
	return ""
}

// GetCurrentItemID returns the ID of the currently highlighted item.
func (l *ListPanelModel) GetCurrentItemID() string {
	return l.getItemID(l.table.Cursor())
}

// ItemCount returns the number of items in the list.
func (l *ListPanelModel) ItemCount() int {
	return len(l.table.Rows())
}

// GetAllIDs returns all item IDs in the current view.
func (l *ListPanelModel) GetAllIDs() []string {
	rows := l.table.Rows()
	ids := make([]string, 0, len(rows))
	for i := range rows {
		id := l.getItemID(i)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// EmptyMessage returns empty state text.
func (l *ListPanelModel) EmptyMessage() string {
	return l.emptyMsg
}
