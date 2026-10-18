package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// newStartController returns a fresh Controller whose game allows between
// minPlayers and maxPlayers, with userUUIDs admitted in order.
func newStartController(t *testing.T, minPlayers int, maxPlayers int, userUUIDs ...string) (*Controller, map[string]runtime.PlayerConnection) {
	t.Helper()
	controller, ok, err := NewRegistry().ControllerFor(context.Background(), "session-1", 1, loadManifest(Manifest{MinPlayers: minPlayers, MaxPlayers: maxPlayers}))
	require.NoError(t, err)
	require.True(t, ok)
	players := make(map[string]runtime.PlayerConnection, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		player, _, admitted := controller.AdmitPlayer(context.Background(), userUUID)
		require.True(t, admitted)
		players[userUUID] = player
	}
	return controller, players
}

// startedWith returns what started reports within a short wait, or false.
func startedWith[S any](t *testing.T, started func(context.Context) (S, bool)) (S, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	return started(ctx)
}

func TestBeginStart(t *testing.T) {
	type test struct {
		minPlayers int
		players    []string
		// before runs against the Controller before the call under test.
		before func(t *testing.T, controller *Controller)

		expectedParticipants []runtime.Participant
		expectedOutcome      runtime.StartSessionOutcome
		expectedOK           bool
		expectedPhase        phase
	}

	tests := map[string]func(t *testing.T) test{
		"lobby_is_frozen_with_participants_in_join_order": func(t *testing.T) test {
			return test{
				minPlayers: 2,
				players:    []string{"user-b", "user-a"},
				expectedParticipants: []runtime.Participant{
					{ParticipantID: "P1", UserUUID: "user-b"},
					{ParticipantID: "P2", UserUUID: "user-a"},
				},
				expectedOK:    true,
				expectedPhase: phaseStarting,
			}
		},
		"fewer_players_than_the_minimum": func(t *testing.T) test {
			return test{
				minPlayers:      2,
				players:         []string{"user-a"},
				expectedOutcome: runtime.StartInvalidPlayerCount,
				expectedPhase:   phaseLobby,
			}
		},
		"attempt_already_in_progress": func(t *testing.T) test {
			return test{
				minPlayers: 1,
				players:    []string{"user-a"},
				before: func(t *testing.T, controller *Controller) {
					_, _, ok := controller.BeginStart()
					require.True(t, ok)
				},
				expectedOutcome: runtime.StartInProgress,
				expectedPhase:   phaseStarting,
			}
		},
		"running_game": func(t *testing.T) test {
			return test{
				minPlayers: 1,
				players:    []string{"user-a"},
				before: func(t *testing.T, controller *Controller) {
					_, _, ok := controller.BeginStart()
					require.True(t, ok)
					controller.CompleteStart(Game{}, runtime.GameFrontend{}, nil)
				},
				expectedOutcome: runtime.StartAlreadyStarted,
				expectedPhase:   phaseRunning,
			}
		},
		"aborted_session": func(t *testing.T) test {
			return test{
				minPlayers: 1,
				players:    []string{"user-a"},
				before: func(t *testing.T, controller *Controller) {
					_, _, ok := controller.BeginStart()
					require.True(t, ok)
					controller.AbortStart()
				},
				expectedOutcome: runtime.StartNotInLobby,
				expectedPhase:   phaseEnded,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, _ := newStartController(t, tc.minPlayers, 8, tc.players...)
			if tc.before != nil {
				tc.before(t, controller)
			}

			participants, outcome, ok := controller.BeginStart()

			require.Equal(t, tc.expectedOK, ok)
			require.Equal(t, tc.expectedOutcome, outcome)
			require.Equal(t, tc.expectedParticipants, participants)
			require.Equal(t, tc.expectedPhase, controller.phase)
			_, rejection, admitted := controller.AdmitPlayer(context.Background(), "late-user")
			switch tc.expectedPhase {
			case phaseLobby:
				require.True(t, admitted)
			case phaseEnded:
				require.False(t, admitted)
				require.Equal(t, runtime.RejectSessionUnavailable, rejection)
			default:
				require.False(t, admitted)
				require.Equal(t, runtime.RejectSessionStarted, rejection, "nobody joins once the lobby is frozen")
			}
		})
	}
}

func TestCancelStart(t *testing.T) {
	type test struct {
		// during runs while the Start attempt is in progress.
		during func(t *testing.T, players map[string]runtime.PlayerConnection)

		expectedLobby      []string
		expectedAdminLobby []string
	}

	tests := map[string]func(t *testing.T) test{
		"session_returns_to_its_lobby": func(t *testing.T) test {
			return test{
				expectedLobby:      []string{"user-a", "user-b"},
				expectedAdminLobby: []string{"user-a", "user-b"},
			}
		},
		"players_who_disconnected_during_the_attempt_leave_the_lobby": func(t *testing.T) test {
			return test{
				during: func(t *testing.T, players map[string]runtime.PlayerConnection) {
					players["user-a"].Disconnect()
				},
				expectedLobby:      []string{"user-b"},
				expectedAdminLobby: []string{"user-b"},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players := newStartController(t, 1, 8, "user-a", "user-b")
			admin := controller.AttachAdmin("creator", nil)
			_, _, ok := controller.BeginStart()
			require.True(t, ok)
			if tc.during != nil {
				tc.during(t, players)
			}
			require.Equal(t, []string{"user-a", "user-b"}, controller.lobbySnapshot(), "the lobby stays frozen during the attempt")

			controller.CancelStart()

			require.Equal(t, phaseLobby, controller.phase)
			require.Equal(t, tc.expectedLobby, controller.lobbySnapshot())
			require.Equal(t, tc.expectedAdminLobby, latestLobby(t, admin.NextLobbyUpdate))
			_, _, admitted := controller.AdmitPlayer(context.Background(), "user-c")
			require.True(t, admitted, "the lobby admits players again")
		})
	}
}

func TestCompleteStart(t *testing.T) {
	type test struct {
		// during runs while the Start attempt is in progress.
		during func(t *testing.T, players map[string]runtime.PlayerConnection)

		expectedStartedA bool
		expectedViewA    bool
	}

	game := Game{ContextID: "context-1", ContextRevision: 1, State: []byte(`{"round":1}`), PRNGState: []byte{7}}
	views := map[string][]byte{"P1": []byte(`{"me":"a"}`), "P2": []byte(`{"me":"b"}`)}
	frontend := runtime.GameFrontend{URL: "https://storage.example/frontend?sig=abc", SHA256: "9f86d081", ExpiresAt: time.Date(2026, 10, 17, 21, 0, 0, 0, time.UTC)}
	roster := runtime.SessionStart{
		Participants: []runtime.Participant{
			{ParticipantID: "P1", UserUUID: "user-a"},
			{ParticipantID: "P2", UserUUID: "user-b"},
		},
		Frontend: frontend,
	}

	tests := map[string]func(t *testing.T) test{
		"every_connection_is_told_who_plays_and_players_get_their_first_view": func(t *testing.T) test {
			return test{expectedStartedA: true, expectedViewA: true}
		},
		"player_who_disconnected_during_the_attempt_stays_a_participant": func(t *testing.T) test {
			return test{
				during: func(t *testing.T, players map[string]runtime.PlayerConnection) {
					players["user-a"].Disconnect()
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players := newStartController(t, 1, 8, "user-a", "user-b")
			admin := controller.AttachAdmin("creator", nil)
			_, _, ok := controller.BeginStart()
			require.True(t, ok)
			if tc.during != nil {
				tc.during(t, players)
			}

			controller.CompleteStart(game, frontend, views)

			require.Equal(t, phaseRunning, controller.phase)
			require.Equal(t, game, controller.game)
			require.Equal(t, []string{"user-a", "user-b"}, latestLobby(t, admin.NextLobbyUpdate), "the pending lobby update still arrives")
			adminStart, started := startedWith(t, admin.Started)
			require.True(t, started)
			require.Equal(t, roster, adminStart)

			playerB := players["user-b"]
			require.Equal(t, []string{"user-a", "user-b"}, latestLobby(t, playerB.NextLobbyUpdate))
			start, started := startedWith(t, playerB.Started)
			require.True(t, started)
			require.Equal(t, runtime.PlayerStart{SessionStart: roster, ParticipantID: "P2"}, start)
			view, ok := playerB.NextView(context.Background())
			require.True(t, ok)
			require.Equal(t, runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P2", Full: true, State: []byte(`{"me":"b"}`)}}, view)

			_, started = startedWith(t, players["user-a"].Started)
			require.Equal(t, tc.expectedStartedA, started)
			if tc.expectedViewA {
				view, ok := players["user-a"].NextView(context.Background())
				require.True(t, ok)
				require.Equal(t, "P1", view.ParticipantID)
			}

			later := controller.AttachAdmin("creator", nil)
			_, more := later.NextLobbyUpdate(context.Background())
			require.False(t, more, "an admin attached to a running game gets no lobby")
			laterStart, started := startedWith(t, later.Started)
			require.True(t, started)
			require.Equal(t, roster, laterStart)
		})
	}
}

func TestAbortStart(t *testing.T) {
	controller, players := newStartController(t, 1, 8, "user-a")
	admin := controller.AttachAdmin("creator", nil)
	_, _, ok := controller.BeginStart()
	require.True(t, ok)

	controller.AbortStart()

	require.Equal(t, phaseEnded, controller.phase)
	require.Equal(t, runtime.EndSessionAborted, endedWith(t, admin.Ended))
	require.Equal(t, runtime.EndSessionAborted, endedWith(t, players["user-a"].Ended))
	_, started := startedWith(t, players["user-a"].Started)
	require.False(t, started)
	_, rejection, admitted := controller.AdmitPlayer(context.Background(), "user-b")
	require.False(t, admitted)
	require.Equal(t, runtime.RejectSessionUnavailable, rejection)
	require.Equal(t, runtime.EndSessionEnded, endedWith(t, controller.AttachAdmin("creator", nil).Ended))
}
