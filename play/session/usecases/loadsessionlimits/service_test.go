package loadsessionlimits

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/loadsessionlimits/internal/repo"
)

func TestLoadSessionLimits(t *testing.T) {
	type dependencies struct {
		limits *MocklimitsFinder
	}
	type test struct {
		sessionUUID string

		expected      sessionpkg.SessionLimits
		expectedErrIs error
		expectedErr   bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	ctx := context.Background()
	lobbyExpiresAt := time.Date(2026, 10, 18, 12, 30, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 10, 18, 16, 0, 0, 0, time.UTC)

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"session_deadlines": func(t *testing.T, deps dependencies) test {
			deps.limits.EXPECT().FindSessionLimits(ctx, sessionUUID).Return(&internalrepo.Limits{LobbyExpiresAt: lobbyExpiresAt, ExpiresAt: expiresAt}, nil)
			return test{sessionUUID: sessionUUID, expected: sessionpkg.SessionLimits{LobbyExpiresAt: lobbyExpiresAt, ExpiresAt: expiresAt}}
		},
		"session_not_found": func(t *testing.T, deps dependencies) test {
			deps.limits.EXPECT().FindSessionLimits(ctx, sessionUUID).Return(nil, nil)
			return test{sessionUUID: sessionUUID, expectedErrIs: sessionpkg.ErrSessionNotFound}
		},
		"session_uuid_is_not_a_uuid": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "not-a-uuid", expectedErrIs: sessionpkg.ErrSessionNotFound}
		},
		"reading_fails": func(t *testing.T, deps dependencies) test {
			deps.limits.EXPECT().FindSessionLimits(ctx, sessionUUID).Return(nil, errors.New("database down"))
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{limits: NewMocklimitsFinder(ctrl)}
			tc := setup(t, deps)

			limits, err := (&Service{limits: deps.limits}).LoadSessionLimits(ctx, tc.sessionUUID)

			switch {
			case tc.expectedErrIs != nil:
				require.ErrorIs(t, err, tc.expectedErrIs)
			case tc.expectedErr:
				require.Error(t, err)
			default:
				require.NoError(t, err)
				require.Equal(t, tc.expected, limits)
			}
		})
	}
}
