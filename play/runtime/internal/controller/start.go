package controller

import (
	"context"
	"fmt"
	"slices"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// Game is the confirmed state of a running game: the Runtime's mirror of its
// execution context.
type Game struct {
	ContextID       string
	ContextRevision int64

	// State is the authoritative game state, as JSON.
	State []byte

	// PRNGState is the opaque state of the script's randomness.
	PRNGState []byte
}

// BeginStart freezes the lobby for one Start attempt and returns its
// participants, numbered P1..Pn in the order they joined. ok is false, with
// the outcome, when the Session cannot start now. Whether the game allows that
// many players is the durable Session's decision.
func (c *Controller) BeginStart() (participants []runtime.Participant, outcome runtime.StartSessionOutcome, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.phase {
	case phaseLobby:
	case phaseStarting:
		return nil, runtime.StartInProgress, false
	case phaseRunning:
		return nil, runtime.StartAlreadyStarted, false
	default:
		return nil, runtime.StartNotInLobby, false
	}
	c.phase = phaseStarting
	c.participants = make([]runtime.Participant, len(c.lobby))
	for i, userUUID := range c.lobby {
		c.participants[i] = runtime.Participant{ParticipantID: fmt.Sprintf("P%d", i+1), UserUUID: userUUID}
	}
	return slices.Clone(c.participants), "", true
}

// CancelStart returns the Session to its lobby after a Start attempt that
// did not start it. Players who disconnected during the attempt leave it now.
func (c *Controller) CancelStart() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != phaseStarting {
		return
	}
	c.phase = phaseLobby
	c.participants = nil
	before := len(c.lobby)
	c.lobby = slices.DeleteFunc(c.lobby, func(userUUID string) bool {
		if c.players[userUUID].isEnded() {
			delete(c.players, userUUID)
			return true
		}
		return false
	})
	if len(c.lobby) != before {
		c.broadcastLobbyLocked()
	}
}

// CompleteStart runs the started game with participants, the roster the
// durable Session committed: the lobby ends, every connection is told who
// plays and gets frontend, and each connected participant gets their first
// FULL view from views, keyed by ParticipantID. A lobby player left out of
// participants is no longer part of the Session: their connection ends.
func (c *Controller) CompleteStart(participants []runtime.Participant, game Game, frontend runtime.GameFrontend, views map[string][]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != phaseStarting {
		return
	}
	c.phase = phaseRunning
	c.participants = slices.Clone(participants)
	c.game = game
	c.frontend = frontend
	c.lobby = nil
	inRoster := make(map[string]bool, len(participants))
	for _, participant := range participants {
		inRoster[participant.UserUUID] = true
	}
	for userUUID, player := range c.players {
		if !inRoster[userUUID] {
			player.end(runtime.EndSessionEnded)
			delete(c.players, userUUID)
		}
	}
	for _, admin := range c.admins {
		admin.lobbyUpdates.finish()
		close(admin.started)
	}
	for _, participant := range c.participants {
		player, found := c.players[participant.UserUUID]
		if !found || player.isEnded() {
			continue
		}
		player.participantID = participant.ParticipantID
		player.lobbyUpdates.finish()
		player.views.publish(runtime.OwnView{PlayerView: runtime.PlayerView{
			ParticipantID: participant.ParticipantID,
			Full:          true,
			State:         views[participant.ParticipantID],
		}})
		// Closed last, so everything above is visible to whoever sees it.
		close(player.started)
	}
}

// AbortStart ends the Session after its game failed to start: every
// connection ends as aborted.
func (c *Controller) AbortStart() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != phaseStarting {
		return
	}
	c.phase = phaseEnded
	c.lobby = nil
	for id, admin := range c.admins {
		admin.end(runtime.EndSessionAborted)
		delete(c.admins, id)
	}
	for userUUID, player := range c.players {
		player.end(runtime.EndSessionAborted)
		delete(c.players, userUUID)
	}
}

// sessionStart blocks until the game starts for conn and returns who plays;
// false means conn ended, or ctx was done, first.
func (c *Controller) sessionStart(ctx context.Context, conn *connection) (runtime.SessionStart, bool) {
	select {
	case <-conn.started:
	case <-conn.ended:
		select {
		case <-conn.started:
		default:
			return runtime.SessionStart{}, false
		}
	case <-ctx.Done():
		return runtime.SessionStart{}, false
	}
	// Written before started was closed and never again.
	return runtime.SessionStart{Participants: slices.Clone(c.participants), Frontend: c.frontend}, true
}
