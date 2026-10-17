// Package liveconnection is the workflow of a client's live connection to a
// Session, from admission until it ends: a Manager whose methods are the
// workflow's operations, one file per operation.
package liveconnection

import "github.com/diegobermudez03/playhoot/play/runtime/internal/controller"

// Manager is the live connection workflow controller.
type Manager struct {
	adminJoins  adminJoinChecker
	controllers *controller.Registry
}

// New constructs a Manager. adminJoins decides who may join a Session as
// admin.
func New(adminJoins adminJoinChecker) *Manager {
	return &Manager{adminJoins: adminJoins, controllers: controller.NewRegistry()}
}
