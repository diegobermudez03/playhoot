package checkadminjoin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/checkadminjoin/internal/repo"
)

func TestEligibleToJoinAsAdmin(t *testing.T) {
	type dependencies struct {
		sessions *MocksessionFinder
	}
	type test struct {
		sessionUUID string
		userUUID    string

		expectedEligibility sessionpkg.AdminJoinEligibility
		expectedErr         bool
	}

	const (
		sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		creatorUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
		otherUUID   = "aaaaaaaa-aaaa-4aaa-8aaa-000000000002"
	)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	session := func(status string, lobbyExpiresAt time.Time) *internalrepo.Session {
		return &internalrepo.Session{Status: status, CreatedByUserUUID: creatorUUID, LobbyExpiresAt: lobbyExpiresAt}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"creator_of_a_lobby": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("LOBBY", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, userUUID: creatorUUID, expectedEligibility: sessionpkg.AdminJoinEligible}
		},
		"creator_of_a_running_session": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("RUNNING", now.Add(-time.Hour)), nil)
			return test{sessionUUID: sessionUUID, userUUID: creatorUUID, expectedEligibility: sessionpkg.AdminJoinEligibleRunning}
		},
		"creator_uuid_in_another_case": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("LOBBY", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, userUUID: strings.ToUpper(creatorUUID), expectedEligibility: sessionpkg.AdminJoinEligible}
		},
		"lobby_whose_expiry_passed": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("LOBBY", now), nil)
			return test{sessionUUID: sessionUUID, userUUID: creatorUUID, expectedEligibility: sessionpkg.AdminJoinSessionNotJoinable}
		},
		"ended_session": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("FINISHED", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, userUUID: creatorUUID, expectedEligibility: sessionpkg.AdminJoinSessionNotJoinable}
		},
		"session_with_an_unknown_status_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("PAUSED", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, userUUID: creatorUUID, expectedErr: true}
		},
		"not_the_creator": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("LOBBY", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, userUUID: otherUUID, expectedEligibility: sessionpkg.AdminJoinNotCreator}
		},
		"not_the_creator_of_an_ended_session": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(session("CANCELLED", now.Add(time.Minute)), nil)
			return test{sessionUUID: sessionUUID, userUUID: otherUUID, expectedEligibility: sessionpkg.AdminJoinNotCreator}
		},
		"session_not_found": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(nil, nil)
			return test{sessionUUID: sessionUUID, userUUID: creatorUUID, expectedEligibility: sessionpkg.AdminJoinSessionNotFound}
		},
		"session_uuid_is_not_a_uuid": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "not-a-uuid", userUUID: creatorUUID, expectedEligibility: sessionpkg.AdminJoinSessionNotFound}
		},
		"reading_the_session_fails": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().FindSession(ctx, sessionUUID).Return(nil, errors.New("database down"))
			return test{sessionUUID: sessionUUID, userUUID: creatorUUID, expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{sessions: NewMocksessionFinder(ctrl)}
			tc := setup(t, deps)
			service := &Service{sessions: deps.sessions, now: func() time.Time { return now }}

			eligibility, err := service.EligibleToJoinAsAdmin(ctx, tc.sessionUUID, tc.userUUID)

			if tc.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedEligibility, eligibility)
		})
	}
}
