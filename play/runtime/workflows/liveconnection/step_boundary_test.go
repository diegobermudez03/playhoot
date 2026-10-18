package liveconnection

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	boundarySessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	boundaryGameVersion = "c1d2e3f4-0000-4000-8000-000000000001"
)

var boundaryScript = []byte(`function start() {}`)

// runningGame is a controller running a one-player game at revision 1 in
// context-1, whose clock the test moves, and that player's connection.
func runningGame(t *testing.T, deps *controllerDependencies) (*sessionController, runtime.PlayerConnection, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	schema, err := inputs.NewSchema([]inputs.Declaration{{Name: "move", Type: inputs.Vector2}})
	require.NoError(t, err)
	deps.now = func() time.Time { return now }
	controller := withManifest(newSessionController(context.Background(), boundarySessionUUID, 1, deps), Manifest{MaxPlayers: 4, Inputs: schema})
	player, _, admitted := controller.joinLobby(context.Background(), "user-a")
	require.True(t, admitted)
	participants, _, ok := controller.beginStart()
	require.True(t, ok)
	controller.contextID = "context-1"
	controller.completeStart(participants, Game{
		GameVersionUUID: boundaryGameVersion, Script: boundaryScript,
		ContextRevision: 1, State: []byte(`{"x":0}`), PRNGState: []byte{1, 0, 0, 0},
	}, runtime.GameFrontend{}, map[string][]byte{"P1": []byte(`{"x":0}`)})
	_, ok = player.NextView(context.Background())
	require.True(t, ok, "the first FULL view")
	return controller, player, &now
}

func nextView(t *testing.T, player runtime.PlayerConnection) (runtime.OwnView, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	return player.NextView(ctx)
}

func TestRunBoundary(t *testing.T) {
	type dependencies struct {
		sandbox *MockgameSandbox
		aborts  *MocksessionAborter
		replay  *MockreplayRecorder
	}
	type test struct {
		// infraFailures is how many boundaries in a row already failed.
		infraFailures int
		// noSample sends no SAMPLE before the boundary.
		noSample bool
		// noContext starts the boundary with the game holding no context.
		noContext bool

		expectedView          *runtime.OwnView
		expectedEnd           runtime.EndReason
		expectedRequeued      bool
		expectedInfraFailures int
		expectedContextID     string
	}

	anyCtx := gomock.Any()
	input := sandbox.SamplesInput{SessionTimeMs: 100, Transitions: []sandbox.Transition{{ParticipantID: "P1", Inputs: []byte(`{"move":[1,0]}`), SessionTimeMs: 40}}}
	snapshot := sandbox.Snapshot{State: []byte(`{"x":0}`), PRNGState: []byte{1, 0, 0, 0}, ContextRevision: 1, SessionTimeMs: 0}
	spec := sandbox.ContextSpec{GameVersionUUID: boundaryGameVersion, Script: boundaryScript}
	executed := sandbox.ExecutionResult{Outcome: sandbox.ScriptSucceeded, State: []byte(`{"x":1}`), PRNGState: []byte{2, 0, 0, 0}, ContextRevision: 2}
	failed := sandbox.ExecutionResult{Outcome: sandbox.ScriptFailed, ScriptError: "boom"}
	movedView := &runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{"x":1}`)}, ConsumedThroughSampleSequence: 1}
	capture := replay.Capture{SessionUUID: boundarySessionUUID, GameVersionUUID: boundaryGameVersion, SessionTimeMs: 100, ContextRevision: 2, State: []byte(`{"x":1}`), Participants: []string{"P1"}}
	restores := func(deps dependencies, contextID string) []any {
		return []any{
			deps.sandbox.EXPECT().CreateContext(anyCtx, spec).Return(contextID, nil),
			deps.sandbox.EXPECT().Restore(anyCtx, contextID, snapshot).Return(sandbox.RestoreResult{Outcome: sandbox.ScriptSucceeded}, nil),
		}
	}
	projects := func(deps dependencies, contextID string) *gomock.Call {
		return deps.sandbox.EXPECT().Project(anyCtx, contextID, "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"x":1}`)}, nil)
	}
	// retriedIn expects the boundary's execution again in a context restored
	// as contextID, succeeding.
	retriedIn := func(deps dependencies, contextID string) []any {
		return append(restores(deps, contextID),
			deps.sandbox.EXPECT().Samples(anyCtx, contextID, input).Return(executed, nil),
			projects(deps, contextID),
			deps.replay.EXPECT().Offer(anyCtx, capture),
		)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"accepted_input_is_executed_projected_and_offered_to_replay": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(executed, nil),
				projects(deps, "context-1"),
				deps.replay.EXPECT().Offer(anyCtx, capture),
			)
			return test{expectedView: movedView, expectedContextID: "context-1"}
		},
		"nothing_to_execute_only_acknowledges": func(t *testing.T, deps dependencies) test {
			return test{noSample: true, expectedContextID: "context-1"}
		},
		"game_without_a_context_runs_in_one_restored_from_the_mirror": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(retriedIn(deps, "context-2")...)
			return test{noContext: true, expectedView: movedView, expectedContextID: "context-2"}
		},
		"script_failing_once_is_retried_in_a_context_restored_from_the_mirror": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(append([]any{
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(failed, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}, retriedIn(deps, "context-2")...)...)
			return test{expectedView: movedView, expectedContextID: "context-2"}
		},
		"script_failing_twice_aborts_the_session": func(t *testing.T, deps dependencies) test {
			calls := []any{
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(failed, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}
			calls = append(calls, restores(deps, "context-2")...)
			calls = append(calls,
				deps.sandbox.EXPECT().Samples(anyCtx, "context-2", input).Return(failed, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-2").Return(nil),
				deps.aborts.EXPECT().AbortRunningSession(anyCtx, boundarySessionUUID, sessionpkg.AbortScriptFailure).Return(nil),
			)
			gomock.InOrder(calls...)
			return test{expectedEnd: runtime.EndSessionAborted}
		},
		"projection_failing_in_the_script_is_a_script_failure": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(append([]any{
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(executed, nil),
				deps.sandbox.EXPECT().Project(anyCtx, "context-1", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptFailed, ScriptError: "hidden"}, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}, retriedIn(deps, "context-2")...)...)
			return test{expectedView: movedView, expectedContextID: "context-2"}
		},
		"infrastructure_failure_is_retried_in_a_restored_context": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(append([]any{
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(sandbox.ExecutionResult{}, errors.New("worker died")),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}, retriedIn(deps, "context-2")...)...)
			return test{expectedView: movedView, expectedContextID: "context-2"}
		},
		"execution_out_of_sync_with_the_mirror_is_retried": func(t *testing.T, deps dependencies) test {
			skipped := executed
			skipped.ContextRevision = 5
			gomock.InOrder(append([]any{
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(skipped, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}, retriedIn(deps, "context-2")...)...)
			return test{expectedView: movedView, expectedContextID: "context-2"}
		},
		"sandbox_unavailable_puts_the_input_back_for_the_next_boundary": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(sandbox.ExecutionResult{}, errors.New("worker died")),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
				deps.sandbox.EXPECT().CreateContext(anyCtx, spec).Return("", sandbox.ErrNoCapacity),
			)
			return test{expectedRequeued: true, expectedInfraFailures: 1}
		},
		"too_many_failed_boundaries_abort_the_session": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(sandbox.ExecutionResult{}, errors.New("worker died")),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
				deps.sandbox.EXPECT().CreateContext(anyCtx, spec).Return("", sandbox.ErrNoCapacity),
				deps.aborts.EXPECT().AbortRunningSession(anyCtx, boundarySessionUUID, sessionpkg.AbortUnrecoverableInfraFailure).Return(nil),
			)
			return test{infraFailures: maxInfraFailures - 1, expectedEnd: runtime.EndSessionAborted, expectedInfraFailures: maxInfraFailures}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{sandbox: NewMockgameSandbox(ctrl), aborts: NewMocksessionAborter(ctrl), replay: NewMockreplayRecorder(ctrl)}
			tc := setup(t, deps)
			controllerDeps := testDependencies()
			controllerDeps.sandbox, controllerDeps.aborts, controllerDeps.replay = deps.sandbox, deps.aborts, deps.replay
			controller, player, now := runningGame(t, controllerDeps)
			controller.infraFailures = tc.infraFailures
			if tc.noContext {
				controller.contextID = ""
			}
			*now = now.Add(40 * time.Millisecond)
			if !tc.noSample {
				require.NoError(t, player.Sample(context.Background(), runtime.Sample{Sequence: 1, Inputs: []byte(`{"move":[1,0]}`)}))
			}
			*now = now.Add(60 * time.Millisecond)

			controller.runBoundary()

			require.Equal(t, tc.expectedInfraFailures, controller.infraFailures)
			require.Equal(t, tc.expectedContextID, controller.contextID)
			view, gotView := nextView(t, player)
			if tc.expectedView != nil {
				require.True(t, gotView)
				require.Equal(t, *tc.expectedView, view)
			} else if tc.expectedEnd == "" {
				require.False(t, gotView, "nothing new to tell the player")
			}
			if tc.expectedEnd != "" {
				require.Equal(t, tc.expectedEnd, endedWith(t, player.Ended))
				requireClosed(t, controller.ended)
				return
			}
			pending, _ := controller.takeBoundary()
			require.Equal(t, tc.expectedRequeued, len(pending.transitions) == 1)
		})
	}
}

func TestTakeBoundary(t *testing.T) {
	controller, players, clock := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, "user-a", "user-b")
	ctx := context.Background()
	require.NoError(t, players["user-a"].Sample(ctx, sample(3, `{"move":[1,0]}`)))
	require.NoError(t, players["user-b"].Sample(ctx, sample(7, `{}`)))
	clock.Advance(100 * time.Millisecond)

	boundary, ok := controller.takeBoundary()

	require.True(t, ok)
	require.Equal(t, int64(100), boundary.sessionTimeMs)
	require.Equal(t, []string{"P1", "P2"}, boundary.participantIDs)
	require.Equal(t, map[string]int64{"P1": 3, "P2": 7}, boundary.acks, "refreshes are acknowledged too")
	require.Equal(t, Game{ContextRevision: 1, State: []byte(`{}`)}, boundary.game)
	next, ok := controller.takeBoundary()
	require.True(t, ok)
	require.Empty(t, next.transitions, "a boundary takes what was pending")
}

func TestRequeueKeepsTheTimelineOrder(t *testing.T) {
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")
	ctx := context.Background()
	require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
	failed, _ := controller.takeBoundary()
	require.NoError(t, players["user-a"].Sample(ctx, sample(2, `{"move":[0,1]}`)))

	controller.requeue(failed)

	retried, _ := controller.takeBoundary()
	require.Equal(t, []uint64{1, 2}, []uint64{retried.transitions[0].Order, retried.transitions[1].Order})
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
			return test{view: `{"round":2}`, revision: 3, expectedErrorIs: errOutOfSync}
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
			boundary, _ := controller.takeBoundary()

			err := controller.commitBoundary(ctx, boundary, Game{ContextRevision: tc.revision, State: []byte(`{"n":1}`)}, map[string][]byte{"P1": []byte(tc.view)})

			if tc.expectedErrorIs != nil {
				require.ErrorIs(t, err, tc.expectedErrorIs)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedViews, drainViews(t, player))
			committed, _ := controller.takeBoundary()
			require.Equal(t, tc.revision, committed.game.ContextRevision, "the mirror follows the execution")
		})
	}
}

func TestAcknowledgeOnly(t *testing.T) {
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{"round":1}`)}, "user-a")
	ctx := context.Background()
	require.NoError(t, players["user-a"].Sample(ctx, sample(9, `{}`)))
	boundary, _ := controller.takeBoundary()
	require.Empty(t, boundary.transitions, "a refresh of the defaults executes nothing")

	controller.acknowledgeOnly(ctx, boundary)

	require.Equal(t, []runtime.OwnView{{PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{}`)}, ConsumedThroughSampleSequence: 9}}, drainViews(t, players["user-a"]))
}

func TestRunningGameEnds(t *testing.T) {
	t.Run("aborted_game_ends_every_connection", func(t *testing.T) {
		controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")
		aborts := NewMocksessionAborter(gomock.NewController(t))
		controller.deps.aborts = aborts
		aborts.EXPECT().AbortRunningSession(context.Background(), "session-1", sessionpkg.AbortScriptFailure).Return(nil)

		controller.abort(context.Background(), sessionpkg.AbortScriptFailure, errScript{detail: "boom"})

		require.Equal(t, runtime.EndSessionAborted, endedWith(t, players["user-a"].Ended))
		requireClosed(t, controller.ended)
		_, running := controller.takeBoundary()
		require.False(t, running)
	})
	t.Run("retired_controller_stops_running", func(t *testing.T) {
		controller, _, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")

		controller.retire()

		requireClosed(t, controller.ended)
		_, running := controller.takeBoundary()
		require.False(t, running)
	})
	t.Run("flush_requests_coalesce", func(t *testing.T) {
		controller, _, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")

		controller.requestFlush()
		controller.requestFlush()

		requireClosed(t, controller.flush)
		select {
		case <-controller.flush:
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
