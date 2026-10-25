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

type testInputRequirements struct {
	ID               uint
	GameVersionID    uint
	UsesKeyboard     bool
	UsesPointerHover bool
	UsesPointerPress bool
	Devices          string
}

func (testInputRequirements) TableName() string { return "play_game_version_input_requirements" }

// seedRequirements stores requirements for versionID.
func seedRequirements(t *testing.T, db *gorm.DB, versionID uint, requirements InputRequirements) {
	t.Helper()
	require.NoError(t, db.Create(&testInputRequirements{
		GameVersionID:    versionID,
		UsesKeyboard:     requirements.UsesKeyboard,
		UsesPointerHover: requirements.UsesPointerHover,
		UsesPointerPress: requirements.UsesPointerPress,
		Devices:          requirements.Devices,
	}).Error)
}

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
		ExpiresAt:         time.Date(2026, 10, 17, 12, 30, 0, 0, time.UTC).Add(4 * time.Hour),
	}
	require.NoError(t, db.Create(&session).Error)
	return session
}

func TestFindSessionManifest(t *testing.T) {
	type test struct {
		sessionUUID string

		expectedFound        bool
		expectedManifest     string
		expectedRequirements *InputRequirements
	}

	const manifest = `{"schema_version":1,"players":{"min":2,"max":6}}`
	keyboardGame := InputRequirements{UsesKeyboard: true, UsesPointerPress: true, Devices: "desktop"}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"pinned_version_manifest_and_requirements": func(t *testing.T, db *gorm.DB) test {
			pinned := seedVersion(t, db, manifest)
			seedRequirements(t, db, pinned.ID, keyboardGame)
			session := seedSession(t, db, pinned.ID)
			other := seedVersion(t, db, `{"schema_version":1,"players":{"min":1,"max":1}}`)
			seedRequirements(t, db, other.ID, InputRequirements{UsesPointerPress: true, Devices: "both"})
			return test{sessionUUID: session.UUID, expectedFound: true, expectedManifest: manifest, expectedRequirements: &keyboardGame}
		},
		"version_without_requirements": func(t *testing.T, db *gorm.DB) test {
			session := seedSession(t, db, seedVersion(t, db, manifest).ID)
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

			stored, found, err := New(db).FindSessionManifest(context.Background(), tc.sessionUUID)

			require.NoError(t, err)
			require.Equal(t, tc.expectedFound, found)
			require.Equal(t, tc.expectedRequirements, stored.InputRequirements)
			if tc.expectedManifest == "" {
				require.Empty(t, stored.Manifest)
				return
			}
			require.JSONEq(t, tc.expectedManifest, string(stored.Manifest))
		})
	}
}
