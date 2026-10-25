package checklivesession

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/checklivesession/internal/repo"
)

func TestCheckLiveSession(t *testing.T) {
	type dependencies struct {
		sessions *MocksessionFinder
	}
	type test struct {
		sessionUUID string

		expectedCheck sessionpkg.LiveSessionCheck
		expectedErr   bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	session := func(status string, lobbyExpiresAt time.Time) *internalrepo.Session {
		return &internalrepo.Session{Status: status, LobbyExpiresAt: lobbyExpiresAt}
	}
	ended := func(status string, endCause string) *internalrepo.Session {
		return &internalrepo.Session{Status: status, LobbyExpiresAt: now.Add(-time.Hour), EndCause: &endCause}
	}
	live := sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionLive}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"lobby": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("LOBBY", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, expectedCheck: live}
		},
		"running_session_whose_lobby_expiry_passed": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("RUNNING", now.Add(-time.Hour)), nil)
			return test{sessionUUID: sessionUUID, expectedCheck: live}
		},
		"lobby_whose_expiry_passed": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("LOBBY", now), nil)
			return test{sessionUUID: sessionUUID, expectedCheck: sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionLobbyExpired}}
		},
		"finished_session_tells_how_it_ended": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(ended("FINISHED", "GAME_FINISHED"), nil)
			return test{sessionUUID: sessionUUID, expectedCheck: sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionEnded, Status: "FINISHED", EndCause: "GAME_FINISHED"}}
		},
		"cancelled_session_tells_how_it_ended": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(ended("CANCELLED", "HOST_CANCELLED"), nil)
			return test{sessionUUID: sessionUUID, expectedCheck: sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionEnded, Status: "CANCELLED", EndCause: "HOST_CANCELLED"}}
		},
		"expired_session_tells_how_it_ended": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(ended("EXPIRED", "LOBBY_EXPIRED"), nil)
			return test{sessionUUID: sessionUUID, expectedCheck: sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionEnded, Status: "EXPIRED", EndCause: "LOBBY_EXPIRED"}}
		},
		"aborted_session_tells_how_it_ended": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(ended("ABORTED", "SCRIPT_FAILURE"), nil)
			return test{sessionUUID: sessionUUID, expectedCheck: sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionEnded, Status: "ABORTED", EndCause: "SCRIPT_FAILURE"}}
		},
		"ended_session_without_end_cause_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("CANCELLED", now), nil)
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
		"unknown_status_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("PAUSED", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
		"session_not_found": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(nil, nil)
			return test{sessionUUID: sessionUUID, expectedCheck: sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionNotFound}}
		},
		"session_uuid_is_not_a_uuid": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "not-a-uuid", expectedCheck: sessionpkg.LiveSessionCheck{Outcome: sessionpkg.LiveSessionNotFound}}
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

			check, err := service.CheckLiveSession(ctx, tc.sessionUUID)

			if tc.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedCheck, check)
		})
	}
}
