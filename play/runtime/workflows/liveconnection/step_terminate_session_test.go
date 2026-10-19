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

func TestTerminateSession(t *testing.T) {
	type dependencies struct {
		adminJoins *MockadminJoinChecker
		manifests  *MockmanifestLoader
		ends       *MocksessionEnder
	}
	type test struct {
		// live attaches an admin connection first, so this instance runs it.
		live bool

		expected    runtime.TerminateSessionResult
		expectedErr bool
		// expectedEnd is how the admin connection ended; zero when not.
		expectedEnd runtime.ConnectionEnd
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	anyCtx := gomock.Any()
	terminate := sessionpkg.SessionEnd{Cause: sessionpkg.EndPlatformTerminated}
	terminated := sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "CANCELLED", EndCause: "PLATFORM_TERMINATED"}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"session_this_instance_runs_ends_live_too": func(t *testing.T, deps dependencies) test {
			deps.ends.EXPECT().EndSession(anyCtx, sessionUUID, terminate).Return(terminated, nil)
			return test{
				live:        true,
				expected:    runtime.TerminateSessionResult{Outcome: runtime.SessionTerminated},
				expectedEnd: sessionEnded("CANCELLED", "PLATFORM_TERMINATED"),
			}
		},
		"session_without_a_controller_here_ends_durably": func(t *testing.T, deps dependencies) test {
			deps.ends.EXPECT().EndSession(anyCtx, sessionUUID, terminate).Return(terminated, nil)
			return test{expected: runtime.TerminateSessionResult{Outcome: runtime.SessionTerminated}}
		},
		"session_that_already_ended": func(t *testing.T, deps dependencies) test {
			deps.ends.EXPECT().EndSession(anyCtx, sessionUUID, terminate).
				Return(sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionNotEnded, Status: "FINISHED", EndCause: "GAME_FINISHED"}, nil)
			return test{expected: runtime.TerminateSessionResult{Outcome: runtime.TerminateAlreadyEnded}}
		},
		"durable_failure": func(t *testing.T, deps dependencies) test {
			deps.ends.EXPECT().EndSession(anyCtx, sessionUUID, terminate).Return(sessionpkg.EndSessionResult{}, errors.New("database down"))
			return test{live: true, expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{adminJoins: NewMockadminJoinChecker(ctrl), manifests: NewMockmanifestLoader(ctrl), ends: NewMocksessionEnder(ctrl)}
			tc := setup(t, deps)
			resolver := New(deps.adminJoins, nil, deps.manifests, nil, nil, deps.ends, nil, nil, nil, nil, nil, ownersReleasing(t, sessionUUID, 1))
			t.Cleanup(resolver.Close)
			var admin runtime.AdminConnection
			if tc.live {
				deps.adminJoins.EXPECT().EligibleToJoinAsAdmin(anyCtx, sessionUUID, "creator").Return(sessionpkg.AdminJoinEligible, nil)
				deps.manifests.EXPECT().LoadRuntimeManifest(anyCtx, sessionUUID).Return(sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 4}, nil)
				attached, err := resolver.AttachAdmin(context.Background(), sessionUUID, "creator", 1)
				require.NoError(t, err)
				admin = attached.Connection
			}

			result, err := resolver.TerminateSession(context.Background(), sessionUUID)

			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, result)
			}
			if !tc.live {
				return
			}
			require.Equal(t, tc.expectedEnd, connectionEnd(t, admin.Ended))
			if tc.expectedEnd != (runtime.ConnectionEnd{}) {
				require.Eventually(t, func() bool {
					resolver.mu.Lock()
					defer resolver.mu.Unlock()
					_, found := resolver.controllers[sessionUUID]
					return !found
				}, time.Second, time.Millisecond, "an ended controller is forgotten")
			}
		})
	}
}
