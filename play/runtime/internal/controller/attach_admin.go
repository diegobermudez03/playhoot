package controller

import (
	"context"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// AdminCount returns how many admin connections are attached.
func (c *Controller) AdminCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.admins)
}

// AttachAdmin registers userUUID's admin connection and returns its
// operations, starting the Session through start. Several admin connections
// of the same creator may be attached at once. Before the game runs, the
// first lobby update is the lobby as it is now.
func (c *Controller) AttachAdmin(userUUID string, start func(ctx context.Context) (runtime.StartSessionResult, error)) runtime.AdminConnection {
	admin := newConnection(uuid.NewString(), userUUID)
	c.mu.Lock()
	switch c.phase {
	case phaseLobby, phaseStarting:
		admin.lobbyUpdates.publish(c.lobbyLocked())
	case phaseEnded:
		admin.end(runtime.EndSessionEnded)
	default:
		admin.lobbyUpdates.finish()
		close(admin.started)
	}
	if c.phase != phaseEnded {
		c.admins[admin.id] = admin
	}
	c.mu.Unlock()

	return runtime.AdminConnection{
		SelectPerspective: func(context.Context, string) (runtime.SelectPerspectiveResult, error) {
			return runtime.SelectPerspectiveResult{}, ErrNotImplemented
		},
		StartSession: start,
		CancelSession: func(context.Context) (runtime.CancelSessionResult, error) {
			return runtime.CancelSessionResult{}, ErrNotImplemented
		},
		NextLobbyUpdate: admin.nextLobbyUpdate,
		Started: func(ctx context.Context) (runtime.SessionStart, bool) {
			return c.sessionStart(ctx, admin)
		},
		NextView: func(ctx context.Context) (runtime.PlayerView, bool) {
			admin.waitWithoutResult(ctx)
			return runtime.PlayerView{}, false
		},
		Ended: admin.wait,
		Disconnect: func() {
			c.mu.Lock()
			delete(c.admins, admin.id)
			c.mu.Unlock()
			admin.end(runtime.EndDisconnected)
		},
	}
}
