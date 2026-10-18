package repo

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

// seedVersion creates a Game with one version in status, whose frontend
// package is at key with hash sha when key is not nil.
func seedVersion(t *testing.T, db *gorm.DB, status string, key *string, sha *string) string {
	t.Helper()
	var gameID uint
	require.NoError(t, db.Raw(`INSERT INTO play_games (game_uuid) VALUES (?) RETURNING id`, uuid.NewString()).Scan(&gameID).Error)
	versionUUID := uuid.NewString()
	require.NoError(t, db.Exec(`
		INSERT INTO play_game_versions (uuid, game_id, status, manifest, frontend_object_key, frontend_sha256, frontend_size)
		VALUES (?, ?, ?, '{"schema_version":1,"players":{"min":1,"max":8}}', ?, ?, 10)
	`, versionUUID, gameID, status, key, sha).Error)
	return versionUUID
}

func TestFindFrontend(t *testing.T) {
	type test struct {
		gameVersionUUID string

		expected *Frontend
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"version_with_its_frontend_package": func(t *testing.T, db *gorm.DB) test {
			key, sha := "game-versions/frontend-packages/"+uuid.NewString(), "9f86d081"
			return test{
				gameVersionUUID: seedVersion(t, db, "EXECUTABLE", &key, &sha),
				expected:        &Frontend{Status: "EXECUTABLE", ObjectKey: &key, SHA256: &sha},
			}
		},
		"version_without_frontend_package": func(t *testing.T, db *gorm.DB) test {
			return test{gameVersionUUID: seedVersion(t, db, "DRAFT", nil, nil), expected: &Frontend{Status: "DRAFT"}}
		},
		"unknown_version": func(t *testing.T, db *gorm.DB) test {
			return test{gameVersionUUID: uuid.NewString()}
		},
	}

	db := testdb.OpenSessionDB(t)
	repo := New(db)
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t, db)

			frontend, err := repo.FindFrontend(context.Background(), tc.gameVersionUUID)

			require.NoError(t, err)
			require.Equal(t, tc.expected, frontend)
		})
	}
}
