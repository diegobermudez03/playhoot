package sessionlifecycle

import (
	"context"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/sessionlock"
	"gorm.io/gorm"
)

// expirationStore is the narrow persistence capability lazy lobby-expiration
// materialization needs. It is satisfied by both joinRepoAPI and
// leaveRepoAPI, since Join and Leave both perform this same materialization
// (WORK-0001's Expiration Ownership) - the shared logic lives once here
// rather than each step reimplementing it.
type expirationStore interface {
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	RevokeActiveJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, revokedAt time.Time) error
}

// materializeExpirationIfDue evaluates whether lockedSession's lobby has
// already expired (phase is LOBBY and now is at or after lobby_expires_at)
// and, if so, requests the persistence mutations that materialize TERMINAL
// and revoke the active JoinCode, reporting whether it did so. This is
// Manager policy, not something the shared sessionlock primitive decides on
// its own behalf (WORK-0001's Database Locking / Serialization Design,
// Expiration Ownership). lockedSession is mutated in place to reflect the
// new state so callers do not need to re-read it. now is both the
// decision's "current time" and the semantic revocation timestamp passed to
// RevokeActiveJoinCode.
func materializeExpirationIfDue(ctx context.Context, tx *gorm.DB, store expirationStore, lockedSession *sessionlock.Session, now time.Time) (bool, error) {
	if lockedSession.Phase != session.PhaseLobby || now.Before(lockedSession.LobbyExpiresAt) {
		return false, nil
	}

	terminalAt := lockedSession.LobbyExpiresAt
	reason := session.TerminalReasonLobbyExpired
	if err := store.SetSessionTerminal(ctx, tx, lockedSession.ID, terminalAt, reason); err != nil {
		return false, err
	}
	if err := store.RevokeActiveJoinCode(ctx, tx, lockedSession.ID, now); err != nil {
		return false, err
	}

	lockedSession.Phase = session.PhaseTerminal
	lockedSession.TerminalAt = &terminalAt
	lockedSession.TerminalReason = &reason
	return true, nil
}
