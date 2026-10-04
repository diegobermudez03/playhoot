package objectstore_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/diegobermudez03/playhoot/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/session/internal/objectstore/storetest"
	"github.com/stretchr/testify/require"
)

func TestFakeSatisfiesStoreContract(t *testing.T) {
	storetest.RunContract(t, func(t *testing.T) objectstore.Store { return objectstore.NewFake() })
}

// TestGCSSatisfiesStoreContract runs the same contract against a real
// bucket. It needs real GCP credentials and a scratch bucket, so it only
// runs when GCS_TEST_PROJECT_ID and GCS_TEST_BUCKET are set.
func TestGCSSatisfiesStoreContract(t *testing.T) {
	projectID, bucket := os.Getenv("GCS_TEST_PROJECT_ID"), os.Getenv("GCS_TEST_BUCKET")
	if projectID == "" || bucket == "" {
		t.Skip("GCS_TEST_PROJECT_ID and GCS_TEST_BUCKET not set; the GCS adapter is not exercised")
	}
	storetest.RunContract(t, func(t *testing.T) objectstore.Store {
		store, err := objectstore.NewGCS(context.Background(), projectID, bucket)
		require.NoError(t, err)
		t.Cleanup(func() { _ = store.Close() })
		return store
	})
}

func TestLocatorForIsContentAddressed(t *testing.T) {
	a := objectstore.LocatorFor("scripts", []byte("same"))
	b := objectstore.LocatorFor("scripts", []byte("same"))
	c := objectstore.LocatorFor("scripts", []byte("different"))

	require.Equal(t, a, b)
	require.NotEqual(t, a.ObjectKey, c.ObjectKey)
	require.Equal(t, int64(4), a.Size)
	require.Equal(t, "scripts/"+a.SHA256, a.ObjectKey)
}

func TestLoader(t *testing.T) {
	ctx := context.Background()

	t.Run("loads verified content and serves the second read from memory", func(t *testing.T) {
		store := objectstore.NewFake()
		locator, err := objectstore.PutContent(ctx, store, "scripts", []byte("function f() {}"), "text/javascript")
		require.NoError(t, err)
		loader := objectstore.NewLoader(store, 0)

		first, err := loader.Load(ctx, locator)
		require.NoError(t, err)
		second, err := loader.Load(ctx, locator)
		require.NoError(t, err)

		require.Equal(t, []byte("function f() {}"), first)
		require.Equal(t, first, second)
		require.Equal(t, 1, store.GetCount(locator.ObjectKey), "the second Load must not hit storage")
	})

	t.Run("a tampered object fails closed and is not cached", func(t *testing.T) {
		store := objectstore.NewFake()
		locator, err := objectstore.PutContent(ctx, store, "scripts", []byte("original"), "text/javascript")
		require.NoError(t, err)
		store.Corrupt(locator.ObjectKey, []byte("tampered"))
		loader := objectstore.NewLoader(store, 0)

		_, err = loader.Load(ctx, locator)
		require.True(t, errors.Is(err, objectstore.ErrHashMismatch), "got %v", err)

		_, err = loader.Load(ctx, locator)
		require.True(t, errors.Is(err, objectstore.ErrHashMismatch), "a failed load must not have been cached")
		require.Equal(t, 2, store.GetCount(locator.ObjectKey))
	})

	t.Run("same-length tampering is still caught by the hash", func(t *testing.T) {
		store := objectstore.NewFake()
		locator, err := objectstore.PutContent(ctx, store, "scripts", []byte("abcd"), "text/javascript")
		require.NoError(t, err)
		store.Corrupt(locator.ObjectKey, []byte("abce"))

		_, err = objectstore.NewLoader(store, 0).Load(ctx, locator)
		require.True(t, errors.Is(err, objectstore.ErrHashMismatch), "got %v", err)
	})

	t.Run("a missing object surfaces ErrNotFound", func(t *testing.T) {
		loader := objectstore.NewLoader(objectstore.NewFake(), 0)
		_, err := loader.Load(ctx, objectstore.LocatorFor("scripts", []byte("never stored")))
		require.True(t, errors.Is(err, objectstore.ErrNotFound), "got %v", err)
	})

	t.Run("a storage outage is returned and nothing is cached", func(t *testing.T) {
		store := objectstore.NewFake()
		locator, err := objectstore.PutContent(ctx, store, "scripts", []byte("x"), "text/javascript")
		require.NoError(t, err)
		outage := errors.New("storage unavailable")
		store.GetErr = outage
		loader := objectstore.NewLoader(store, 0)

		_, err = loader.Load(ctx, locator)
		require.ErrorIs(t, err, outage)

		store.GetErr = nil
		got, err := loader.Load(ctx, locator)
		require.NoError(t, err)
		require.Equal(t, []byte("x"), got)
	})

	t.Run("evicts the oldest entry beyond its bound", func(t *testing.T) {
		store := objectstore.NewFake()
		loader := objectstore.NewLoader(store, 2)
		var locators []objectstore.Locator
		for _, body := range []string{"one", "two", "three"} {
			locator, err := objectstore.PutContent(ctx, store, "scripts", []byte(body), "text/javascript")
			require.NoError(t, err)
			locators = append(locators, locator)
			_, err = loader.Load(ctx, locator)
			require.NoError(t, err)
		}

		_, err := loader.Load(ctx, locators[0])
		require.NoError(t, err)
		require.Equal(t, 2, store.GetCount(locators[0].ObjectKey), "the evicted first entry must be fetched again")
		_, err = loader.Load(ctx, locators[2])
		require.NoError(t, err)
		require.Equal(t, 1, store.GetCount(locators[2].ObjectKey), "the newest entry stays cached")
	})
}
