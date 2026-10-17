// Package liveconnection is the workflow of a client's live connection to a
// Session, from admission until it ends.
package liveconnection

import (
	"context"
	"fmt"

	sessionpkg "github.com/diegobermudez03/playhoot/play"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/controller"
)

//go:generate mockgen -package=liveconnection -destination=admin_join_checker_mock_test.go . adminJoinChecker

// adminJoinChecker is the durable Session's side of admitting an admin.
type adminJoinChecker interface {
	EligibleToJoinAsAdmin(ctx context.Context, sessionUUID string, userUUID string) (sessionpkg.AdminJoinEligibility, error)
}

// Manager is the live connection workflow controller.
type Manager struct {
	sessions    adminJoinChecker
	controllers *controller.Registry
}

// New constructs a Manager. sessions decides who may join a Session as admin.
func New(sessions adminJoinChecker) *Manager {
	return &Manager{sessions: sessions, controllers: controller.NewRegistry()}
}

// AttachAdmin admits userUUID's admin connection into sessionUUID, routed
// here under ownerEpoch, creating the Session's controller if it has none. An
// error means admission could not be decided.
func (m *Manager) AttachAdmin(ctx context.Context, sessionUUID string, userUUID string, ownerEpoch int64) (runtime.AttachAdminResult, error) {
	eligibility, err := m.sessions.EligibleToJoinAsAdmin(ctx, sessionUUID, userUUID)
	if err != nil {
		return runtime.AttachAdminResult{}, fmt.Errorf("checking admin eligibility: %w", err)
	}
	switch eligibility {
	case sessionpkg.AdminJoinEligible:
	case sessionpkg.AdminJoinNotCreator:
		return rejectAdmin(ctx, runtime.RejectNotCreator), nil
	case sessionpkg.AdminJoinSessionNotFound, sessionpkg.AdminJoinSessionNotJoinable:
		return rejectAdmin(ctx, runtime.RejectSessionUnavailable), nil
	default:
		return runtime.AttachAdminResult{}, fmt.Errorf("unknown admin eligibility %q", eligibility)
	}

	sessionController, ok := m.controllers.ControllerFor(ctx, sessionUUID, ownerEpoch)
	if !ok {
		return rejectAdmin(ctx, runtime.RejectOwnerChanged), nil
	}
	adminAttaches.Inc(ctx, attachOutcomeLabel.Value("accepted"))
	return runtime.AttachAdminResult{
		Outcome:    runtime.AttachAccepted,
		Connection: sessionController.AttachAdmin(userUUID),
	}, nil
}

func rejectAdmin(ctx context.Context, reason runtime.RejectReason) runtime.AttachAdminResult {
	adminAttaches.Inc(ctx, attachOutcomeLabel.Value(string(reason)))
	return runtime.AttachAdminResult{Outcome: runtime.AttachRejected, Rejection: reason}
}
