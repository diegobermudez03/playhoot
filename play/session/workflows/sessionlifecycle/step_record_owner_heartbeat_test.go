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

func TestRecordOwnerHeartbeatInTx(t *testing.T) {
	type test struct {
		expected        sessionpkg.OwnerHeartbeatResult
		expectErr       bool
		expectedErrorIs error
	}

	ctx := context.Background()
	tx := &gorm.DB{}
	nowInDB := startNow.Truncate(time.Microsecond)
	sessionID := uint(31)
	aborted := "OWNER_LOST"

	tests := map[string]func(t *testing.T, repo *MockrecordOwnerHeartbeatRepo) test{
		"running_session_records_it": func(t *testing.T, repo *MockrecordOwnerHeartbeatRepo) test {
			gomock.InOrder(
				repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "RUNNING"}, nil),
				repo.EXPECT().RenewOwnerLease(ctx, tx, sessionID, nowInDB.Add(10*time.Minute)).Return(nil),
			)
			return test{expected: sessionpkg.OwnerHeartbeatResult{Outcome: sessionpkg.HeartbeatRecorded}}
		},
		"ended_session_tells_how_it_ended": func(t *testing.T, repo *MockrecordOwnerHeartbeatRepo) test {
			repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "ABORTED", EndCause: &aborted}, nil)
			return test{expected: sessionpkg.OwnerHeartbeatResult{Outcome: sessionpkg.HeartbeatSessionNotRunning, Status: "ABORTED", EndCause: "OWNER_LOST"}}
		},
		"unknown_session": func(t *testing.T, repo *MockrecordOwnerHeartbeatRepo) test {
			repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(nil, nil)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrSessionNotFound}
		},
		"recording_fails": func(t *testing.T, repo *MockrecordOwnerHeartbeatRepo) test {
			gomock.InOrder(
				repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "RUNNING"}, nil),
				repo.EXPECT().RenewOwnerLease(ctx, tx, sessionID, nowInDB.Add(10*time.Minute)).Return(errors.New("database down")),
			)
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			repo := NewMockrecordOwnerHeartbeatRepo(gomock.NewController(t))
			tc := setup(t, repo)
			manager := &Manager{recordOwnerHeartbeatRepo: repo, now: func() time.Time { return startNow }, ownerLease: 10 * time.Minute}

			result, err := manager.recordOwnerHeartbeatInTx(ctx, tx, startSessionUUID)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}
