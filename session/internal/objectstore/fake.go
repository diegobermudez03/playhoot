package objectstore

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Fake is an in-memory Store for tests. It is safe for concurrent use.
type Fake struct {
	mu      sync.Mutex
	objects map[string]fakeObject
	gets    map[string]int

	// GetErr, if set, is returned by every Get, letting a test simulate a
	// storage outage.
	GetErr error
}

type fakeObject struct {
	content     []byte
	contentType string
}

// NewFake constructs an empty Fake.
func NewFake() *Fake {
	return &Fake{objects: make(map[string]fakeObject), gets: make(map[string]int)}
}

// Get implements Store.
func (f *Fake) Get(ctx context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets[key]++
	if f.GetErr != nil {
		return nil, f.GetErr
	}
	object, ok := f.objects[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), object.content...), nil
}

// Put implements Store.
func (f *Fake) Put(ctx context.Context, key string, content []byte, contentType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = fakeObject{content: append([]byte(nil), content...), contentType: contentType}
	return nil
}

// PresignGet implements Store. The returned URL is not fetchable; it only
// encodes the key and expiry so a test can assert what was signed.
func (f *Fake) PresignGet(ctx context.Context, key string, ttl time.Duration) (SignedURL, error) {
	expiresAt := time.Now().UTC().Add(ttl)
	return SignedURL{URL: fmt.Sprintf("fake://%s?expires=%d", key, expiresAt.Unix()), ExpiresAt: expiresAt}, nil
}

// GetCount reports how many times Get was called for key, letting a test
// prove a cache avoided a second fetch.
func (f *Fake) GetCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gets[key]
}

// Corrupt overwrites the content stored at key without changing its key,
// simulating an object that no longer matches the hash it was published
// with.
func (f *Fake) Corrupt(key string, content []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	object := f.objects[key]
	object.content = append([]byte(nil), content...)
	f.objects[key] = object
}
