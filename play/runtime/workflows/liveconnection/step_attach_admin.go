package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=step_attach_admin_mock_test.go . adminJoinChecker

// adminJoinChecker is the durable Session's side of admitting an admin.
type adminJoinChecker interface {
	EligibleToJoinAsAdmin(ctx context.Context, sessionUUID string, userUUID string) (sessionpkg.AdminJoinEligibility, error)
}

var (
	attachOutcomeLabel = metrics.NewLabel("outcome",
		"accepted",
		string(runtime.RejectNotCreator),
		string(runtime.RejectSessionUnavailable),
		string(runtime.RejectOwnerChanged),
	)

	adminAttaches = metrics.NewCounter(
		"play_runtime_admin_attach_total",
		"Admin connections attached to a live Session, by outcome.",
		attachOutcomeLabel,
	)
)

// AttachAdmin admits userUUID's admin connection into sessionUUID, routed
// here under ownerEpoch, creating the Session's controller if it has none. An
// error means admission could not be decided.
func (m *Manager) AttachAdmin(ctx context.Context, sessionUUID string, userUUID string, ownerEpoch int64) (runtime.AttachAdminResult, error) {
	eligibility, err := m.adminJoins.EligibleToJoinAsAdmin(ctx, sessionUUID, userUUID)
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

	sessionController, ok, err := m.controllerFor(ctx, sessionUUID, ownerEpoch)
	if err != nil {
		return runtime.AttachAdminResult{}, fmt.Errorf("finding session controller: %w", err)
	}
	if !ok {
		return rejectAdmin(ctx, runtime.RejectOwnerChanged), nil
	}
	adminAttaches.Inc(ctx, attachOutcomeLabel.Value("accepted"))
	start := func(ctx context.Context) (runtime.StartSessionResult, error) {
		return m.startSession(ctx, sessionController, sessionUUID, userUUID)
	}
	return runtime.AttachAdminResult{
		Outcome:    runtime.AttachAccepted,
		Connection: sessionController.AttachAdmin(userUUID, start),
	}, nil
}

func rejectAdmin(ctx context.Context, reason runtime.RejectReason) runtime.AttachAdminResult {
	adminAttaches.Inc(ctx, attachOutcomeLabel.Value(string(reason)))
	return runtime.AttachAdminResult{Outcome: runtime.AttachRejected, Rejection: reason}
}
