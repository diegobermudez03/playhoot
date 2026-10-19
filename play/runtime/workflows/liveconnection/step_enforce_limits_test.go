package liveconnection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestEnforceLimits(t *testing.T) {
	type dependencies struct {
		limits *MocklimitsLoader
		ends   *MocksessionEnder
	}
	type test struct {
		// running starts the game before the check; otherwise it is a lobby.
		running bool
		// elapsed is how much platform time passes before the check.
		elapsed time.Duration
		// play runs against the running game's player before the check.
		play func(t *testing.T, controller *sessionController, player runtime.PlayerConnection, clock *testClock)

		expectedEnd runtime.ConnectionEnd
	}

	anyCtx := gomock.Any()
	start := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	limits := sessionpkg.SessionLimits{LobbyExpiresAt: start.Add(30 * time.Minute), ExpiresAt: start.Add(4 * time.Hour)}
	loads := func(deps dependencies) *gomock.Call {
		return deps.limits.EXPECT().LoadSessionLimits(anyCtx, "session-1").Return(limits, nil)
	}
	endsFor := func(deps dependencies, cause sessionpkg.EndCause, status string) {
		deps.ends.EXPECT().EndSession(anyCtx, "session-1", sessionpkg.SessionEnd{Cause: cause}).
			Return(sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: status, EndCause: string(cause)}, nil)
	}
	moves := func(t *testing.T, _ *sessionController, player runtime.PlayerConnection, clock *testClock) {
		clock.Advance(9 * time.Minute)
		require.NoError(t, player.Sample(context.Background(), sample(1, `{"move":[1,0]}`)))
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"lobby_before_its_deadlines_stays": func(t *testing.T, deps dependencies) test {
			loads(deps)
			return test{elapsed: 29 * time.Minute}
		},
		"lobby_that_did_not_start_in_time_expires": func(t *testing.T, deps dependencies) test {
			loads(deps)
			endsFor(deps, sessionpkg.EndLobbyExpired, "EXPIRED")
			return test{elapsed: 30 * time.Minute, expectedEnd: sessionEnded("EXPIRED", "LOBBY_EXPIRED")}
		},
		"running_game_reaching_its_lifetime_expires_whatever_its_activity": func(t *testing.T, deps dependencies) test {
			loads(deps)
			endsFor(deps, sessionpkg.EndMaxLifetime, "EXPIRED")
			return test{
				running: true,
				play: func(t *testing.T, _ *sessionController, player runtime.PlayerConnection, clock *testClock) {
					clock.Advance(4*time.Hour - time.Second)
					require.NoError(t, player.Sample(context.Background(), sample(1, `{"move":[1,0]}`)))
				},
				elapsed:     time.Second,
				expectedEnd: sessionEnded("EXPIRED", "MAX_LIFETIME"),
			}
		},
		"running_game_nobody_plays_expires": func(t *testing.T, deps dependencies) test {
			loads(deps)
			endsFor(deps, sessionpkg.EndInactivityTimeout, "EXPIRED")
			return test{running: true, elapsed: 10 * time.Minute, expectedEnd: sessionEnded("EXPIRED", "INACTIVITY_TIMEOUT")}
		},
		"distinct_input_keeps_it_playing": func(t *testing.T, deps dependencies) test {
			loads(deps)
			return test{running: true, play: moves, elapsed: 9 * time.Minute}
		},
		"command_keeps_it_playing": func(t *testing.T, deps dependencies) test {
			loads(deps)
			return test{
				running: true,
				play: func(t *testing.T, controller *sessionController, player runtime.PlayerConnection, clock *testClock) {
					clock.Advance(9 * time.Minute)
					sendCommand(player, runtime.Command{Token: "t1", Name: "pass"})
					waitForTimeline(t, controller, 1)
				},
				elapsed: 9 * time.Minute,
			}
		},
		"refreshes_are_not_playing": func(t *testing.T, deps dependencies) test {
			loads(deps)
			endsFor(deps, sessionpkg.EndInactivityTimeout, "EXPIRED")
			return test{
				running: true,
				play: func(t *testing.T, controller *sessionController, player runtime.PlayerConnection, clock *testClock) {
					moves(t, controller, player, clock)
					clock.Advance(time.Minute)
					require.NoError(t, player.Sample(context.Background(), sample(2, `{"move":[1,0]}`)))
					require.NoError(t, player.Resync(context.Background()))
				},
				elapsed:     9 * time.Minute,
				expectedEnd: sessionEnded("EXPIRED", "INACTIVITY_TIMEOUT"),
			}
		},
		"limits_that_cannot_be_read_end_nothing": func(t *testing.T, deps dependencies) test {
			deps.limits.EXPECT().LoadSessionLimits(anyCtx, "session-1").Return(sessionpkg.SessionLimits{}, errors.New("database down"))
			return test{elapsed: time.Hour}
		},
		"durable_failure_ends_nothing": func(t *testing.T, deps dependencies) test {
			loads(deps)
			deps.ends.EXPECT().EndSession(anyCtx, "session-1", sessionpkg.SessionEnd{Cause: sessionpkg.EndLobbyExpired}).Return(sessionpkg.EndSessionResult{}, errors.New("database down"))
			return test{elapsed: time.Hour}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{limits: NewMocklimitsLoader(ctrl), ends: NewMocksessionEnder(ctrl)}
			tc := setup(t, deps)
			var controller *sessionController
			var player runtime.PlayerConnection
			clock := &testClock{now: start}
			if tc.running {
				var players map[string]runtime.PlayerConnection
				controller, players, clock = runningController(t, map[string][]byte{"P1": []byte(`{}`)}, "user-a")
				player = players["user-a"]
			} else {
				var players map[string]runtime.PlayerConnection
				controller, players = newStartController(t, 4, "user-a")
				controller.deps.now = clock.Now
				controller.lock().now = clock.Now
				controller.unlock()
				player = players["user-a"]
			}
			controller.deps.limits, controller.deps.ends = deps.limits, deps.ends
			if tc.play != nil {
				tc.play(t, controller, player, clock)
			}
			clock.Advance(tc.elapsed)

			controller.enforceLimits()

			require.Equal(t, tc.expectedEnd, connectionEnd(t, player.Ended))
		})
	}
}

func TestEnforceLimitsReadsTheLimitsOnce(t *testing.T) {
	limits := NewMocklimitsLoader(gomock.NewController(t))
	controller, _ := newStartController(t, 4, "user-a")
	controller.deps.limits = limits
	limits.EXPECT().LoadSessionLimits(gomock.Any(), "session-1").Return(sessionpkg.SessionLimits{LobbyExpiresAt: time.Now().Add(time.Hour), ExpiresAt: time.Now().Add(time.Hour)}, nil)

	controller.enforceLimits()
	controller.enforceLimits()

	require.Equal(t, phaseLobby, controller.currentPhase())
}
