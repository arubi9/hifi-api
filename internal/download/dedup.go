package download

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	json "github.com/goccy/go-json"
	"github.com/mrpir/hifi-tui/internal/logger"
)

// DedupStore tracks downloaded track IDs to avoid re-downloading.
type DedupStore struct {
	mu    sync.RWMutex
	ids   map[string]dedupEntry
	dirty bool
	path  string
}

type dedupEntry struct {
	Ts float64 `json:"ts"`
}

// NewDedupStore loads or creates the downloaded IDs store.
func NewDedupStore(configDir string) *DedupStore {
	ds := &DedupStore{
		ids:  make(map[string]dedupEntry),
		path: filepath.Join(configDir, "downloaded_ids.json"),
	}
	ds.load()
	return ds
}

// Contains checks if a track has been downloaded and the file still exists.
func (ds *DedupStore) Contains(trackID int, checkPath string) bool {
	key := strconv.Itoa(trackID)

	ds.mu.RLock()
	_, ok := ds.ids[key]
	ds.mu.RUnlock()

	if !ok {
		return false
	}

	// Verify file exists and has content
	if checkPath != "" {
		info, err := os.Stat(checkPath)
		if err == nil && info.Size() > 1000 {
			return true
		}
		// Check alternate extension
		ext := filepath.Ext(checkPath)
		var altPath string
		if ext == ".flac" {
			altPath = checkPath[:len(checkPath)-len(ext)] + ".m4a"
		} else {
			altPath = checkPath[:len(checkPath)-len(ext)] + ".flac"
		}
		info, err = os.Stat(altPath)
		if err == nil && info.Size() > 1000 {
			return true
		}
	}

	return true // ID is tracked even if file missing
}

// Add marks a track as downloaded.
func (ds *DedupStore) Add(trackID int) {
	key := strconv.Itoa(trackID)
	ds.mu.Lock()
	ds.ids[key] = dedupEntry{Ts: float64(time.Now().Unix())}
	ds.dirty = true
	ds.mu.Unlock()
}

// Flush writes dirty state to disk atomically.
func (ds *DedupStore) Flush() {
	ds.mu.RLock()
	if !ds.dirty {
		ds.mu.RUnlock()
		return
	}
	data, err := json.Marshal(ds.ids)
	ds.mu.RUnlock()

	if err != nil {
		logger.Log.Warn("failed to marshal dedup store", "err", err)
		return
	}

	tmp := ds.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		logger.Log.Warn("failed to write dedup store", "err", err)
		return
	}
	if err := os.Rename(tmp, ds.path); err != nil {
		logger.Log.Warn("failed to rename dedup store", "err", err)
		return
	}

	ds.mu.Lock()
	ds.dirty = false
	ds.mu.Unlock()
}

func (ds *DedupStore) load() {
	data, err := os.ReadFile(ds.path)
	if err != nil {
		return
	}
	var entries map[string]dedupEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		logger.Log.Warn("failed to parse dedup store", "err", err)
		return
	}
	ds.ids = entries
}
