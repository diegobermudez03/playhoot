package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=step_attach_player_mock_test.go . playerJoinChecker

// playerJoinChecker is the durable Session's side of admitting a player.
type playerJoinChecker interface {
	EligibleToJoinAsPlayer(ctx context.Context, sessionUUID string) (sessionpkg.PlayerJoinEligibility, error)
}

var (
	playerAttachOutcomeLabel = metrics.NewLabel("outcome",
		"accepted",
		string(runtime.RejectLobbyFull),
		string(runtime.RejectSessionStarted),
		string(runtime.RejectSessionUnavailable),
		string(runtime.RejectOwnerChanged),
	)

	playerAttaches = metrics.NewCounter(
		"play_runtime_player_attach_total",
		"Player connections attached to a live Session's lobby, by outcome.",
		playerAttachOutcomeLabel,
	)
)

// AttachPlayer admits userUUID's player connection into sessionUUID's lobby,
// routed here under ownerEpoch, creating the Session's controller if it has
// none. An error means admission could not be decided.
func (m *Manager) AttachPlayer(ctx context.Context, sessionUUID string, userUUID string, ownerEpoch int64) (runtime.AttachPlayerResult, error) {
	eligibility, err := m.playerJoins.EligibleToJoinAsPlayer(ctx, sessionUUID)
	if err != nil {
		return runtime.AttachPlayerResult{}, fmt.Errorf("checking player eligibility: %w", err)
	}
	switch eligibility {
	case sessionpkg.PlayerJoinEligible:
	case sessionpkg.PlayerJoinSessionStarted:
		return rejectPlayer(ctx, runtime.RejectSessionStarted), nil
	case sessionpkg.PlayerJoinSessionNotFound, sessionpkg.PlayerJoinSessionNotJoinable:
		return rejectPlayer(ctx, runtime.RejectSessionUnavailable), nil
	default:
		return runtime.AttachPlayerResult{}, fmt.Errorf("unknown player eligibility %q", eligibility)
	}

	sessionController, ok, err := m.controllerFor(ctx, sessionUUID, ownerEpoch)
	if err != nil {
		return runtime.AttachPlayerResult{}, fmt.Errorf("finding session controller: %w", err)
	}
	if !ok {
		return rejectPlayer(ctx, runtime.RejectOwnerChanged), nil
	}
	connection, rejection, admitted := sessionController.AdmitPlayer(ctx, userUUID)
	if !admitted {
		return rejectPlayer(ctx, rejection), nil
	}
	playerAttaches.Inc(ctx, playerAttachOutcomeLabel.Value("accepted"))
	return runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted, Connection: connection}, nil
}

func rejectPlayer(ctx context.Context, reason runtime.RejectReason) runtime.AttachPlayerResult {
	playerAttaches.Inc(ctx, playerAttachOutcomeLabel.Value(string(reason)))
	return runtime.AttachPlayerResult{Outcome: runtime.AttachRejected, Rejection: reason}
}
