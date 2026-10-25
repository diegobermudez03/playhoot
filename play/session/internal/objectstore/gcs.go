package objectstore

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

// maxObjectBytes bounds how much a single Get will read into memory. Game
// scripts are small; anything larger is not a script this port serves.
const maxObjectBytes = 16 << 20

// emulatorAccessID is the identity URLs are signed as under an emulator.
const emulatorAccessID = "emulator@playhoot-local.iam.gserviceaccount.com"

// GCS is a Store backed by one private Google Cloud Storage bucket. It signs
// URLs through the IAM Credentials API as its own identity, so that identity
// needs permission to sign for itself.
type GCS struct {
	client *storage.Client
	bucket string
	// emulator is how URLs are signed under STORAGE_EMULATOR_HOST; nil
	// against real Cloud Storage.
	emulator *emulatorSigning
}

// emulatorSigning signs with a throwaway key: an emulator, which has no
// credentials to sign with, does not check signatures.
type emulatorSigning struct {
	privateKey []byte
	insecure   bool
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
	store := &GCS{client: client, bucket: bucket}
	if host := os.Getenv("STORAGE_EMULATOR_HOST"); host != "" {
		if store.emulator, err = newEmulatorSigning(host); err != nil {
			_ = client.Close()
			return nil, err
		}
	}
	return store, nil
}

func newEmulatorSigning(host string) (*emulatorSigning, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generating emulator signing key: %w", err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("encoding emulator signing key: %w", err)
	}
	return &emulatorSigning{
		privateKey: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}),
		insecure:   strings.HasPrefix(host, "http://"),
	}, nil
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
func (g *GCS) PresignGet(ctx context.Context, key string, expiresAt time.Time) (string, error) {
	options := &storage.SignedURLOptions{
		Method: "GET",
		Scheme: storage.SigningSchemeV4,
		// A V4 URL lives whole seconds counted from the second it is signed
		// in, rounded down, so asking for one more second keeps it working
		// until expiresAt.
		Expires: expiresAt.Add(time.Second),
	}
	if g.emulator != nil {
		options.GoogleAccessID = emulatorAccessID
		options.PrivateKey = g.emulator.privateKey
		options.Insecure = g.emulator.insecure
	}
	url, err := g.client.Bucket(g.bucket).SignedURL(key, options)
	if err != nil {
		return "", fmt.Errorf("signing gcs url: %w", err)
	}
	return url, nil
}
