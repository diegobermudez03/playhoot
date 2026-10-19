// Package reconciliation decides how the platform ends a Session past one of
// its deadlines that nothing live ended: a lobby nobody started, or a running
// Session past its lifetime or whose owner lease ran out.
package reconciliation

import (
	"time"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/endcause"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
)

// Session is what reconciliation knows about a Session.
type Session struct {
	Status         sessionstatus.Status
	LobbyExpiresAt time.Time
	ExpiresAt      time.Time
	// OwnerLeaseExpiresAt is when its owner lease runs out; zero if it has
	// none.
	OwnerLeaseExpiresAt time.Time
	// ReplayFinalized means its game finished and its final replay is stored.
	ReplayFinalized bool
}

// CauseFor returns why session must be ended at now; false when none of its
// deadlines has passed, so it must be left alone.
func CauseFor(session Session, now time.Time) (endcause.Cause, bool) {
	switch session.Status {
	case sessionstatus.Lobby:
		if !now.Before(session.LobbyExpiresAt) {
			return endcause.LobbyExpired, true
		}
	case sessionstatus.Running:
		pastLifetime := !now.Before(session.ExpiresAt)
		leaseOver := !session.OwnerLeaseExpiresAt.IsZero() && !now.Before(session.OwnerLeaseExpiresAt)
		switch {
		case !pastLifetime && !leaseOver:
			return "", false
		case session.ReplayFinalized:
			// Its owner died between storing the end and committing it.
			return endcause.GameFinished, true
		case pastLifetime:
			return endcause.MaxLifetime, true
		default:
			return endcause.OwnerLost, true
		}
	}
	return "", false
}
