// Package liveconnection is the workflow of a client's live connection to a
// Session, from admission until it ends.
//
// Placeholder: there is no live Session yet, so every caller is admitted.
package liveconnection

import (
	"context"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

const connectionBuffer = 16

// Manager is the live connection workflow controller.
type Manager struct{}

// New constructs a Manager.
func New() *Manager {
	return &Manager{}
}

// AttachAdmin admits userUUID's admin connection into sessionUUID, routed
// here under ownerEpoch. An error means admission could not be decided.
//
// Placeholder: admits everyone; the connection never sends anything and
// discards what the client sends.
func (m *Manager) AttachAdmin(_ context.Context, _ string, _ string, _ int64) (runtime.AttachAdminResult, error) {
	inbound := make(chan runtime.ClientMessage, connectionBuffer)
	outbound := make(chan runtime.ServerMessage, connectionBuffer)
	go func() {
		for range inbound {
		}
		close(outbound)
	}()
	return runtime.AttachAdminResult{
		Outcome:    runtime.AttachAccepted,
		Connection: runtime.Connection{Inbound: inbound, Outbound: outbound},
	}, nil
}
