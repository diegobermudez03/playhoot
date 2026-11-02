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

type seededSession struct {
	id            uint
	uuid          string
	gameVersionID uint
	playerUUID    string
}

// seedSession creates a RUNNING Session with one participant, P1.
func seedSession(t *testing.T, db *gorm.DB) seededSession {
	t.Helper()
	session := seededSession{uuid: uuid.NewString(), gameVersionID: uint(time.Now().UnixNano() % 1_000_000), playerUUID: uuid.NewString()}
	require.NoError(t, db.Raw(`
		INSERT INTO play_sessions (uuid, game_version_id, created_by_user_uuid, status, lobby_expires_at, expires_at)
		VALUES (?, ?, ?, 'RUNNING', NOW(), NOW() + INTERVAL '1 hour') RETURNING id
	`, session.uuid, session.gameVersionID, uuid.NewString()).Scan(&session.id).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO play_session_participants (session_id, user_uuid, participant_id) VALUES (?, ?, 'P1')
	`, session.id, session.playerUUID).Error)
	return session
}

func TestFindTarget(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	ctx := context.Background()
	session := seedSession(t, db)

	player, err := repo.FindTarget(ctx, db, session.uuid, session.playerUUID)
	require.NoError(t, err)
	require.Equal(t, int64(session.id), player.SessionID)
	require.Equal(t, int64(session.gameVersionID), player.GameVersionID)
	require.Equal(t, "P1", *player.ParticipantID)

	outsider, err := repo.FindTarget(ctx, db, session.uuid, uuid.NewString())
	require.NoError(t, err)
	require.Nil(t, outsider.ParticipantID, "a user who never played is still reported")

	unknown, err := repo.FindTarget(ctx, db, uuid.NewString(), session.playerUUID)
	require.NoError(t, err)
	require.Nil(t, unknown)
}

func TestCreateReport(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	ctx := context.Background()
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	session := seedSession(t, db)
	for _, count := range []int{40, 3} {
		require.NoError(t, repo.CreateReport(ctx, db, Report{
			SessionID: int64(session.id), GameVersionID: int64(session.gameVersionID), UserUUID: session.playerUUID,
			Source: "SERVER", Kind: "SERVER_LIMIT", Detail: `{"limit":"api.command.rate"}`, Occurrences: count, ReportedAt: at,
		}))
		at = at.Add(5 * time.Second)
	}

	var rows []struct {
		Detail      string
		Occurrences int
	}
	require.NoError(t, db.Raw(`SELECT detail::text AS detail, occurrences FROM play_game_fault_reports WHERE session_id = ? ORDER BY id`, session.id).Scan(&rows).Error)
	require.Len(t, rows, 2, "one report per flush, none overwritten")
	require.Equal(t, 40, rows[0].Occurrences)
	require.Equal(t, 3, rows[1].Occurrences)
	var detail map[string]string
	require.NoError(t, json.Unmarshal([]byte(rows[0].Detail), &detail))
	require.Equal(t, map[string]string{"limit": "api.command.rate"}, detail)
}
