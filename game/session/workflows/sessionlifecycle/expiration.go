package sessionlifecycle

import (
	"context"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

// expirationStore is the narrow persistence capability lazy lobby-expiration
// materialization needs. It is satisfied by both joinRepoAPI and
// leaveRepoAPI, since Join and Leave both perform this same materialization
// (WORK-0001's Expiration Ownership) - the shared logic lives once here
// rather than each step reimplementing it.
type expirationStore interface {
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string, now time.Time) error
	RevokeActiveJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, now time.Time) error
}

// materializeExpirationIfDue evaluates whether row's lobby has already
// expired (phase is LOBBY and now is at or after lobby_expires_at) and, if
// so, requests the persistence mutations that materialize TERMINAL and
// revoke the active JoinCode, reporting whether it did so. This is Manager
// policy, not something the shared sessionlock primitive decides on its own
// behalf (WORK-0001's Database Locking / Serialization Design, Expiration
// Ownership). row is mutated in place to reflect the new state so callers do
// not need to re-read it.
func materializeExpirationIfDue(ctx context.Context, tx *gorm.DB, store expirationStore, row *internalrepo.SessionRow, now time.Time) (bool, error) {
	if row.Phase != session.PhaseLobby || now.Before(row.LobbyExpiresAt) {
		return false, nil
	}

	terminalAt := row.LobbyExpiresAt
	reason := session.TerminalReasonLobbyExpired
	if err := store.SetSessionTerminal(ctx, tx, row.ID, terminalAt, reason, now); err != nil {
		return false, err
	}
	if err := store.RevokeActiveJoinCode(ctx, tx, row.ID, now); err != nil {
		return false, err
	}

	row.Phase = session.PhaseTerminal
	row.TerminalAt = &terminalAt
	row.TerminalReason = &reason
	return true, nil
}
