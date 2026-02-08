package api

import (
	"context"
	"fmt"

	json "github.com/goccy/go-json"
	"github.com/mrpir/hifi-tui/internal/logger"
	"github.com/mrpir/hifi-tui/internal/models"
)

// GetAlbumTracks fetches album metadata and its tracks.
// Falls back to search-based reconstruction if /album/ is rate-limited.
func (c *Client) GetAlbumTracks(ctx context.Context, albumID int) (models.Album, []models.Track, error) {
	data, err := c.doGet(ctx, fmt.Sprintf("/album/?id=%d", albumID))
	if err != nil {
		logger.Log.Warn("album endpoint failed, trying search fallback", "albumID", albumID, "err", err)
		return c.albumFallback(ctx, albumID)
	}

	var resp struct {
		Data struct {
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
			Items []struct {
				Item json.RawMessage `json:"item"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return models.Album{}, nil, fmt.Errorf("parse album: %w", err)
	}

	year := parseYear(resp.Data.ReleaseDate)
	album := models.Album{
		ID:           resp.Data.ID,
		Title:        resp.Data.Title,
		Artist:       resp.Data.Artist.Name,
		ArtistID:     resp.Data.Artist.ID,
		NumTracks:    resp.Data.NumberOfTracks,
		Year:         year,
		AudioQuality: resp.Data.AudioQuality,
		Cover:        resp.Data.Cover,
	}

	tracks := make([]models.Track, 0, len(resp.Data.Items))
	for _, item := range resp.Data.Items {
		t, err := parseTrackJSON(item.Item)
		if err != nil {
			continue
		}
		// Fill in album info if missing from track
		if t.Album == "" {
			t.Album = album.Title
		}
		if t.AlbumID == 0 {
			t.AlbumID = album.ID
		}
		if t.Cover == "" {
			t.Cover = album.Cover
		}
		if t.Year == 0 {
			t.Year = album.Year
		}
		tracks = append(tracks, t)
	}

	return album, tracks, nil
}

// albumFallback reconstructs album tracks from search results when /album/ fails.
func (c *Client) albumFallback(ctx context.Context, albumID int) (models.Album, []models.Track, error) {
	// Search for albums to find the album name
	albums, err := c.SearchAlbums(ctx, fmt.Sprintf("%d", albumID), 5)
	if err != nil || len(albums) == 0 {
		return models.Album{}, nil, fmt.Errorf("album fallback: could not find album %d", albumID)
	}

	var album models.Album
	for _, a := range albums {
		if a.ID == albumID {
			album = a
			break
		}
	}
	if album.ID == 0 {
		album = albums[0]
	}

	// Search for tracks by album name
	tracks, err := c.SearchTracks(ctx, album.Title+" "+album.Artist, 50)
	if err != nil {
		return album, nil, err
	}

	// Filter to matching album
	var filtered []models.Track
	for _, t := range tracks {
		if t.AlbumID == albumID {
			filtered = append(filtered, t)
		}
	}

	return album, filtered, nil
}

// GetArtistAlbums fetches an artist's discography (albums only).
func (c *Client) GetArtistAlbums(ctx context.Context, artistID int) (models.Artist, []models.Album, error) {
	data, err := c.doGet(ctx, fmt.Sprintf("/artist/?f=%d&skip_tracks=true", artistID))
	if err != nil {
		return models.Artist{}, nil, err
	}

	var resp struct {
		Albums struct {
			Items []json.RawMessage `json:"items"`
		} `json:"albums"`
		// Artist info may come from the response too
		ID         int    `json:"id"`
		Name       string `json:"name"`
		Picture    string `json:"picture"`
		Popularity int    `json:"popularity"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return models.Artist{}, nil, fmt.Errorf("parse artist: %w", err)
	}

	artist := models.Artist{
		ID:         resp.ID,
		Name:       resp.Name,
		Picture:    resp.Picture,
		Popularity: resp.Popularity,
	}

	albums := make([]models.Album, 0, len(resp.Albums.Items))
	for _, raw := range resp.Albums.Items {
		a, err := parseAlbumJSON(raw)
		if err != nil {
			continue
		}
		albums = append(albums, a)
	}

	return artist, albums, nil
}

// parseYear extracts year from "2024-01-15" format.
func parseYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	y := 0
	for i := 0; i < 4 && i < len(date); i++ {
		c := date[i]
		if c >= '0' && c <= '9' {
			y = y*10 + int(c-'0')
		}
	}
	return y
}
