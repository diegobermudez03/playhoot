package sessionlifecycle

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

// NewWithGRPCExecutor constructs a Manager backed by db and a production
// Executor dialed against executorAddr over gRPC. session/internal/executor
// is unexported outside this package's own tree, so this is the supported
// entry point for a caller (main.go) outside it that still needs a real,
// separately deployed Executor connection rather than a test Fake. It uses
// insecure transport credentials - no TLS material exists anywhere in this
// repository yet, consistent with every other internal service boundary
// today. The returned close func releases the underlying connection and
// should be called once during shutdown.
func NewWithGRPCExecutor(db *gorm.DB, executorAddr string) (*Manager, func() error, error) {
	client, err := executor.NewGRPCClient(executorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dialing executor at %q: %w", executorAddr, err)
	}
	return New(db, client), client.Close, nil
}
