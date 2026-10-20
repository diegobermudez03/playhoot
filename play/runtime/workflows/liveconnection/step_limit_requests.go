package liveconnection

import (
	"context"
)

// timelineRoom is the limit that keeps one more of participantID's own
// operations out of the timeline, or "" when it fits.
func (s *liveState) timelineRoom(participantID string) string {
	switch {
	case s.queued[participantID] >= s.limits.pendingPerParticipant:
		return limitTimelineParticipant
	case s.queuedTotal >= s.limits.pendingPerSession:
		return limitTimelineSession
	}
	return ""
}

// enqueuePlayer appends an operation a player sent to the timeline, counted
// against its limits.
func (s *liveState) enqueuePlayer(operation Operation) {
	operation.fromPlayer = true
	s.queued[operation.ParticipantID]++
	s.queuedTotal++
	s.enqueue(operation)
}

// recount counts the players' operations again after the timeline changed
// as a whole.
func (s *liveState) recount() {
	clear(s.queued)
	s.queuedTotal = 0
	for _, operation := range s.timeline {
		if operation.fromPlayer {
			s.queued[operation.ParticipantID]++
			s.queuedTotal++
		}
	}
}

// commandRoom is the limit that refuses one more COMMAND of participantID,
// or "" when it may wait in the timeline. One the Session's rate refuses
// still counts against its participant's.
func (s *liveState) commandRoom(participantID string) string {
	tokens := s.commands[participantID]
	if len(tokens.pending) >= s.limits.pendingCommands {
		return limitCommandPending
	}
	if limit := s.timelineRoom(participantID); limit != "" {
		return limit
	}
	now := s.now()
	if allowed, _ := tokens.rate.Allow(now); !allowed {
		return limitCommandParticipant
	}
	if allowed, _ := s.commandRate.Allow(now); !allowed {
		return limitCommandSession
	}
	return ""
}

// refused records a request refused by limit.
func refused(ctx context.Context, limit string) {
	limitRejections.Inc(ctx, limitLabel.Value(limit))
}
