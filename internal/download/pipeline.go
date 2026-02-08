package download

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/mrpir/hifi-tui/internal/api"
	"github.com/mrpir/hifi-tui/internal/cache"
	"github.com/mrpir/hifi-tui/internal/logger"
	"github.com/mrpir/hifi-tui/internal/models"
	"golang.org/x/sync/semaphore"
)

// Downloader orchestrates the full download pipeline.
type Downloader struct {
	api         *api.Client
	fetcher     *Fetcher
	dedup       *DedupStore
	covers      *cache.CoverCache
	downloadDir string
	quality     string
	trackSem    *semaphore.Weighted
	postSem     *semaphore.Weighted
	coverSem    *semaphore.Weighted
	progress    *Progress
}

// NewDownloader creates a download pipeline.
func NewDownloader(apiClient *api.Client, dedup *DedupStore, covers *cache.CoverCache, downloadDir, quality string, parallelCount int) *Downloader {
	fetcher := NewFetcher(apiClient.CDNClient(), int64(parallelCount)*4)

	return &Downloader{
		api:         apiClient,
		fetcher:     fetcher,
		dedup:       dedup,
		covers:      covers,
		downloadDir: downloadDir,
		quality:     quality,
		trackSem:    semaphore.NewWeighted(int64(parallelCount)),
		postSem:     semaphore.NewWeighted(int64(runtime.NumCPU())),
		coverSem:    semaphore.NewWeighted(4),
		progress:    NewProgress(),
	}
}

// Progress returns the shared progress tracker.
func (d *Downloader) Progress() *Progress {
	return d.progress
}

// UpdateSettings reconfigures the downloader.
func (d *Downloader) UpdateSettings(downloadDir, quality string, parallelCount int) {
	d.downloadDir = downloadDir
	d.quality = quality
	d.trackSem = semaphore.NewWeighted(int64(parallelCount))
	d.fetcher = NewFetcher(d.api.CDNClient(), int64(parallelCount)*4)
}

// TrackResult holds the result of downloading a single track.
type TrackResult struct {
	Track models.Track
	Path  string
	Error error
}

// DownloadAlbum downloads all tracks of an album using the batch endpoint.
func (d *Downloader) DownloadAlbum(ctx context.Context, albumID int, logItem *models.DownloadItem) ([]TrackResult, error) {
	d.progress.SetStatus("downloading")
	d.progress.ResetTimer()

	// Stage 1: Batch manifest resolution
	result, err := d.api.BatchAlbumTracks(ctx, albumID)
	if err != nil {
		return nil, fmt.Errorf("batch album: %w", err)
	}

	if logItem != nil {
		logItem.Total = len(result.Tracks)
	}
	d.progress.SetTracksTotal(len(result.Tracks))

	// Fetch cover art once for the album
	var coverData []byte
	if result.Album.Cover != "" {
		if err := d.coverSem.Acquire(ctx, 1); err == nil {
			coverData = d.covers.Get(ctx, result.Album.Cover)
			d.coverSem.Release(1)
		}
	}

	// Save folder.jpg
	albumDir := d.albumDir(result.Album)
	if coverData != nil {
		cache.SaveFolderJpg(albumDir, coverData)
	}

	// Stage 2-4: Fan-out download + remux + metadata
	results := d.downloadEntries(ctx, result.Tracks, result.Album, coverData, logItem)

	// Stage 5: Flush dedup
	d.dedup.Flush()
	d.progress.SetStatus("completed")

	return results, nil
}

// DownloadTracks downloads specific tracks using batch endpoint.
func (d *Downloader) DownloadTracks(ctx context.Context, tracks []models.Track, logItem *models.DownloadItem) ([]TrackResult, error) {
	d.progress.SetStatus("downloading")
	d.progress.ResetTimer()

	if logItem != nil {
		logItem.Total = len(tracks)
	}
	d.progress.SetTracksTotal(len(tracks))

	// Collect track IDs
	ids := make([]int, len(tracks))
	for i, t := range tracks {
		ids[i] = t.ID
	}

	// Batch resolve manifests
	entries, _, err := d.api.BatchTracks(ctx, ids)
	if err != nil {
		// Fallback to individual resolution
		logger.Log.Warn("batch tracks failed, falling back to individual", "err", err)
		entries = d.fallbackResolve(ctx, tracks)
	}

	// Group by album for cover art sharing
	results := d.downloadMixedEntries(ctx, entries, logItem)

	d.dedup.Flush()
	d.progress.SetStatus("completed")
	return results, nil
}

// DownloadArtist downloads all albums of an artist.
func (d *Downloader) DownloadArtist(ctx context.Context, artistID int, logItem *models.DownloadItem) ([]TrackResult, error) {
	d.progress.SetStatus("downloading")
	d.progress.ResetTimer()

	albums, entries, _, err := d.api.BatchArtistDiscography(ctx, artistID)
	if err != nil {
		return nil, fmt.Errorf("batch artist: %w", err)
	}

	if logItem != nil {
		logItem.Total = len(entries)
	}
	d.progress.SetTracksTotal(len(entries))

	// Fetch cover art per album
	coverMap := make(map[string][]byte)
	var coverMu sync.Mutex
	var wg sync.WaitGroup
	for _, album := range albums {
		if album.Cover == "" {
			continue
		}
		wg.Add(1)
		go func(cover string, albumDir string) {
			defer wg.Done()
			if err := d.coverSem.Acquire(ctx, 1); err != nil {
				return
			}
			defer d.coverSem.Release(1)
			data := d.covers.Get(ctx, cover)
			if data != nil {
				coverMu.Lock()
				coverMap[cover] = data
				coverMu.Unlock()
				cache.SaveFolderJpg(albumDir, data)
			}
		}(album.Cover, d.albumDir(album))
	}
	wg.Wait()

	// Download all tracks
	var results []TrackResult
	var mu sync.Mutex

	for i := range entries {
		entry := entries[i]
		if err := d.trackSem.Acquire(ctx, 1); err != nil {
			break
		}
		wg.Add(1)
		go func(e api.BatchTrackEntry) {
			defer wg.Done()
			defer d.trackSem.Release(1)

			coverMu.Lock()
			cover := coverMap[e.Track.Cover]
			coverMu.Unlock()

			tr := d.processEntry(ctx, e, cover)
			mu.Lock()
			results = append(results, tr)
			if tr.Error == nil {
				if logItem != nil {
					logItem.Success++
					logItem.Progress++
				}
			} else {
				if logItem != nil {
					logItem.Failed++
					logItem.Progress++
				}
			}
			mu.Unlock()
		}(entry)
	}
	wg.Wait()

	d.dedup.Flush()
	d.progress.SetStatus("completed")
	return results, nil
}

// downloadEntries downloads batch entries for a single album.
func (d *Downloader) downloadEntries(ctx context.Context, entries []api.BatchTrackEntry, album models.Album, coverData []byte, logItem *models.DownloadItem) []TrackResult {
	var results []TrackResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := range entries {
		entry := entries[i]
		if err := d.trackSem.Acquire(ctx, 1); err != nil {
			break
		}
		wg.Add(1)
		go func(e api.BatchTrackEntry) {
			defer wg.Done()
			defer d.trackSem.Release(1)

			tr := d.processEntry(ctx, e, coverData)
			mu.Lock()
			results = append(results, tr)
			if tr.Error == nil {
				if logItem != nil {
					logItem.Success++
					logItem.Progress++
				}
			} else {
				if logItem != nil {
					logItem.Failed++
					logItem.Progress++
				}
			}
			mu.Unlock()
		}(entry)
	}
	wg.Wait()

	return results
}

// downloadMixedEntries downloads tracks that may span multiple albums.
func (d *Downloader) downloadMixedEntries(ctx context.Context, entries []api.BatchTrackEntry, logItem *models.DownloadItem) []TrackResult {
	var results []TrackResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Cover art cache per cover UUID
	coverMap := make(map[string][]byte)
	var coverMu sync.Mutex

	for i := range entries {
		entry := entries[i]
		if err := d.trackSem.Acquire(ctx, 1); err != nil {
			break
		}
		wg.Add(1)
		go func(e api.BatchTrackEntry) {
			defer wg.Done()
			defer d.trackSem.Release(1)

			// Get cover art (cached per UUID)
			var cover []byte
			if e.Track.Cover != "" {
				coverMu.Lock()
				c, ok := coverMap[e.Track.Cover]
				coverMu.Unlock()
				if ok {
					cover = c
				} else {
					if err := d.coverSem.Acquire(ctx, 1); err == nil {
						cover = d.covers.Get(ctx, e.Track.Cover)
						d.coverSem.Release(1)
						coverMu.Lock()
						coverMap[e.Track.Cover] = cover
						coverMu.Unlock()
					}
				}
			}

			tr := d.processEntry(ctx, e, cover)
			mu.Lock()
			results = append(results, tr)
			if tr.Error == nil && logItem != nil {
				logItem.Success++
				logItem.Progress++
			} else if tr.Error != nil && logItem != nil {
				logItem.Failed++
				logItem.Progress++
			}
			mu.Unlock()
		}(entry)
	}
	wg.Wait()

	return results
}

// processEntry handles a single track: dedup check → download → remux → metadata.
func (d *Downloader) processEntry(ctx context.Context, entry api.BatchTrackEntry, coverData []byte) TrackResult {
	track := entry.Track
	d.progress.SetTrack(track.Title, track.Artist)

	// Build file path
	basePath := d.trackBasePath(track)

	// Dedup check
	if existing, ok := CheckExistingFile(basePath); ok {
		d.dedup.Add(track.ID)
		d.progress.IncTracksDone()
		logger.Log.Debug("skipped (exists)", "track", track.Title)
		return TrackResult{Track: track, Path: existing}
	}

	if entry.Error != "" {
		d.progress.IncTracksFailed()
		return TrackResult{Track: track, Error: fmt.Errorf("batch error: %s", entry.Error)}
	}

	if entry.Manifest == "" {
		// Fallback to single track endpoint
		single, err := d.api.GetSingleTrack(ctx, track.ID)
		if err != nil {
			d.progress.IncTracksFailed()
			return TrackResult{Track: track, Error: err}
		}
		entry = single
		entry.Track = track
	}

	// Parse manifest
	info, err := api.ParseManifest(entry.Manifest, entry.MimeType)
	if err != nil {
		d.progress.IncTracksFailed()
		return TrackResult{Track: track, Error: fmt.Errorf("parse manifest: %w", err)}
	}

	// Download
	result, err := d.fetcher.Download(ctx, info, basePath, func(downloaded int64) {
		d.progress.AddBytes(downloaded)
	})
	if err != nil {
		d.progress.IncTracksFailed()
		return TrackResult{Track: track, Error: fmt.Errorf("download: %w", err)}
	}

	// Remux if needed (DASH+FLAC)
	finalPath := result.Path
	if result.NeedsRemux {
		d.progress.SetStatus("remuxing")
		if err := d.postSem.Acquire(ctx, 1); err == nil {
			outPath := basePath + ".flac"
			if err := Remux(ctx, result.Path, outPath); err != nil {
				logger.Log.Warn("remux failed", "track", track.Title, "err", err)
				// File was renamed to .m4a by Remux fallback
				finalPath = basePath + ".m4a"
			} else {
				finalPath = outPath
			}
			d.postSem.Release(1)
		}
		d.progress.SetStatus("downloading")
	}

	// Embed metadata
	if err := d.postSem.Acquire(ctx, 1); err == nil {
		if err := EmbedMetadata(ctx, finalPath, track, coverData); err != nil {
			logger.Log.Debug("metadata embed failed", "track", track.Title, "err", err)
		}
		d.postSem.Release(1)
	}

	// Mark downloaded
	d.dedup.Add(track.ID)
	d.progress.IncTracksDone()

	return TrackResult{Track: track, Path: finalPath}
}

// fallbackResolve resolves tracks individually when batch fails.
func (d *Downloader) fallbackResolve(ctx context.Context, tracks []models.Track) []api.BatchTrackEntry {
	entries := make([]api.BatchTrackEntry, len(tracks))
	var wg sync.WaitGroup
	sem := semaphore.NewWeighted(8)

	for i, t := range tracks {
		wg.Add(1)
		go func(idx int, track models.Track) {
			defer wg.Done()
			if err := sem.Acquire(ctx, 1); err != nil {
				entries[idx] = api.BatchTrackEntry{Track: track, Error: "context cancelled"}
				return
			}
			defer sem.Release(1)

			entry, err := d.api.GetSingleTrack(ctx, track.ID)
			if err != nil {
				entries[idx] = api.BatchTrackEntry{Track: track, Error: err.Error()}
			} else {
				entry.Track = track
				entries[idx] = entry
			}
		}(i, t)
	}
	wg.Wait()
	return entries
}

// trackBasePath builds the file path without extension.
func (d *Downloader) trackBasePath(track models.Track) string {
	artist := models.SanitizeFilename(track.Artist, 50)
	album := models.SanitizeFilename(track.Album, 50)
	title := models.SanitizeFilename(track.Title, 50)

	if artist == "" {
		artist = "Unknown Artist"
	}
	if album == "" {
		album = "Unknown Album"
	}

	filename := title
	if track.TrackNumber > 0 {
		filename = fmt.Sprintf("%02d - %s", track.TrackNumber, title)
	}

	return filepath.Join(d.downloadDir, artist, album, filename)
}

// albumDir returns the album directory path.
func (d *Downloader) albumDir(album models.Album) string {
	artist := models.SanitizeFilename(album.Artist, 50)
	albumName := models.SanitizeFilename(album.Title, 50)
	if artist == "" {
		artist = "Unknown Artist"
	}
	if albumName == "" {
		albumName = "Unknown Album"
	}
	return filepath.Join(d.downloadDir, artist, albumName)
}

// SetDedup is used for testing.
func (d *Downloader) SetDedup(ds *DedupStore) {
	d.dedup = ds
}
