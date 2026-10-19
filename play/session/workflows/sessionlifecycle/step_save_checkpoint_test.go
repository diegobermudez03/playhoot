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

func TestSaveCheckpointInTx(t *testing.T) {
	type test struct {
		expected        sessionpkg.SaveCheckpointOutcome
		expectErr       bool
		expectedErrorIs error
	}

	ctx := context.Background()
	tx := &gorm.DB{}
	nowInDB := startNow.Truncate(time.Microsecond)
	sessionID := uint(31)
	checkpoint := sessionpkg.Checkpoint{ContextRevision: 7, Content: []byte(`{"state":{}}`)}

	tests := map[string]func(t *testing.T, repo *MocksaveCheckpointRepo) test{
		"running_session_stores_it": func(t *testing.T, repo *MocksaveCheckpointRepo) test {
			gomock.InOrder(
				repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "RUNNING"}, nil),
				repo.EXPECT().SaveCheckpoint(ctx, tx, sessionID, int64(7), checkpoint.Content, nowInDB).Return(nil),
			)
			return test{expected: sessionpkg.CheckpointSaved}
		},
		"session_that_ended_stores_nothing": func(t *testing.T, repo *MocksaveCheckpointRepo) test {
			repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "FINISHED"}, nil)
			return test{expected: sessionpkg.CheckpointSessionNotRunning}
		},
		"unknown_session": func(t *testing.T, repo *MocksaveCheckpointRepo) test {
			repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(nil, nil)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrSessionNotFound}
		},
		"storing_fails": func(t *testing.T, repo *MocksaveCheckpointRepo) test {
			gomock.InOrder(
				repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "RUNNING"}, nil),
				repo.EXPECT().SaveCheckpoint(ctx, tx, sessionID, int64(7), checkpoint.Content, nowInDB).Return(errors.New("database down")),
			)
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			repo := NewMocksaveCheckpointRepo(gomock.NewController(t))
			tc := setup(t, repo)
			manager := &Manager{saveCheckpointRepo: repo, now: func() time.Time { return startNow }}

			outcome, err := manager.saveCheckpointInTx(ctx, tx, startSessionUUID, checkpoint)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, outcome)
		})
	}
}
