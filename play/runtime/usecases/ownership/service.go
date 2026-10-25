// Package ownership finds or claims the instance that owns a live Session.
package ownership

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=ownership -destination=service_mock_test.go . liveSessionChecker

// liveSessionChecker is the durable Session's side of claiming: only a
// Session that can have live connections is ever owned.
type liveSessionChecker interface {
	CheckLiveSession(ctx context.Context, sessionUUID string) (sessionpkg.LiveSessionCheck, error)
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
// the Session when it has none. A Session that cannot be joined live is never
// claimed: the outcome says whether it does not exist, ended (and how), or is
// otherwise unavailable.
func (s *Service) LocateOrClaimOwner(ctx context.Context, sessionUUID string) (runtime.OwnerPlacement, error) {
	check, err := s.sessions.CheckLiveSession(ctx, sessionUUID)
	if err != nil {
		return runtime.OwnerPlacement{}, fmt.Errorf("checking live session: %w", err)
	}
	switch check.Outcome {
	case sessionpkg.LiveSessionLive:
	case sessionpkg.LiveSessionNotFound:
		return runtime.OwnerPlacement{Outcome: runtime.OwnerSessionNotFound}, nil
	case sessionpkg.LiveSessionEnded:
		return runtime.OwnerPlacement{
			Outcome: runtime.OwnerSessionEnded,
			Ended:   runtime.EndedSession{Status: check.Status, EndCause: check.EndCause},
		}, nil
	case sessionpkg.LiveSessionLobbyExpired:
		return runtime.OwnerPlacement{Outcome: runtime.OwnerSessionUnavailable}, nil
	default:
		return runtime.OwnerPlacement{}, fmt.Errorf("unknown live session outcome %q", check.Outcome)
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
