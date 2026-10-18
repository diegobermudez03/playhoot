package liveconnection

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/internal/business/commands"
	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// testClock is a clock tests move by hand.
type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time                { return c.now }
func (c *testClock) Advance(elapsed time.Duration) { c.now = c.now.Add(elapsed) }

// runningController returns a controller whose game runs with userUUIDs as
// P1..Pn, every input `move` (a vector2), the commands `pass` and `move_to`
// (payload `x`, a number from 0 to 10), and the given first views.
func runningController(t *testing.T, views map[string][]byte, userUUIDs ...string) (*sessionController, map[string]runtime.PlayerConnection, *testClock) {
	t.Helper()
	return runningControllerWithTokens(t, views, nil, userUUIDs...)
}

// runningControllerWithTokens is runningController with the COMMAND tokens
// each participant already executed.
func runningControllerWithTokens(t *testing.T, views map[string][]byte, processed map[string][]string, userUUIDs ...string) (*sessionController, map[string]runtime.PlayerConnection, *testClock) {
	t.Helper()
	clock := &testClock{now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	schema, err := inputs.NewSchema([]inputs.Declaration{{Name: "move", Type: inputs.Vector2}})
	require.NoError(t, err)
	zero, ten := 0.0, 10.0
	catalog, err := commands.NewCatalog([]commands.Declaration{
		{Name: "pass"},
		{Name: "move_to", Payload: []inputs.Declaration{{Name: "x", Type: inputs.Number, Min: &zero, Max: &ten}}},
	})
	require.NoError(t, err)
	deps := testDependencies()
	deps.now = clock.Now
	controller := withManifest(newSessionController(context.Background(), "session-1", 1, deps), Manifest{MaxPlayers: 8, Inputs: schema, Commands: catalog})
	players := make(map[string]runtime.PlayerConnection, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		player, _, admitted := controller.joinLobby(context.Background(), userUUID)
		require.True(t, admitted)
		players[userUUID] = player
	}
	participants, _, ok := controller.beginStart()
	require.True(t, ok)
	controller.completeStart(participants, Game{ContextRevision: 1, State: []byte(`{}`)}, runtime.GameFrontend{}, views, processed)
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
		expectedTransitions []Operation
	}

	ctx := context.Background()
	tests := map[string]func(t *testing.T) test{
		"new_input_state_enters_the_timeline_at_its_session_time": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, players map[string]runtime.PlayerConnection, clock *testClock) error {
					clock.Advance(40 * time.Millisecond)
					return players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`))
				},
				expectedTransitions: []Operation{{ParticipantID: "P1", Inputs: `{"move":[1,0]}`, SessionTimeMs: 40, Order: 1}},
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
				expectedTransitions: []Operation{
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
				expectedTransitions: []Operation{{ParticipantID: "P1", Inputs: `{"move":[1,0]}`, SessionTimeMs: 0, Order: 1}},
			}
		},
		"stale_sequence_is_ignored": func(t *testing.T) test {
			return test{
				send: func(t *testing.T, players map[string]runtime.PlayerConnection, clock *testClock) error {
					require.NoError(t, players["user-a"].Sample(ctx, sample(5, `{"move":[1,0]}`)))
					return players["user-a"].Sample(ctx, sample(4, `{"move":[0,1]}`))
				},
				expectedTransitions: []Operation{{ParticipantID: "P1", Inputs: `{"move":[1,0]}`, SessionTimeMs: 0, Order: 1}},
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
				expectedTransitions: []Operation{
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
			boundary, ok := controller.takeBoundary()
			require.True(t, ok)
			require.Equal(t, tc.expectedTransitions, boundary.operations)
		})
	}
}

func TestSampleBeforeTheGameRunsIsDropped(t *testing.T) {
	controller := newTestController(t, 8)
	player, _, ok := controller.joinLobby(context.Background(), "user-a")
	require.True(t, ok)

	require.NoError(t, player.Sample(context.Background(), sample(1, `{"anything":1}`)))

	_, running := controller.takeBoundary()
	require.False(t, running)
}
