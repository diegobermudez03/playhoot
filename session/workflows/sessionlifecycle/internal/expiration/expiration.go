// Package expiration provides lazy lobby-expiration materialization,
// shared by sessionlifecycle's Join/Leave/Start steps so the logic lives
// once rather than being reimplemented per step.
package expiration

import (
	"context"
	"time"

	"github.com/diegobermudez03/playhoot/session"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"gorm.io/gorm"
)

// Store is the narrow persistence capability Expirer needs.
type Store interface {
	SetSessionTerminal(ctx context.Context, tx *gorm.DB, sessionID uint, terminalAt time.Time, terminalReason string) error
	RevokeActiveJoinCode(ctx context.Context, tx *gorm.DB, sessionID uint, revokedAt time.Time) error
}

// Expirer performs lazy lobby-expiration materialization. It depends on its
// own Store, bound once at construction, rather than each caller supplying
// one per call - this workflow's own shared mechanism, not a domain-wide
// protocol other workflows are bound to follow the same way.
type Expirer struct {
	store Store
}

// NewExpirer constructs an Expirer bound to store.
func NewExpirer(store Store) *Expirer {
	return &Expirer{store: store}
}

// MaterializeIfDue evaluates whether lockedSession's lobby has already
// expired (phase is LOBBY and now is at or after lobby_expires_at) and, if
// so, persists the mutations that materialize TERMINAL and revoke the
// active JoinCode, reporting whether it did so. This is workflow policy,
// not something the shared sessionlock primitive decides on its own
// behalf. lockedSession is mutated in place to reflect the new state so
// callers do not need to re-read it. now serves both as the decision's
// "current time" and as the semantic revocation timestamp passed to
// RevokeActiveJoinCode.
func (e *Expirer) MaterializeIfDue(ctx context.Context, tx *gorm.DB, lockedSession *internalrepo.Session, now time.Time) (bool, error) {
	if lockedSession.Phase != session.PhaseLobby || now.Before(lockedSession.LobbyExpiresAt) {
		return false, nil
	}

	terminalAt := lockedSession.LobbyExpiresAt
	reason := session.TerminalReasonLobbyExpired
	if err := e.store.SetSessionTerminal(ctx, tx, lockedSession.ID, terminalAt, reason); err != nil {
		return false, err
	}
	if err := e.store.RevokeActiveJoinCode(ctx, tx, lockedSession.ID, now); err != nil {
		return false, err
	}

	lockedSession.Phase = session.PhaseTerminal
	lockedSession.TerminalAt = &terminalAt
	lockedSession.TerminalReason = &reason
	return true, nil
}
