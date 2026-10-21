package liveconnection

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestExpireGraces(t *testing.T) {
	type test struct {
		// away runs before the clock moves by elapsed.
		away    func(players map[string]runtime.PlayerConnection)
		elapsed time.Duration

		expectedOperations   []Operation
		expectedDisconnected bool
	}

	tests := map[string]func(t *testing.T) test{
		"grace_running_out_tells_the_game": func(t *testing.T) test {
			return test{
				away:                 func(players map[string]runtime.PlayerConnection) { players["user-a"].Disconnect() },
				elapsed:              3 * time.Second,
				expectedOperations:   []Operation{{ParticipantID: "P1", SessionTimeMs: 3000, Order: 1, Disconnected: true}},
				expectedDisconnected: true,
			}
		},
		"within_the_grace_the_game_is_not_told": func(t *testing.T) test {
			return test{
				away:    func(players map[string]runtime.PlayerConnection) { players["user-a"].Disconnect() },
				elapsed: 2999 * time.Millisecond,
			}
		},
		"attached_participant_never_expires": func(t *testing.T) test {
			return test{away: func(map[string]runtime.PlayerConnection) {}, elapsed: time.Hour}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, clock := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, "user-a", "user-b")
			tc.away(players)
			clock.Advance(tc.elapsed)

			work, _ := controller.takeBoundary()

			require.Equal(t, tc.expectedOperations, work.operations)
			require.Equal(t, tc.expectedDisconnected, controller.state.presence["P1"].disconnected)
			require.False(t, controller.state.presence["P2"].disconnected)
			again, _ := controller.takeBoundary()
			require.Empty(t, again.operations, "the game is told once")
		})
	}
}

func TestParticipantWithoutConnectionAtStartGetsTheGrace(t *testing.T) {
	controller := newTestController(t, 4)
	clock := &testClock{now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	controller.deps.now = clock.Now
	controller.state.now = clock.Now
	ctx := context.Background()
	_, _, admitted := controller.joinLobby(ctx, runtime.Player{UserUUID: "user-a"})
	require.True(t, admitted)
	gone, _, admitted := controller.joinLobby(ctx, runtime.Player{UserUUID: "user-b"})
	require.True(t, admitted)
	participants, _, ok := controller.beginStart()
	require.True(t, ok)
	gone.Disconnect()
	controller.completeStart(participants, Game{ContextRevision: 1, State: []byte(`{}`)}, runtime.GameFrontend{}, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, nil)

	reattached, participant, _ := controller.reattachPlayer(ctx, runtime.Player{UserUUID: "user-b"}, nil)
	require.True(t, participant)
	require.Equal(t, runtime.AttachAccepted, reattached.Outcome, "within the grace they may still come")
	reattached.Connection.Disconnect()
	clock.Advance(3 * time.Second)
	work, _ := controller.takeBoundary()

	require.Equal(t, []Operation{{ParticipantID: "P2", SessionTimeMs: 3000, Order: 1, Disconnected: true}}, work.operations)
}

// namedRunningGame is a running game of Ana (P1) and Bruno (P2), their
// connections' first views already taken.
func namedRunningGame(t *testing.T) (*sessionController, map[string]runtime.PlayerConnection, *testClock) {
	t.Helper()
	controller := newTestController(t, 4)
	clock := &testClock{now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	controller.deps.now = clock.Now
	controller.state.now = clock.Now
	players := make(map[string]runtime.PlayerConnection)
	for _, player := range []runtime.Player{{UserUUID: "user-a", DisplayName: "Ana"}, {UserUUID: "user-b", DisplayName: "Bruno"}} {
		connection, _, admitted := controller.joinLobby(context.Background(), player)
		require.True(t, admitted)
		players[player.UserUUID] = connection
	}
	participants, _, ok := controller.beginStart()
	require.True(t, ok)
	controller.completeStart(participants, Game{ContextRevision: 1, State: []byte(`{}`)}, runtime.GameFrontend{}, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, nil)
	for _, player := range players {
		_, ok := player.NextView(context.Background())
		require.True(t, ok, "the first FULL view")
	}
	return controller, players, clock
}

// latestRoster returns the newest roster update player has waiting.
func latestRoster(t *testing.T, player runtime.PlayerConnection) runtime.RosterUpdate {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	update, ok := player.NextRosterUpdate(ctx)
	require.True(t, ok, "a roster update is waiting")
	return update
}

func rosterOf(statusA runtime.PresenceStatus, statusB runtime.PresenceStatus) runtime.RosterUpdate {
	return runtime.RosterUpdate{Members: []runtime.RosterMember{
		{Participant: runtime.Participant{ParticipantID: "P1", UserUUID: "user-a", DisplayName: "Ana"}, Status: statusA},
		{Participant: runtime.Participant{ParticipantID: "P2", UserUUID: "user-b", DisplayName: "Bruno"}, Status: statusB},
	}}
}

func TestRosterUpdates(t *testing.T) {
	type test struct {
		// change runs against the running game after the start's roster was taken.
		change func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock)

		expected runtime.RosterUpdate
	}

	tests := map[string]func(t *testing.T) test{
		"a_lost_transport_is_reconnecting": func(t *testing.T) test {
			return test{
				change: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection, _ *testClock) {
					players["user-b"].Disconnect()
				},
				expected: rosterOf(runtime.PresenceActive, runtime.PresenceReconnecting),
			}
		},
		"the_grace_running_out_is_disconnected": func(t *testing.T) test {
			return test{
				change: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					players["user-b"].Disconnect()
					clock.Advance(3 * time.Second)
					controller.takeBoundary()
				},
				expected: rosterOf(runtime.PresenceActive, runtime.PresenceDisconnected),
			}
		},
		"leaving_is_left": func(t *testing.T) test {
			return test{
				change: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection, _ *testClock) {
					_, err := players["user-b"].Leave(context.Background())
					require.NoError(t, err)
				},
				expected: rosterOf(runtime.PresenceActive, runtime.PresenceLeft),
			}
		},
		"coming_back_within_the_grace_is_active": func(t *testing.T) test {
			return test{
				change: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, _ *testClock) {
					players["user-b"].Disconnect()
					_, participant, _ := controller.reattachPlayer(context.Background(), runtime.Player{UserUUID: "user-b", DisplayName: "Bruno"}, nil)
					require.True(t, participant)
				},
				expected: rosterOf(runtime.PresenceActive, runtime.PresenceActive),
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, clock := namedRunningGame(t)
			require.Equal(t, rosterOf(runtime.PresenceActive, runtime.PresenceActive), latestRoster(t, players["user-a"]), "the start tells who plays")

			tc.change(t, controller, players, clock)

			require.Equal(t, tc.expected, latestRoster(t, players["user-a"]))
		})
	}
}

func TestDisplayNamesReachTheLobbyAndTheStart(t *testing.T) {
	controller := newTestController(t, 4)
	ctx := context.Background()
	_, _, admitted := controller.joinLobby(ctx, runtime.Player{UserUUID: "user-a", DisplayName: "Ana"})
	require.True(t, admitted)
	second, _, admitted := controller.joinLobby(ctx, runtime.Player{UserUUID: "user-b", DisplayName: "Bruno"})
	require.True(t, admitted)

	lobby, ok := second.NextLobbyUpdate(ctx)
	require.True(t, ok)
	require.Equal(t, []runtime.LobbyMember{{UserUUID: "user-a", DisplayName: "Ana"}, {UserUUID: "user-b", DisplayName: "Bruno"}}, lobby.Members)
	participants, _, ok := controller.beginStart()
	require.True(t, ok)
	require.Equal(t, []runtime.Participant{
		{ParticipantID: "P1", UserUUID: "user-a", DisplayName: "Ana"},
		{ParticipantID: "P2", UserUUID: "user-b", DisplayName: "Bruno"},
	}, participants)
}
