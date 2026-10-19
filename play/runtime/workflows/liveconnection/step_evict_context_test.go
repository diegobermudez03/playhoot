package liveconnection

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
)

func TestEvictIdleContext(t *testing.T) {
	type dependencies struct {
		sandbox *MockgameSandbox
	}
	type test struct {
		// unusedFor is how long ago the context was last used.
		unusedFor time.Duration
		update    bool
		noContext bool

		expectedContextID string
		expectedEvicted   bool
	}

	anyCtx := gomock.Any()
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"idle_game_without_update_is_evicted": func(t *testing.T, deps dependencies) test {
			deps.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil)
			return test{unusedFor: 30 * time.Second, expectedEvicted: true}
		},
		"recently_used_context_is_kept": func(t *testing.T, deps dependencies) test {
			return test{unusedFor: 29 * time.Second, expectedContextID: "context-1"}
		},
		"game_with_update_stays_hot": func(t *testing.T, deps dependencies) test {
			return test{unusedFor: time.Hour, update: true, expectedContextID: "context-1"}
		},
		"game_without_a_context_has_nothing_to_evict": func(t *testing.T, deps dependencies) test {
			return test{unusedFor: time.Hour, noContext: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			deps := dependencies{sandbox: NewMockgameSandbox(gomock.NewController(t))}
			tc := setup(t, deps)
			controllerDeps := testDependencies()
			controllerDeps.sandbox = deps.sandbox
			controllerDeps.contextIdleTimeout = 30 * time.Second
			controller, _, now := runningGame(t, controllerDeps)
			controller.lastSandboxUse = now.Add(-tc.unusedFor)
			controller.lock().manifest.Update = tc.update
			controller.unlock()
			if tc.noContext {
				controller.contextID = ""
			}

			controller.evictIdleContext()

			require.Equal(t, tc.expectedContextID, controller.contextID)
			require.Equal(t, tc.expectedEvicted, controller.evicted)
		})
	}
}

func TestEvictedContextIsRecreatedByTheNextExecution(t *testing.T) {
	ctrl := gomock.NewController(t)
	games, recorder := NewMockgameSandbox(ctrl), NewMockreplayRecorder(ctrl)
	deps := testDependencies()
	deps.sandbox, deps.replay, deps.contextIdleTimeout = games, recorder, 30*time.Second
	controller, player, now := runningGame(t, deps)
	controller.lastSandboxUse = *now
	anyCtx := gomock.Any()
	games.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil)
	*now = now.Add(30 * time.Second)
	controller.evictIdleContext()
	require.Empty(t, controller.contextID)

	gomock.InOrder(
		games.EXPECT().CreateContext(anyCtx, sandbox.ContextSpec{GameVersionUUID: boundaryGameVersion, Script: boundaryScript}).Return("context-2", nil),
		games.EXPECT().Restore(anyCtx, "context-2", sandbox.Snapshot{State: []byte(`{"x":0}`), PRNGState: []byte{1, 0, 0, 0}, ContextRevision: 1, SessionTimeMs: 0}).
			Return(sandbox.RestoreResult{Outcome: sandbox.ScriptSucceeded}, nil),
		games.EXPECT().Execute(anyCtx, "context-2", sandbox.Batch{Operations: []sandbox.Operation{{SessionTimeMs: 30000, Transitions: []sandbox.Transition{{ParticipantID: "P1", Inputs: []byte(`{"move":[1,0]}`), SessionTimeMs: 30000}}}}}).
			Return(sandbox.ExecutionResult{Outcome: sandbox.ScriptSucceeded, State: []byte(`{"x":1}`), PRNGState: []byte{2, 0, 0, 0}, ContextRevision: 2}, nil),
		games.EXPECT().Project(anyCtx, "context-2", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"x":1}`)}, nil),
		recorder.EXPECT().Offer(anyCtx, testRecordingID, replay.Capture{
			SessionUUID: boundarySessionUUID, GameVersionUUID: boundaryGameVersion, SessionTimeMs: 30000,
			ContextRevision: 2, State: []byte(`{"x":1}`), Participants: []string{"P1"},
		}),
	)
	require.NoError(t, player.Sample(context.Background(), runtime.Sample{Sequence: 1, Inputs: []byte(`{"move":[1,0]}`)}))

	controller.runBoundary()

	require.Equal(t, "context-2", controller.contextID)
	require.False(t, controller.evicted)
	require.Equal(t, *now, controller.lastSandboxUse)
	view, ok := nextView(t, player)
	require.True(t, ok)
	require.Equal(t, runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", Revision: 2, State: []byte(`{"x":1}`)}, ConsumedThroughSampleSequence: 1}, view)
}
