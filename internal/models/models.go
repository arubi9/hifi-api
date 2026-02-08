package models

import (
	"fmt"
	"regexp"
	"strings"
)

// ViewMode represents the current navigation mode.
type ViewMode int

const (
	ViewTracks ViewMode = iota
	ViewAlbums
	ViewArtists
	ViewQueue
)

func (v ViewMode) String() string {
	switch v {
	case ViewTracks:
		return "Tracks"
	case ViewAlbums:
		return "Albums"
	case ViewArtists:
		return "Artists"
	case ViewQueue:
		return "Downloads"
	default:
		return "Tracks"
	}
}

// Quality represents audio quality levels.
type Quality string

const (
	QualityHiResLossless Quality = "HI_RES_LOSSLESS"
	QualityLossless      Quality = "LOSSLESS"
	QualityHigh          Quality = "HIGH"
	QualityLow           Quality = "LOW"
)

// AllQualities returns all quality levels for UI selectors.
func AllQualities() []Quality {
	return []Quality{QualityHiResLossless, QualityLossless, QualityHigh, QualityLow}
}

// Track represents a music track.
type Track struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Artist       string  `json:"artist"`
	ArtistID     int     `json:"artist_id"`
	Album        string  `json:"album"`
	AlbumID      int     `json:"album_id"`
	TrackNumber  int     `json:"track_number"`
	Duration     int     `json:"duration"`
	Year         int     `json:"year"`
	AudioQuality string  `json:"audio_quality"`
	Explicit     bool    `json:"explicit"`
	Cover        string  `json:"cover"`
}

// Album represents a music album.
type Album struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	ArtistID     int    `json:"artist_id"`
	NumTracks    int    `json:"num_tracks"`
	Year         int    `json:"year"`
	AudioQuality string `json:"audio_quality"`
	Cover        string `json:"cover"`
}

// Artist represents a music artist.
type Artist struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Picture    string `json:"picture"`
	Popularity int    `json:"popularity"`
}

// DownloadItem represents a download queue entry.
type DownloadItem struct {
	Name     string `json:"name"`
	ItemType string `json:"item_type"` // "Track", "Album", "Artist"
	SourceID int    `json:"source_id"`
	Artist   string `json:"artist"`
	SpeedStr string `json:"speed_str"`
	JobID    string `json:"job_id"`
	Status   string `json:"status"` // pending, processing, completed, failed, partial
	Progress int    `json:"progress"`
	Total    int    `json:"total"`
	Success  int    `json:"success"`
	Failed   int    `json:"failed"`
}

// StreamInfo holds parsed manifest information.
type StreamInfo struct {
	URLs  []string // Single URL for BTS, multiple for DASH segments
	IsRaw bool     // true for BTS (direct download), false for DASH
	Codec string   // "flac" or "aac"
}

// NavState holds navigation state.
type NavState struct {
	Mode        ViewMode
	SearchQuery string
	SelectedIDs map[string]bool
	History     []HistoryEntry
	Breadcrumb  string
}

// HistoryEntry stores a navigation snapshot for back navigation.
type HistoryEntry struct {
	Mode       ViewMode
	Items      interface{}
	Query      string
	Breadcrumb string
}

// NewNavState creates a new NavState with defaults.
func NewNavState() NavState {
	return NavState{
		Mode:        ViewTracks,
		SelectedIDs: make(map[string]bool),
	}
}

var reSanitize = regexp.MustCompile(`[<>:"/\\|?*]`)

// SanitizeFilename replaces illegal filename characters and truncates.
func SanitizeFilename(name string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 50
	}
	s := reSanitize.ReplaceAllString(name, "_")
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	s = strings.TrimRight(s, ". ")
	return s
}

// FormatDuration formats seconds as mm:ss.
func FormatDuration(seconds int) string {
	m := seconds / 60
	s := seconds % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

// CoverURL builds the Tidal cover art URL from a cover UUID.
func CoverURL(coverUUID string) string {
	if coverUUID == "" {
		return ""
	}
	path := strings.ReplaceAll(coverUUID, "-", "/")
	return "https://resources.tidal.com/images/" + path + "/1280x1280.jpg"
}
