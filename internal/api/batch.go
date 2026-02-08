package api

import (
	"context"
	"fmt"

	json "github.com/goccy/go-json"
	"github.com/mrpir/hifi-tui/internal/models"
)

// BatchAlbumResult holds the result of a batch album tracks request.
type BatchAlbumResult struct {
	Album  models.Album
	Tracks []BatchTrackEntry
	Stats  BatchStats
}

// BatchTrackEntry holds a track with its playback manifest.
type BatchTrackEntry struct {
	Track    models.Track
	Manifest string // base64 encoded manifest
	MimeType string // manifestMimeType
	Quality  string // audioQuality
	Error    string // non-empty if this track failed
}

// BatchStats holds batch operation statistics.
type BatchStats struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

// BatchAlbumTracks fetches an album and all track manifests in a single request.
func (c *Client) BatchAlbumTracks(ctx context.Context, albumID int) (BatchAlbumResult, error) {
	path := fmt.Sprintf("/batch/album/%d/tracks?quality=%s&concurrency=15", albumID, c.quality)
	data, err := c.doGet(ctx, path)
	if err != nil {
		return BatchAlbumResult{}, err
	}

	var resp struct {
		Album struct {
			ID             int    `json:"id"`
			Title          string `json:"title"`
			ReleaseDate    string `json:"releaseDate"`
			NumberOfTracks int    `json:"numberOfTracks"`
			AudioQuality   string `json:"audioQuality"`
			Cover          string `json:"cover"`
			Artist         struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"artist"`
		} `json:"album"`
		Tracks []struct {
			Track struct {
				ID           int    `json:"id"`
				Title        string `json:"title"`
				Duration     int    `json:"duration"`
				TrackNumber  int    `json:"trackNumber"`
				Explicit     bool   `json:"explicit"`
				AudioQuality string `json:"audioQuality"`
				Artist       struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"track"`
			Playback *struct {
				Manifest         string `json:"manifest"`
				ManifestMimeType string `json:"manifestMimeType"`
				AudioQuality     string `json:"audioQuality"`
			} `json:"playback"`
			Error *string `json:"error"`
		} `json:"tracks"`
		Stats BatchStats `json:"stats"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return BatchAlbumResult{}, fmt.Errorf("parse batch album: %w", err)
	}

	year := parseYear(resp.Album.ReleaseDate)
	album := models.Album{
		ID:           resp.Album.ID,
		Title:        resp.Album.Title,
		Artist:       resp.Album.Artist.Name,
		ArtistID:     resp.Album.Artist.ID,
		NumTracks:    resp.Album.NumberOfTracks,
		Year:         year,
		AudioQuality: resp.Album.AudioQuality,
		Cover:        resp.Album.Cover,
	}

	entries := make([]BatchTrackEntry, 0, len(resp.Tracks))
	for _, t := range resp.Tracks {
		entry := BatchTrackEntry{
			Track: models.Track{
				ID:           t.Track.ID,
				Title:        t.Track.Title,
				Artist:       t.Track.Artist.Name,
				ArtistID:     t.Track.Artist.ID,
				Album:        album.Title,
				AlbumID:      album.ID,
				TrackNumber:  t.Track.TrackNumber,
				Duration:     t.Track.Duration,
				AudioQuality: t.Track.AudioQuality,
				Explicit:     t.Track.Explicit,
				Cover:        album.Cover,
				Year:         year,
			},
		}
		if t.Playback != nil {
			entry.Manifest = t.Playback.Manifest
			entry.MimeType = t.Playback.ManifestMimeType
			entry.Quality = t.Playback.AudioQuality
		}
		if t.Error != nil {
			entry.Error = *t.Error
		}
		entries = append(entries, entry)
	}

	return BatchAlbumResult{
		Album:  album,
		Tracks: entries,
		Stats:  resp.Stats,
	}, nil
}

// BatchTracks resolves manifests for multiple track IDs in a single request.
func (c *Client) BatchTracks(ctx context.Context, trackIDs []int) ([]BatchTrackEntry, BatchStats, error) {
	payload := struct {
		TrackIDs []int  `json:"track_ids"`
		Quality  string `json:"quality"`
	}{
		TrackIDs: trackIDs,
		Quality:  c.quality,
	}

	data, err := c.doPost(ctx, "/batch/tracks/?concurrency=15", payload)
	if err != nil {
		return nil, BatchStats{}, err
	}

	var resp struct {
		Results []struct {
			TrackID int  `json:"track_id"`
			Success bool `json:"success"`
			Data    *struct {
				ID               int    `json:"id"`
				Title            string `json:"title"`
				Duration         int    `json:"duration"`
				TrackNumber      int    `json:"trackNumber"`
				AudioQuality     string `json:"audioQuality"`
				Manifest         string `json:"manifest"`
				ManifestMimeType string `json:"manifestMimeType"`
				Artist           struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				} `json:"artist"`
				Album struct {
					ID          int    `json:"id"`
					Title       string `json:"title"`
					ReleaseDate string `json:"releaseDate"`
					Cover       string `json:"cover"`
				} `json:"album"`
			} `json:"data"`
			Error *string `json:"error"`
		} `json:"results"`
		Stats BatchStats `json:"stats"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, BatchStats{}, fmt.Errorf("parse batch tracks: %w", err)
	}

	entries := make([]BatchTrackEntry, 0, len(resp.Results))
	for _, r := range resp.Results {
		entry := BatchTrackEntry{}
		if r.Error != nil {
			entry.Error = *r.Error
		}
		if r.Data != nil {
			year := parseYear(r.Data.Album.ReleaseDate)
			entry.Track = models.Track{
				ID:           r.Data.ID,
				Title:        r.Data.Title,
				Artist:       r.Data.Artist.Name,
				ArtistID:     r.Data.Artist.ID,
				Album:        r.Data.Album.Title,
				AlbumID:      r.Data.Album.ID,
				TrackNumber:  r.Data.TrackNumber,
				Duration:     r.Data.Duration,
				AudioQuality: r.Data.AudioQuality,
				Cover:        r.Data.Album.Cover,
				Year:         year,
			}
			entry.Manifest = r.Data.Manifest
			entry.MimeType = r.Data.ManifestMimeType
			entry.Quality = r.Data.AudioQuality
		} else {
			entry.Track.ID = r.TrackID
		}
		entries = append(entries, entry)
	}

	return entries, resp.Stats, nil
}

// BatchArtistDiscography fetches all albums and tracks for an artist.
func (c *Client) BatchArtistDiscography(ctx context.Context, artistID int) ([]models.Album, []BatchTrackEntry, BatchStats, error) {
	path := fmt.Sprintf("/batch/artist/%d/discography?quality=%s&concurrency=15&include_albums=true&include_eps=true&include_singles=true", artistID, c.quality)
	data, err := c.doPost(ctx, path, nil)
	if err != nil {
		return nil, nil, BatchStats{}, err
	}

	var resp struct {
		Albums []struct {
			ID             int    `json:"id"`
			Title          string `json:"title"`
			ReleaseDate    string `json:"releaseDate"`
			NumberOfTracks int    `json:"numberOfTracks"`
			AudioQuality   string `json:"audioQuality"`
			Cover          string `json:"cover"`
			Artist         struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"artist"`
		} `json:"albums"`
		Tracks []struct {
			Track struct {
				ID           int    `json:"id"`
				Title        string `json:"title"`
				Duration     int    `json:"duration"`
				TrackNumber  int    `json:"trackNumber"`
				Explicit     bool   `json:"explicit"`
				AudioQuality string `json:"audioQuality"`
				Artist       struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				} `json:"artist"`
				Album struct {
					ID          int    `json:"id"`
					Title       string `json:"title"`
					ReleaseDate string `json:"releaseDate"`
					Cover       string `json:"cover"`
				} `json:"album"`
			} `json:"track"`
			Playback *struct {
				Manifest         string `json:"manifest"`
				ManifestMimeType string `json:"manifestMimeType"`
				AudioQuality     string `json:"audioQuality"`
			} `json:"playback"`
			Error *string `json:"error"`
		} `json:"tracks"`
		Stats struct {
			TotalAlbums int `json:"total_albums"`
			TotalTracks int `json:"total_tracks"`
			Success     int `json:"success"`
			Failed      int `json:"failed"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, nil, BatchStats{}, fmt.Errorf("parse batch artist: %w", err)
	}

	albums := make([]models.Album, 0, len(resp.Albums))
	for _, a := range resp.Albums {
		year := parseYear(a.ReleaseDate)
		albums = append(albums, models.Album{
			ID:           a.ID,
			Title:        a.Title,
			Artist:       a.Artist.Name,
			ArtistID:     a.Artist.ID,
			NumTracks:    a.NumberOfTracks,
			Year:         year,
			AudioQuality: a.AudioQuality,
			Cover:        a.Cover,
		})
	}

	entries := make([]BatchTrackEntry, 0, len(resp.Tracks))
	for _, t := range resp.Tracks {
		year := parseYear(t.Track.Album.ReleaseDate)
		entry := BatchTrackEntry{
			Track: models.Track{
				ID:           t.Track.ID,
				Title:        t.Track.Title,
				Artist:       t.Track.Artist.Name,
				ArtistID:     t.Track.Artist.ID,
				Album:        t.Track.Album.Title,
				AlbumID:      t.Track.Album.ID,
				TrackNumber:  t.Track.TrackNumber,
				Duration:     t.Track.Duration,
				AudioQuality: t.Track.AudioQuality,
				Explicit:     t.Track.Explicit,
				Cover:        t.Track.Album.Cover,
				Year:         year,
			},
		}
		if t.Playback != nil {
			entry.Manifest = t.Playback.Manifest
			entry.MimeType = t.Playback.ManifestMimeType
			entry.Quality = t.Playback.AudioQuality
		}
		if t.Error != nil {
			entry.Error = *t.Error
		}
		entries = append(entries, entry)
	}

	stats := BatchStats{
		Total:   resp.Stats.TotalTracks,
		Success: resp.Stats.Success,
		Failed:  resp.Stats.Failed,
	}

	return albums, entries, stats, nil
}

// GetSingleTrack fetches a single track with manifest (fallback when batch fails).
func (c *Client) GetSingleTrack(ctx context.Context, trackID int) (BatchTrackEntry, error) {
	path := fmt.Sprintf("/track/?id=%d&quality=%s", trackID, c.quality)
	data, err := c.doGet(ctx, path)
	if err != nil {
		return BatchTrackEntry{}, err
	}

	var resp struct {
		Data struct {
			ID               int    `json:"id"`
			Title            string `json:"title"`
			Duration         int    `json:"duration"`
			TrackNumber      int    `json:"trackNumber"`
			Explicit         bool   `json:"explicit"`
			AudioQuality     string `json:"audioQuality"`
			Manifest         string `json:"manifest"`
			ManifestMimeType string `json:"manifestMimeType"`
			Artist           struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"artist"`
			Album struct {
				ID          int    `json:"id"`
				Title       string `json:"title"`
				ReleaseDate string `json:"releaseDate"`
				Cover       string `json:"cover"`
			} `json:"album"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return BatchTrackEntry{}, fmt.Errorf("parse single track: %w", err)
	}

	year := parseYear(resp.Data.Album.ReleaseDate)
	return BatchTrackEntry{
		Track: models.Track{
			ID:           resp.Data.ID,
			Title:        resp.Data.Title,
			Artist:       resp.Data.Artist.Name,
			ArtistID:     resp.Data.Artist.ID,
			Album:        resp.Data.Album.Title,
			AlbumID:      resp.Data.Album.ID,
			TrackNumber:  resp.Data.TrackNumber,
			Duration:     resp.Data.Duration,
			AudioQuality: resp.Data.AudioQuality,
			Explicit:     resp.Data.Explicit,
			Cover:        resp.Data.Album.Cover,
			Year:         year,
		},
		Manifest: resp.Data.Manifest,
		MimeType: resp.Data.ManifestMimeType,
		Quality:  resp.Data.AudioQuality,
	}, nil
}
