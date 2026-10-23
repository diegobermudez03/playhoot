package liveconnection

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestLeave(t *testing.T) {
	type test struct {
		// leaving returns the connection that leaves, after anything it does
		// against the running game.
		leaving func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection

		expectedTimeline     []Operation
		expectedDisconnected bool
		expectedFlush        bool
	}

	ctx := context.Background()

	tests := map[string]func(t *testing.T) test{
		"leaving_a_running_game_disconnects_at_once": func(t *testing.T) test {
			return test{
				leaving: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection {
					require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
					return players["user-a"]
				},
				expectedTimeline: []Operation{
					{ParticipantID: "P1", Order: 1, Inputs: inputs.Snapshot(`{"move":[1,0]}`), fromPlayer: true, sampleSequence: 1},
					{ParticipantID: "P1", Order: 2, Inputs: inputs.Snapshot(`{"move":[0,0]}`)},
					{ParticipantID: "P1", Order: 3, Disconnected: true},
				},
				expectedDisconnected: true,
				expectedFlush:        true,
			}
		},
		"replaced_connection_leaving_changes_nothing": func(t *testing.T) test {
			return test{
				leaving: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection) runtime.PlayerConnection {
					_, participant, _ := controller.reattachPlayer(ctx, runtime.Player{UserUUID: "user-a"}, nil)
					require.True(t, participant)
					return players["user-a"]
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, "user-a", "user-b")
			leaving := tc.leaving(t, controller, players)

			result, err := leaving.Leave(ctx)

			require.NoError(t, err)
			require.Equal(t, runtime.LeaveResult{Outcome: runtime.PlayerLeft}, result)
			require.Equal(t, tc.expectedFlush, len(controller.flush) == 1)
			require.Equal(t, tc.expectedDisconnected, controller.state.presence["P1"].disconnected)
			work, _ := controller.takeBoundary()
			require.Equal(t, tc.expectedTimeline, withoutTimesOrStreams(work.operations))
			if tc.expectedDisconnected {
				require.Equal(t, runtime.EndLeft, endedWith(t, leaving.Ended))
				back, participant, err := controller.attachParticipant(ctx, runtime.Player{UserUUID: "user-a"})
				require.NoError(t, err)
				require.True(t, participant)
				require.Equal(t, runtime.RejectParticipantDisconnected, back.Rejection, "only rejoining brings them back")
			}
		})
	}
}

func TestLeaveDuringStartAttempt(t *testing.T) {
	type test struct {
		// settle ends the Start attempt the leave waits for.
		settle func(t *testing.T, controller *sessionController, participants []runtime.Participant)

		expectedPhase        phase
		expectedLobby        []string
		expectedEnd          runtime.EndReason
		expectedDisconnected bool
	}

	views := map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}

	tests := map[string]func(t *testing.T) test{
		"game_started_disconnects_them_at_once": func(t *testing.T) test {
			return test{
				settle: func(t *testing.T, controller *sessionController, participants []runtime.Participant) {
					controller.completeStart(participants, Game{ContextRevision: 1, State: []byte(`{}`)}, runtime.GameFrontend{}, views, nil)
				},
				expectedPhase:        phaseRunning,
				expectedEnd:          runtime.EndLeft,
				expectedDisconnected: true,
			}
		},
		"failed_start_takes_them_out_of_the_lobby": func(t *testing.T) test {
			return test{
				settle: func(t *testing.T, controller *sessionController, _ []runtime.Participant) {
					controller.cancelStart()
				},
				expectedPhase: phaseLobby,
				expectedLobby: []string{"user-b"},
				expectedEnd:   runtime.EndLeft,
			}
		},
		"session_ended_by_the_attempt_ends_them_with_it": func(t *testing.T) test {
			return test{
				settle: func(t *testing.T, controller *sessionController, _ []runtime.Participant) {
					controller.end(controller.lock(), sessionEnded("ABORTED", "GAME_START_FAILURE"))
					controller.unlock()
				},
				expectedPhase: phaseEnded,
				expectedEnd:   runtime.EndSessionEnded,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			ctx := context.Background()
			controller, players := newStartController(t, 8, "user-a", "user-b")
			participants, _, ok := controller.beginStart()
			require.True(t, ok)

			type leaveResult struct {
				result runtime.LeaveResult
				err    error
			}
			left := make(chan leaveResult, 1)
			go func() {
				result, err := players["user-a"].Leave(ctx)
				left <- leaveResult{result: result, err: err}
			}()
			require.Never(t, func() bool { return len(left) > 0 }, 30*time.Millisecond, 5*time.Millisecond, "the leave waits for the attempt")
			require.Equal(t, []string{"user-a", "user-b"}, controller.lobbySnapshot(), "the waiting leave holds no lock and the lobby stays frozen")

			tc.settle(t, controller, participants)

			var got leaveResult
			select {
			case got = <-left:
			case <-time.After(time.Second):
				t.Fatal("the leave did not apply once the attempt ended")
			}
			require.NoError(t, got.err)
			require.Equal(t, runtime.LeaveResult{Outcome: runtime.PlayerLeft}, got.result)
			require.Equal(t, tc.expectedPhase, controller.currentPhase())
			require.Equal(t, tc.expectedEnd, endedWith(t, players["user-a"].Ended))
			require.Equal(t, tc.expectedLobby, controller.state.lobby)
			if tc.expectedPhase == phaseRunning {
				require.Equal(t, tc.expectedDisconnected, controller.state.presence["P1"].disconnected)
				require.False(t, controller.state.presence["P2"].disconnected)
				require.Len(t, controller.flush, 1, "a boundary tells the game at once")
				work, _ := controller.takeBoundary()
				operations := withoutTimesOrStreams(work.operations)
				require.NotEmpty(t, operations)
				require.Equal(t, Operation{ParticipantID: "P1", Order: operations[len(operations)-1].Order, Disconnected: true}, operations[len(operations)-1])
			}
		})
	}
}

func TestLeaveDuringStartAttemptGivesUpWithItsContext(t *testing.T) {
	controller, players := newStartController(t, 8, "user-a")
	_, _, ok := controller.beginStart()
	require.True(t, ok)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := players["user-a"].Leave(ctx)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, phaseStarting, controller.currentPhase())
	require.Equal(t, []string{"user-a"}, controller.lobbySnapshot())
}
