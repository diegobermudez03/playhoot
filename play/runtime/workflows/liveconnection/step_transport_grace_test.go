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
	_, _, admitted := controller.joinLobby(ctx, "user-a")
	require.True(t, admitted)
	gone, _, admitted := controller.joinLobby(ctx, "user-b")
	require.True(t, admitted)
	participants, _, ok := controller.beginStart()
	require.True(t, ok)
	gone.Disconnect()
	controller.completeStart(participants, Game{ContextRevision: 1, State: []byte(`{}`)}, runtime.GameFrontend{}, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, nil)

	reattached, participant, _ := controller.reattachPlayer(ctx, "user-b")
	require.True(t, participant)
	require.Equal(t, runtime.AttachAccepted, reattached.Outcome, "within the grace they may still come")
	reattached.Connection.Disconnect()
	clock.Advance(3 * time.Second)
	work, _ := controller.takeBoundary()

	require.Equal(t, []Operation{{ParticipantID: "P2", SessionTimeMs: 3000, Order: 1, Disconnected: true}}, work.operations)
}
