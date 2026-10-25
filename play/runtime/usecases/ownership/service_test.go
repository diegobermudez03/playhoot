package ownership

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestLocateOrClaimOwner(t *testing.T) {
	type test struct {
		expectedPlacement runtime.OwnerPlacement
		expectedErr       bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	ctx := context.Background()
	checks := func(sessions *MockliveSessionChecker, check sessionpkg.LiveSessionCheck) {
		sessions.EXPECT().CheckLiveSession(ctx, sessionUUID).Return(check, nil)
	}

	tests := map[string]func(t *testing.T, sessions *MockliveSessionChecker) test{
		"live_session_is_owned_here": func(t *testing.T, sessions *MockliveSessionChecker) test {
			checks(sessions, sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionLive})
			return test{expectedPlacement: runtime.OwnerPlacement{Outcome: runtime.OwnerHere, Epoch: singleInstanceEpoch}}
		},
		"unknown_session_is_not_found_and_never_claimed": func(t *testing.T, sessions *MockliveSessionChecker) test {
			checks(sessions, sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionNotFound})
			return test{expectedPlacement: runtime.OwnerPlacement{Outcome: runtime.OwnerSessionNotFound}}
		},
		"ended_session_tells_how_and_is_never_claimed": func(t *testing.T, sessions *MockliveSessionChecker) test {
			checks(sessions, sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionEnded, Status: "CANCELLED", EndCause: "HOST_CANCELLED"})
			return test{expectedPlacement: runtime.OwnerPlacement{
				Outcome: runtime.OwnerSessionEnded,
				Ended:   runtime.EndedSession{Status: "CANCELLED", EndCause: "HOST_CANCELLED"},
			}}
		},
		"overdue_lobby_is_unavailable_and_never_claimed": func(t *testing.T, sessions *MockliveSessionChecker) test {
			checks(sessions, sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionLobbyExpired})
			return test{expectedPlacement: runtime.OwnerPlacement{Outcome: runtime.OwnerSessionUnavailable}}
		},
		"unknown_check_outcome": func(t *testing.T, sessions *MockliveSessionChecker) test {
			checks(sessions, sessionpkg.LiveSessionCheck{Outcome: "something_new"})
			return test{expectedErr: true}
		},
		"checking_the_session_fails": func(t *testing.T, sessions *MockliveSessionChecker) test {
			sessions.EXPECT().CheckLiveSession(ctx, sessionUUID).Return(sessionpkg.LiveSessionCheck{}, errors.New("database down"))
			return test{expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			sessions := NewMockliveSessionChecker(gomock.NewController(t))
			tc := setup(t, sessions)

			placement, err := New(sessions).LocateOrClaimOwner(ctx, sessionUUID)

			if tc.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedPlacement, placement)
		})
	}
}
