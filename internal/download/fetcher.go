package download

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"

	"github.com/mrpir/hifi-tui/internal/logger"
	"github.com/mrpir/hifi-tui/internal/models"
	"golang.org/x/sync/semaphore"
)

const (
	streamBufSize = 256 * 1024 // 256KB for BTS streaming
	segBufSize    = 64 * 1024  // 64KB for individual segments
	largeBufSize  = 1024 * 1024 // 1MB for segment assembly
)

// Buffer pools for zero-alloc steady state.
var (
	streamBufPool = sync.Pool{New: func() interface{} { return make([]byte, streamBufSize) }}
	segBufPool    = sync.Pool{New: func() interface{} { return make([]byte, segBufSize) }}
	largeBufPool  = sync.Pool{New: func() interface{} { return make([]byte, largeBufSize) }}
)

// Fetcher handles downloading audio data from CDN URLs.
type Fetcher struct {
	client     *http.Client
	segmentSem *semaphore.Weighted
}

// NewFetcher creates a fetcher with the given HTTP client and segment concurrency.
func NewFetcher(client *http.Client, maxSegments int64) *Fetcher {
	return &Fetcher{
		client:     client,
		segmentSem: semaphore.NewWeighted(maxSegments),
	}
}

// DownloadResult holds the output of a download operation.
type DownloadResult struct {
	Path      string
	NeedsRemux bool
	Codec     string
}

// Download fetches audio for a track based on its stream info.
// Returns the path to the downloaded file (may be temporary if remux needed).
func (f *Fetcher) Download(ctx context.Context, info models.StreamInfo, destPath string, onProgress func(downloaded int64)) (DownloadResult, error) {
	if info.IsRaw {
		return f.downloadBTS(ctx, info.URLs[0], destPath, info.Codec, onProgress)
	}
	return f.downloadDASH(ctx, info, destPath, onProgress)
}

// downloadBTS streams a single BTS URL directly to disk.
func (f *Fetcher) downloadBTS(ctx context.Context, url, destPath, codec string, onProgress func(int64)) (DownloadResult, error) {
	ext := ".flac"
	if codec == "aac" {
		ext = ".m4a"
	}
	finalPath := destPath + ext

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("create BTS request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("BTS download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return DownloadResult{}, fmt.Errorf("BTS download HTTP %d", resp.StatusCode)
	}

	dir := destPath[:max(0, lastSlash(destPath))]
	if dir != "" {
		os.MkdirAll(dir, 0755)
	}

	file, err := os.Create(finalPath)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	bw := bufio.NewWriterSize(file, streamBufSize)
	buf := streamBufPool.Get().([]byte)
	defer streamBufPool.Put(buf)

	var total int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := bw.Write(buf[:n]); writeErr != nil {
				return DownloadResult{}, fmt.Errorf("write: %w", writeErr)
			}
			total += int64(n)
			if onProgress != nil {
				onProgress(total)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return DownloadResult{}, fmt.Errorf("read: %w", readErr)
		}
	}
	if err := bw.Flush(); err != nil {
		return DownloadResult{}, fmt.Errorf("flush: %w", err)
	}

	return DownloadResult{
		Path:       finalPath,
		NeedsRemux: false,
		Codec:      codec,
	}, nil
}

// downloadDASH downloads DASH segments in parallel and assembles them.
func (f *Fetcher) downloadDASH(ctx context.Context, info models.StreamInfo, destPath string, onProgress func(int64)) (DownloadResult, error) {
	needsRemux := info.Codec == "flac"
	var tmpPath, finalPath string
	if needsRemux {
		tmpPath = destPath + ".m4a.tmp"
		finalPath = destPath + ".flac"
	} else {
		tmpPath = destPath + ".m4a"
		finalPath = tmpPath
	}

	dir := destPath[:max(0, lastSlash(destPath))]
	if dir != "" {
		os.MkdirAll(dir, 0755)
	}

	// Download all segments in parallel
	type segResult struct {
		index int
		data  []byte
		err   error
	}

	results := make([]segResult, len(info.URLs))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var totalBytes int64

	for i, url := range info.URLs {
		wg.Add(1)
		go func(idx int, segURL string) {
			defer wg.Done()
			if err := f.segmentSem.Acquire(ctx, 1); err != nil {
				results[idx] = segResult{index: idx, err: err}
				return
			}
			defer f.segmentSem.Release(1)

			data, err := f.fetchSegment(ctx, segURL)
			results[idx] = segResult{index: idx, data: data, err: err}

			if err == nil && onProgress != nil {
				mu.Lock()
				totalBytes += int64(len(data))
				current := totalBytes
				mu.Unlock()
				onProgress(current)
			}
		}(i, url)
	}
	wg.Wait()

	// Check for errors
	for _, r := range results {
		if r.err != nil {
			return DownloadResult{}, fmt.Errorf("segment %d: %w", r.index, r.err)
		}
	}

	// Sort and assemble
	sort.Slice(results, func(i, j int) bool { return results[i].index < results[j].index })

	file, err := os.Create(tmpPath)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("create temp: %w", err)
	}
	bw := bufio.NewWriterSize(file, largeBufSize)
	for _, r := range results {
		if _, err := bw.Write(r.data); err != nil {
			file.Close()
			return DownloadResult{}, fmt.Errorf("write segment: %w", err)
		}
	}
	if err := bw.Flush(); err != nil {
		file.Close()
		return DownloadResult{}, fmt.Errorf("flush: %w", err)
	}
	file.Close()

	if needsRemux {
		return DownloadResult{
			Path:       tmpPath,
			NeedsRemux: true,
			Codec:      info.Codec,
		}, nil
	}

	return DownloadResult{
		Path:       finalPath,
		NeedsRemux: false,
		Codec:      info.Codec,
	}, nil
}

// fetchSegment downloads a single DASH segment.
func (f *Fetcher) fetchSegment(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' || s[i] == '\\' {
			return i
		}
	}
	return -1
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// FileExists checks if a file exists and is larger than minSize bytes.
func FileExists(path string, minSize int64) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Size() > minSize
}

// CheckExistingFile checks if a track file already exists in either FLAC or M4A format.
func CheckExistingFile(basePath string) (string, bool) {
	for _, ext := range []string{".flac", ".m4a"} {
		p := basePath + ext
		if FileExists(p, 1000) {
			return p, true
		}
	}
	return "", false
}

// LogFetcher provides a logger-compatible interface for the fetcher.
func (f *Fetcher) LogStats() {
	logger.Log.Debug("fetcher stats logged")
}
