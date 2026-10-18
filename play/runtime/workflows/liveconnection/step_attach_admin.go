package liveconnection

import (
	"context"
	"fmt"

	"github.com/google/uuid"

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

// AttachAdmin hands userUUID's admin connection to sessionUUID's controller
// under ownerEpoch, creating the controller if it has none. An error means
// admission could not be decided.
func (r *ControllerResolver) AttachAdmin(ctx context.Context, sessionUUID string, userUUID string, ownerEpoch int64) (runtime.AttachAdminResult, error) {
	sessionController, ok := r.controllerFor(ctx, sessionUUID, ownerEpoch)
	if !ok {
		return rejectAdmin(ctx, runtime.RejectOwnerChanged), nil
	}
	return sessionController.attachAdmin(ctx, userUUID)
}

// attachAdmin admits userUUID's admin connection if the durable Session
// accepts them as its admin.
func (c *sessionController) attachAdmin(ctx context.Context, userUUID string) (runtime.AttachAdminResult, error) {
	eligibility, err := c.deps.adminJoins.EligibleToJoinAsAdmin(ctx, c.sessionUUID, userUUID)
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
	if err := c.loadManifest(ctx); err != nil {
		return runtime.AttachAdminResult{}, err
	}
	adminAttaches.Inc(ctx, attachOutcomeLabel.Value("accepted"))
	return runtime.AttachAdminResult{Outcome: runtime.AttachAccepted, Connection: c.registerAdmin(userUUID)}, nil
}

func rejectAdmin(ctx context.Context, reason runtime.RejectReason) runtime.AttachAdminResult {
	adminAttaches.Inc(ctx, attachOutcomeLabel.Value(string(reason)))
	return runtime.AttachAdminResult{Outcome: runtime.AttachRejected, Rejection: reason}
}

// registerAdmin registers userUUID's admin connection and returns its
// operations. Several admin connections of the same creator may be attached
// at once. Before the game runs, the first lobby update is the lobby as it is
// now.
func (c *sessionController) registerAdmin(userUUID string) runtime.AdminConnection {
	admin := newConnection(uuid.NewString(), userUUID)
	s := c.lock()
	switch s.phase {
	case phaseLobby, phaseStarting:
		admin.lobbyUpdates.publish(s.lobbyUpdate())
	case phaseEnded:
		admin.end(runtime.EndSessionEnded)
	default:
		admin.markStarted(s.start)
	}
	if s.phase != phaseEnded {
		s.admins[admin.id] = admin
	}
	c.unlock()

	return runtime.AdminConnection{
		SelectPerspective: func(context.Context, string) (runtime.SelectPerspectiveResult, error) {
			return runtime.SelectPerspectiveResult{}, ErrNotImplemented
		},
		StartSession: func(ctx context.Context) (runtime.StartSessionResult, error) {
			return c.requestStart(ctx, userUUID)
		},
		CancelSession: func(context.Context) (runtime.CancelSessionResult, error) {
			return runtime.CancelSessionResult{}, ErrNotImplemented
		},
		NextLobbyUpdate: admin.nextLobbyUpdate,
		Started:         admin.sessionStart,
		NextView: func(ctx context.Context) (runtime.PlayerView, bool) {
			admin.waitWithoutResult(ctx)
			return runtime.PlayerView{}, false
		},
		Ended: admin.wait,
		Disconnect: func() {
			s := c.lock()
			delete(s.admins, admin.id)
			c.unlock()
			admin.end(runtime.EndDisconnected)
		},
	}
}
