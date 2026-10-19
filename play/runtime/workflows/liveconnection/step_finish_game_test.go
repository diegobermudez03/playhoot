package liveconnection

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestFinishingBoundary(t *testing.T) {
	type dependencies struct {
		sandbox *MockgameSandbox
		ends    *MocksessionEnder
		replay  *MockreplayRecorder
	}
	type test struct {
		// boundaries is how many ticks run after the finishing one.
		boundaries int

		expectedView *runtime.OwnView
		expectedEnd  runtime.ConnectionEnd
	}

	anyCtx := gomock.Any()
	pass := &sandbox.Command{ParticipantID: "P1", Name: "pass", Payload: []byte(`{}`)}
	batch := sandbox.Batch{Operations: []sandbox.Operation{{SessionTimeMs: 20, Command: pass}, {SessionTimeMs: 30, Command: pass}}}
	finished := sandbox.ExecutionResult{
		Outcome: sandbox.ScriptSucceeded, State: []byte(`{"x":9}`), PRNGState: []byte{2, 0, 0, 0}, ContextRevision: 2,
		CommandResponses: [][]byte{[]byte(`{"won":true}`)}, Events: []sandbox.GameEvent{{Operation: 0, Event: []byte(`"GAME_OVER"`)}},
		Finished: true, FinishedOperation: 0,
	}
	finalCapture := replay.Capture{
		SessionUUID: boundarySessionUUID, GameVersionUUID: boundaryGameVersion, SessionTimeMs: 20, ContextRevision: 2,
		State: []byte(`{"x":9}`), Participants: []string{"P1"},
		CommandResponses: []replay.CommandResponse{{ParticipantID: "P1", SessionTimeMs: 20, Order: 1, Response: []byte(`{"won":true}`)}},
		Events:           []replay.GameEvent{{SessionTimeMs: 20, Order: 1, Event: []byte(`"GAME_OVER"`)}},
	}
	gameFinished := sessionpkg.SessionEnd{Cause: sessionpkg.EndGameFinished}
	finishedNow := sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "FINISHED", EndCause: "GAME_FINISHED"}
	finalView := &runtime.OwnView{
		PlayerView: runtime.PlayerView{ParticipantID: "P1", Revision: 2, State: []byte(`{"x":9}`)},
		Events:     [][]byte{[]byte(`"GAME_OVER"`)},
	}
	executes := func(deps dependencies) []any {
		return []any{
			deps.sandbox.EXPECT().Execute(anyCtx, "context-1", batch).Return(finished, nil),
			deps.sandbox.EXPECT().Project(anyCtx, "context-1", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"x":9}`)}, nil),
			deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
		}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"final_state_reaches_players_once_the_ending_is_durable": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(append(executes(deps),
				deps.replay.EXPECT().Finish(anyCtx, finalCapture).Return(nil),
				deps.ends.EXPECT().EndSession(anyCtx, boundarySessionUUID, gameFinished).Return(finishedNow, nil),
			)...)
			return test{expectedView: finalView, expectedEnd: sessionEnded("FINISHED", "GAME_FINISHED")}
		},
		"replay_failing_to_store_the_final_state_is_retried_next_tick": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(append(executes(deps),
				deps.replay.EXPECT().Finish(anyCtx, finalCapture).Return(errors.New("bucket unavailable")),
				deps.replay.EXPECT().Finish(anyCtx, finalCapture).Return(nil),
				deps.ends.EXPECT().EndSession(anyCtx, boundarySessionUUID, gameFinished).Return(finishedNow, nil),
			)...)
			return test{boundaries: 1, expectedView: finalView, expectedEnd: sessionEnded("FINISHED", "GAME_FINISHED")}
		},
		"durable_failure_is_retried_without_recording_the_final_state_twice": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(append(executes(deps),
				deps.replay.EXPECT().Finish(anyCtx, finalCapture).Return(nil),
				deps.ends.EXPECT().EndSession(anyCtx, boundarySessionUUID, gameFinished).Return(sessionpkg.EndSessionResult{}, errors.New("database down")),
				deps.ends.EXPECT().EndSession(anyCtx, boundarySessionUUID, gameFinished).Return(finishedNow, nil),
			)...)
			return test{boundaries: 1, expectedView: finalView, expectedEnd: sessionEnded("FINISHED", "GAME_FINISHED")}
		},
		"session_that_ended_otherwise_meanwhile_ends_as_it_did": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(append(executes(deps),
				deps.replay.EXPECT().Finish(anyCtx, finalCapture).Return(nil),
				deps.ends.EXPECT().EndSession(anyCtx, boundarySessionUUID, gameFinished).
					Return(sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionNotEnded, Status: "EXPIRED", EndCause: "MAX_LIFETIME"}, nil),
			)...)
			return test{expectedEnd: sessionEnded("EXPIRED", "MAX_LIFETIME")}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{sandbox: NewMockgameSandbox(ctrl), ends: NewMocksessionEnder(ctrl), replay: NewMockreplayRecorder(ctrl)}
			tc := setup(t, deps)
			controllerDeps := testDependencies()
			controllerDeps.sandbox, controllerDeps.ends, controllerDeps.replay = deps.sandbox, deps.ends, deps.replay
			controller, player, now := runningGame(t, controllerDeps)
			*now = now.Add(20 * time.Millisecond)
			winning := sendCommand(player, runtime.Command{Token: "t1", Name: "pass"})
			waitForTimeline(t, controller, 1)
			*now = now.Add(10 * time.Millisecond)
			late := sendCommand(player, runtime.Command{Token: "t2", Name: "pass"})
			waitForTimeline(t, controller, 2)

			controller.runBoundary()
			if tc.boundaries > 0 {
				_, ok := nextView(t, player)
				require.False(t, ok, "players see nothing of the final state before its ending is durable")
				require.Equal(t, phaseFinishing, controller.currentPhase())
			}
			for range tc.boundaries {
				controller.runBoundary()
			}

			require.Equal(t, runtime.CommandResult{Token: "t1", Outcome: runtime.CommandExecuted, Response: []byte(`{"won":true}`)}, receive(t, winning))
			require.Equal(t, runtime.CommandResult{Token: "t2", Outcome: runtime.CommandNotRunning}, receive(t, late), "what the batch did not reach never ran")
			view, ok := nextView(t, player)
			if tc.expectedView == nil {
				require.False(t, ok)
			} else {
				require.True(t, ok)
				require.Equal(t, *tc.expectedView, view)
			}
			require.Equal(t, tc.expectedEnd, connectionEnd(t, player.Ended))
			requireClosed(t, controller.ended)
			require.Empty(t, controller.contextID, "a finished game keeps no context")
			require.Nil(t, controller.finishing)
		})
	}
}

func TestFinishedAt(t *testing.T) {
	type test struct {
		index int

		expected    finishPoint
		expectedErr bool
	}

	operations := []Operation{{Order: 1}, {Order: 2}, {Order: 3}, {Order: 4}}
	work := boundary{operations: operations}
	// Two transitions share a call, a command runs alone, update() ends it.
	batch := sandbox.Batch{Operations: []sandbox.Operation{
		{SessionTimeMs: 20}, {SessionTimeMs: 30}, {SessionTimeMs: 40}, {SessionTimeMs: 100, Update: true},
	}}
	orders := []uint64{2, 3, 4, 0}

	tests := map[string]test{
		"call_running_several_transitions":  {index: 0, expected: finishPoint{operations: 2, sessionTimeMs: 20}},
		"command_in_the_middle":             {index: 1, expected: finishPoint{operations: 3, sessionTimeMs: 30}},
		"update_after_every_operation":      {index: 3, expected: finishPoint{operations: 4, sessionTimeMs: 100}},
		"operation_the_batch_does_not_have": {index: 4, expectedErr: true},
		"negative_operation":                {index: -1, expectedErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			point, err := finishedAt(work, batch, orders, tc.index)

			if tc.expectedErr {
				require.ErrorIs(t, err, errOutOfSync)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, point)
		})
	}
}
