package controller

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// testClock is a clock tests move by hand.
type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time                { return c.now }
func (c *testClock) Advance(elapsed time.Duration) { c.now = c.now.Add(elapsed) }

// runningController returns a Controller whose game runs with userUUIDs as
// P1..Pn, every input `move` (a vector2), and the given first views.
func runningController(t *testing.T, views map[string][]byte, userUUIDs ...string) (*Controller, map[string]runtime.PlayerConnection, *testClock) {
	t.Helper()
	clock := &testClock{now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	schema, err := inputs.NewSchema([]inputs.Declaration{{Name: "move", Type: inputs.Vector2}})
	require.NoError(t, err)
	registry := NewRegistry()
	registry.now = clock.Now
	controller, ok, err := registry.ControllerFor(context.Background(), "session-1", 1, loadManifest(Manifest{MaxPlayers: 8, Inputs: schema}))
	require.NoError(t, err)
	require.True(t, ok)
	players := make(map[string]runtime.PlayerConnection, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		player, _, admitted := controller.AdmitPlayer(context.Background(), userUUID)
		require.True(t, admitted)
		players[userUUID] = player
	}
	participants, _, ok := controller.BeginStart()
	require.True(t, ok)
	controller.CompleteStart(participants, Game{ContextID: "context-1", ContextRevision: 1, State: []byte(`{}`)}, runtime.GameFrontend{}, views)
	for _, player := range players {
		_, ok := player.NextView(context.Background())
		require.True(t, ok, "the first FULL view")
	}
	return controller, players, clock
}

func sample(sequence int64, raw string) runtime.Sample {
	return runtime.Sample{Sequence: sequence, Inputs: []byte(raw)}
}

func TestAcceptSample(t *testing.T) {
	type test struct {
		// send runs against the running game; its last error is the result.
		send func(t *testing.T, players map[string]runtime.PlayerConnection, clock *testClock) error

		expectedErrorIs     error
		expectedTransitions []Transition
	}

	ctx := context.Background()
	tests := map[string]func(t *testing.T) test{
		"new_input_state_enters_the_timeline_at_its_session_time": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, players map[string]runtime.PlayerConnection, clock *testClock) error {
					clock.Advance(40 * time.Millisecond)
					return players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`))
				},
				expectedTransitions: []Transition{{ParticipantID: "P1", Inputs: `{"move":[1,0]}`, SessionTimeMs: 40, Order: 1}},
			}
		},
		"every_distinct_transition_is_kept_in_arrival_order_across_players": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, players map[string]runtime.PlayerConnection, clock *testClock) error {
					require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
					clock.Advance(10 * time.Millisecond)
					require.NoError(t, players["user-b"].Sample(ctx, sample(1, `{"move":[0,1]}`)))
					clock.Advance(10 * time.Millisecond)
					require.NoError(t, players["user-a"].Sample(ctx, sample(2, `{"move":[0,0]}`)))
					return players["user-a"].Sample(ctx, sample(3, `{"move":[1,0]}`))
				},
				expectedTransitions: []Transition{
					{ParticipantID: "P1", Inputs: `{"move":[1,0]}`, SessionTimeMs: 0, Order: 1},
					{ParticipantID: "P2", Inputs: `{"move":[0,1]}`, SessionTimeMs: 10, Order: 2},
					{ParticipantID: "P1", Inputs: `{"move":[0,0]}`, SessionTimeMs: 20, Order: 3},
					{ParticipantID: "P1", Inputs: `{"move":[1,0]}`, SessionTimeMs: 20, Order: 4},
				},
			}
		},
		"refresh_of_the_current_state_is_not_a_transition": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, players map[string]runtime.PlayerConnection, clock *testClock) error {
					require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
					return players["user-a"].Sample(ctx, sample(2, `{"move":[1.0,0]}`))
				},
				expectedTransitions: []Transition{{ParticipantID: "P1", Inputs: `{"move":[1,0]}`, SessionTimeMs: 0, Order: 1}},
			}
		},
		"stale_sequence_is_ignored": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, players map[string]runtime.PlayerConnection, clock *testClock) error {
					require.NoError(t, players["user-a"].Sample(ctx, sample(5, `{"move":[1,0]}`)))
					return players["user-a"].Sample(ctx, sample(4, `{"move":[0,1]}`))
				},
				expectedTransitions: []Transition{{ParticipantID: "P1", Inputs: `{"move":[1,0]}`, SessionTimeMs: 0, Order: 1}},
			}
		},
		"defaults_sent_first_are_a_refresh": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, players map[string]runtime.PlayerConnection, clock *testClock) error {
					return players["user-a"].Sample(ctx, sample(1, `{}`))
				},
			}
		},
		"undeclared_input_is_invalid": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, players map[string]runtime.PlayerConnection, clock *testClock) error {
					return players["user-a"].Sample(ctx, sample(1, `{"jump":true}`))
				},
				expectedErrorIs: runtime.ErrInvalidSample,
			}
		},
		"disconnecting_returns_inputs_to_their_defaults": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, players map[string]runtime.PlayerConnection, clock *testClock) error {
					require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
					clock.Advance(30 * time.Millisecond)
					players["user-a"].Disconnect()
					return nil
				},
				expectedTransitions: []Transition{
					{ParticipantID: "P1", Inputs: `{"move":[1,0]}`, SessionTimeMs: 0, Order: 1},
					{ParticipantID: "P1", Inputs: `{"move":[0,0]}`, SessionTimeMs: 30, Order: 2},
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, clock := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, "user-a", "user-b")

			err := tc.send(t, players, clock)

			if tc.expectedErrorIs != nil {
				require.ErrorIs(t, err, tc.expectedErrorIs)
			} else {
				require.NoError(t, err)
			}
			boundary, ok := controller.TakeBoundary()
			require.True(t, ok)
			require.Equal(t, tc.expectedTransitions, boundary.Transitions)
		})
	}
}

func TestSampleBeforeTheGameRunsIsDropped(t *testing.T) {
	controller := newTestController(t, 8)
	player, _, ok := controller.AdmitPlayer(context.Background(), "user-a")
	require.True(t, ok)

	require.NoError(t, player.Sample(context.Background(), sample(1, `{"anything":1}`)))

	_, running := controller.TakeBoundary()
	require.False(t, running)
}

func TestTakeBoundary(t *testing.T) {
	controller, players, clock := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, "user-a", "user-b")
	ctx := context.Background()
	require.NoError(t, players["user-a"].Sample(ctx, sample(3, `{"move":[1,0]}`)))
	require.NoError(t, players["user-b"].Sample(ctx, sample(7, `{}`)))
	clock.Advance(100 * time.Millisecond)

	boundary, ok := controller.TakeBoundary()

	require.True(t, ok)
	require.Equal(t, int64(100), boundary.SessionTimeMs)
	require.Equal(t, []string{"P1", "P2"}, boundary.ParticipantIDs)
	require.Equal(t, map[string]int64{"P1": 3, "P2": 7}, boundary.acks, "refreshes are acknowledged too")
	require.Equal(t, Game{ContextID: "context-1", ContextRevision: 1, State: []byte(`{}`)}, boundary.Game)
	next, ok := controller.TakeBoundary()
	require.True(t, ok)
	require.Empty(t, next.Transitions, "a boundary takes what was pending")
}

func TestRequeueKeepsTheTimelineOrder(t *testing.T) {
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")
	ctx := context.Background()
	require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
	failed, _ := controller.TakeBoundary()
	require.NoError(t, players["user-a"].Sample(ctx, sample(2, `{"move":[0,1]}`)))

	controller.Requeue(failed)

	retried, _ := controller.TakeBoundary()
	require.Equal(t, []uint64{1, 2}, []uint64{retried.Transitions[0].Order, retried.Transitions[1].Order})
}

func TestCommitBoundary(t *testing.T) {
	type test struct {
		// sequence is the SAMPLE P1 sends before the boundary; 0 sends none.
		sequence int64
		view     string
		revision int64

		expectedErrorIs error
		expectedViews   []runtime.OwnView
	}

	ctx := context.Background()
	tests := map[string]func(t *testing.T) test{
		"changed_fields_are_sent_with_the_acknowledgement": func(t *testing.T) test {
			return test{
				sequence: 4, view: `{"round":1,"me":{"x":2,"y":0}}`, revision: 2,
				expectedViews: []runtime.OwnView{{PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{"me":{"x":2}}`)}, ConsumedThroughSampleSequence: 4}},
			}
		},
		"unchanged_view_with_a_new_acknowledgement_is_an_empty_patch": func(t *testing.T) test {
			return test{
				sequence: 4, view: `{"round":1,"me":{"x":0,"y":0}}`, revision: 2,
				expectedViews: []runtime.OwnView{{PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{}`)}, ConsumedThroughSampleSequence: 4}},
			}
		},
		"nothing_new_sends_nothing": func(t *testing.T) test {
			return test{view: `{"round":1,"me":{"x":0,"y":0}}`, revision: 2}
		},
		"field_that_disappeared_sends_the_view_whole": func(t *testing.T) test {
			return test{
				view: `{"round":1}`, revision: 2,
				expectedViews: []runtime.OwnView{{PlayerView: runtime.PlayerView{ParticipantID: "P1", Full: true, State: []byte(`{"round":1}`)}}},
			}
		},
		"execution_that_skips_a_revision_is_out_of_sync": func(t *testing.T) test {
			return test{view: `{"round":2}`, revision: 3, expectedErrorIs: ErrOutOfSync}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{"round":1,"me":{"x":0,"y":0}}`)}, "user-a")
			player := players["user-a"]
			if tc.sequence > 0 {
				require.NoError(t, player.Sample(ctx, sample(tc.sequence, `{"move":[1,0]}`)))
			}
			boundary, _ := controller.TakeBoundary()

			err := controller.CommitBoundary(ctx, boundary, Game{ContextID: "context-1", ContextRevision: tc.revision, State: []byte(`{"n":1}`)}, map[string][]byte{"P1": []byte(tc.view)})

			if tc.expectedErrorIs != nil {
				require.ErrorIs(t, err, tc.expectedErrorIs)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedViews, drainViews(t, player))
			committed, _ := controller.TakeBoundary()
			require.Equal(t, tc.revision, committed.Game.ContextRevision, "the mirror follows the execution")
		})
	}
}

func TestAcknowledgeOnly(t *testing.T) {
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{"round":1}`)}, "user-a")
	ctx := context.Background()
	require.NoError(t, players["user-a"].Sample(ctx, sample(9, `{}`)))
	boundary, _ := controller.TakeBoundary()
	require.Empty(t, boundary.Transitions, "a refresh of the defaults executes nothing")

	controller.AcknowledgeOnly(ctx, boundary)

	require.Equal(t, []runtime.OwnView{{PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{}`)}, ConsumedThroughSampleSequence: 9}}, drainViews(t, players["user-a"]))
}

func TestRunningGameEnds(t *testing.T) {
	t.Run("aborted_game_ends_every_connection", func(t *testing.T) {
		controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")

		controller.AbortRunning()

		require.Equal(t, runtime.EndSessionAborted, endedWith(t, players["user-a"].Ended))
		requireClosed(t, controller.Done())
		_, running := controller.TakeBoundary()
		require.False(t, running)
	})
	t.Run("retired_controller_stops_running", func(t *testing.T) {
		controller, _, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")

		controller.retire()

		requireClosed(t, controller.Done())
		_, running := controller.TakeBoundary()
		require.False(t, running)
	})
	t.Run("flush_requests_coalesce", func(t *testing.T) {
		controller, _, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")

		controller.RequestFlush()
		controller.RequestFlush()

		requireClosed(t, controller.FlushRequests())
		select {
		case <-controller.FlushRequests():
			t.Fatal("two requests before a boundary are one flush")
		default:
		}
	})
}

// drainViews returns the views player has ready.
func drainViews(t *testing.T, player runtime.PlayerConnection) []runtime.OwnView {
	t.Helper()
	var views []runtime.OwnView
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		view, ok := player.NextView(ctx)
		cancel()
		if !ok {
			return views
		}
		if !json.Valid(view.State) {
			t.Fatalf("view state is not JSON: %s", view.State)
		}
		views = append(views, view)
	}
}

// requireClosed fails unless ch is ready to receive.
func requireClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("channel was not ready")
	}
}
