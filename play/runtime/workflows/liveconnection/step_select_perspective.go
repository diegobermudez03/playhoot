package liveconnection

import (
	"context"
	"slices"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

var (
	perspectiveOutcomeLabel = metrics.NewLabel("outcome",
		string(runtime.PerspectiveSelected),
		string(runtime.PerspectiveUnknownParticipant),
		string(runtime.PerspectiveCreatorPlaying),
		string(runtime.PerspectiveNotRunning),
	)

	perspectiveSelections = metrics.NewCounter(
		"play_runtime_perspective_selections_total",
		"Admin requests to watch a participant, by outcome.",
		perspectiveOutcomeLabel,
	)
	adminResyncsServed = metrics.NewCounter(
		"play_runtime_admin_resyncs_total",
		"FULL views sent because an admin asked for a resync of who they watch.",
	)
)

// selectPerspective makes admin watch participantID: their current view is
// sent whole from the projection cache, as the connection's fresh baseline.
// A creator who ever had a player connection here never watches anyone.
func (c *sessionController) selectPerspective(ctx context.Context, admin *connection, participantID string) runtime.SelectPerspectiveResult {
	s := c.lock()
	defer c.unlock()
	outcome := s.selectPerspective(ctx, admin, participantID)
	perspectiveSelections.Inc(ctx, perspectiveOutcomeLabel.Value(string(outcome)))
	return runtime.SelectPerspectiveResult{Outcome: outcome}
}

func (s *liveState) selectPerspective(ctx context.Context, admin *connection, participantID string) runtime.SelectPerspectiveOutcome {
	if s.creatorPlayed(admin.userUUID) {
		return runtime.PerspectiveCreatorPlaying
	}
	if s.phase != phaseRunning || s.admins[admin.id] != admin {
		return runtime.PerspectiveNotRunning
	}
	plays := slices.ContainsFunc(s.participants, func(participant runtime.Participant) bool {
		return participant.ParticipantID == participantID
	})
	if !plays {
		return runtime.PerspectiveUnknownParticipant
	}
	admin.watching = participantID
	s.sendFull(ctx, admin, participantID)
	return runtime.PerspectiveSelected
}

// creatorPlayed reports whether the creator had a player connection here or
// is a participant, which the roster remembers after a recovery.
func (s *liveState) creatorPlayed(creatorUUID string) bool {
	_, participant := s.participantOf(creatorUUID)
	return s.playedBy[creatorUUID] || participant
}

// resyncAdmin sends admin the view of who they watch whole again, as a fresh
// baseline. Without a selection, or once the game stopped running, it does
// nothing.
func (c *sessionController) resyncAdmin(ctx context.Context, admin *connection) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning || admin.watching == "" || s.admins[admin.id] != admin {
		return
	}
	s.sendFull(ctx, admin, admin.watching)
	adminResyncsServed.Inc(ctx)
}
