// Package orchestrator is the coordination layer for cross-domain write
// workflows (ARCHITECTURE.md -> Cross-Domain Writes). CreateSession is its
// first concrete workflow: creating a Session is a write, so this
// composition (read Game Management's visibility, then conditionally call
// Session Runtime's Create) belongs here rather than in Composer, which is
// reserved for read-only composition.
package orchestrator

import (
	"context"

	"github.com/diegobermudez03/playhoot/session"
)

//go:generate mockgen -package=orchestrator -destination=mocks_test.go . visibilityChecker,sessionCreator

// visibilityChecker is CreateSession's own narrow dependency on Game
// Management's visibility-check capability.
type visibilityChecker interface {
	IsVisible(ctx context.Context, gameUUID string) (visible bool, found bool, err error)
}

// sessionCreator is CreateSession's own narrow dependency on Session
// Runtime's Create.
type sessionCreator interface {
	Create(ctx context.Context, gameUUID session.GameUUID, hostUserUUID session.UserUUID, idempotencyKey session.IdempotencyKey) (session.CreatedSession, error)
}

// Orchestrator coordinates cross-domain write workflows. It owns no
// business entity and no retry/compensation state of its own for
// CreateSession - a failed write is simply reported, not compensated.
type Orchestrator struct {
	visibility visibilityChecker
	sessions   sessionCreator
}

// New constructs an Orchestrator.
func New(visibility visibilityChecker, sessions sessionCreator) *Orchestrator {
	return &Orchestrator{visibility: visibility, sessions: sessions}
}

// CreateSession reads Game Management's visibility state for gameUUID
// first; if the Game does not exist or is not currently playable/visible,
// it returns session.ErrGameNotFound without calling Session Runtime at
// all. Otherwise it calls sessionlifecycle.Manager.Create (through the
// sessionCreator port) unchanged and returns its result as-is.
func (o *Orchestrator) CreateSession(ctx context.Context, gameUUID session.GameUUID, hostUserUUID session.UserUUID, idempotencyKey session.IdempotencyKey) (session.CreatedSession, error) {
	visible, found, err := o.visibility.IsVisible(ctx, string(gameUUID))
	if err != nil {
		return session.CreatedSession{}, err
	}
	if !found || !visible {
		return session.CreatedSession{}, session.ErrGameNotFound
	}

	return o.sessions.Create(ctx, gameUUID, hostUserUUID, idempotencyKey)
}
