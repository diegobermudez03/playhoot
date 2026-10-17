// Package controller holds the live state of the Sessions this instance owns:
// one Controller per Session, and the registry that finds or creates them.
package controller

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// ErrNotImplemented is returned by connection operations not built yet.
var ErrNotImplemented = errors.New("live operation not implemented yet")

// Registry finds or creates the Controller of each Session this instance
// owns.
type Registry struct {
	mu          sync.Mutex
	controllers map[string]*Controller
}

// Controller is the live state of one Session on its owner instance.
type Controller struct {
	sessionUUID string
	epoch       int64

	mu     sync.Mutex
	admins map[string]*adminConnection
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{controllers: make(map[string]*Controller)}
}

// ControllerFor returns sessionUUID's Controller under epoch, creating it when
// there is none. A Controller from an older epoch is stale (ownership was lost
// and won back): it is retired and replaced. ok is false when epoch is older
// than the existing Controller's, so the request is the stale one.
func (r *Registry) ControllerFor(ctx context.Context, sessionUUID string, epoch int64) (*Controller, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, found := r.controllers[sessionUUID]; found {
		switch {
		case epoch == existing.epoch:
			return existing, true
		case epoch < existing.epoch:
			return nil, false
		}
		existing.retire()
		controllersRetired.Inc(ctx)
	}
	controller := &Controller{sessionUUID: sessionUUID, epoch: epoch, admins: make(map[string]*adminConnection)}
	r.controllers[sessionUUID] = controller
	controllersCreated.Inc(ctx)
	return controller, true
}

// retire ends every connection of a Controller that no longer serves its
// Session under the current ownership.
func (c *Controller) retire() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, admin := range c.admins {
		admin.end(runtime.EndOwnerChanged)
		delete(c.admins, id)
	}
}

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
		CancelSession: func(context.Context) (runtime.CancelSessionResult, error) {
			return runtime.CancelSessionResult{}, ErrNotImplemented
		},
		NextView: func(ctx context.Context) (runtime.PlayerView, bool) {
			select {
			case <-connection.ended:
			case <-ctx.Done():
			}
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
