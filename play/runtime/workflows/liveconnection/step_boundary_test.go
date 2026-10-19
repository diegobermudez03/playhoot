package liveconnection

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/internal/business/commands"
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
// context-1, whose clock the test moves, and that player's connection. The
// game takes the input `move` and the command `pass`.
func runningGame(t *testing.T, deps *controllerDependencies) (*sessionController, runtime.PlayerConnection, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	schema, err := inputs.NewSchema([]inputs.Declaration{{Name: "move", Type: inputs.Vector2}})
	require.NoError(t, err)
	catalog, err := commands.NewCatalog([]commands.Declaration{{Name: "pass"}})
	require.NoError(t, err)
	deps.now = func() time.Time { return now }
	controller := withManifest(newSessionController(context.Background(), boundarySessionUUID, 1, deps), Manifest{MaxPlayers: 4, Inputs: schema, Commands: catalog})
	player, _, admitted := controller.joinLobby(context.Background(), "user-a")
	require.True(t, admitted)
	participants, _, ok := controller.beginStart()
	require.True(t, ok)
	controller.contextID = "context-1"
	controller.completeStart(participants, Game{
		GameVersionUUID: boundaryGameVersion, Script: boundaryScript,
		ContextRevision: 1, State: []byte(`{"x":0}`), PRNGState: []byte{1, 0, 0, 0},
	}, runtime.GameFrontend{}, map[string][]byte{"P1": []byte(`{"x":0}`)}, nil)
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
	input := sandbox.Batch{Operations: []sandbox.Operation{{SessionTimeMs: 100, Transitions: []sandbox.Transition{{ParticipantID: "P1", Inputs: []byte(`{"move":[1,0]}`), SessionTimeMs: 40}}}}}
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
			deps.sandbox.EXPECT().Execute(anyCtx, contextID, input).Return(executed, nil),
			projects(deps, contextID),
			deps.replay.EXPECT().Offer(anyCtx, capture),
		)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"accepted_input_is_executed_projected_and_offered_to_replay": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(executed, nil),
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
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(failed, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}, retriedIn(deps, "context-2")...)...)
			return test{expectedView: movedView, expectedContextID: "context-2"}
		},
		"script_failing_twice_aborts_the_session": func(t *testing.T, deps dependencies) test {
			calls := []any{
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(failed, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}
			calls = append(calls, restores(deps, "context-2")...)
			calls = append(calls,
				deps.sandbox.EXPECT().Execute(anyCtx, "context-2", input).Return(failed, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-2").Return(nil),
				deps.aborts.EXPECT().AbortRunningSession(anyCtx, boundarySessionUUID, sessionpkg.AbortScriptFailure).Return(nil),
			)
			gomock.InOrder(calls...)
			return test{expectedEnd: runtime.EndSessionAborted}
		},
		"projection_failing_in_the_script_is_a_script_failure": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(append([]any{
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(executed, nil),
				deps.sandbox.EXPECT().Project(anyCtx, "context-1", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptFailed, ScriptError: "hidden"}, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}, retriedIn(deps, "context-2")...)...)
			return test{expectedView: movedView, expectedContextID: "context-2"}
		},
		"infrastructure_failure_is_retried_in_a_restored_context": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(append([]any{
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(sandbox.ExecutionResult{}, errors.New("worker died")),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}, retriedIn(deps, "context-2")...)...)
			return test{expectedView: movedView, expectedContextID: "context-2"}
		},
		"execution_out_of_sync_with_the_mirror_is_retried": func(t *testing.T, deps dependencies) test {
			skipped := executed
			skipped.ContextRevision = 5
			gomock.InOrder(append([]any{
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(skipped, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}, retriedIn(deps, "context-2")...)...)
			return test{expectedView: movedView, expectedContextID: "context-2"}
		},
		"sandbox_unavailable_puts_the_input_back_for_the_next_boundary": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(sandbox.ExecutionResult{}, errors.New("worker died")),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
				deps.sandbox.EXPECT().CreateContext(anyCtx, spec).Return("", sandbox.ErrNoCapacity),
			)
			return test{expectedRequeued: true, expectedInfraFailures: 1}
		},
		"logical_errors_reach_their_player_with_the_view_and_replay": func(t *testing.T, deps dependencies) test {
			rejected := executed
			rejected.Errors = []sandbox.GameError{{Operation: 0, ParticipantID: "P1", Error: []byte(`{"code":"MOVE_NOT_ALLOWED"}`)}}
			recorded := capture
			recorded.Errors = []replay.GameError{{ParticipantID: "P1", SessionTimeMs: 100, Order: 1, Error: []byte(`{"code":"MOVE_NOT_ALLOWED"}`)}}
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(rejected, nil),
				projects(deps, "context-1"),
				deps.replay.EXPECT().Offer(anyCtx, recorded),
			)
			view := *movedView
			view.Errors = [][]byte{[]byte(`{"code":"MOVE_NOT_ALLOWED"}`)}
			return test{expectedView: &view, expectedContextID: "context-1"}
		},
		"logical_events_reach_their_players_with_the_view_and_replay": func(t *testing.T, deps dependencies) test {
			emitted := executed
			emitted.Events = []sandbox.GameEvent{
				{Operation: 0, Event: []byte(`{"type":"EXPLOSION"}`)},
				{Operation: 0, Participants: []string{"P1"}, Event: []byte(`"CARD_REVEALED"`)},
			}
			recorded := capture
			recorded.Events = []replay.GameEvent{
				{SessionTimeMs: 100, Order: 1, Event: []byte(`{"type":"EXPLOSION"}`)},
				{Participants: []string{"P1"}, SessionTimeMs: 100, Order: 1, Event: []byte(`"CARD_REVEALED"`)},
			}
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(emitted, nil),
				projects(deps, "context-1"),
				deps.replay.EXPECT().Offer(anyCtx, recorded),
			)
			view := *movedView
			view.Events = [][]byte{[]byte(`{"type":"EXPLOSION"}`), []byte(`"CARD_REVEALED"`)}
			return test{expectedView: &view, expectedContextID: "context-1"}
		},
		"event_for_someone_who_does_not_play_is_a_script_failure": func(t *testing.T, deps dependencies) test {
			stray := executed
			stray.Events = []sandbox.GameEvent{{Operation: 0, Participants: []string{"P1", "P9"}, Event: []byte(`"BOOM"`)}}
			gomock.InOrder(append([]any{
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(stray, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}, retriedIn(deps, "context-2")...)...)
			return test{expectedView: movedView, expectedContextID: "context-2"}
		},
		"error_to_someone_who_does_not_play_is_a_script_failure": func(t *testing.T, deps dependencies) test {
			stray := executed
			stray.Errors = []sandbox.GameError{{Operation: 0, ParticipantID: "P9", Error: []byte(`"LATE"`)}}
			gomock.InOrder(append([]any{
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(stray, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			}, retriedIn(deps, "context-2")...)...)
			return test{expectedView: movedView, expectedContextID: "context-2"}
		},
		"too_many_failed_boundaries_abort_the_session": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", input).Return(sandbox.ExecutionResult{}, errors.New("worker died")),
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
			require.Equal(t, tc.expectedRequeued, len(pending.operations) == 1)
		})
	}
}

func TestRunBoundaryWithCommands(t *testing.T) {
	type dependencies struct {
		sandbox *MockgameSandbox
		aborts  *MocksessionAborter
		replay  *MockreplayRecorder
	}
	type test struct {
		// sampleAfter sends a second SAMPLE after the command.
		sampleAfter bool

		expectedResult runtime.CommandResult
		expectedView   *runtime.OwnView
		expectedEnd    runtime.EndReason
		// expectedExecutedThrough is the mirror's time after the boundary.
		expectedExecutedThrough int64
	}

	anyCtx := gomock.Any()
	spec := sandbox.ContextSpec{GameVersionUUID: boundaryGameVersion, Script: boundaryScript}
	snapshot := sandbox.Snapshot{State: []byte(`{"x":0}`), PRNGState: []byte{1, 0, 0, 0}, ContextRevision: 1, SessionTimeMs: 0}
	pass := &sandbox.Command{ParticipantID: "P1", Name: "pass", Payload: []byte(`{}`)}
	firstMove := sandbox.Transition{ParticipantID: "P1", Inputs: []byte(`{"move":[1,0]}`), SessionTimeMs: 40}
	split := sandbox.Batch{Operations: []sandbox.Operation{
		{SessionTimeMs: 40, Transitions: []sandbox.Transition{firstMove}},
		{SessionTimeMs: 50, Command: pass},
		{SessionTimeMs: 100, Transitions: []sandbox.Transition{{ParticipantID: "P1", Inputs: []byte(`{"move":[0,0]}`), SessionTimeMs: 70}}},
	}}
	endingWithCommand := sandbox.Batch{Operations: []sandbox.Operation{
		{SessionTimeMs: 40, Transitions: []sandbox.Transition{firstMove}},
		{SessionTimeMs: 50, Command: pass},
	}}
	executed := sandbox.ExecutionResult{Outcome: sandbox.ScriptSucceeded, State: []byte(`{"x":1}`), PRNGState: []byte{2, 0, 0, 0}, ContextRevision: 2, CommandResponses: [][]byte{[]byte(`{"ok":true}`)}}
	answered := runtime.CommandResult{Token: "t1", Outcome: runtime.CommandExecuted, Response: []byte(`{"ok":true}`)}
	projects := func(deps dependencies, contextID string) *gomock.Call {
		return deps.sandbox.EXPECT().Project(anyCtx, contextID, "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"x":1}`)}, nil)
	}
	capture := func(sessionTimeMs int64) replay.Capture {
		return replay.Capture{
			SessionUUID: boundarySessionUUID, GameVersionUUID: boundaryGameVersion, SessionTimeMs: sessionTimeMs, ContextRevision: 2,
			State: []byte(`{"x":1}`), Participants: []string{"P1"},
			CommandResponses: []replay.CommandResponse{{ParticipantID: "P1", SessionTimeMs: 50, Order: 2, Response: []byte(`{"ok":true}`)}},
		}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"command_splits_the_transitions_around_it_in_one_batch": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", split).Return(executed, nil),
				projects(deps, "context-1"),
				deps.replay.EXPECT().Offer(anyCtx, capture(100)),
			)
			return test{
				sampleAfter:             true,
				expectedResult:          answered,
				expectedView:            &runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{"x":1}`)}, ConsumedThroughSampleSequence: 2},
				expectedExecutedThrough: 100,
			}
		},
		"boundary_ending_with_a_command_ends_at_its_time": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", endingWithCommand).Return(executed, nil),
				projects(deps, "context-1"),
				deps.replay.EXPECT().Offer(anyCtx, capture(50)),
			)
			return test{
				expectedResult:          answered,
				expectedView:            &runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{"x":1}`)}, ConsumedThroughSampleSequence: 1},
				expectedExecutedThrough: 50,
			}
		},
		"execution_missing_a_command_response_is_retried": func(t *testing.T, deps dependencies) test {
			unanswered := executed
			unanswered.CommandResponses = nil
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", endingWithCommand).Return(unanswered, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
				deps.sandbox.EXPECT().CreateContext(anyCtx, spec).Return("context-2", nil),
				deps.sandbox.EXPECT().Restore(anyCtx, "context-2", snapshot).Return(sandbox.RestoreResult{Outcome: sandbox.ScriptSucceeded}, nil),
				deps.sandbox.EXPECT().Execute(anyCtx, "context-2", endingWithCommand).Return(executed, nil),
				projects(deps, "context-2"),
				deps.replay.EXPECT().Offer(anyCtx, capture(50)),
			)
			return test{
				expectedResult:          answered,
				expectedView:            &runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{"x":1}`)}, ConsumedThroughSampleSequence: 1},
				expectedExecutedThrough: 50,
			}
		},
		"command_failing_twice_in_the_script_aborts_the_session": func(t *testing.T, deps dependencies) test {
			failed := sandbox.ExecutionResult{Outcome: sandbox.ScriptFailed, ScriptError: "command() threw"}
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", endingWithCommand).Return(failed, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
				deps.sandbox.EXPECT().CreateContext(anyCtx, spec).Return("context-2", nil),
				deps.sandbox.EXPECT().Restore(anyCtx, "context-2", snapshot).Return(sandbox.RestoreResult{Outcome: sandbox.ScriptSucceeded}, nil),
				deps.sandbox.EXPECT().Execute(anyCtx, "context-2", endingWithCommand).Return(failed, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-2").Return(nil),
				deps.aborts.EXPECT().AbortRunningSession(anyCtx, boundarySessionUUID, sessionpkg.AbortScriptFailure).Return(nil),
			)
			return test{
				expectedResult: runtime.CommandResult{Token: "t1", Outcome: runtime.CommandNotRunning},
				expectedEnd:    runtime.EndSessionAborted,
			}
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
			ctx := context.Background()
			*now = now.Add(40 * time.Millisecond)
			require.NoError(t, player.Sample(ctx, runtime.Sample{Sequence: 1, Inputs: []byte(`{"move":[1,0]}`)}))
			*now = now.Add(10 * time.Millisecond)
			answers := sendCommand(player, runtime.Command{Token: "t1", Name: "pass"})
			waitForTimeline(t, controller, 2)
			*now = now.Add(20 * time.Millisecond)
			if tc.sampleAfter {
				require.NoError(t, player.Sample(ctx, runtime.Sample{Sequence: 2, Inputs: []byte(`{"move":[0,0]}`)}))
			}
			*now = now.Add(30 * time.Millisecond)

			controller.runBoundary()

			require.Equal(t, tc.expectedResult, receive(t, answers))
			if tc.expectedEnd != "" {
				require.Equal(t, tc.expectedEnd, endedWith(t, player.Ended))
				return
			}
			view, ok := nextView(t, player)
			require.True(t, ok)
			require.Equal(t, *tc.expectedView, view)
			committed, _ := controller.takeBoundary()
			require.Equal(t, tc.expectedExecutedThrough, committed.game.ExecutedThroughSessionTimeMs)
			retried, err := player.Command(ctx, runtime.Command{Token: "t1", Name: "pass"})
			require.NoError(t, err)
			require.Equal(t, runtime.CommandAlreadyProcessed, retried.Outcome, "the token is processed once committed")
		})
	}
}

func TestRunBoundaryWithUpdate(t *testing.T) {
	type dependencies struct {
		sandbox *MockgameSandbox
		aborts  *MocksessionAborter
		replay  *MockreplayRecorder
	}
	type test struct {
		// sample sends a SAMPLE at 40 ms before the boundary at 100 ms.
		sample bool

		expectedView          *runtime.OwnView
		expectedEnd           runtime.EndReason
		expectedInfraFailures int
		// expectedExecutedThrough is the mirror's time after the boundary.
		expectedExecutedThrough int64
	}

	anyCtx := gomock.Any()
	spec := sandbox.ContextSpec{GameVersionUUID: boundaryGameVersion, Script: boundaryScript}
	snapshot := sandbox.Snapshot{State: []byte(`{"x":0}`), PRNGState: []byte{1, 0, 0, 0}, ContextRevision: 1, SessionTimeMs: 0}
	updateOnly := sandbox.Batch{Operations: []sandbox.Operation{{SessionTimeMs: 100, Update: true}}}
	afterSample := sandbox.Batch{Operations: []sandbox.Operation{
		{SessionTimeMs: 40, Transitions: []sandbox.Transition{{ParticipantID: "P1", Inputs: []byte(`{"move":[1,0]}`), SessionTimeMs: 40}}},
		{SessionTimeMs: 100, Update: true},
	}}
	executed := sandbox.ExecutionResult{Outcome: sandbox.ScriptSucceeded, State: []byte(`{"x":1}`), PRNGState: []byte{2, 0, 0, 0}, ContextRevision: 2}
	failed := sandbox.ExecutionResult{Outcome: sandbox.ScriptFailed, ScriptError: "update() threw"}
	projects := func(deps dependencies) *gomock.Call {
		return deps.sandbox.EXPECT().Project(anyCtx, "context-1", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"x":1}`)}, nil)
	}
	capture := replay.Capture{SessionUUID: boundarySessionUUID, GameVersionUUID: boundaryGameVersion, SessionTimeMs: 100, ContextRevision: 2, State: []byte(`{"x":1}`), Participants: []string{"P1"}}
	moved := func(acked int64) *runtime.OwnView {
		return &runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{"x":1}`)}, ConsumedThroughSampleSequence: acked}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"update_runs_although_nothing_was_sent": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", updateOnly).Return(executed, nil),
				projects(deps),
				deps.replay.EXPECT().Offer(anyCtx, capture),
			)
			return test{expectedView: moved(0), expectedExecutedThrough: 100}
		},
		"update_follows_the_transitions_at_their_own_time": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", afterSample).Return(executed, nil),
				projects(deps),
				deps.replay.EXPECT().Offer(anyCtx, capture),
			)
			return test{sample: true, expectedView: moved(1), expectedExecutedThrough: 100}
		},
		"failed_update_is_not_queued_for_later": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", updateOnly).Return(sandbox.ExecutionResult{}, errors.New("worker died")),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
				deps.sandbox.EXPECT().CreateContext(anyCtx, spec).Return("", sandbox.ErrNoCapacity),
			)
			return test{expectedInfraFailures: 1}
		},
		"update_failing_twice_in_the_script_aborts_the_session": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.sandbox.EXPECT().Execute(anyCtx, "context-1", updateOnly).Return(failed, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
				deps.sandbox.EXPECT().CreateContext(anyCtx, spec).Return("context-2", nil),
				deps.sandbox.EXPECT().Restore(anyCtx, "context-2", snapshot).Return(sandbox.RestoreResult{Outcome: sandbox.ScriptSucceeded}, nil),
				deps.sandbox.EXPECT().Execute(anyCtx, "context-2", updateOnly).Return(failed, nil),
				deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-2").Return(nil),
				deps.aborts.EXPECT().AbortRunningSession(anyCtx, boundarySessionUUID, sessionpkg.AbortScriptFailure).Return(nil),
			)
			return test{expectedEnd: runtime.EndSessionAborted}
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
			controller.lock().manifest.Update = true
			controller.unlock()
			*now = now.Add(40 * time.Millisecond)
			if tc.sample {
				require.NoError(t, player.Sample(context.Background(), runtime.Sample{Sequence: 1, Inputs: []byte(`{"move":[1,0]}`)}))
			}
			*now = now.Add(60 * time.Millisecond)

			controller.runBoundary()

			require.Equal(t, tc.expectedInfraFailures, controller.infraFailures)
			if tc.expectedEnd != "" {
				require.Equal(t, tc.expectedEnd, endedWith(t, player.Ended))
				return
			}
			view, gotView := nextView(t, player)
			if tc.expectedView == nil {
				require.False(t, gotView)
			} else {
				require.True(t, gotView)
				require.Equal(t, *tc.expectedView, view)
			}
			next, _ := controller.takeBoundary()
			require.Empty(t, next.operations, "update is never queued")
			require.True(t, next.update)
			require.Equal(t, tc.expectedExecutedThrough, next.game.ExecutedThroughSessionTimeMs)
		})
	}
}

func TestBatchOf(t *testing.T) {
	type test struct {
		work boundary

		expected       sandbox.Batch
		expectedOrders []uint64
	}

	// transition is ordered by its time, which the tests never repeat.
	transition := func(at int64) Operation {
		return Operation{ParticipantID: "P1", Inputs: `{}`, SessionTimeMs: at, Order: uint64(at)}
	}
	sandboxTransition := func(at int64) sandbox.Transition {
		return sandbox.Transition{ParticipantID: "P1", Inputs: []byte(`{}`), SessionTimeMs: at}
	}
	pass := &pendingCommand{token: "t1", name: "pass", payload: []byte(`{}`)}
	sandboxPass := &sandbox.Command{ParticipantID: "P1", Name: "pass", Payload: []byte(`{}`)}

	tests := map[string]func(t *testing.T) test{
		"transitions_ending_a_batch_run_until_the_boundary": func(t *testing.T) test {
			return test{
				work:           boundary{operations: []Operation{transition(40), transition(70)}, sessionTimeMs: 100},
				expected:       sandbox.Batch{Operations: []sandbox.Operation{{SessionTimeMs: 100, Transitions: []sandbox.Transition{sandboxTransition(40), sandboxTransition(70)}}}},
				expectedOrders: []uint64{70},
			}
		},
		"transitions_before_update_run_until_their_last_one": func(t *testing.T) test {
			return test{
				work: boundary{operations: []Operation{transition(40), transition(70)}, sessionTimeMs: 100, update: true},
				expected: sandbox.Batch{Operations: []sandbox.Operation{
					{SessionTimeMs: 70, Transitions: []sandbox.Transition{sandboxTransition(40), sandboxTransition(70)}},
					{SessionTimeMs: 100, Update: true},
				}},
				expectedOrders: []uint64{70, 0},
			}
		},
		"command_splits_the_transitions_each_at_its_time": func(t *testing.T) test {
			command := Operation{ParticipantID: "P1", SessionTimeMs: 50, Order: 50, Command: pass}
			return test{
				work: boundary{operations: []Operation{transition(40), command, transition(70)}, sessionTimeMs: 100, update: true},
				expected: sandbox.Batch{Operations: []sandbox.Operation{
					{SessionTimeMs: 40, Transitions: []sandbox.Transition{sandboxTransition(40)}},
					{SessionTimeMs: 50, Command: sandboxPass},
					{SessionTimeMs: 70, Transitions: []sandbox.Transition{sandboxTransition(70)}},
					{SessionTimeMs: 100, Update: true},
				}},
				expectedOrders: []uint64{40, 50, 70, 0},
			}
		},
		"update_alone": func(t *testing.T) test {
			return test{
				work:           boundary{sessionTimeMs: 100, update: true},
				expected:       sandbox.Batch{Operations: []sandbox.Operation{{SessionTimeMs: 100, Update: true}}},
				expectedOrders: []uint64{0},
			}
		},
		"nothing_runs_before_the_confirmed_state": func(t *testing.T) test {
			return test{
				work: boundary{operations: []Operation{transition(40)}, sessionTimeMs: 100, update: true, game: Game{ExecutedThroughSessionTimeMs: 60}},
				expected: sandbox.Batch{Operations: []sandbox.Operation{
					{SessionTimeMs: 60, Transitions: []sandbox.Transition{sandboxTransition(40)}},
					{SessionTimeMs: 100, Update: true},
				}},
				expectedOrders: []uint64{40, 0},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			batch, orders := batchOf(tc.work)

			require.Equal(t, tc.expected, batch)
			require.Equal(t, tc.expectedOrders, orders)
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
	require.Empty(t, next.operations, "a boundary takes what was pending")
}

func TestRequeueKeepsTheTimelineOrder(t *testing.T) {
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")
	ctx := context.Background()
	require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
	failed, _ := controller.takeBoundary()
	require.NoError(t, players["user-a"].Sample(ctx, sample(2, `{"move":[0,1]}`)))

	controller.requeue(failed)

	retried, _ := controller.takeBoundary()
	require.Equal(t, []uint64{1, 2}, []uint64{retried.operations[0].Order, retried.operations[1].Order})
}

func TestCommitBoundary(t *testing.T) {
	type test struct {
		// sequence is the SAMPLE P1 sends before the boundary; 0 sends none.
		sequence int64
		view     string
		revision int64
		errors   []gameError

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
		"errors_are_sent_although_nothing_else_changed": func(t *testing.T) test {
			return test{
				view: `{"round":1,"me":{"x":0,"y":0}}`, revision: 2,
				errors: []gameError{
					{participantID: "P1", sessionTimeMs: 10, order: 1, value: []byte(`"NOT_YOUR_TURN"`)},
					{participantID: "P1", sessionTimeMs: 20, value: []byte(`{"code":"OUT_OF_FUEL"}`)},
				},
				expectedViews: []runtime.OwnView{{
					PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{}`)},
					Errors:     [][]byte{[]byte(`"NOT_YOUR_TURN"`), []byte(`{"code":"OUT_OF_FUEL"}`)},
				}},
			}
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

			err := controller.commitBoundary(ctx, boundary, execution{game: Game{ContextRevision: tc.revision, State: []byte(`{"n":1}`)}, views: map[string][]byte{"P1": []byte(tc.view)}, errors: tc.errors})

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

func TestCommitBoundaryTransientsReachOnlyTheirParticipants(t *testing.T) {
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`), "P3": []byte(`{}`)}, "user-a", "user-b", "user-c")
	ctx := context.Background()
	boundary, _ := controller.takeBoundary()

	err := controller.commitBoundary(ctx, boundary, execution{
		game:   Game{ContextRevision: 2, State: []byte(`{}`)},
		views:  map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`), "P3": []byte(`{}`)},
		errors: []gameError{{participantID: "P2", sessionTimeMs: 10, order: 1, value: []byte(`"NOT_YOUR_TURN"`)}},
		events: []gameEvent{
			{sessionTimeMs: 10, order: 1, value: []byte(`"ROUND_CHANGED"`)},
			{participantIDs: []string{"P1", "P2"}, sessionTimeMs: 20, value: []byte(`"CARD_REVEALED"`)},
		},
	})

	require.NoError(t, err)
	require.Equal(t, []runtime.OwnView{{
		PlayerView: runtime.PlayerView{ParticipantID: "P1", State: []byte(`{}`)},
		Events:     [][]byte{[]byte(`"ROUND_CHANGED"`), []byte(`"CARD_REVEALED"`)},
	}}, drainViews(t, players["user-a"]))
	require.Equal(t, []runtime.OwnView{{
		PlayerView: runtime.PlayerView{ParticipantID: "P2", State: []byte(`{}`)},
		Errors:     [][]byte{[]byte(`"NOT_YOUR_TURN"`)},
		Events:     [][]byte{[]byte(`"ROUND_CHANGED"`), []byte(`"CARD_REVEALED"`)},
	}}, drainViews(t, players["user-b"]))
	require.Equal(t, []runtime.OwnView{{
		PlayerView: runtime.PlayerView{ParticipantID: "P3", State: []byte(`{}`)},
		Events:     [][]byte{[]byte(`"ROUND_CHANGED"`)},
	}}, drainViews(t, players["user-c"]))
}

func TestAcknowledgeOnly(t *testing.T) {
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{"round":1}`)}, "user-a")
	ctx := context.Background()
	require.NoError(t, players["user-a"].Sample(ctx, sample(9, `{}`)))
	boundary, _ := controller.takeBoundary()
	require.Empty(t, boundary.operations, "a refresh of the defaults executes nothing")

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
