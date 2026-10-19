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

type testGame struct {
	ID       uint
	GameUUID string
}

func (testGame) TableName() string { return "play_games" }

type testGameVersion struct {
	ID       uint
	UUID     string
	GameID   uint
	Status   string
	Manifest string
}

func (testGameVersion) TableName() string { return "play_game_versions" }

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

func seedSession(t *testing.T, db *gorm.DB, status string, lobbyExpiresAt time.Time) testSession {
	t.Helper()
	game := testGame{GameUUID: uuid.NewString()}
	require.NoError(t, db.Create(&game).Error)
	version := testGameVersion{UUID: uuid.NewString(), GameID: game.ID, Status: "EXECUTABLE", Manifest: `{"schema_version":1,"players":{"min":1,"max":8}}`}
	require.NoError(t, db.Create(&version).Error)
	session := testSession{
		UUID:              uuid.NewString(),
		GameVersionID:     version.ID,
		CreatedByUserUUID: uuid.NewString(),
		Status:            status,
		LobbyExpiresAt:    lobbyExpiresAt,
		ExpiresAt:         lobbyExpiresAt.Add(4 * time.Hour),
	}
	require.NoError(t, db.Create(&session).Error)
	return session
}

func TestFindSession(t *testing.T) {
	type test struct {
		sessionUUID     string
		expectedSession *Session
	}

	lobbyExpiresAt := time.Date(2026, 10, 17, 12, 30, 0, 0, time.UTC)

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"existing_session": func(t *testing.T, db *gorm.DB) test {
			seeded := seedSession(t, db, "LOBBY", lobbyExpiresAt)
			return test{sessionUUID: seeded.UUID, expectedSession: &Session{Status: "LOBBY", LobbyExpiresAt: lobbyExpiresAt}}
		},
		"unknown_session": func(t *testing.T, db *gorm.DB) test {
			seedSession(t, db, "LOBBY", lobbyExpiresAt)
			return test{sessionUUID: uuid.NewString()}
		},
	}

	db := testdb.OpenSessionDB(t)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			session, err := New(db).FindSession(context.Background(), tc.sessionUUID)

			require.NoError(t, err)
			if tc.expectedSession == nil {
				require.Nil(t, session)
				return
			}
			require.NotNil(t, session)
			require.Equal(t, tc.expectedSession.Status, session.Status)
			require.True(t, tc.expectedSession.LobbyExpiresAt.Equal(session.LobbyExpiresAt))
		})
	}
}
