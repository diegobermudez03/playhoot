package controller

import (
	"context"
	"sync"

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
// operations. Several admin connections of the same creator may be attached
// at once.
func (c *Controller) AttachAdmin(userUUID string) runtime.AdminConnection {
	connection := &adminConnection{id: uuid.NewString(), userUUID: userUUID, ended: make(chan struct{})}
	c.mu.Lock()
	c.admins[connection.id] = connection
	c.mu.Unlock()

	return runtime.AdminConnection{
		SelectPerspective: func(context.Context, string) (runtime.SelectPerspectiveResult, error) {
			return runtime.SelectPerspectiveResult{}, ErrNotImplemented
		},
		StartSession: func(context.Context) (runtime.StartSessionResult, error) {
			return runtime.StartSessionResult{}, ErrNotImplemented
		},
		CancelSession: func(context.Context) (runtime.CancelSessionResult, error) {
			return runtime.CancelSessionResult{}, ErrNotImplemented
		},
		NextLobbyUpdate: func(ctx context.Context) (runtime.LobbyUpdate, bool) {
			connection.waitWithoutResult(ctx)
			return runtime.LobbyUpdate{}, false
		},
		NextView: func(ctx context.Context) (runtime.PlayerView, bool) {
			connection.waitWithoutResult(ctx)
			return runtime.PlayerView{}, false
		},
		Ended: connection.wait,
		Disconnect: func() {
			c.mu.Lock()
			delete(c.admins, connection.id)
			c.mu.Unlock()
			connection.end(runtime.EndDisconnected)
		},
	}
}

// adminConnection is one attached admin connection.
type adminConnection struct {
	id       string
	userUUID string

	once   sync.Once
	ended  chan struct{}
	reason runtime.EndReason
}

func (a *adminConnection) end(reason runtime.EndReason) {
	a.once.Do(func() {
		a.reason = reason
		close(a.ended)
	})
}

func (a *adminConnection) wait(ctx context.Context) (runtime.ConnectionEnd, bool) {
	select {
	case <-a.ended:
		return runtime.ConnectionEnd{Reason: a.reason}, true
	case <-ctx.Done():
		return runtime.ConnectionEnd{}, false
	}
}

// waitWithoutResult blocks until the connection ends or ctx is done; it backs
// the streams that never produce anything yet.
func (a *adminConnection) waitWithoutResult(ctx context.Context) {
	select {
	case <-a.ended:
	case <-ctx.Done():
	}
}
