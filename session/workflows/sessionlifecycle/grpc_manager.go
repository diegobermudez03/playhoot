package sessionlifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"github.com/diegobermudez03/playhoot/session/internal/objectstore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

// ProductionConfig is the external wiring a production Manager needs beyond
// its database: where the separately deployed JavaScript Executor listens,
// and the private Google Cloud Storage bucket holding every game version's
// script and asset content.
type ProductionConfig struct {
	// ExecutorAddr is the JavaScript Executor's gRPC address.
	ExecutorAddr string
	// GCSProjectID and GCSBucket identify the private bucket. Credentials
	// come from Application Default Credentials (the deployment's service
	// identity), never from this config.
	GCSProjectID string
	GCSBucket    string
	// SignedURLTTL is how long a signed content URL stays valid. Zero uses
	// the default.
	SignedURLTTL time.Duration
}

// NewProduction constructs a Manager backed by db, a production Executor
// dialed over gRPC, and a Google Cloud Storage object store. The internal
// Executor client and object store are unexported outside this package's
// tree, so this is the supported entry point for a caller (main.go) outside
// it that needs real infrastructure rather than test doubles. It uses
// insecure transport credentials for the Executor - no TLS material exists
// anywhere in this repository yet, consistent with every other internal
// service boundary today. The returned close func releases the Executor
// connection and the storage client and should be called once during
// shutdown.
func NewProduction(ctx context.Context, db *gorm.DB, cfg ProductionConfig) (*Manager, func() error, error) {
	client, err := executor.NewGRPCClient(cfg.ExecutorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dialing executor at %q: %w", cfg.ExecutorAddr, err)
	}

	store, err := objectstore.NewGCS(ctx, cfg.GCSProjectID, cfg.GCSBucket)
	if err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("connecting to object storage: %w", err)
	}

	manager := New(db, client, store)
	if cfg.SignedURLTTL > 0 {
		manager.signedURLTTL = cfg.SignedURLTTL
	}

	closeAll := func() error {
		executorErr := client.Close()
		storeErr := store.Close()
		if executorErr != nil {
			return executorErr
		}
		return storeErr
	}
	return manager, closeAll, nil
}
