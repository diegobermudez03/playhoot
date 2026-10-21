package liveconnection

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestReattachPlayer(t *testing.T) {
	type test struct {
		// before runs against the running game before user-a attaches again.
		before func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock)
		// userUUID attaches; user-a when empty.
		userUUID string

		expectedParticipant bool
		expectedResult      runtime.AttachPlayerResult
		expectedOlderEnd    runtime.EndReason
		// expectedTimeline is what the next boundary executes.
		expectedTimeline []Operation
	}

	ctx := context.Background()
	moving := inputs.Snapshot(`{"move":[1,0]}`)
	defaults := inputs.Snapshot(`{"move":[0,0]}`)

	tests := map[string]func(t *testing.T) test{
		"lost_transport_comes_back_within_the_grace_without_telling_the_game": func(t *testing.T) test {
			return test{
				before: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
					players["user-a"].Disconnect()
					clock.Advance(2900 * time.Millisecond)
				},
				expectedParticipant: true,
				expectedResult:      runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted},
				expectedOlderEnd:    runtime.EndDisconnected,
				expectedTimeline: []Operation{
					{ParticipantID: "P1", Order: 1, Inputs: moving, fromPlayer: true, sampleSequence: 1},
					{ParticipantID: "P1", Order: 2, Inputs: defaults},
				},
			}
		},
		"new_device_replaces_the_older_connection_and_resets_the_inputs": func(t *testing.T) test {
			return test{
				before: func(t *testing.T, _ *sessionController, players map[string]runtime.PlayerConnection, _ *testClock) {
					require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
				},
				expectedParticipant: true,
				expectedResult:      runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted},
				expectedOlderEnd:    runtime.EndReplaced,
				expectedTimeline: []Operation{
					{ParticipantID: "P1", Order: 1, Inputs: moving, fromPlayer: true, sampleSequence: 1},
					{ParticipantID: "P1", Order: 2, Inputs: defaults},
				},
			}
		},
		"participant_away_past_the_grace_of_a_game_without_rejoin_is_disconnected": func(t *testing.T) test {
			return test{
				before: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					players["user-a"].Disconnect()
					clock.Advance(3 * time.Second)
					expired, _ := controller.takeBoundary()
					require.Equal(t, []Operation{{ParticipantID: "P1", SessionTimeMs: 3000, Order: 1, Disconnected: true}}, expired.operations)
				},
				expectedParticipant: true,
				expectedResult:      runtime.AttachPlayerResult{Outcome: runtime.AttachRejected, Rejection: runtime.RejectParticipantDisconnected},
				expectedOlderEnd:    runtime.EndDisconnected,
			}
		},
		"user_who_does_not_play_is_not_a_reattach": func(t *testing.T) test {
			return test{userUUID: "user-z"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, clock := runningController(t, map[string][]byte{"P1": []byte(`{"x":1}`), "P2": []byte(`{}`)}, "user-a", "user-b")
			if tc.before != nil {
				tc.before(t, controller, players, clock)
			}
			userUUID := tc.userUUID
			if userUUID == "" {
				userUUID = "user-a"
			}

			result, participant, err := controller.attachParticipant(ctx, runtime.Player{UserUUID: userUUID})

			require.NoError(t, err)
			require.Equal(t, tc.expectedParticipant, participant)
			if !participant {
				return
			}
			connection := result.Connection
			result.Connection = runtime.PlayerConnection{}
			require.Equal(t, tc.expectedResult, result)
			require.Equal(t, tc.expectedOlderEnd, endedWith(t, players["user-a"].Ended))
			if result.Outcome != runtime.AttachAccepted {
				return
			}
			start, started := connection.Started(ctx)
			require.True(t, started)
			require.Equal(t, "P1", start.ParticipantID)
			view, ok := connection.NextView(ctx)
			require.True(t, ok)
			require.Equal(t, runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", Full: true, Revision: 1, State: []byte(`{"x":1}`)}}, view)
			clock.Advance(time.Second)
			next, _ := controller.takeBoundary()
			require.Equal(t, tc.expectedTimeline, withoutTimesOrStreams(next.operations), "the game is never told about a reattach")
		})
	}
}

func TestReattachedConnectionNumbersItsSamplesAnew(t *testing.T) {
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")
	ctx := context.Background()
	older := players["user-a"]
	require.NoError(t, older.Sample(ctx, sample(40, `{"move":[1,0]}`)))
	inFlight, _ := controller.takeBoundary()

	reattached, participant, _ := controller.reattachPlayer(ctx, runtime.Player{UserUUID: "user-a"}, nil)
	require.True(t, participant)
	newer := reattached.Connection
	_, ok := newer.NextView(ctx)
	require.True(t, ok, "the FULL view")
	require.NoError(t, controller.commitBoundary(ctx, inFlight, execution{game: Game{ContextRevision: 2, State: []byte(`{}`)}, views: map[string][]byte{"P1": []byte(`{}`)}}))
	require.Empty(t, drainViews(t, newer), "the older connection's acknowledgement is not the newer one's")
	require.NoError(t, older.Sample(ctx, sample(41, `{"move":[0,1]}`)))
	require.NoError(t, newer.Sample(ctx, sample(1, `{"move":[0,1]}`)))

	next, _ := controller.takeBoundary()

	require.Equal(t, []Operation{
		{ParticipantID: "P1", Order: 2, Inputs: inputs.Snapshot(`{"move":[0,0]}`)},
		{ParticipantID: "P1", Order: 3, Inputs: inputs.Snapshot(`{"move":[0,1]}`), fromPlayer: true, sampleSequence: 1},
	}, withoutTimesOrStreams(next.operations), "only the newer connection plays")
	require.Equal(t, int64(1), next.acks["P1"].through)
}

// withoutTimesOrStreams returns operations with their Session times and the
// random IDs of the connections their SAMPLEs came from cleared.
func withoutTimesOrStreams(operations []Operation) []Operation {
	var cleared []Operation
	for _, operation := range withoutStreams(operations) {
		operation.SessionTimeMs = 0
		cleared = append(cleared, operation)
	}
	return cleared
}

// withoutStreams returns operations with the random IDs of the connections
// their SAMPLEs came from cleared.
func withoutStreams(operations []Operation) []Operation {
	var cleared []Operation
	for _, operation := range operations {
		operation.sampleStream = ""
		cleared = append(cleared, operation)
	}
	return cleared
}
