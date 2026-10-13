package flag

import (
	"encoding/json"
	"sort"
	"sync"

	"github.com/VictoriaMetrics/fastcache"
)

// defaultCacheBytes is the fastcache arena size. fastcache grows into it lazily,
// so a small node never pays for the whole thing.
const defaultCacheBytes = 64 << 20

// Cache is the read path: fastcache holds one marshalled Flag per key, so an
// evaluation or an admin list never waits on sqlite.
//
// keys is the authoritative sorted key list, kept outside fastcache: fastcache
// can drop entries when its arena fills, and List must never go silently short
// because of it (a miss falls back to sqlite and refills).
//
// Every access goes through mu, reads under RLock and writes under Lock. Callers
// in Store hold Store.mu across these calls, so the order is always Store.mu
// then Cache.mu and never the other way round.
type Cache struct {
	mu   sync.RWMutex
	fc   *fastcache.Cache
	keys []string
}

func NewCache(maxBytes int) *Cache {
	if maxBytes <= 0 {
		maxBytes = defaultCacheBytes
	}
	return &Cache{fc: fastcache.New(maxBytes)}
}

// Get returns the cached flag. false means never set or evicted, not deleted.
func (c *Cache) Get(key string) (Flag, bool) {
	c.mu.RLock()
	raw := c.fc.Get(nil, []byte(key))
	c.mu.RUnlock()
	if len(raw) == 0 {
		return Flag{}, false
	}
	var f Flag
	if err := json.Unmarshal(raw, &f); err != nil {
		return Flag{}, false
	}
	return f, true
}

// Put stores a flag and keeps the key index sorted for List.
func (c *Cache) Put(f Flag) {
	if f.Key == "" {
		return
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fc.Set([]byte(f.Key), raw)
	i := sort.SearchStrings(c.keys, f.Key)
	if i == len(c.keys) || c.keys[i] != f.Key {
		c.keys = append(c.keys, "")
		copy(c.keys[i+1:], c.keys[i:])
		c.keys[i] = f.Key
	}
}

// Del drops a key, which is what a tombstone does to the read path.
func (c *Cache) Del(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fc.Del([]byte(key))
	i := sort.SearchStrings(c.keys, key)
	if i < len(c.keys) && c.keys[i] == key {
		c.keys = append(c.keys[:i], c.keys[i+1:]...)
	}
}

// Keys copies the sorted key list. List walks it, so the returned slice is the
// caller's to keep.
func (c *Cache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.keys))
	copy(out, c.keys)
	return out
}

// Len is the number of cached flags.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.keys)
}
