package reconcilesessions

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/batches/reconcilesessions/internal/repo"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/endcause"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestReconcileSessionInTx(t *testing.T) {
	type test struct {
		expected  endcause.Cause
		expectErr bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	ctx := context.Background()
	tx := &gorm.DB{}
	now := time.Date(2026, 10, 19, 12, 0, 0, 123456789, time.UTC)
	nowInDB := now.Truncate(time.Microsecond)
	sessionID := uint(31)
	lockTimeout := 5 * time.Second
	hourAhead := now.Add(time.Hour)
	lease := func(fromNow time.Duration) *time.Time {
		at := now.Add(fromNow)
		return &at
	}
	locks := func(sessions *MocksessionRepo, session repo.Session) *gomock.Call {
		session.ID = sessionID
		return sessions.EXPECT().LockSession(ctx, tx, sessionUUID, lockTimeout).Return(&session, nil)
	}

	tests := map[string]func(t *testing.T, sessions *MocksessionRepo) test{
		"expired_lobby_expires": func(t *testing.T, sessions *MocksessionRepo) test {
			gomock.InOrder(
				locks(sessions, repo.Session{Status: "LOBBY", LobbyExpiresAt: now, ExpiresAt: hourAhead}),
				sessions.EXPECT().EndSession(ctx, tx, sessionID, "LOBBY", "EXPIRED", "LOBBY_EXPIRED", nowInDB).Return(nil),
			)
			return test{expected: endcause.LobbyExpired}
		},
		"finished_game_whose_lease_ran_out_finishes": func(t *testing.T, sessions *MocksessionRepo) test {
			gomock.InOrder(
				locks(sessions, repo.Session{Status: "RUNNING", ExpiresAt: hourAhead, OwnerLeaseExpiresAt: lease(-time.Second), ReplayFinalized: true}),
				sessions.EXPECT().EndSession(ctx, tx, sessionID, "RUNNING", "FINISHED", "GAME_FINISHED", nowInDB).Return(nil),
			)
			return test{expected: endcause.GameFinished}
		},
		"past_its_lifetime_expires": func(t *testing.T, sessions *MocksessionRepo) test {
			gomock.InOrder(
				locks(sessions, repo.Session{Status: "RUNNING", ExpiresAt: now, OwnerLeaseExpiresAt: lease(time.Minute)}),
				sessions.EXPECT().EndSession(ctx, tx, sessionID, "RUNNING", "EXPIRED", "MAX_LIFETIME", nowInDB).Return(nil),
			)
			return test{expected: endcause.MaxLifetime}
		},
		"lease_that_ran_out_aborts": func(t *testing.T, sessions *MocksessionRepo) test {
			gomock.InOrder(
				locks(sessions, repo.Session{Status: "RUNNING", ExpiresAt: hourAhead, OwnerLeaseExpiresAt: lease(0)}),
				sessions.EXPECT().EndSession(ctx, tx, sessionID, "RUNNING", "ABORTED", "OWNER_LOST", nowInDB).Return(nil),
			)
			return test{expected: endcause.OwnerLost}
		},
		"lease_renewed_since_it_was_read_keeps_it": func(t *testing.T, sessions *MocksessionRepo) test {
			locks(sessions, repo.Session{Status: "RUNNING", ExpiresAt: hourAhead, OwnerLeaseExpiresAt: lease(10 * time.Minute)})
			return test{}
		},
		"session_gone_since_it_was_read": func(t *testing.T, sessions *MocksessionRepo) test {
			sessions.EXPECT().LockSession(ctx, tx, sessionUUID, lockTimeout).Return(nil, nil)
			return test{}
		},
		"locking_fails": func(t *testing.T, sessions *MocksessionRepo) test {
			sessions.EXPECT().LockSession(ctx, tx, sessionUUID, lockTimeout).Return(nil, errors.New("lock timeout"))
			return test{expectErr: true}
		},
		"ending_fails": func(t *testing.T, sessions *MocksessionRepo) test {
			gomock.InOrder(
				locks(sessions, repo.Session{Status: "LOBBY", LobbyExpiresAt: now, ExpiresAt: hourAhead}),
				sessions.EXPECT().EndSession(ctx, tx, sessionID, "LOBBY", "EXPIRED", "LOBBY_EXPIRED", nowInDB).Return(errors.New("database down")),
			)
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			sessions := NewMocksessionRepo(gomock.NewController(t))
			tc := setup(t, sessions)
			service := &Service{sessions: sessions, now: func() time.Time { return now }, lockTimeout: lockTimeout}

			cause, err := service.reconcileSessionInTx(ctx, tx, sessionUUID)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, cause)
		})
	}
}
