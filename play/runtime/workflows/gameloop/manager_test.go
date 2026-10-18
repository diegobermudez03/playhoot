package gameloop

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/controller"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	sessionUUID     = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	gameVersionUUID = "c1d2e3f4-0000-4000-8000-000000000001"
	userA           = "aaaaaaaa-aaaa-4aaa-8aaa-000000000001"
)

var script = []byte(`function start() {}`)

// runningGame is a Controller running a one-player game at revision 1, whose
// clock the test moves, and that player's connection.
func runningGame(t *testing.T) (*controller.Controller, runtime.PlayerConnection, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	schema, err := inputs.NewSchema([]inputs.Declaration{{Name: "move", Type: inputs.Vector2}})
	require.NoError(t, err)
	registry := controller.NewRegistryWithClock(func() time.Time { return now })
	sessionController, ok, err := registry.ControllerFor(context.Background(), sessionUUID, 1, func(context.Context) (controller.Manifest, error) {
		return controller.Manifest{MaxPlayers: 4, Inputs: schema}, nil
	})
	require.NoError(t, err)
	require.True(t, ok)
	player, _, admitted := sessionController.AdmitPlayer(context.Background(), userA)
	require.True(t, admitted)
	participants, _, ok := sessionController.BeginStart()
	require.True(t, ok)
	sessionController.CompleteStart(participants, controller.Game{
		ContextID: "context-1", GameVersionUUID: gameVersionUUID, Script: script,
		ContextRevision: 1, State: []byte(`{"x":0}`), PRNGState: []byte{1, 0, 0, 0},
	}, runtime.GameFrontend{}, map[string][]byte{"P1": []byte(`{"x":0}`)})
	_, ok = player.NextView(context.Background())
	require.True(t, ok, "the first FULL view")
	return sessionController, player, &now
}

func nextView(t *testing.T, player runtime.PlayerConnection) (runtime.OwnView, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	return player.NextView(ctx)
}

func TestBoundary(t *testing.T) {
	type dependencies struct {
		sandbox  *MockgameSandbox
		sessions *MocksessionAborter
		replay   *MockreplayRecorder
	}
	type test struct {
		// infraFailures is how many boundaries in a row already failed.
		infraFailures int
		// noSample sends no SAMPLE before the boundary.
		noSample bool

		expectedKeepRunning   bool
		expectedView          *runtime.OwnView
		expectedEnd           runtime.EndReason
		expectedRequeued      bool
		expectedInfraFailures int
	}

	anyCtx := gomock.Any()
	input := sandbox.SamplesInput{SessionTimeMs: 100, Transitions: []sandbox.Transition{{ParticipantID: "P1", Inputs: []byte(`{"move":[1,0]}`), SessionTimeMs: 40}}}
	snapshot := sandbox.Snapshot{State: []byte(`{"x":0}`), PRNGState: []byte{1, 0, 0, 0}, ContextRevision: 1, SessionTimeMs: 0}
	spec := sandbox.ContextSpec{GameVersionUUID: gameVersionUUID, Script: script}
	executed := sandbox.ExecutionResult{Outcome: sandbox.ScriptSucceeded, State: []byte(`{"x":1}`), PRNGState: []byte{2, 0, 0, 0}, ContextRevision: 2}
	failed := sandbox.ExecutionResult{Outcome: sandbox.ScriptFailed, ScriptError: "boom"}
	movedView := &runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{"x":1}`)}, ConsumedThroughSampleSequence: 1}
	capture := replay.Capture{SessionUUID: sessionUUID, GameVersionUUID: gameVersionUUID, SessionTimeMs: 100, ContextRevision: 2, State: []byte(`{"x":1}`), Participants: []string{"P1"}}
	restores := func(deps dependencies, contextID string) []any {
		return []any{
			deps.sandbox.EXPECT().CreateContext(anyCtx, spec).Return(contextID, nil),
			deps.sandbox.EXPECT().Restore(anyCtx, contextID, snapshot).Return(sandbox.RestoreResult{Outcome: sandbox.ScriptSucceeded}, nil),
		}
	}
	projects := func(deps dependencies, contextID string) *gomock.Call {
		return deps.sandbox.EXPECT().Project(anyCtx, contextID, "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"x":1}`)}, nil)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"accepted_input_is_executed_projected_and_offered_to_replay": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(executed, nil),
				projects(deps, "context-1"),
				deps.replay.EXPECT().Offer(anyCtx, capture),
			)
			return test{expectedKeepRunning: true, expectedView: movedView}
		},
		"nothing_to_execute_only_acknowledges": func(t *testing.T, deps dependencies) test {
			return test{noSample: true, expectedKeepRunning: true}
		},
		"script_failing_once_is_retried_in_a_context_restored_from_the_mirror": func(t *testing.T, deps dependencies) test {
			calls := []any{
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(failed, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}
			calls = append(calls, restores(deps, "context-2")...)
			calls = append(calls,
				deps.sandbox.EXPECT().Samples(anyCtx, "context-2", input).Return(executed, nil),
				projects(deps, "context-2"),
				deps.replay.EXPECT().Offer(anyCtx, capture),
			)
			gomock.InOrder(calls...)
			return test{expectedKeepRunning: true, expectedView: movedView}
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
				deps.sessions.EXPECT().AbortRunningSession(anyCtx, sessionUUID, sessionpkg.AbortScriptFailure).Return(nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			)
			gomock.InOrder(calls...)
			return test{expectedEnd: runtime.EndSessionAborted}
		},
		"projection_failing_in_the_script_is_a_script_failure": func(t *testing.T, deps dependencies) test {
			calls := []any{
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(executed, nil),
				deps.sandbox.EXPECT().Project(anyCtx, "context-1", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptFailed, ScriptError: "hidden"}, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}
			calls = append(calls, restores(deps, "context-2")...)
			calls = append(calls,
				deps.sandbox.EXPECT().Samples(anyCtx, "context-2", input).Return(executed, nil),
				projects(deps, "context-2"),
				deps.replay.EXPECT().Offer(anyCtx, capture),
			)
			gomock.InOrder(calls...)
			return test{expectedKeepRunning: true, expectedView: movedView}
		},
		"infrastructure_failure_is_retried_in_a_restored_context": func(t *testing.T, deps dependencies) test {
			calls := []any{
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(sandbox.ExecutionResult{}, errors.New("worker died")),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}
			calls = append(calls, restores(deps, "context-2")...)
			calls = append(calls,
				deps.sandbox.EXPECT().Samples(anyCtx, "context-2", input).Return(executed, nil),
				projects(deps, "context-2"),
				deps.replay.EXPECT().Offer(anyCtx, capture),
			)
			gomock.InOrder(calls...)
			return test{expectedKeepRunning: true, expectedView: movedView}
		},
		"execution_out_of_sync_with_the_mirror_is_retried": func(t *testing.T, deps dependencies) test {
			skipped := executed
			skipped.ContextRevision = 5
			calls := []any{
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(skipped, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}
			calls = append(calls, restores(deps, "context-2")...)
			calls = append(calls,
				deps.sandbox.EXPECT().Samples(anyCtx, "context-2", input).Return(executed, nil),
				projects(deps, "context-2"),
				deps.replay.EXPECT().Offer(anyCtx, capture),
			)
			gomock.InOrder(calls...)
			return test{expectedKeepRunning: true, expectedView: movedView}
		},
		"sandbox_unavailable_puts_the_input_back_for_the_next_boundary": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(sandbox.ExecutionResult{}, errors.New("worker died")),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
				deps.sandbox.EXPECT().CreateContext(anyCtx, spec).Return("", sandbox.ErrNoCapacity),
			)
			return test{expectedKeepRunning: true, expectedRequeued: true, expectedInfraFailures: 1}
		},
		"too_many_failed_boundaries_abort_the_session": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Samples(anyCtx, "context-1", input).Return(sandbox.ExecutionResult{}, errors.New("worker died")),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
				deps.sandbox.EXPECT().CreateContext(anyCtx, spec).Return("", sandbox.ErrNoCapacity),
				deps.sessions.EXPECT().AbortRunningSession(anyCtx, sessionUUID, sessionpkg.AbortUnrecoverableInfraFailure).Return(nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			)
			return test{infraFailures: maxInfraFailures - 1, expectedEnd: runtime.EndSessionAborted, expectedInfraFailures: maxInfraFailures}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{sandbox: NewMockgameSandbox(ctrl), sessions: NewMocksessionAborter(ctrl), replay: NewMockreplayRecorder(ctrl)}
			tc := setup(t, deps)
			manager := New(deps.sandbox, deps.sessions, deps.replay)
			t.Cleanup(manager.Close)
			sessionController, player, now := runningGame(t)
			*now = now.Add(40 * time.Millisecond)
			if !tc.noSample {
				require.NoError(t, player.Sample(context.Background(), runtime.Sample{Sequence: 1, Inputs: []byte(`{"move":[1,0]}`)}))
			}
			*now = now.Add(60 * time.Millisecond)
			state := &laneState{infraFailures: tc.infraFailures}

			keepRunning := manager.boundary(sessionUUID, sessionController, state)

			require.Equal(t, tc.expectedKeepRunning, keepRunning)
			require.Equal(t, tc.expectedInfraFailures, state.infraFailures)
			view, gotView := nextView(t, player)
			if tc.expectedView != nil {
				require.True(t, gotView)
				require.Equal(t, *tc.expectedView, view)
			} else if tc.expectedEnd == "" {
				require.False(t, gotView, "nothing new to tell the player")
			}
			if tc.expectedEnd != "" {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				end, ended := player.Ended(ctx)
				require.True(t, ended)
				require.Equal(t, runtime.ConnectionEnd{Reason: tc.expectedEnd}, end)
				return
			}
			pending, _ := sessionController.TakeBoundary()
			require.Equal(t, tc.expectedRequeued, len(pending.Transitions) == 1)
		})
	}
}

func TestLane(t *testing.T) {
	ctrl := gomock.NewController(t)
	sandboxMock := NewMockgameSandbox(ctrl)
	manager := New(sandboxMock, NewMocksessionAborter(ctrl), NewMockreplayRecorder(ctrl))
	sessionController, player, _ := runningGame(t)

	t.Run("flush_runs_a_boundary_without_waiting_for_the_tick", func(t *testing.T) {
		require.NoError(t, player.Sample(context.Background(), runtime.Sample{Sequence: 1, Inputs: []byte(`{}`)}))
		// A tick that never comes: only the flush can run the boundary.
		manager.Run(sessionUUID, sessionController, time.Hour)

		sessionController.RequestFlush()

		view, ok := nextView(t, player)
		require.True(t, ok)
		require.Equal(t, runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{}`)}, ConsumedThroughSampleSequence: 1}, view)
	})
	t.Run("close_stops_every_lane", func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			manager.Close()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Close did not stop the lane")
		}
	})
}
