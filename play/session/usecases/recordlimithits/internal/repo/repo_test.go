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

type limitRow struct {
	GameVersionID uint
	UserUUID      string
	ParticipantID *string
	Source        string
	Kind          string
	Detail        string
	Occurrences   int
}

func limitRowsOf(t *testing.T, db *gorm.DB, sessionID uint) []limitRow {
	t.Helper()
	var rows []limitRow
	require.NoError(t, db.Raw(`
		SELECT game_version_id, user_uuid, participant_id, source, kind, detail::text AS detail, occurrences
		FROM play_game_faults WHERE session_id = ? ORDER BY source, participant_id NULLS LAST
	`, sessionID).Scan(&rows).Error)
	return rows
}

func limitsOf(t *testing.T, detail string) map[string]int {
	t.Helper()
	var parsed struct {
		Limits map[string]int `json:"limits"`
	}
	require.NoError(t, json.Unmarshal([]byte(detail), &parsed))
	return parsed.Limits
}

func TestRecordHits(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	ctx := context.Background()
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

	t.Run("creates_one_server_record_per_user_counting_each_limit", func(t *testing.T) {
		session := seedSession(t, db)

		recorded, err := repo.RecordHits(ctx, []Hit{
			{SessionUUID: session.uuid, UserUUID: session.playerUUID, Limit: "api.command.rate", Count: 40},
			{SessionUUID: session.uuid, UserUUID: session.playerUUID, Limit: "runtime.sample_rate", Count: 3},
		}, at)
		require.NoError(t, err)
		require.Equal(t, 2, recorded)
		_, err = repo.RecordHits(ctx, []Hit{
			{SessionUUID: session.uuid, UserUUID: session.playerUUID, Limit: "api.command.rate", Count: 2},
		}, at.Add(5*time.Second))
		require.NoError(t, err)

		rows := limitRowsOf(t, db, session.id)
		require.Len(t, rows, 1)
		require.Equal(t, "SERVER", rows[0].Source)
		require.Equal(t, "SERVER_LIMIT", rows[0].Kind)
		require.Equal(t, session.gameVersionID, rows[0].GameVersionID)
		require.Equal(t, "P1", *rows[0].ParticipantID)
		require.Equal(t, 45, rows[0].Occurrences)
		require.Equal(t, map[string]int{"api.command.rate": 42, "runtime.sample_rate": 3}, limitsOf(t, rows[0].Detail))
	})

	t.Run("a_user_who_is_no_participant_has_no_participant_id", func(t *testing.T) {
		session := seedSession(t, db)
		admin := uuid.NewString()

		_, err := repo.RecordHits(ctx, []Hit{{SessionUUID: session.uuid, UserUUID: admin, Limit: "api.select_perspective.rate", Count: 7}}, at)

		require.NoError(t, err)
		rows := limitRowsOf(t, db, session.id)
		require.Len(t, rows, 1)
		require.Nil(t, rows[0].ParticipantID)
		require.Equal(t, admin, rows[0].UserUUID)
	})

	t.Run("a_session_that_does_not_exist_is_skipped", func(t *testing.T) {
		recorded, err := repo.RecordHits(ctx, []Hit{{SessionUUID: uuid.NewString(), UserUUID: uuid.NewString(), Limit: "api.command.rate", Count: 1}}, at)

		require.NoError(t, err)
		require.Zero(t, recorded)
	})

	t.Run("client_reports_and_server_limits_are_separate_records", func(t *testing.T) {
		session := seedSession(t, db)
		require.NoError(t, db.Exec(`
			INSERT INTO play_game_faults (session_id, game_version_id, user_uuid, participant_id, source, kind, detail,
				occurrences, first_reported_at, last_reported_at)
			VALUES (?, ?, ?, 'P1', 'CLIENT', 'BRIDGE_ABUSE', '{}', 1, NOW(), NOW())
		`, session.id, session.gameVersionID, session.playerUUID).Error)

		_, err := repo.RecordHits(ctx, []Hit{{SessionUUID: session.uuid, UserUUID: session.playerUUID, Limit: "api.command.rate", Count: 1}}, at)

		require.NoError(t, err)
		require.Len(t, limitRowsOf(t, db, session.id), 2)
	})

	t.Run("the_database_refuses_a_client_record_without_a_participant", func(t *testing.T) {
		session := seedSession(t, db)

		err := db.Exec(`
			INSERT INTO play_game_faults (session_id, game_version_id, user_uuid, participant_id, source, kind, detail,
				occurrences, first_reported_at, last_reported_at)
			VALUES (?, ?, ?, NULL, 'CLIENT', 'BRIDGE_ABUSE', '{}', 1, NOW(), NOW())
		`, session.id, session.gameVersionID, uuid.NewString()).Error

		require.ErrorContains(t, err, "play_game_faults_kind_check")
	})
}
