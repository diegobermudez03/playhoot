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
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// connectionEnd returns how ended reports its connection ended, or the zero
// value when it did not end within a short wait.
func connectionEnd(t *testing.T, ended func(context.Context) (runtime.ConnectionEnd, bool)) runtime.ConnectionEnd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	end, _ := ended(ctx)
	return end
}

// sessionEnded is how a connection ends when its Session reached status for
// cause.
func sessionEnded(status string, cause string) runtime.ConnectionEnd {
	return runtime.ConnectionEnd{Reason: runtime.EndSessionEnded, SessionStatus: status, EndCause: cause}
}

func TestEndSession(t *testing.T) {
	type test struct {
		expectedErr bool
		// expectedEnd is how every connection ended; zero when they did not.
		expectedEnd runtime.ConnectionEnd
		// expectedOnEnded is whether onEnded ran.
		expectedOnEnded bool
	}

	ctx := context.Background()
	end := sessionpkg.SessionEnd{Cause: sessionpkg.EndInactivityTimeout}

	tests := map[string]func(t *testing.T, ends *MocksessionEnder) test{
		"session_ended_now_ends_every_connection": func(t *testing.T, ends *MocksessionEnder) test {
			ends.EXPECT().EndSession(ctx, "session-1", end).
				Return(sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "EXPIRED", EndCause: "INACTIVITY_TIMEOUT"}, nil)
			return test{expectedEnd: sessionEnded("EXPIRED", "INACTIVITY_TIMEOUT"), expectedOnEnded: true}
		},
		"session_that_had_ended_otherwise_ends_as_the_database_says": func(t *testing.T, ends *MocksessionEnder) test {
			ends.EXPECT().EndSession(ctx, "session-1", end).
				Return(sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionNotEnded, Status: "CANCELLED", EndCause: "HOST_CANCELLED"}, nil)
			return test{expectedEnd: sessionEnded("CANCELLED", "HOST_CANCELLED")}
		},
		"session_the_cause_does_not_apply_to_stays_live": func(t *testing.T, ends *MocksessionEnder) test {
			ends.EXPECT().EndSession(ctx, "session-1", end).Return(sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionNotEnded, Status: "LOBBY"}, nil)
			return test{}
		},
		"durable_failure_leaves_it_live": func(t *testing.T, ends *MocksessionEnder) test {
			ends.EXPECT().EndSession(ctx, "session-1", end).Return(sessionpkg.EndSessionResult{}, errors.New("database down"))
			return test{expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ends := NewMocksessionEnder(gomock.NewController(t))
			tc := setup(t, ends)
			controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")
			controller.deps.ends = ends
			admin := controller.mustRegisterAdmin("creator")
			onEnded := false

			_, err := controller.endSession(ctx, end, func(*liveState) { onEnded = true })

			require.Equal(t, tc.expectedErr, err != nil)
			require.Equal(t, tc.expectedOnEnded, onEnded)
			require.Equal(t, tc.expectedEnd, connectionEnd(t, players["user-a"].Ended))
			require.Equal(t, tc.expectedEnd, connectionEnd(t, admin.Ended))
			if tc.expectedEnd != (runtime.ConnectionEnd{}) {
				requireClosed(t, controller.ended)
			}
		})
	}
}

func TestEndIfAbandoned(t *testing.T) {
	type test struct {
		// gone runs against the running game of user-a and user-b.
		gone func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock)

		expectedEnded bool
	}

	ctx := context.Background()
	// told takes the pending timeline, as a committed boundary does.
	told := func(controller *sessionController) { controller.takeBoundary() }
	allLeft := sessionpkg.SessionEnd{Cause: sessionpkg.EndAllPlayersLeft}

	tests := map[string]func(t *testing.T, ends *MocksessionEnder) test{
		"every_participant_gone_ends_the_session": func(t *testing.T, ends *MocksessionEnder) test {
			ends.EXPECT().EndSession(ctx, "session-1", allLeft).
				Return(sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "CANCELLED", EndCause: "ALL_PLAYERS_LEFT"}, nil)
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
		"disconnection_the_game_was_not_told_yet_waits": func(t *testing.T, _ *MocksessionEnder) test {
			return test{
				gone: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					for _, player := range players {
						_, err := player.Leave(ctx)
						require.NoError(t, err)
					}
				},
			}
		},
		"rejoin_waiting_for_the_game_keeps_it_running": func(t *testing.T, _ *MocksessionEnder) test {
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
		"someone_still_playing_keeps_it_running": func(t *testing.T, _ *MocksessionEnder) test {
			return test{
				gone: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) {
					_, err := players["user-a"].Leave(ctx)
					require.NoError(t, err)
					players["user-b"].Disconnect()
					told(controller)
				},
			}
		},
		"durable_failure_keeps_it_running_for_the_next_boundary": func(t *testing.T, ends *MocksessionEnder) test {
			ends.EXPECT().EndSession(ctx, "session-1", allLeft).Return(sessionpkg.EndSessionResult{}, errors.New("database down"))
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
			ends := NewMocksessionEnder(gomock.NewController(t))
			tc := setup(t, ends)
			controller, players, clock := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, "user-a", "user-b")
			controller.deps.ends = ends
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
	sandboxMock, replayMock, ends := NewMockgameSandbox(ctrl), NewMockreplayRecorder(ctrl), NewMocksessionEnder(ctrl)
	deps := testDependencies()
	deps.sandbox, deps.replay, deps.ends = sandboxMock, replayMock, ends
	controller, player, now := runningGame(t, deps)
	anyCtx := gomock.Any()
	gomock.InOrder(
		sandboxMock.EXPECT().Execute(anyCtx, "context-1", sandbox.Batch{Operations: []sandbox.Operation{{SessionTimeMs: 40, Disconnected: "P1"}}}).
			Return(sandbox.ExecutionResult{Outcome: sandbox.ScriptSucceeded, State: []byte(`{"x":0}`), PRNGState: []byte{1, 0, 0, 0}, ContextRevision: 2}, nil),
		sandboxMock.EXPECT().Project(anyCtx, "context-1", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"x":0}`)}, nil),
		replayMock.EXPECT().Offer(anyCtx, testRecordingID, replay.Capture{
			SessionUUID: boundarySessionUUID, GameVersionUUID: boundaryGameVersion, SessionTimeMs: 40, ContextRevision: 2,
			State: []byte(`{"x":0}`), Participants: []string{"P1"},
		}),
		ends.EXPECT().EndSession(anyCtx, boundarySessionUUID, sessionpkg.SessionEnd{Cause: sessionpkg.EndAllPlayersLeft}).
			Return(sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "CANCELLED", EndCause: "ALL_PLAYERS_LEFT"}, nil),
	)
	*now = now.Add(40 * time.Millisecond)
	_, err := player.Leave(context.Background())
	require.NoError(t, err)
	*now = now.Add(60 * time.Millisecond)

	controller.runBoundary()

	require.Equal(t, runtime.EndLeft, endedWith(t, player.Ended))
	requireClosed(t, controller.ended)
}
