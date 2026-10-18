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
}

func (testSession) TableName() string { return "play_sessions" }

type testJoinCode struct {
	ID         uint
	SessionID  uint
	Code       string
	ExpiresAt  time.Time
	ReleasedAt *time.Time
}

func (testJoinCode) TableName() string { return "play_session_join_codes" }

// seedJoinCode creates a LOBBY Session holding code, released at releasedAt
// when set.
func seedJoinCode(t *testing.T, db *gorm.DB, code string, expiresAt time.Time, releasedAt *time.Time) testSession {
	t.Helper()
	game := testGame{GameUUID: uuid.NewString()}
	require.NoError(t, db.Create(&game).Error)
	version := testGameVersion{UUID: uuid.NewString(), GameID: game.ID, Status: "EXECUTABLE", Manifest: `{"schema_version":1,"players":{"min":1,"max":8}}`}
	require.NoError(t, db.Create(&version).Error)
	session := testSession{
		UUID:              uuid.NewString(),
		GameVersionID:     version.ID,
		CreatedByUserUUID: uuid.NewString(),
		Status:            "LOBBY",
		LobbyExpiresAt:    expiresAt,
	}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&testJoinCode{SessionID: session.ID, Code: code, ExpiresAt: expiresAt, ReleasedAt: releasedAt}).Error)
	return session
}

func TestFindActiveJoinCode(t *testing.T) {
	type test struct {
		code             string
		expectedJoinCode *JoinCode
	}

	expiresAt := time.Date(2026, 10, 17, 12, 30, 0, 0, time.UTC)
	releasedAt := expiresAt.Add(time.Minute)

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"active_code": func(t *testing.T, db *gorm.DB) test {
			seeded := seedJoinCode(t, db, "AAAAA1", expiresAt, nil)
			return test{code: "AAAAA1", expectedJoinCode: &JoinCode{SessionUUID: seeded.UUID, ExpiresAt: expiresAt}}
		},
		"active_code_reused_after_a_released_one": func(t *testing.T, db *gorm.DB) test {
			seedJoinCode(t, db, "AAAAA2", expiresAt, &releasedAt)
			current := seedJoinCode(t, db, "AAAAA2", expiresAt.Add(time.Hour), nil)
			return test{code: "AAAAA2", expectedJoinCode: &JoinCode{SessionUUID: current.UUID, ExpiresAt: expiresAt.Add(time.Hour)}}
		},
		"released_code": func(t *testing.T, db *gorm.DB) test {
			seedJoinCode(t, db, "AAAAA3", expiresAt, &releasedAt)
			return test{code: "AAAAA3"}
		},
		"unknown_code": func(t *testing.T, db *gorm.DB) test {
			seedJoinCode(t, db, "AAAAA4", expiresAt, nil)
			return test{code: "ZZZZZ9"}
		},
	}

	db := testdb.OpenSessionDB(t)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			joinCode, err := New(db).FindActiveJoinCode(context.Background(), tc.code)

			require.NoError(t, err)
			if tc.expectedJoinCode == nil {
				require.Nil(t, joinCode)
				return
			}
			require.NotNil(t, joinCode)
			require.Equal(t, tc.expectedJoinCode.SessionUUID, joinCode.SessionUUID)
			require.True(t, tc.expectedJoinCode.ExpiresAt.Equal(joinCode.ExpiresAt))
		})
	}
}
