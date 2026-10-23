package resolvejoincode

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/resolvejoincode/internal/repo"
)

func TestResolveJoinCode(t *testing.T) {
	type dependencies struct {
		joinCodes *MockjoinCodeFinder
	}
	type test struct {
		joinCode string

		expectedResolution sessionpkg.JoinCodeResolution
		expectedErr        bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	latest := func(status string, expiresAt time.Time) *internalrepo.JoinCode {
		return &internalrepo.JoinCode{SessionUUID: sessionUUID, SessionStatus: status, ExpiresAt: expiresAt}
	}
	found := sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeFound, SessionUUID: sessionUUID}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"lobby_code_not_expired": func(t *testing.T, deps dependencies) test {
			deps.joinCodes.EXPECT().FindLatestJoinCode(ctx, "K7QX2M").Return(latest("LOBBY", now.Add(time.Minute)), nil)
			return test{joinCode: "K7QX2M", expectedResolution: found}
		},
		"code_typed_in_lowercase": func(t *testing.T, deps dependencies) test {
			deps.joinCodes.EXPECT().FindLatestJoinCode(ctx, "K7QX2M").Return(latest("LOBBY", now.Add(time.Minute)), nil)
			return test{joinCode: "k7qx2m", expectedResolution: found}
		},
		"lobby_code_whose_expiry_passed": func(t *testing.T, deps dependencies) test {
			deps.joinCodes.EXPECT().FindLatestJoinCode(ctx, "K7QX2M").Return(latest("LOBBY", now), nil)
			return test{joinCode: "K7QX2M", expectedResolution: sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeExpired}}
		},
		"running_session": func(t *testing.T, deps dependencies) test {
			deps.joinCodes.EXPECT().FindLatestJoinCode(ctx, "K7QX2M").Return(latest("RUNNING", now.Add(time.Minute)), nil)
			return test{joinCode: "K7QX2M", expectedResolution: sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeSessionStarted}}
		},
		"running_session_whose_code_expired": func(t *testing.T, deps dependencies) test {
			deps.joinCodes.EXPECT().FindLatestJoinCode(ctx, "K7QX2M").Return(latest("RUNNING", now.Add(-time.Hour)), nil)
			return test{joinCode: "K7QX2M", expectedResolution: sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeSessionStarted}}
		},
		"finished_session": func(t *testing.T, deps dependencies) test {
			deps.joinCodes.EXPECT().FindLatestJoinCode(ctx, "K7QX2M").Return(latest("FINISHED", now.Add(time.Minute)), nil)
			return test{joinCode: "K7QX2M", expectedResolution: sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeInvalid}}
		},
		"cancelled_session": func(t *testing.T, deps dependencies) test {
			deps.joinCodes.EXPECT().FindLatestJoinCode(ctx, "K7QX2M").Return(latest("CANCELLED", now.Add(time.Minute)), nil)
			return test{joinCode: "K7QX2M", expectedResolution: sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeInvalid}}
		},
		"expired_session": func(t *testing.T, deps dependencies) test {
			deps.joinCodes.EXPECT().FindLatestJoinCode(ctx, "K7QX2M").Return(latest("EXPIRED", now.Add(-time.Minute)), nil)
			return test{joinCode: "K7QX2M", expectedResolution: sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeInvalid}}
		},
		"aborted_session": func(t *testing.T, deps dependencies) test {
			deps.joinCodes.EXPECT().FindLatestJoinCode(ctx, "K7QX2M").Return(latest("ABORTED", now.Add(time.Minute)), nil)
			return test{joinCode: "K7QX2M", expectedResolution: sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeInvalid}}
		},
		"code_never_given_out": func(t *testing.T, deps dependencies) test {
			deps.joinCodes.EXPECT().FindLatestJoinCode(ctx, "K7QX2M").Return(nil, nil)
			return test{joinCode: "K7QX2M", expectedResolution: sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeInvalid}}
		},
		"reading_the_code_fails": func(t *testing.T, deps dependencies) test {
			deps.joinCodes.EXPECT().FindLatestJoinCode(ctx, "K7QX2M").Return(nil, errors.New("database down"))
			return test{joinCode: "K7QX2M", expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{joinCodes: NewMockjoinCodeFinder(ctrl)}
			tc := setup(t, deps)
			service := &Service{joinCodes: deps.joinCodes, now: func() time.Time { return now }}

			resolution, err := service.ResolveJoinCode(ctx, tc.joinCode)

			if tc.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedResolution, resolution)
		})
	}
}
