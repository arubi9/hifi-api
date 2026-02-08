package api

import (
	"context"
	"fmt"
	"strconv"

	json "github.com/goccy/go-json"
	"github.com/mrpir/hifi-tui/internal/models"
)

// SearchTracks searches for tracks by query.
func (c *Client) SearchTracks(ctx context.Context, query string, limit int) ([]models.Track, error) {
	if limit <= 0 {
		limit = 50
	}
	data, err := c.doGet(ctx, fmt.Sprintf("/search/?s=%s&limit=%d", urlEncode(query), limit))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse search tracks: %w", err)
	}

	tracks := make([]models.Track, 0, len(resp.Data.Items))
	for _, raw := range resp.Data.Items {
		t, err := parseTrackJSON(raw)
		if err != nil {
			continue
		}
		tracks = append(tracks, t)
	}
	return tracks, nil
}

// SearchAlbums searches for albums by query.
func (c *Client) SearchAlbums(ctx context.Context, query string, limit int) ([]models.Album, error) {
	if limit <= 0 {
		limit = 50
	}
	data, err := c.doGet(ctx, fmt.Sprintf("/search/?al=%s&limit=%d", urlEncode(query), limit))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data struct {
			Albums struct {
				Items []json.RawMessage `json:"items"`
			} `json:"albums"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse search albums: %w", err)
	}

	albums := make([]models.Album, 0, len(resp.Data.Albums.Items))
	for _, raw := range resp.Data.Albums.Items {
		a, err := parseAlbumJSON(raw)
		if err != nil {
			continue
		}
		albums = append(albums, a)
	}
	return albums, nil
}

// SearchArtists searches for artists by query.
func (c *Client) SearchArtists(ctx context.Context, query string, limit int) ([]models.Artist, error) {
	if limit <= 0 {
		limit = 20
	}
	data, err := c.doGet(ctx, fmt.Sprintf("/search/?a=%s&limit=%d", urlEncode(query), limit))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data struct {
			Artists struct {
				Items []json.RawMessage `json:"items"`
			} `json:"artists"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse search artists: %w", err)
	}

	artists := make([]models.Artist, 0, len(resp.Data.Artists.Items))
	for _, raw := range resp.Data.Artists.Items {
		ar, err := parseArtistJSON(raw)
		if err != nil {
			continue
		}
		artists = append(artists, ar)
	}
	return artists, nil
}

// parseTrackJSON parses a Tidal track JSON object into a Track model.
func parseTrackJSON(raw json.RawMessage) (models.Track, error) {
	var j struct {
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
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		return models.Track{}, err
	}
	year := 0
	if len(j.Album.ReleaseDate) >= 4 {
		year, _ = strconv.Atoi(j.Album.ReleaseDate[:4])
	}
	return models.Track{
		ID:           j.ID,
		Title:        j.Title,
		Artist:       j.Artist.Name,
		ArtistID:     j.Artist.ID,
		Album:        j.Album.Title,
		AlbumID:      j.Album.ID,
		TrackNumber:  j.TrackNumber,
		Duration:     j.Duration,
		Year:         year,
		AudioQuality: j.AudioQuality,
		Explicit:     j.Explicit,
		Cover:        j.Album.Cover,
	}, nil
}

// parseAlbumJSON parses a Tidal album JSON object into an Album model.
func parseAlbumJSON(raw json.RawMessage) (models.Album, error) {
	var j struct {
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
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		return models.Album{}, err
	}
	year := 0
	if len(j.ReleaseDate) >= 4 {
		year, _ = strconv.Atoi(j.ReleaseDate[:4])
	}
	return models.Album{
		ID:           j.ID,
		Title:        j.Title,
		Artist:       j.Artist.Name,
		ArtistID:     j.Artist.ID,
		NumTracks:    j.NumberOfTracks,
		Year:         year,
		AudioQuality: j.AudioQuality,
		Cover:        j.Cover,
	}, nil
}

// parseArtistJSON parses a Tidal artist JSON object into an Artist model.
func parseArtistJSON(raw json.RawMessage) (models.Artist, error) {
	var j struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		Picture    string `json:"picture"`
		Popularity int    `json:"popularity"`
	}
	if err := json.Unmarshal(raw, &j); err != nil {
		return models.Artist{}, err
	}
	return models.Artist{
		ID:         j.ID,
		Name:       j.Name,
		Picture:    j.Picture,
		Popularity: j.Popularity,
	}, nil
}

// urlEncode is a simple URL encoding for query params.
func urlEncode(s string) string {
	// Use net/url for proper encoding
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b = append(b, c)
		} else if c == ' ' {
			b = append(b, '+')
		} else {
			b = append(b, '%')
			b = append(b, "0123456789ABCDEF"[c>>4])
			b = append(b, "0123456789ABCDEF"[c&0x0f])
		}
	}
	return string(b)
}
