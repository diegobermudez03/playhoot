package liveconnection

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

func TestTakeBoundaryTakesALongTimelineInPart(t *testing.T) {
	controller, players, clock := runningControllerWithTokens(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, nil, func(limits *liveLimits) {
		limits.boundaryOperations = 2
	}, "user-a", "user-b")
	controller.state.manifest.Update = true
	ctx := context.Background()
	require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
	clock.Advance(10 * time.Millisecond)
	require.NoError(t, players["user-b"].Sample(ctx, sample(5, `{"move":[0,1]}`)))
	clock.Advance(10 * time.Millisecond)
	require.NoError(t, players["user-a"].Sample(ctx, sample(2, `{"move":[0,0]}`)))
	// A refresh moves the high-water mark, never past what is still waiting.
	require.NoError(t, players["user-a"].Sample(ctx, sample(3, `{"move":[0,0]}`)))
	clock.Advance(80 * time.Millisecond)

	first, ok := controller.takeBoundary()

	require.True(t, ok)
	require.Equal(t, []Operation{
		{ParticipantID: "P1", Inputs: inputs.Snapshot(`{"move":[1,0]}`), SessionTimeMs: 0, Order: 1, fromPlayer: true, sampleSequence: 1},
		{ParticipantID: "P2", Inputs: inputs.Snapshot(`{"move":[0,1]}`), SessionTimeMs: 10, Order: 2, fromPlayer: true, sampleSequence: 5},
	}, withoutStreams(first.operations))
	require.False(t, first.update, "update() waits for the boundary that takes the rest")
	require.Equal(t, int64(10), first.sessionTimeMs, "a partial boundary ends at its last operation")
	require.Equal(t, int64(1), first.acks["P1"].through, "P1's later transition is still waiting")
	require.Equal(t, int64(5), first.acks["P2"].through)
	requireClosed(t, controller.flush)

	rest, ok := controller.takeBoundary()

	require.True(t, ok)
	require.Equal(t, []Operation{
		{ParticipantID: "P1", Inputs: inputs.Snapshot(`{"move":[0,0]}`), SessionTimeMs: 20, Order: 3, fromPlayer: true, sampleSequence: 2},
	}, withoutStreams(rest.operations))
	require.True(t, rest.update)
	require.Equal(t, int64(100), rest.sessionTimeMs)
	require.Equal(t, int64(3), rest.acks["P1"].through)
}

func TestTimelineRoomCountsWhatIsWaiting(t *testing.T) {
	controller, players, _ := runningControllerWithTokens(t, map[string][]byte{"P1": []byte(`{}`)}, nil, func(limits *liveLimits) {
		limits.pendingPerParticipant = 1
	}, "user-a")
	ctx := context.Background()
	require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
	failed, _ := controller.takeBoundary()
	require.Empty(t, controller.state.timelineRoom("P1"), "a taken operation no longer waits")

	controller.requeue(failed)

	require.Equal(t, limitTimelineParticipant, controller.state.timelineRoom("P1"), "a requeued one waits again")
}

func TestParticipantCommandsRemember(t *testing.T) {
	type test struct {
		processed []string
		executed  []string

		expectedOrder     []string
		expectedForgotten []string
	}

	tests := map[string]func(t *testing.T) test{
		"tokens_beyond_the_maximum_forget_the_oldest": func(t *testing.T) test {
			return test{
				processed:         []string{"a", "b"},
				executed:          []string{"c", "d"},
				expectedOrder:     []string{"b", "c", "d"},
				expectedForgotten: []string{"a"},
			}
		},
		"recovered_tokens_beyond_the_maximum_keep_the_latest": func(t *testing.T) test {
			return test{
				processed:         []string{"a", "b", "c", "d"},
				expectedOrder:     []string{"b", "c", "d"},
				expectedForgotten: []string{"a"},
			}
		},
		"a_token_remembered_twice_counts_once": func(t *testing.T) test {
			return test{
				processed:     []string{"a", "a"},
				executed:      []string{"b"},
				expectedOrder: []string{"a", "b"},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			limits := defaultLiveLimits()
			limits.processedTokens = 3
			tokens := newParticipantCommands(tc.processed, limits)

			for _, token := range tc.executed {
				tokens.remember(token)
			}

			require.Equal(t, tc.expectedOrder, tokens.order)
			require.Len(t, tokens.processed, len(tc.expectedOrder))
			for _, token := range tc.expectedOrder {
				require.True(t, tokens.processed[token])
			}
			for _, token := range tc.expectedForgotten {
				require.False(t, tokens.processed[token])
			}
		})
	}
}

func TestRegisterAdminBeyondTheSessionsLimit(t *testing.T) {
	controller := newTestController(t, 8)
	controller.deps.liveLimits.admins = 2
	first := controller.mustRegisterAdmin("creator")
	controller.mustRegisterAdmin("creator")

	_, registered := controller.registerAdmin("creator")
	require.False(t, registered)
	require.Equal(t, 2, controller.AdminCount())

	first.Disconnect()
	_, registered = controller.registerAdmin("creator")
	require.True(t, registered, "a disconnected admin gives its place back")
}

func TestRunRequestedBoundary(t *testing.T) {
	type test struct {
		// ticks run before the requested boundaries.
		ticks     int
		requested int

		expectedRan []bool
	}

	tests := map[string]func(t *testing.T) test{
		"requested_boundaries_beyond_the_rate_wait_for_the_next_tick": func(t *testing.T) test {
			return test{requested: 3, expectedRan: []bool{true, true, false}}
		},
		"ticks_count_against_the_rate": func(t *testing.T) test {
			return test{ticks: 1, requested: 2, expectedRan: []bool{true, false}}
		},
		"ticks_always_run_even_beyond_the_rate": func(t *testing.T) test {
			return test{ticks: 3, requested: 1, expectedRan: []bool{false}}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			deps := testDependencies()
			deps.liveLimits.boundaries = []ratelimit.Limit{{Count: 2, Per: time.Minute}}
			// A controller without a running game: a boundary does nothing.
			controller := newSessionController(context.Background(), "session-1", 1, deps)
			for range tc.ticks {
				controller.runTick()
			}

			var ran []bool
			for range tc.requested {
				ran = append(ran, controller.runRequestedBoundary())
			}

			require.Equal(t, tc.expectedRan, ran)
		})
	}
}

func TestViewFeedPublish(t *testing.T) {
	type test struct {
		pending []runtime.OwnView
		view    runtime.OwnView
		whole   []byte

		expectedCollapsed bool
		expectedPending   []runtime.OwnView
	}

	patch := func(revision int64, events ...string) runtime.OwnView {
		view := runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", Revision: revision, State: []byte(`{"x":1}`)}}
		for _, event := range events {
			view.Events = append(view.Events, []byte(event))
		}
		return view
	}

	tests := map[string]func(t *testing.T) test{
		"a_connection_keeping_up_gets_every_view": func(t *testing.T) test {
			return test{
				pending:         []runtime.OwnView{patch(1)},
				view:            patch(2),
				whole:           []byte(`{"x":2}`),
				expectedPending: []runtime.OwnView{patch(1), patch(2)},
			}
		},
		"a_connection_behind_gets_one_full_view_keeping_its_events": func(t *testing.T) test {
			latest := patch(4, `"c"`)
			latest.ConsumedThroughSampleSequence = 9
			latest.Errors = [][]byte{[]byte(`"late"`)}
			watchedBefore := patch(2, `"other"`)
			watchedBefore.ParticipantID = "P2"
			return test{
				pending:           []runtime.OwnView{patch(1, `"a"`), watchedBefore, patch(3, `"b"`)},
				view:              latest,
				whole:             []byte(`{"x":4}`),
				expectedCollapsed: true,
				expectedPending: []runtime.OwnView{{
					PlayerView:                    runtime.PlayerView{ParticipantID: "P1", Full: true, Revision: 4, State: []byte(`{"x":4}`)},
					ConsumedThroughSampleSequence: 9,
					Errors:                        [][]byte{[]byte(`"late"`)},
					Events:                        [][]byte{[]byte(`"a"`), []byte(`"b"`), []byte(`"c"`)},
				}},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			feed := newViewFeed()
			feed.pending = tc.pending

			collapsed := feed.publish(tc.view, tc.whole, 3)

			require.Equal(t, tc.expectedCollapsed, collapsed)
			require.Equal(t, tc.expectedPending, feed.pending)
		})
	}
}
