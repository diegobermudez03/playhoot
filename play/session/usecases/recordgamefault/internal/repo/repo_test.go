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

// seedSession creates a Session in status pinned to a version, with one
// participant, P1.
func seedSession(t *testing.T, db *gorm.DB, status string) seededSession {
	t.Helper()
	session := seededSession{uuid: uuid.NewString(), gameVersionID: uint(time.Now().UnixNano() % 1_000_000), playerUUID: uuid.NewString()}
	require.NoError(t, db.Raw(`
		INSERT INTO play_sessions (uuid, game_version_id, created_by_user_uuid, status, lobby_expires_at, expires_at)
		VALUES (?, ?, ?, ?, NOW(), NOW() + INTERVAL '1 hour') RETURNING id
	`, session.uuid, session.gameVersionID, uuid.NewString(), status).Scan(&session.id).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO play_session_participants (session_id, user_uuid, participant_id) VALUES (?, ?, 'P1')
	`, session.id, session.playerUUID).Error)
	return session
}

type reportRow struct {
	GameVersionID int64
	UserUUID      string
	ParticipantID *string
	Source        string
	Kind          string
	Detail        string
	Occurrences   int
	ReportedAt    time.Time
}

func reportsOf(t *testing.T, db *gorm.DB, sessionID uint) []reportRow {
	t.Helper()
	var rows []reportRow
	require.NoError(t, db.Raw(`
		SELECT game_version_id, user_uuid, participant_id, source, kind, detail::text AS detail, occurrences, reported_at
		FROM play_game_fault_reports WHERE session_id = ? ORDER BY id
	`, sessionID).Scan(&rows).Error)
	return rows
}

func requireJSONEqual(t *testing.T, expected, actual string) {
	t.Helper()
	var e, a any
	require.NoError(t, json.Unmarshal([]byte(expected), &e))
	require.NoError(t, json.Unmarshal([]byte(actual), &a))
	require.Equal(t, e, a)
}

func TestFindReporter(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	ctx := context.Background()
	session := seedSession(t, db, "RUNNING")

	player, err := repo.FindReporter(ctx, session.uuid, session.playerUUID)
	require.NoError(t, err)
	require.Equal(t, int64(session.id), player.SessionID)
	require.Equal(t, int64(session.gameVersionID), player.GameVersionID)
	require.Equal(t, "RUNNING", player.Status)
	require.Equal(t, "P1", *player.ParticipantID)

	stranger, err := repo.FindReporter(ctx, session.uuid, uuid.NewString())
	require.NoError(t, err)
	require.Nil(t, stranger.ParticipantID)

	unknown, err := repo.FindReporter(ctx, uuid.NewString(), session.playerUUID)
	require.NoError(t, err)
	require.Nil(t, unknown)
}

func TestCreateReport(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	ctx := context.Background()
	first := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	session := seedSession(t, db, "RUNNING")
	p1 := "P1"
	report := Report{
		SessionID: int64(session.id), GameVersionID: int64(session.gameVersionID), UserUUID: session.playerUUID, ParticipantID: &p1,
		Source: "CLIENT", Kind: "CONTAINER_VIOLATION", Detail: `{"n":1}`, Occurrences: 1, ReportedAt: first,
	}
	require.NoError(t, repo.CreateReport(ctx, report))
	report.Detail, report.ReportedAt = `{"n":2}`, first.Add(time.Minute)
	require.NoError(t, repo.CreateReport(ctx, report))

	rows := reportsOf(t, db, session.id)
	require.Len(t, rows, 2, "every report is kept")
	requireJSONEqual(t, `{"n":1}`, rows[0].Detail)
	requireJSONEqual(t, `{"n":2}`, rows[1].Detail)
	require.True(t, rows[0].ReportedAt.Equal(first))
	require.Equal(t, "P1", *rows[1].ParticipantID)
	require.Equal(t, int64(session.gameVersionID), rows[1].GameVersionID)
}
