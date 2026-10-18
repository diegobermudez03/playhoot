package ownership

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestLocateOrClaimOwner(t *testing.T) {
	type test struct {
		expectedPlacement runtime.OwnerPlacement
		expectedErr       bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	ctx := context.Background()

	tests := map[string]func(t *testing.T, sessions *MockliveSessionChecker) test{
		"live_session_is_owned_here": func(t *testing.T, sessions *MockliveSessionChecker) test {
			sessions.EXPECT().IsSessionLive(ctx, sessionUUID).Return(true, nil)
			return test{expectedPlacement: runtime.OwnerPlacement{Outcome: runtime.OwnerHere, Epoch: singleInstanceEpoch}}
		},
		"session_that_is_not_live_is_unavailable_and_never_claimed": func(t *testing.T, sessions *MockliveSessionChecker) test {
			sessions.EXPECT().IsSessionLive(ctx, sessionUUID).Return(false, nil)
			return test{expectedPlacement: runtime.OwnerPlacement{Outcome: runtime.OwnerSessionUnavailable}}
		},
		"checking_the_session_fails": func(t *testing.T, sessions *MockliveSessionChecker) test {
			sessions.EXPECT().IsSessionLive(ctx, sessionUUID).Return(false, errors.New("database down"))
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
