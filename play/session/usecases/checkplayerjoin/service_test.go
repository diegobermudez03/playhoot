package checkplayerjoin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/checkplayerjoin/internal/repo"
)

func TestEligibleToJoinAsPlayer(t *testing.T) {
	type dependencies struct {
		sessions *MocksessionFinder
	}
	type test struct {
		sessionUUID string

		expectedEligibility sessionpkg.PlayerJoinEligibility
		expectedErr         bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	session := func(status string, lobbyExpiresAt time.Time) *internalrepo.Session {
		return &internalrepo.Session{Status: status, LobbyExpiresAt: lobbyExpiresAt}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"open_lobby": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("LOBBY", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, expectedEligibility: sessionpkg.PlayerJoinEligible}
		},
		"lobby_whose_expiry_passed": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("LOBBY", now), nil)
			return test{sessionUUID: sessionUUID, expectedEligibility: sessionpkg.PlayerJoinSessionNotJoinable}
		},
		"running_session": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("RUNNING", now.Add(-time.Hour)), nil)
			return test{sessionUUID: sessionUUID, expectedEligibility: sessionpkg.PlayerJoinSessionStarted}
		},
		"ended_session": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("CANCELLED", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, expectedEligibility: sessionpkg.PlayerJoinSessionNotJoinable}
		},
		"session_with_an_unknown_status_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("PAUSED", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
		"session_not_found": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(nil, nil)
			return test{sessionUUID: sessionUUID, expectedEligibility: sessionpkg.PlayerJoinSessionNotFound}
		},
		"session_uuid_is_not_a_uuid": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "not-a-uuid", expectedEligibility: sessionpkg.PlayerJoinSessionNotFound}
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

			eligibility, err := service.EligibleToJoinAsPlayer(ctx, tc.sessionUUID)

			if tc.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedEligibility, eligibility)
		})
	}
}
