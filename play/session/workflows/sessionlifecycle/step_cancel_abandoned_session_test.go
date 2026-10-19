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

func TestCancelAbandonedSessionInTx(t *testing.T) {
	type dependencies struct {
		repo *MockcancelAbandonedSessionRepo
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
	cause := "ALL_PLAYERS_LEFT"

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"running_session_is_cancelled_by_the_platform": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "RUNNING"}, nil),
				deps.repo.EXPECT().ChangeSessionStatus(ctx, tx, sessionID, "RUNNING", "CANCELLED", &cause, (*string)(nil), nowInDB).Return(nil),
			)
			return test{expected: true}
		},
		"session_that_already_ended_is_left_as_it_is": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "ABORTED"}, nil)
			return test{expected: false}
		},
		"unknown_session": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(nil, nil)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrSessionNotFound}
		},
		"changing_the_status_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "RUNNING"}, nil),
				deps.repo.EXPECT().ChangeSessionStatus(ctx, tx, sessionID, "RUNNING", "CANCELLED", &cause, (*string)(nil), nowInDB).Return(errors.New("database down")),
			)
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockcancelAbandonedSessionRepo(ctrl)}
			tc := setup(t, deps)
			manager := &Manager{cancelAbandonedSessionRepo: deps.repo, now: func() time.Time { return startNow }}

			cancelled, err := manager.cancelAbandonedSessionInTx(ctx, tx, startSessionUUID)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, cancelled)
		})
	}
}
