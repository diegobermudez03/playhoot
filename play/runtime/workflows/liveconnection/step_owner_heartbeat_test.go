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

func TestBeatOwnerHeartbeat(t *testing.T) {
	type test struct {
		expectedRecorded bool
		// expectedEnd is how the player's connection ends; nil when it stays.
		expectedEnd *runtime.ConnectionEnd
	}

	anyCtx := gomock.Any()
	tests := map[string]func(t *testing.T, heartbeats *MockownerHeartbeater) test{
		"running_game_records_its_owner": func(t *testing.T, heartbeats *MockownerHeartbeater) test {
			heartbeats.EXPECT().RecordOwnerHeartbeat(anyCtx, boundarySessionUUID).Return(sessionpkg.OwnerHeartbeatResult{Outcome: sessionpkg.HeartbeatRecorded}, nil)
			return test{expectedRecorded: true}
		},
		"game_ended_elsewhere_ends_here_as_the_database_says": func(t *testing.T, heartbeats *MockownerHeartbeater) test {
			heartbeats.EXPECT().RecordOwnerHeartbeat(anyCtx, boundarySessionUUID).
				Return(sessionpkg.OwnerHeartbeatResult{Outcome: sessionpkg.HeartbeatSessionNotRunning, Status: "ABORTED", EndCause: "OWNER_LOST"}, nil)
			return test{
				expectedRecorded: true,
				expectedEnd:      &runtime.ConnectionEnd{Reason: runtime.EndSessionEnded, SessionStatus: "ABORTED", EndCause: "OWNER_LOST"},
			}
		},
		"failure_is_tried_again": func(t *testing.T, heartbeats *MockownerHeartbeater) test {
			heartbeats.EXPECT().RecordOwnerHeartbeat(anyCtx, boundarySessionUUID).Return(sessionpkg.OwnerHeartbeatResult{}, errors.New("database down"))
			return test{}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			heartbeats := NewMockownerHeartbeater(gomock.NewController(t))
			tc := setup(t, heartbeats)
			deps := testDependencies()
			deps.heartbeats = heartbeats
			controller, player, _ := runningGame(t, deps)

			recorded := controller.beatOwnerHeartbeat()

			require.Equal(t, tc.expectedRecorded, recorded)
			if tc.expectedEnd == nil {
				require.Equal(t, phaseRunning, controller.currentPhase())
				return
			}
			require.Equal(t, *tc.expectedEnd, connectionEnd(t, player.Ended))
			requireClosed(t, controller.ended)
		})
	}
}

func TestBeatOwnerHeartbeats(t *testing.T) {
	type test struct {
		// running starts the game before the heartbeats run.
		running bool
	}

	tests := map[string]func(t *testing.T, heartbeats *MockownerHeartbeater) test{
		"running_game_beats_at_once_and_not_again_before_the_interval": func(t *testing.T, heartbeats *MockownerHeartbeater) test {
			heartbeats.EXPECT().RecordOwnerHeartbeat(gomock.Any(), boundarySessionUUID).Return(sessionpkg.OwnerHeartbeatResult{Outcome: sessionpkg.HeartbeatRecorded}, nil)
			return test{running: true}
		},
		"lobby_never_beats": func(t *testing.T, heartbeats *MockownerHeartbeater) test {
			return test{}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			heartbeats := NewMockownerHeartbeater(gomock.NewController(t))
			tc := setup(t, heartbeats)
			deps := testDependencies()
			deps.heartbeats, deps.limitsInterval, deps.heartbeatInterval = heartbeats, 5*time.Millisecond, time.Hour
			controller := withManifest(newSessionController(context.Background(), boundarySessionUUID, 1, deps), Manifest{MaxPlayers: 4})
			if tc.running {
				controller, _, _ = runningGame(t, deps)
			}
			stop, stopped := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(stopped)
				controller.beatOwnerHeartbeats(stop)
			}()

			// Many checks, but only the first is due within the interval.
			time.Sleep(100 * time.Millisecond)
			close(stop)
			<-stopped
		})
	}
}
