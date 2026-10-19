package liveconnection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/checkpoint"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestOfferCheckpoint(t *testing.T) {
	type test struct {
		// play runs against the running game of user-a (P1) and user-b (P2).
		play func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock)

		expected *checkpoint.Checkpoint
	}

	ctx := context.Background()
	committed := Game{GameVersionUUID: "game-version", ContextRevision: 4, ExecutedThroughSessionTimeMs: 900, State: []byte(`{"round":2}`), PRNGState: []byte{3, 0, 0, 0}}
	commit := func(controller *sessionController) {
		s := controller.lock()
		s.game = committed
		controller.unlock()
	}
	base := checkpoint.Checkpoint{
		GameVersionUUID: "game-version", ContextRevision: 4, ExecutedThroughSessionTimeMs: 900,
		State: []byte(`{"round":2}`), PRNGState: []byte{3, 0, 0, 0}, Participants: []string{"P1", "P2"},
		ProcessedCommandTokens: map[string][]string{"P1": {"a", "b"}},
	}

	tests := map[string]func(t *testing.T) test{
		"interval_not_elapsed_offers_nothing": func(t *testing.T) test {
			return test{play: func(t *testing.T, controller *sessionController, _ map[string]runtime.PlayerConnection, clock *testClock) {
				clock.Advance(4 * time.Second)
				commit(controller)
			}}
		},
		"committed_boundary_after_the_interval_is_offered": func(t *testing.T) test {
			return test{
				play: func(t *testing.T, controller *sessionController, _ map[string]runtime.PlayerConnection, clock *testClock) {
					clock.Advance(5 * time.Second)
					commit(controller)
				},
				expected: &base,
			}
		},
		"participant_the_game_was_told_left_is_disconnected": func(t *testing.T) test {
			told := base
			told.Disconnected = []string{"P2"}
			return test{
				play: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					_, err := players["user-b"].Leave(ctx)
					require.NoError(t, err)
					controller.takeBoundary()
					clock.Advance(5 * time.Second)
					commit(controller)
				},
				expected: &told,
			}
		},
		"disconnection_the_game_was_not_told_yet_is_still_playing": func(t *testing.T) test {
			return test{
				play: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					_, err := players["user-b"].Leave(ctx)
					require.NoError(t, err)
					clock.Advance(5 * time.Second)
					commit(controller)
				},
				expected: &base,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, clock := runningControllerWithTokens(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, map[string][]string{"P1": {"b", "a"}}, "user-a", "user-b")
			controller.deps.checkpointInterval = 5 * time.Second
			tc.play(t, controller, players, clock)

			controller.offerCheckpoint()

			offered, ok := controller.checkpointWriter.take()
			if tc.expected == nil {
				require.False(t, ok)
				return
			}
			require.True(t, ok)
			require.Equal(t, *tc.expected, offered)
		})
	}
}

func TestWriteCheckpoints(t *testing.T) {
	type test struct {
		expectedSaved bool
	}

	snapshot := checkpoint.Checkpoint{GameVersionUUID: "game-version", ContextRevision: 8, State: []byte(`{"round":5}`), Participants: []string{"P1"}}
	encoded, err := checkpoint.Encode(snapshot)
	require.NoError(t, err)
	stored := sessionpkg.Checkpoint{ContextRevision: 8, Content: encoded}
	anyCtx := gomock.Any()

	tests := map[string]func(t *testing.T, store *MockcheckpointStore) test{
		"offered_checkpoint_is_stored": func(t *testing.T, store *MockcheckpointStore) test {
			store.EXPECT().SaveCheckpoint(anyCtx, "session-1", stored).Return(sessionpkg.CheckpointSaved, nil)
			return test{expectedSaved: true}
		},
		"session_that_ended_meanwhile_stores_nothing": func(t *testing.T, store *MockcheckpointStore) test {
			store.EXPECT().SaveCheckpoint(anyCtx, "session-1", stored).Return(sessionpkg.CheckpointSessionNotRunning, nil)
			return test{}
		},
		"failure_waits_for_the_next_checkpoint": func(t *testing.T, store *MockcheckpointStore) test {
			store.EXPECT().SaveCheckpoint(anyCtx, "session-1", stored).Return(sessionpkg.SaveCheckpointOutcome(""), errors.New("database down"))
			return test{}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store := NewMockcheckpointStore(ctrl)
			setup(t, store)
			controller := newTestController(t, 4)
			controller.deps.checkpoints = store
			stop, stopped := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(stopped)
				controller.writeCheckpoints(stop)
			}()

			controller.checkpointWriter.offer(snapshot)

			require.Eventually(t, ctrl.Satisfied, time.Second, time.Millisecond, "written on the writer's own goroutine")
			close(stop)
			requireClosed(t, stopped)
		})
	}
}

func TestNewerCheckpointReplacesOneNotWrittenYet(t *testing.T) {
	writer := newCheckpointWriter()
	writer.offer(checkpoint.Checkpoint{ContextRevision: 3})
	writer.offer(checkpoint.Checkpoint{ContextRevision: 4})

	latest, ok := writer.take()

	require.True(t, ok)
	require.Equal(t, int64(4), latest.ContextRevision)
	_, ok = writer.take()
	require.False(t, ok, "nothing queues up behind it")
}

func TestCommittedBoundaryOffersACheckpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	sandboxMock, replayMock := NewMockgameSandbox(ctrl), NewMockreplayRecorder(ctrl)
	deps := testDependencies()
	deps.sandbox, deps.replay = sandboxMock, replayMock
	controller, player, now := runningGame(t, deps)
	controller.deps.checkpointInterval = 0
	anyCtx := gomock.Any()
	gomock.InOrder(
		sandboxMock.EXPECT().Execute(anyCtx, "context-1", gomock.Any()).
			Return(sandbox.ExecutionResult{Outcome: sandbox.ScriptSucceeded, State: []byte(`{"x":1}`), PRNGState: []byte{2, 0, 0, 0}, ContextRevision: 2}, nil),
		sandboxMock.EXPECT().Project(anyCtx, "context-1", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"x":1}`)}, nil),
		replayMock.EXPECT().Offer(anyCtx, gomock.Any()),
	)
	*now = now.Add(40 * time.Millisecond)
	require.NoError(t, player.Sample(context.Background(), runtime.Sample{Sequence: 1, Inputs: []byte(`{"move":[1,0]}`)}))
	*now = now.Add(60 * time.Millisecond)

	controller.runBoundary()

	offered, ok := controller.checkpointWriter.take()
	require.True(t, ok)
	require.Equal(t, checkpoint.Checkpoint{
		GameVersionUUID: boundaryGameVersion, ContextRevision: 2, ExecutedThroughSessionTimeMs: 100,
		State: []byte(`{"x":1}`), PRNGState: []byte{2, 0, 0, 0}, Participants: []string{"P1"},
		ProcessedCommandTokens: map[string][]string{},
	}, offered)
}
