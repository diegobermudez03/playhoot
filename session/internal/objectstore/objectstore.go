// Package objectstore is Session Runtime's narrow port onto private object
// storage, where immutable game version content (scripts and assets) lives.
// The database keeps only a Locator for each object; the bytes themselves are
// fetched and verified through this package, so nothing above it depends on
// which storage provider is in use.
package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

var (
	// ErrNotFound is returned by Store.Get when no object exists at the key.
	ErrNotFound = errors.New("object not found")
	// ErrHashMismatch is returned when fetched content does not match the
	// hash or size its Locator recorded - the stored object is not the
	// content the version was published with.
	ErrHashMismatch = errors.New("object content does not match its recorded hash")
)

// Store is the private object storage a Session Runtime reads game version
// content from. Keys are opaque to Store; the layout is chosen by whoever
// writes content (see LocatorFor).
type Store interface {
	// Get returns the whole content stored at key, or ErrNotFound.
	Get(ctx context.Context, key string) ([]byte, error)
	// Put stores content at key with the given content type, replacing any
	// existing object. Content-addressed keys make replacement a no-op in
	// practice.
	Put(ctx context.Context, key string, content []byte, contentType string) error
	// PresignGet returns a read-only URL for exactly the object at key that
	// stops working after ttl. It is a bearer credential until it expires.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (SignedURL, error)
}

// SignedURL is a short-lived, read-only URL for one stored object.
type SignedURL struct {
	URL       string
	ExpiresAt time.Time
}

// Locator identifies one immutable stored object and lets a reader verify it:
// where it lives, the SHA-256 of its content (lowercase hex), and its size in
// bytes. No URL is ever part of a Locator.
type Locator struct {
	ObjectKey string
	SHA256    string
	Size      int64
}

// LocatorFor returns the Locator content would have if stored under prefix,
// content-addressed by its SHA-256 so identical bytes share one object and a
// published version always references immutable content.
func LocatorFor(prefix string, content []byte) Locator {
	sum := HashHex(content)
	return Locator{ObjectKey: prefix + "/" + sum, SHA256: sum, Size: int64(len(content))}
}

// HashHex returns content's SHA-256 as lowercase hex.
func HashHex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// PutContent stores content content-addressed under prefix and returns its
// Locator.
func PutContent(ctx context.Context, store Store, prefix string, content []byte, contentType string) (Locator, error) {
	locator := LocatorFor(prefix, content)
	if err := store.Put(ctx, locator.ObjectKey, content, contentType); err != nil {
		return Locator{}, err
	}
	return locator, nil
}
