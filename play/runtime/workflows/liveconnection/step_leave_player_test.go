package liveconnection

import (
	"context"
	"testing"

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
