// Package listoperations lists every operation with the minimum
// authentication level it requires, so clients know which operations a
// person can reach and where to send them to authenticate.
package listoperations

import (
	"context"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/operationlevel"
)

// Service is Identity's operation catalog listing.
type Service struct{}

// New constructs a Service.
func New() *Service {
	return &Service{}
}

// ListOperations returns the catalog Authorize applies, ordered by operation.
func (s *Service) ListOperations(_ context.Context) []identity.OperationLevel {
	return operationlevel.Catalog()
}
