package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/noamsto/resolved/internal/model"
)

const day = 24 * time.Hour

type entry struct {
	State     string    `json:"state"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
	FetchedAt time.Time `json:"fetched_at"`
}

// ttlFor is the state-tier freshness policy. UpdatedAt is stored so a recency
// branch can be added later without a schema change.
func ttlFor(e entry) time.Duration {
	switch e.State {
	case "closed", "merged":
		return 7 * day
	case "gone":
		return day
	default: // open / unknown
		return time.Hour
	}
}

// Cache is a JSON-file-backed map of reference key -> entry.
type Cache struct {
	mu      sync.Mutex
	path    string
	entries map[string]entry
	now     func() time.Time
}

// New loads (or starts) a cache rooted at dir.
func New(dir string) *Cache {
	c := &Cache{
		path:    filepath.Join(dir, "cache.json"),
		entries: map[string]entry{},
		now:     time.Now,
	}
	if data, err := os.ReadFile(c.path); err == nil {
		_ = json.Unmarshal(data, &c.entries)
	}
	return c
}

// Get returns the cached status if present and still fresh per ttlFor.
func (c *Cache) Get(key string) (model.Status, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return model.Status{}, false
	}
	if c.now().Sub(e.FetchedAt) >= ttlFor(e) {
		return model.Status{}, false
	}
	return model.Status{State: e.State, Title: e.Title, UpdatedAt: e.UpdatedAt}, true
}

// Put records a status and flushes the whole cache to disk.
func (c *Cache) Put(key string, s model.Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = entry{
		State:     s.State,
		Title:     s.Title,
		UpdatedAt: s.UpdatedAt,
		FetchedAt: c.now(),
	}
	c.flush()
}

// flush writes entries to disk. Caller must hold c.mu.
func (c *Cache) flush() {
	_ = os.MkdirAll(filepath.Dir(c.path), 0o755)
	data, err := json.MarshalIndent(c.entries, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(c.path, data, 0o644)
}
