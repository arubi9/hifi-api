package cache

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/mrpir/hifi-tui/internal/logger"
	"github.com/mrpir/hifi-tui/internal/models"
)

const maxCoverEntries = 50

// CoverCache caches cover art bytes in memory, keyed by cover UUID.
type CoverCache struct {
	mu      sync.RWMutex
	entries map[string][]byte // nil value means "failed, don't retry"
	order   []string
	client  *http.Client
}

// NewCoverCache creates a cover art cache.
func NewCoverCache(client *http.Client) *CoverCache {
	return &CoverCache{
		entries: make(map[string][]byte),
		client:  client,
	}
}

// Get returns cached cover art bytes, fetching if needed.
// Returns nil if the cover cannot be fetched.
func (cc *CoverCache) Get(ctx context.Context, coverUUID string) []byte {
	if coverUUID == "" {
		return nil
	}

	cc.mu.RLock()
	data, ok := cc.entries[coverUUID]
	cc.mu.RUnlock()
	if ok {
		return data // may be nil (failed)
	}

	// Fetch
	url := models.CoverURL(coverUUID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cc.store(coverUUID, nil)
		return nil
	}

	resp, err := cc.client.Do(req)
	if err != nil {
		logger.Log.Debug("cover fetch failed", "uuid", coverUUID, "err", err)
		cc.store(coverUUID, nil)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		cc.store(coverUUID, nil)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		cc.store(coverUUID, nil)
		return nil
	}

	cc.store(coverUUID, body)
	return body
}

func (cc *CoverCache) store(key string, data []byte) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.entries[key] = data
	cc.order = append(cc.order, key)

	// FIFO eviction
	for len(cc.entries) > maxCoverEntries && len(cc.order) > 0 {
		old := cc.order[0]
		cc.order = cc.order[1:]
		delete(cc.entries, old)
	}
}

// SaveFolderJpg saves cover art as folder.jpg in the album directory.
func SaveFolderJpg(dir string, data []byte) {
	if data == nil || len(data) == 0 {
		return
	}
	p := filepath.Join(dir, "folder.jpg")
	if _, err := os.Stat(p); err == nil {
		return // already exists
	}
	os.MkdirAll(dir, 0755)
	os.WriteFile(p, data, 0644)
}
