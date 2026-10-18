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

func seedVersion(t *testing.T, db *gorm.DB, manifest string) testGameVersion {
	t.Helper()
	game := testGame{GameUUID: uuid.NewString()}
	require.NoError(t, db.Create(&game).Error)
	version := testGameVersion{UUID: uuid.NewString(), GameID: game.ID, Status: "EXECUTABLE", Manifest: manifest}
	require.NoError(t, db.Create(&version).Error)
	return version
}

// seedSession creates a Session pinned to versionID.
func seedSession(t *testing.T, db *gorm.DB, versionID uint) testSession {
	t.Helper()
	session := testSession{
		UUID:              uuid.NewString(),
		GameVersionID:     versionID,
		CreatedByUserUUID: uuid.NewString(),
		Status:            "LOBBY",
		LobbyExpiresAt:    time.Date(2026, 10, 17, 12, 30, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&session).Error)
	return session
}

func TestFindSessionManifest(t *testing.T) {
	type test struct {
		sessionUUID string

		expectedFound    bool
		expectedManifest string
	}

	const manifest = `{"schema_version":1,"players":{"min":2,"max":6}}`

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"pinned_version_manifest": func(t *testing.T, db *gorm.DB) test {
			session := seedSession(t, db, seedVersion(t, db, manifest).ID)
			seedVersion(t, db, `{"schema_version":1,"players":{"min":1,"max":1}}`)
			return test{sessionUUID: session.UUID, expectedFound: true, expectedManifest: manifest}
		},
		"version_row_missing": func(t *testing.T, db *gorm.DB) test {
			session := seedSession(t, db, 999999)
			return test{sessionUUID: session.UUID, expectedFound: true}
		},
		"unknown_session": func(t *testing.T, db *gorm.DB) test {
			seedSession(t, db, seedVersion(t, db, manifest).ID)
			return test{sessionUUID: uuid.NewString()}
		},
	}

	db := testdb.OpenSessionDB(t)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			raw, found, err := New(db).FindSessionManifest(context.Background(), tc.sessionUUID)

			require.NoError(t, err)
			require.Equal(t, tc.expectedFound, found)
			if tc.expectedManifest == "" {
				require.Empty(t, raw)
				return
			}
			require.JSONEq(t, tc.expectedManifest, string(raw))
		})
	}
}
