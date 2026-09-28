// Package activity provides lazy RUNNING-phase inactivity-expiration
// materialization, shared by sessionlifecycle's
// AnswerInteraction/SubmitUserIntent/ExpireTimer/CancelSession steps so the
// logic lives once rather than being reimplemented per step - the same shape
// internal/expiration already provides for LOBBY-phase lobby_expires_at.
package activity

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
	CloseAllActiveInteractionsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
	CancelAllActiveTimerObligationsForSession(ctx context.Context, tx *gorm.DB, sessionID uint, reason string) error
}

// Expirer performs lazy RUNNING-phase inactivity-expiration materialization.
// It depends on its own Store, bound once at construction, rather than each
// caller supplying one per call - this workflow's own shared mechanism, not
// a domain-wide protocol other workflows are bound to follow the same way.
type Expirer struct {
	store Store
}

// NewExpirer constructs an Expirer bound to store.
func NewExpirer(store Store) *Expirer {
	return &Expirer{store: store}
}

// MaterializeIfDue evaluates whether lockedSession's RUNNING-phase inactivity
// deadline has already passed (phase is RUNNING, activity_expires_at is set,
// and now is at or after it) and, if so, persists the mutations that
// materialize TERMINAL under TerminalReasonRuntimeInactivityExpired and close
// every still-ACTIVE interaction/timer obligation, reporting whether it did
// so. This is workflow policy, not something the shared sessionlock
// primitive decides on its own behalf. lockedSession is mutated in place to
// reflect the new state so callers do not need to re-read it. Unlike a
// runtime failure, this termination cause is an ordinary, expected lifecycle
// outcome - no session_runtime_failures row is created. terminal_at is
// always set to the deadline that actually passed, never to now - so a
// delayed materialization (a later call, or an eventual background sweep)
// still records the instant the Session actually became inactive, not the
// instant it happened to be noticed.
func (e *Expirer) MaterializeIfDue(ctx context.Context, tx *gorm.DB, lockedSession *internalrepo.Session, now time.Time) (bool, error) {
	if lockedSession.Phase != session.PhaseRunning || lockedSession.ActivityExpiresAt == nil || now.Before(*lockedSession.ActivityExpiresAt) {
		return false, nil
	}

	terminalAt := *lockedSession.ActivityExpiresAt
	reason := session.TerminalReasonRuntimeInactivityExpired
	if err := e.store.SetSessionTerminal(ctx, tx, lockedSession.ID, terminalAt, reason); err != nil {
		return false, err
	}
	if err := e.store.CloseAllActiveInteractionsForSession(ctx, tx, lockedSession.ID, session.InteractionClosureReasonSessionTerminated); err != nil {
		return false, err
	}
	if err := e.store.CancelAllActiveTimerObligationsForSession(ctx, tx, lockedSession.ID, session.TimerObligationClosureReasonSessionTerminated); err != nil {
		return false, err
	}

	lockedSession.Phase = session.PhaseTerminal
	lockedSession.TerminalAt = &terminalAt
	lockedSession.TerminalReason = &reason
	return true, nil
}
