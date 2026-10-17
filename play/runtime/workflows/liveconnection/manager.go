// Package liveconnection is the workflow of a client's live connection to a
// Session, from admission until it ends.
//
// Placeholder: there is no live Session yet, so every caller is admitted.
package liveconnection

import (
	"context"
	"sync"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// Manager is the live connection workflow controller.
type Manager struct{}

// New constructs a Manager.
func New() *Manager {
	return &Manager{}
}

// AttachAdmin admits userUUID's admin connection into sessionUUID, routed
// here under ownerEpoch. An error means admission could not be decided.
//
// Placeholder: admits everyone, sends no views, and cancelling only ends the
// connection.
func (m *Manager) AttachAdmin(_ context.Context, _ string, _ string, _ int64) (runtime.AttachAdminResult, error) {
	connection := newPlaceholderConnection()
	return runtime.AttachAdminResult{
		Outcome: runtime.AttachAccepted,
		Connection: runtime.AdminConnection{
			SelectPerspective: func(context.Context, string) (runtime.SelectPerspectiveResult, error) {
				return runtime.SelectPerspectiveResult{Outcome: runtime.PerspectiveSelected}, nil
			},
			CancelSession: func(context.Context) (runtime.CancelSessionResult, error) {
				connection.end(runtime.EndSessionEnded)
				return runtime.CancelSessionResult{Outcome: runtime.SessionCancelled}, nil
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
				connection.end(runtime.EndDisconnected)
			},
		},
	}, nil
}

type placeholderConnection struct {
	once   sync.Once
	ended  chan struct{}
	reason runtime.EndReason
}

func newPlaceholderConnection() *placeholderConnection {
	return &placeholderConnection{ended: make(chan struct{})}
}

func (c *placeholderConnection) end(reason runtime.EndReason) {
	c.once.Do(func() {
		c.reason = reason
		close(c.ended)
	})
}

func (c *placeholderConnection) wait(ctx context.Context) (runtime.ConnectionEnd, bool) {
	select {
	case <-c.ended:
		return runtime.ConnectionEnd{Reason: c.reason}, true
	case <-ctx.Done():
		return runtime.ConnectionEnd{}, false
	}
}
