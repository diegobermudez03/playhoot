package liveconnection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
)

func TestEndIfAbandoned(t *testing.T) {
	type test struct {
		// gone runs against the running game of user-a and user-b.
		gone func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock)

		expectedEnded bool
	}

	ctx := context.Background()
	// told takes the pending timeline, as a committed boundary does.
	told := func(controller *sessionController) { controller.takeBoundary() }

	tests := map[string]func(t *testing.T, cancels *MocksessionCanceller) test{
		"every_participant_gone_ends_the_session": func(t *testing.T, cancels *MocksessionCanceller) test {
			cancels.EXPECT().CancelAbandonedSession(ctx, "session-1").Return(nil)
			return test{
				gone: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					_, err := players["user-a"].Leave(ctx)
					require.NoError(t, err)
					players["user-b"].Disconnect()
					clock.Advance(time.Hour)
					told(controller)
				},
				expectedEnded: true,
			}
		},
		"disconnection_the_game_was_not_told_yet_waits": func(t *testing.T, _ *MocksessionCanceller) test {
			return test{
				gone: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					for _, player := range players {
						_, err := player.Leave(ctx)
						require.NoError(t, err)
					}
				},
			}
		},
		"rejoin_waiting_for_the_game_keeps_it_running": func(t *testing.T, _ *MocksessionCanceller) test {
			return test{
				gone: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					for _, player := range players {
						_, err := player.Leave(ctx)
						require.NoError(t, err)
					}
					told(controller)
					controller.lock().enqueue(Operation{ParticipantID: "P1", Rejoin: &pendingRejoin{done: make(chan struct{})}})
					controller.unlock()
				},
			}
		},
		"someone_still_playing_keeps_it_running": func(t *testing.T, _ *MocksessionCanceller) test {
			return test{
				gone: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					_, err := players["user-a"].Leave(ctx)
					require.NoError(t, err)
					players["user-b"].Disconnect()
					told(controller)
				},
			}
		},
		"durable_failure_keeps_it_running_for_the_next_boundary": func(t *testing.T, cancels *MocksessionCanceller) test {
			cancels.EXPECT().CancelAbandonedSession(ctx, "session-1").Return(errors.New("database down"))
			return test{
				gone: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					for _, player := range players {
						_, err := player.Leave(ctx)
						require.NoError(t, err)
					}
					told(controller)
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			cancels := NewMocksessionCanceller(gomock.NewController(t))
			tc := setup(t, cancels)
			controller, players, clock := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, "user-a", "user-b")
			controller.deps.cancels = cancels
			tc.gone(t, controller, players, clock)

			controller.endIfAbandoned(ctx)

			select {
			case <-controller.ended:
				require.True(t, tc.expectedEnded)
			default:
				require.False(t, tc.expectedEnded)
			}
		})
	}
}

func TestLastPlayerLeavingEndsTheSessionAfterTellingTheGame(t *testing.T) {
	ctrl := gomock.NewController(t)
	sandboxMock, replayMock, cancels := NewMockgameSandbox(ctrl), NewMockreplayRecorder(ctrl), NewMocksessionCanceller(ctrl)
	deps := testDependencies()
	deps.sandbox, deps.replay, deps.cancels = sandboxMock, replayMock, cancels
	controller, player, now := runningGame(t, deps)
	anyCtx := gomock.Any()
	gomock.InOrder(
		sandboxMock.EXPECT().Execute(anyCtx, "context-1", sandbox.Batch{Operations: []sandbox.Operation{{SessionTimeMs: 40, Disconnected: "P1"}}}).
			Return(sandbox.ExecutionResult{Outcome: sandbox.ScriptSucceeded, State: []byte(`{"x":0}`), PRNGState: []byte{1, 0, 0, 0}, ContextRevision: 2}, nil),
		sandboxMock.EXPECT().Project(anyCtx, "context-1", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"x":0}`)}, nil),
		replayMock.EXPECT().Offer(anyCtx, replay.Capture{
			SessionUUID: boundarySessionUUID, GameVersionUUID: boundaryGameVersion, SessionTimeMs: 40, ContextRevision: 2,
			State: []byte(`{"x":0}`), Participants: []string{"P1"},
		}),
		cancels.EXPECT().CancelAbandonedSession(anyCtx, boundarySessionUUID).Return(nil),
	)
	*now = now.Add(40 * time.Millisecond)
	_, err := player.Leave(context.Background())
	require.NoError(t, err)
	*now = now.Add(60 * time.Millisecond)

	controller.runBoundary()

	require.Equal(t, runtime.EndLeft, endedWith(t, player.Ended))
	requireClosed(t, controller.ended)
}
