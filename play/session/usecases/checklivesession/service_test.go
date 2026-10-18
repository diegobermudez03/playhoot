package checklivesession

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/checklivesession/internal/repo"
)

func TestIsSessionLive(t *testing.T) {
	type dependencies struct {
		sessions *MocksessionFinder
	}
	type test struct {
		sessionUUID string

		expectedLive bool
		expectedErr  bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	session := func(status string, lobbyExpiresAt time.Time) *internalrepo.Session {
		return &internalrepo.Session{Status: status, LobbyExpiresAt: lobbyExpiresAt}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"lobby": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("LOBBY", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, expectedLive: true}
		},
		"running_session_whose_lobby_expiry_passed": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("RUNNING", now.Add(-time.Hour)), nil)
			return test{sessionUUID: sessionUUID, expectedLive: true}
		},
		"lobby_whose_expiry_passed": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("LOBBY", now), nil)
			return test{sessionUUID: sessionUUID}
		},
		"ended_session": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("ABORTED", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID}
		},
		"session_not_found": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(nil, nil)
			return test{sessionUUID: sessionUUID}
		},
		"session_uuid_is_not_a_uuid": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "not-a-uuid"}
		},
		"reading_the_session_fails": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(nil, errors.New("database down"))
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{sessions: NewMocksessionFinder(ctrl)}
			tc := setup(t, deps)
			service := &Service{sessions: deps.sessions, now: func() time.Time { return now }}

			live, err := service.IsSessionLive(ctx, tc.sessionUUID)

			if tc.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedLive, live)
		})
	}
}
