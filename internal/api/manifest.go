package api

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	json "github.com/goccy/go-json"
	"github.com/mrpir/hifi-tui/internal/models"
)

// Pre-compiled regexes for DASH manifest parsing.
var (
	reInit     = regexp.MustCompile(`initialization="([^"]+)"`)
	reMedia    = regexp.MustCompile(`media="([^"]+)"`)
	reSegments = regexp.MustCompile(`<S d="\d+"(?:\s+r="(\d+)")?\s*/>`)
	reBaseURL  = regexp.MustCompile(`<BaseURL>([^<]+)</BaseURL>`)
)

// ParseManifest decodes a base64 manifest and extracts stream info.
func ParseManifest(manifestB64, mimeType string) (models.StreamInfo, error) {
	decoded, err := base64.StdEncoding.DecodeString(manifestB64)
	if err != nil {
		// Try URL-safe or no-padding variants
		decoded, err = base64.RawStdEncoding.DecodeString(manifestB64)
		if err != nil {
			return models.StreamInfo{}, fmt.Errorf("decode manifest: %w", err)
		}
	}
	content := string(decoded)

	if strings.Contains(mimeType, "vnd.tidal.bts") {
		return parseBTS(content)
	}
	if strings.Contains(mimeType, "dash+xml") {
		return parseDASH(content)
	}
	return models.StreamInfo{}, fmt.Errorf("unknown manifest type: %s", mimeType)
}

// parseBTS parses a BTS (direct URL) manifest.
func parseBTS(content string) (models.StreamInfo, error) {
	var manifest struct {
		URLs  []string `json:"urls"`
		Codec string   `json:"codecs"`
	}
	if err := json.Unmarshal([]byte(content), &manifest); err != nil {
		return models.StreamInfo{}, fmt.Errorf("parse BTS JSON: %w", err)
	}
	if len(manifest.URLs) == 0 {
		return models.StreamInfo{}, fmt.Errorf("BTS manifest has no URLs")
	}

	codec := "flac"
	c := strings.ToLower(manifest.Codec)
	if strings.Contains(c, "aac") || strings.Contains(c, "mp4a") {
		codec = "aac"
	}

	return models.StreamInfo{
		URLs:  []string{manifest.URLs[0]},
		IsRaw: true,
		Codec: codec,
	}, nil
}

// parseDASH parses a DASH MPD manifest with segment URLs.
func parseDASH(content string) (models.StreamInfo, error) {
	// Extract base URL
	baseURL := ""
	if m := reBaseURL.FindStringSubmatch(content); len(m) > 1 {
		baseURL = m[1]
	}

	// Extract initialization segment
	initSeg := ""
	if m := reInit.FindStringSubmatch(content); len(m) > 1 {
		initSeg = m[1]
	}

	// Extract media template
	mediaTemplate := ""
	if m := reMedia.FindStringSubmatch(content); len(m) > 1 {
		mediaTemplate = m[1]
	}

	// Count segments from <S> elements
	totalSegments := 0
	for _, m := range reSegments.FindAllStringSubmatch(content, -1) {
		r := 0
		if m[1] != "" {
			r, _ = strconv.Atoi(m[1])
		}
		totalSegments += r + 1
	}

	if totalSegments == 0 {
		return models.StreamInfo{}, fmt.Errorf("DASH manifest: no segments found")
	}

	// Build URL list
	urls := make([]string, 0, totalSegments+1)
	if initSeg != "" {
		urls = append(urls, resolveURL(baseURL, initSeg))
	}
	for i := 1; i <= totalSegments; i++ {
		seg := strings.ReplaceAll(mediaTemplate, "$Number$", strconv.Itoa(i))
		urls = append(urls, resolveURL(baseURL, seg))
	}

	// Detect codec
	codec := "flac"
	lower := strings.ToLower(content)
	if strings.Contains(lower, "mp4a") {
		codec = "aac"
	} else if strings.Contains(lower, "flac") || strings.Contains(lower, "flac") {
		codec = "flac"
	}

	return models.StreamInfo{
		URLs:  urls,
		IsRaw: false,
		Codec: codec,
	}, nil
}

// resolveURL prepends baseURL if the segment path is relative.
func resolveURL(baseURL, path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if baseURL != "" {
		// Ensure no double slashes
		base := strings.TrimRight(baseURL, "/")
		path = strings.TrimLeft(path, "/")
		return base + "/" + path
	}
	return path
}
