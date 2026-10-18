package sessionlifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
)

func TestAbortStartInTx(t *testing.T) {
	type dependencies struct {
		repo *MockabortStartRepo
	}
	type test struct {
		expected        bool
		expectErr       bool
		expectedErrorIs error
	}

	ctx := context.Background()
	tx := &gorm.DB{}
	nowInDB := startNow.Truncate(time.Microsecond)
	sessionID := uint(31)
	cause := "GAME_START_FAILURE"

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"lobby_is_aborted_by_the_start_failure": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "LOBBY"}, nil),
				deps.repo.EXPECT().ChangeSessionStatus(ctx, tx, sessionID, "LOBBY", "ABORTED", &cause, (*string)(nil), nowInDB).Return(nil),
				deps.repo.EXPECT().ReleaseJoinCodes(ctx, tx, sessionID, nowInDB).Return(nil),
			)
			return test{expected: true}
		},
		"session_no_longer_in_its_lobby_is_left_as_it_is": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "EXPIRED"}, nil)
			return test{expected: false}
		},
		"unknown_session": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(nil, nil)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrSessionNotFound}
		},
		"updating_the_status_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "LOBBY"}, nil),
				deps.repo.EXPECT().ChangeSessionStatus(ctx, tx, sessionID, "LOBBY", "ABORTED", &cause, (*string)(nil), nowInDB).Return(errors.New("database down")),
			)
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockabortStartRepo(ctrl)}
			tc := setup(t, deps)
			manager := &Manager{abortStartRepo: deps.repo, now: func() time.Time { return startNow }}

			aborted, err := manager.abortStartInTx(ctx, tx, startSessionUUID)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, aborted)
		})
	}
}
