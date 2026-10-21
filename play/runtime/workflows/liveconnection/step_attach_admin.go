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
		string(runtime.RejectTooManyAdmins),
		string(runtime.RejectServerBusy),
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
	sessionController, rejection := r.controllerFor(ctx, sessionUUID, ownerEpoch)
	if sessionController == nil {
		return rejectAdmin(ctx, rejection), nil
	}
	return sessionController.attachAdmin(ctx, userUUID)
}

// attachAdmin admits userUUID's admin connection if the durable Session
// accepts them as its admin, recovering a game the database runs and this
// instance does not.
func (c *sessionController) attachAdmin(ctx context.Context, userUUID string) (runtime.AttachAdminResult, error) {
	eligibility, err := c.deps.adminJoins.EligibleToJoinAsAdmin(ctx, c.sessionUUID, userUUID)
	if err != nil {
		return runtime.AttachAdminResult{}, fmt.Errorf("checking admin eligibility: %w", err)
	}
	switch eligibility {
	case sessionpkg.AdminJoinEligible, sessionpkg.AdminJoinEligibleRunning:
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
	if eligibility == sessionpkg.AdminJoinEligibleRunning {
		recovery, err := c.recoverIfNeeded(ctx)
		if err != nil {
			return runtime.AttachAdminResult{}, err
		}
		if recovery == recoveryEnded {
			return rejectAdmin(ctx, runtime.RejectSessionUnavailable), nil
		}
	}
	admin, registered := c.registerAdmin(userUUID)
	if !registered {
		limitRejections.Inc(ctx, limitLabel.Value(limitAdminConnections))
		return rejectAdmin(ctx, runtime.RejectTooManyAdmins), nil
	}
	adminAttaches.Inc(ctx, attachOutcomeLabel.Value("accepted"))
	return runtime.AttachAdminResult{Outcome: runtime.AttachAccepted, Connection: admin}, nil
}

func rejectAdmin(ctx context.Context, reason runtime.RejectReason) runtime.AttachAdminResult {
	adminAttaches.Inc(ctx, attachOutcomeLabel.Value(string(reason)))
	return runtime.AttachAdminResult{Outcome: runtime.AttachRejected, Rejection: reason}
}

// registerAdmin registers userUUID's admin connection and returns its
// operations; false when the Session has as many admin connections as it
// allows. Several admin connections of the same creator may be attached at
// once. Before the game runs, the first lobby update is the lobby as it is
// now.
func (c *sessionController) registerAdmin(userUUID string) (runtime.AdminConnection, bool) {
	admin := newConnection(uuid.NewString(), runtime.Player{UserUUID: userUUID})
	s := c.lock()
	if len(s.admins) >= c.deps.liveLimits.admins {
		c.unlock()
		return runtime.AdminConnection{}, false
	}
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
		SelectPerspective: func(ctx context.Context, participantID string) (runtime.SelectPerspectiveResult, error) {
			return c.selectPerspective(ctx, admin, participantID), nil
		},
		Resync: func(ctx context.Context) error {
			c.resyncAdmin(ctx, admin)
			return nil
		},
		StartSession: func(ctx context.Context) (runtime.StartSessionResult, error) {
			return c.requestStart(ctx, userUUID)
		},
		CancelSession: func(ctx context.Context) (runtime.CancelSessionResult, error) {
			return c.requestCancel(ctx, userUUID)
		},
		NextLobbyUpdate: admin.nextLobbyUpdate,
		Started:         admin.sessionStart,
		NextView: func(ctx context.Context) (runtime.WatchedView, bool) {
			view, ok := admin.views.next(ctx, admin.ended)
			return runtime.WatchedView{PlayerView: view.PlayerView, Events: view.Events}, ok
		},
		Ended: admin.wait,
		Disconnect: func() {
			s := c.lock()
			delete(s.admins, admin.id)
			c.unlock()
			admin.end(runtime.EndDisconnected)
		},
	}, true
}
