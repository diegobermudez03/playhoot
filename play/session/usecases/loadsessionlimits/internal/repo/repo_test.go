package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

type testSession struct {
	ID                uint
	UUID              string
	GameVersionID     uint
	CreatedByUserUUID string
	Status            string
	LobbyExpiresAt    time.Time
	ExpiresAt         time.Time
}

func (testSession) TableName() string { return "play_sessions" }

func TestFindSessionLimits(t *testing.T) {
	type test struct {
		sessionUUID string

		expected *Limits
	}

	lobbyExpiresAt := time.Date(2026, 10, 18, 12, 30, 0, 0, time.UTC)
	expiresAt := time.Date(2026, 10, 18, 16, 0, 0, 0, time.UTC)

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"session_deadlines": func(t *testing.T, db *gorm.DB) test {
			session := testSession{
				UUID: uuid.NewString(), GameVersionID: 1, CreatedByUserUUID: uuid.NewString(), Status: "LOBBY",
				LobbyExpiresAt: lobbyExpiresAt, ExpiresAt: expiresAt,
			}
			require.NoError(t, db.Create(&session).Error)
			return test{sessionUUID: session.UUID, expected: &Limits{LobbyExpiresAt: lobbyExpiresAt, ExpiresAt: expiresAt}}
		},
		"unknown_session": func(t *testing.T, db *gorm.DB) test {
			return test{sessionUUID: uuid.NewString()}
		},
	}

	db := testdb.OpenSessionDB(t)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			limits, err := New(db).FindSessionLimits(context.Background(), tc.sessionUUID)

			require.NoError(t, err)
			if tc.expected == nil {
				require.Nil(t, limits)
				return
			}
			require.NotNil(t, limits)
			require.True(t, tc.expected.LobbyExpiresAt.Equal(limits.LobbyExpiresAt))
			require.True(t, tc.expected.ExpiresAt.Equal(limits.ExpiresAt))
		})
	}
}
