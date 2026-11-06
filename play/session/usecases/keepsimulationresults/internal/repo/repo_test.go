package repo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

// seedVersion creates a Game with one DRAFT version and returns its id and
// uuid.
func seedVersion(t *testing.T, db *gorm.DB) (uint, string) {
	t.Helper()
	var gameID uint
	require.NoError(t, db.Raw(`INSERT INTO play_games (game_uuid) VALUES (?) RETURNING id`, uuid.NewString()).Scan(&gameID).Error)
	versionUUID := uuid.NewString()
	var versionID uint
	require.NoError(t, db.Raw(`
		INSERT INTO play_game_versions (uuid, game_id, status, script_profile) VALUES (?, ?, 'DRAFT', 'sdk-v1') RETURNING id
	`, versionUUID, gameID).Scan(&versionID).Error)
	return versionID, versionUUID
}

func TestSimulationResults(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	versionID, versionUUID := seedVersion(t, db)
	_, otherVersion := seedVersion(t, db)

	id, found, err := repo.FindVersionID(ctx, versionUUID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, versionID, id)
	_, found, err = repo.FindVersionID(ctx, uuid.NewString())
	require.NoError(t, err)
	require.False(t, found)

	kept, expired, old := uuid.NewString(), uuid.NewString(), uuid.NewString()
	require.NoError(t, repo.Insert(ctx, kept, versionID, json.RawMessage(`{"n":1}`), now, now.Add(30*time.Minute)))
	require.NoError(t, repo.Insert(ctx, expired, versionID, json.RawMessage(`{"n":2}`), now.Add(-time.Hour), now.Add(-30*time.Minute)))
	require.NoError(t, repo.Insert(ctx, old, versionID, json.RawMessage(`{"n":3}`), now.Add(-10*24*time.Hour), now.Add(-10*24*time.Hour)))

	result, found, err := repo.Find(ctx, kept, versionUUID, now)
	require.NoError(t, err)
	require.True(t, found)
	require.JSONEq(t, `{"n":1}`, string(result))
	_, found, err = repo.Find(ctx, kept, otherVersion, now)
	require.NoError(t, err)
	require.False(t, found, "another version's result is not found")
	_, found, err = repo.Find(ctx, expired, versionUUID, now)
	require.NoError(t, err)
	require.False(t, found, "an expired result is not found")

	require.NoError(t, repo.ClearExpired(ctx, now, now.Add(-7*24*time.Hour)))
	var rows []struct {
		UUID    string
		Cleared bool
	}
	require.NoError(t, db.Raw(`SELECT uuid::text AS uuid, result IS NULL AS cleared FROM play_simulation_results WHERE game_version_id = ? ORDER BY created_at`, versionID).Scan(&rows).Error)
	require.Len(t, rows, 2, "a record older than the retention is forgotten")
	require.True(t, rows[0].Cleared, "an expired record keeps no result")
	require.False(t, rows[1].Cleared)

	count, err := repo.CountSince(ctx, versionUUID, now.Add(-2*time.Hour))
	require.NoError(t, err)
	require.Equal(t, 2, count, "cleared records still count")
}
