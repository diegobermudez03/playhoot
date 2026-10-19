package liveconnection

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestCancelSession(t *testing.T) {
	type test struct {
		// running starts the game before cancelling; otherwise it is a lobby.
		running bool
		// phase, when set, is the phase the controller is in when cancelling.
		phase phase

		expectedResult runtime.CancelSessionResult
		expectedErr    bool
		// expectedEnd is how the player's connection ended; zero when not.
		expectedEnd runtime.ConnectionEnd
	}

	ctx := context.Background()
	cancel := sessionpkg.SessionEnd{Cause: sessionpkg.EndHostCancelled, ChangedBy: "creator"}
	cancelled := sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "CANCELLED", EndCause: "HOST_CANCELLED"}

	tests := map[string]func(t *testing.T, ends *MocksessionEnder) test{
		"lobby_is_cancelled": func(t *testing.T, ends *MocksessionEnder) test {
			ends.EXPECT().EndSession(ctx, "session-1", cancel).Return(cancelled, nil)
			return test{
				expectedResult: runtime.CancelSessionResult{Outcome: runtime.SessionCancelled},
				expectedEnd:    sessionEnded("CANCELLED", "HOST_CANCELLED"),
			}
		},
		"running_game_is_cancelled": func(t *testing.T, ends *MocksessionEnder) test {
			ends.EXPECT().EndSession(ctx, "session-1", cancel).Return(cancelled, nil)
			return test{
				running:        true,
				expectedResult: runtime.CancelSessionResult{Outcome: runtime.SessionCancelled},
				expectedEnd:    sessionEnded("CANCELLED", "HOST_CANCELLED"),
			}
		},
		"session_that_already_ended_durably": func(t *testing.T, ends *MocksessionEnder) test {
			ends.EXPECT().EndSession(ctx, "session-1", cancel).
				Return(sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionNotEnded, Status: "EXPIRED", EndCause: "LOBBY_EXPIRED"}, nil)
			return test{
				expectedResult: runtime.CancelSessionResult{Outcome: runtime.SessionAlreadyEnded},
				expectedEnd:    sessionEnded("EXPIRED", "LOBBY_EXPIRED"),
			}
		},
		"game_its_script_finished_is_already_over": func(t *testing.T, _ *MocksessionEnder) test {
			return test{running: true, phase: phaseFinishing, expectedResult: runtime.CancelSessionResult{Outcome: runtime.SessionAlreadyEnded}}
		},
		"durable_failure_keeps_it_live": func(t *testing.T, ends *MocksessionEnder) test {
			ends.EXPECT().EndSession(ctx, "session-1", cancel).Return(sessionpkg.EndSessionResult{}, errors.New("database down"))
			return test{expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ends := NewMocksessionEnder(gomock.NewController(t))
			tc := setup(t, ends)
			var controller *sessionController
			var player runtime.PlayerConnection
			if tc.running {
				var players map[string]runtime.PlayerConnection
				controller, players, _ = runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")
				player = players["user-a"]
			} else {
				var players map[string]runtime.PlayerConnection
				controller, players = newStartController(t, 4, "user-a")
				player = players["user-a"]
			}
			controller.deps.ends = ends
			if tc.phase != "" {
				controller.lock().phase = tc.phase
				controller.unlock()
			}

			result, err := controller.cancelSession(ctx, "creator")

			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedResult, result)
			}
			require.Equal(t, tc.expectedEnd, connectionEnd(t, player.Ended))
		})
	}
}

func TestRequestCancelOfAnEndedController(t *testing.T) {
	controller := newTestController(t, 4)
	controller.retire()
	// Without its main goroutine, the operation can never begin.
	close(controller.exited)

	result, err := controller.requestCancel(context.Background(), "creator")

	require.NoError(t, err)
	require.Equal(t, runtime.CancelSessionResult{Outcome: runtime.SessionAlreadyEnded}, result)
}
