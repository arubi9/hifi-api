package ui

import (
	"github.com/mrpir/hifi-tui/internal/download"
	"github.com/mrpir/hifi-tui/internal/models"
)

// SearchResultMsg carries search results back to the UI.
type SearchResultMsg struct {
	Mode  models.ViewMode
	Items interface{} // []models.Track, []models.Album, or []models.Artist
	Query string
	Err   error
}

// AlbumDetailMsg carries album detail for popup display.
type AlbumDetailMsg struct {
	Album  models.Album
	Tracks []models.Track
	Err    error
}

// ArtistDetailMsg carries artist detail for popup display.
type ArtistDetailMsg struct {
	Artist models.Artist
	Albums []models.Album
	Err    error
}

// DownloadTickMsg carries periodic download progress updates.
type DownloadTickMsg struct {
	Snap download.Snapshot
}

// DownloadCompleteMsg signals that a download batch has finished.
type DownloadCompleteMsg struct {
	Summary string
	Total   int
	Failed  int
}

// DownloadStartMsg signals a download queue item was added.
type DownloadStartMsg struct {
	Item *models.DownloadItem
}

// NavChangeMsg signals the user changed the nav mode.
type NavChangeMsg struct {
	Mode models.ViewMode
}

// ContextMenuResultMsg carries the selected action from the context menu.
type ContextMenuResultMsg struct {
	Action string
	ItemID string
}

// SettingsChangedMsg carries updated settings.
type SettingsChangedMsg struct {
	Settings interface{} // config.Settings
}

// ErrorMsg carries a general error for display.
type ErrorMsg struct {
	Err error
}
