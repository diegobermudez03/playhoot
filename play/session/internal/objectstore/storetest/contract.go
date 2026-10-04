// Package storetest holds the behavioral contract every objectstore.Store
// implementation must satisfy, so the in-memory fake and the real adapter are
// held to the same suite.
package storetest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/session/internal/objectstore"
	"github.com/stretchr/testify/require"
)

// RunContract runs the shared Store contract against newStore, which must
// return a store isolated enough that its keys do not collide with other
// concurrent runs.
func RunContract(t *testing.T, newStore func(t *testing.T) objectstore.Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("put then get round trips content", func(t *testing.T) {
		store := newStore(t)
		content := []byte("function backend() {}")
		locator := objectstore.LocatorFor("scripts", content)
		require.NoError(t, store.Put(ctx, locator.ObjectKey, content, "text/javascript"))

		got, err := store.Get(ctx, locator.ObjectKey)
		require.NoError(t, err)
		require.Equal(t, content, got)
	})

	t.Run("get of a missing key returns ErrNotFound", func(t *testing.T) {
		store := newStore(t)
		_, err := store.Get(ctx, "scripts/does-not-exist")
		require.True(t, errors.Is(err, objectstore.ErrNotFound), "got %v", err)
	})

	t.Run("put replaces existing content at the same key", func(t *testing.T) {
		store := newStore(t)
		require.NoError(t, store.Put(ctx, "scripts/k", []byte("one"), "text/plain"))
		require.NoError(t, store.Put(ctx, "scripts/k", []byte("two"), "text/plain"))
		got, err := store.Get(ctx, "scripts/k")
		require.NoError(t, err)
		require.Equal(t, []byte("two"), got)
	})

	t.Run("presign of a stored object returns a URL expiring after ttl", func(t *testing.T) {
		store := newStore(t)
		require.NoError(t, store.Put(ctx, "assets/a", []byte("png-bytes"), "image/png"))
		before := time.Now().UTC()
		signed, err := store.PresignGet(ctx, "assets/a", 2*time.Minute)
		require.NoError(t, err)
		require.NotEmpty(t, signed.URL)
		require.True(t, strings.Contains(signed.URL, "assets/a"), "URL should reference the object key")
		require.WithinDuration(t, before.Add(2*time.Minute), signed.ExpiresAt, 5*time.Second)
	})
}
