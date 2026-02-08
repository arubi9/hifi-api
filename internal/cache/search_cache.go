package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	json "github.com/goccy/go-json"
	"github.com/mrpir/hifi-tui/internal/logger"
)

const (
	searchCacheTTL = 1800 // 30 minutes
	maxCachedKeys  = 200
)

// SearchCache provides a TTL + LRU search result cache, persisted to disk.
type SearchCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	order   []string // LRU order (most recent at end)
	path    string
}

type cacheEntry struct {
	Ts   float64         `json:"ts"`
	Data json.RawMessage `json:"data"`
}

// NewSearchCache loads or creates a search cache.
func NewSearchCache(configDir string) *SearchCache {
	sc := &SearchCache{
		entries: make(map[string]*cacheEntry),
		path:    filepath.Join(configDir, "search_cache.json"),
	}
	sc.load()
	return sc
}

// Key builds a cache key from kind, query, and limit.
func Key(kind, query string, limit int) string {
	return fmt.Sprintf("%s:%s:%d", kind, query, limit)
}

// Get retrieves a cached entry. Returns nil if not found or expired.
func (sc *SearchCache) Get(key string) json.RawMessage {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	e, ok := sc.entries[key]
	if !ok {
		return nil
	}
	if time.Now().Unix()-int64(e.Ts) > searchCacheTTL {
		return nil
	}
	return e.Data
}

// Set stores data in the cache.
func (sc *SearchCache) Set(key string, data json.RawMessage) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.entries[key] = &cacheEntry{
		Ts:   float64(time.Now().Unix()),
		Data: data,
	}

	// Update LRU order
	sc.removeFromOrder(key)
	sc.order = append(sc.order, key)

	// Evict oldest if over limit
	for len(sc.entries) > maxCachedKeys && len(sc.order) > 0 {
		old := sc.order[0]
		sc.order = sc.order[1:]
		delete(sc.entries, old)
	}
}

func (sc *SearchCache) removeFromOrder(key string) {
	for i, k := range sc.order {
		if k == key {
			sc.order = append(sc.order[:i], sc.order[i+1:]...)
			return
		}
	}
}

// Save persists cache to disk.
func (sc *SearchCache) Save() {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	data, err := json.Marshal(sc.entries)
	if err != nil {
		logger.Log.Warn("failed to marshal search cache", "err", err)
		return
	}
	tmp := sc.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		logger.Log.Warn("failed to write search cache", "err", err)
		return
	}
	os.Rename(tmp, sc.path)
}

func (sc *SearchCache) load() {
	data, err := os.ReadFile(sc.path)
	if err != nil {
		return
	}
	var entries map[string]*cacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		logger.Log.Warn("failed to parse search cache", "err", err)
		return
	}
	sc.entries = entries
	// Rebuild order from keys
	for k := range entries {
		sc.order = append(sc.order, k)
	}
}
