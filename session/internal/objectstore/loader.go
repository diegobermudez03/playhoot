package objectstore

import (
	"context"
	"fmt"
	"sync"
)

// DefaultLoaderEntries bounds how many distinct verified objects a Loader
// keeps in memory when no other bound is given.
const DefaultLoaderEntries = 256

// Loader fetches content by Locator, verifies it against the Locator's
// recorded hash and size, and caches the verified bytes keyed by hash. A
// cache keyed by content hash never needs invalidating: identical hashes are
// identical bytes, and a published version's content never changes.
type Loader struct {
	store      Store
	maxEntries int

	mu    sync.Mutex
	cache map[string][]byte
	order []string
}

// NewLoader constructs a Loader over store keeping at most maxEntries
// verified objects in memory (DefaultLoaderEntries if maxEntries < 1).
func NewLoader(store Store, maxEntries int) *Loader {
	if maxEntries < 1 {
		maxEntries = DefaultLoaderEntries
	}
	return &Loader{store: store, maxEntries: maxEntries, cache: make(map[string][]byte)}
}

// Load returns the verified content locator refers to, from memory when
// already loaded. A missing object returns an error wrapping ErrNotFound; a
// fetched object whose hash or size differs from locator's returns an error
// wrapping ErrHashMismatch and is never cached or returned.
func (l *Loader) Load(ctx context.Context, locator Locator) ([]byte, error) {
	l.mu.Lock()
	cached, ok := l.cache[locator.SHA256]
	l.mu.Unlock()
	if ok {
		return cached, nil
	}

	content, err := l.store.Get(ctx, locator.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("loading object %q: %w", locator.ObjectKey, err)
	}
	if int64(len(content)) != locator.Size || HashHex(content) != locator.SHA256 {
		return nil, fmt.Errorf("loading object %q: %w", locator.ObjectKey, ErrHashMismatch)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.cache[locator.SHA256]; !exists {
		if len(l.order) >= l.maxEntries {
			evict := l.order[0]
			l.order = l.order[1:]
			delete(l.cache, evict)
		}
		l.cache[locator.SHA256] = content
		l.order = append(l.order, locator.SHA256)
	}
	return content, nil
}
