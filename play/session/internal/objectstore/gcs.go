package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/storage"
)

// maxObjectBytes bounds how much a single Get will read into memory. Game
// scripts are small; anything larger is not a script this port serves.
const maxObjectBytes = 16 << 20

// GCS is a Store backed by one private Google Cloud Storage bucket.
// Authentication uses Application Default Credentials, so a deployed
// service authenticates as its own service identity. Signed URLs are
// produced without a locally held private key: the client signs through the
// IAM Credentials API on behalf of that identity, so the identity needs
// permission to sign for itself.
type GCS struct {
	client *storage.Client
	bucket string
}

// NewGCS constructs a GCS store for bucket using Application Default
// Credentials. projectID is required configuration but object access is
// scoped by bucket alone.
func NewGCS(ctx context.Context, projectID, bucket string) (*GCS, error) {
	if projectID == "" || bucket == "" {
		return nil, errors.New("gcs project id and bucket are required")
	}
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating gcs client: %w", err)
	}
	return &GCS{client: client, bucket: bucket}, nil
}

// Close releases the underlying client.
func (g *GCS) Close() error { return g.client.Close() }

// Get implements Store.
func (g *GCS) Get(ctx context.Context, key string) ([]byte, error) {
	reader, err := g.client.Bucket(g.bucket).Object(key).NewReader(ctx)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("opening gcs object: %w", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, maxObjectBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading gcs object: %w", err)
	}
	if len(content) > maxObjectBytes {
		return nil, fmt.Errorf("gcs object exceeds %d bytes", maxObjectBytes)
	}
	return content, nil
}

// Put implements Store.
func (g *GCS) Put(ctx context.Context, key string, content []byte, contentType string) error {
	writer := g.client.Bucket(g.bucket).Object(key).NewWriter(ctx)
	writer.ContentType = contentType
	if _, err := writer.Write(content); err != nil {
		_ = writer.Close()
		return fmt.Errorf("writing gcs object: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finalizing gcs object: %w", err)
	}
	return nil
}

// PresignGet implements Store. It does not check that the object exists:
// a signed URL for a missing object simply returns 404 when fetched.
func (g *GCS) PresignGet(ctx context.Context, key string, ttl time.Duration) (SignedURL, error) {
	expiresAt := time.Now().UTC().Add(ttl)
	url, err := g.client.Bucket(g.bucket).SignedURL(key, &storage.SignedURLOptions{
		Method:  "GET",
		Scheme:  storage.SigningSchemeV4,
		Expires: expiresAt,
	})
	if err != nil {
		return SignedURL{}, fmt.Errorf("signing gcs url: %w", err)
	}
	return SignedURL{URL: url, ExpiresAt: expiresAt}, nil
}
