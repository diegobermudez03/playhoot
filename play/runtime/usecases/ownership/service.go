// Package ownership finds or claims the instance that owns a live Session.
//
// Placeholder: there is no registry yet, so this instance owns every Session.
package ownership

import (
	"context"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// singleInstanceEpoch never changes because, with one instance and no
// registry, ownership never changes hands.
const singleInstanceEpoch = 1

// Service is the Runtime's ownership use case.
type Service struct{}

// New constructs a Service.
func New() *Service {
	return &Service{}
}

// LocateOrClaimOwner returns the owner of sessionUUID, atomically claiming
// the Session when it has none. Sessions that cannot be joined live are
// reported as unavailable, never claimed.
//
// Placeholder: reports every Session as owned here, without looking it up.
func (s *Service) LocateOrClaimOwner(_ context.Context, _ string) (runtime.OwnerPlacement, error) {
	return runtime.OwnerPlacement{
		Outcome: runtime.OwnerHere,
		Epoch:   singleInstanceEpoch,
	}, nil
}
