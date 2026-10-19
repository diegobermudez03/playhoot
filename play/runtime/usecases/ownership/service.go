// Package ownership finds or claims the instance that owns a live Session.
package ownership

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

//go:generate mockgen -package=ownership -destination=service_mock_test.go . liveSessionChecker

// liveSessionChecker is the durable Session's side of claiming: only a
// Session that can have live connections is ever owned.
type liveSessionChecker interface {
	IsSessionLive(ctx context.Context, sessionUUID string) (bool, error)
}

// singleInstanceEpoch never changes because, with one instance and no
// registry, ownership never changes hands.
const singleInstanceEpoch = 1

// Service is the Runtime's ownership use case.
type Service struct {
	sessions liveSessionChecker
}

// New constructs a Service claiming only the Sessions sessions reports live.
func New(sessions liveSessionChecker) *Service {
	return &Service{sessions: sessions}
}

// LocateOrClaimOwner returns the owner of sessionUUID, atomically claiming
// the Session when it has none. Sessions that cannot be joined live are
// reported as unavailable, never claimed.
func (s *Service) LocateOrClaimOwner(ctx context.Context, sessionUUID string) (runtime.OwnerPlacement, error) {
	live, err := s.sessions.IsSessionLive(ctx, sessionUUID)
	if err != nil {
		return runtime.OwnerPlacement{}, fmt.Errorf("checking live session: %w", err)
	}
	if !live {
		return runtime.OwnerPlacement{Outcome: runtime.OwnerSessionUnavailable}, nil
	}
	// Placeholder: claims every live Session for this instance, without a registry, lease or epoch change.
	return runtime.OwnerPlacement{Outcome: runtime.OwnerHere, Epoch: singleInstanceEpoch}, nil
}

// ReleaseOwner gives up this instance's ownership of sessionUUID under epoch,
// so the next connection claims it again. A release under an epoch that is no
// longer the owner's changes nothing.
func (s *Service) ReleaseOwner(ctx context.Context, sessionUUID string, epoch int64) error {
	// Placeholder: there is no registry to release from.
	return nil
}
