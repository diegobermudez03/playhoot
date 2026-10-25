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

type faultRow struct {
	GameVersionID   uint
	UserUUID        string
	ParticipantID   string
	Kind            string
	Detail          string
	Occurrences     int
	FirstReportedAt time.Time
	LastReportedAt  time.Time
}

func faultsOf(t *testing.T, db *gorm.DB, sessionID uint) []faultRow {
	t.Helper()
	var rows []faultRow
	require.NoError(t, db.Raw(`
		SELECT game_version_id, user_uuid, participant_id, kind, detail::text AS detail, occurrences, first_reported_at, last_reported_at
		FROM play_game_faults WHERE session_id = ? ORDER BY kind
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

func TestRecordFault(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	ctx := context.Background()
	first := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	later := first.Add(time.Minute)

	t.Run("records_a_participant_fault_with_the_session_version", func(t *testing.T) {
		session := seedSession(t, db, "RUNNING")

		recorded, err := repo.RecordFault(ctx, session.uuid,
			Fault{UserUUID: session.playerUUID, ParticipantID: "P1", Kind: "BRIDGE_ABUSE", Detail: []byte(`{"refused":{"sample":900}}`)}, first)

		require.NoError(t, err)
		require.True(t, recorded)
		rows := faultsOf(t, db, session.id)
		require.Len(t, rows, 1)
		require.Equal(t, session.gameVersionID, rows[0].GameVersionID)
		require.Equal(t, session.playerUUID, rows[0].UserUUID)
		require.Equal(t, "P1", rows[0].ParticipantID)
		require.Equal(t, "BRIDGE_ABUSE", rows[0].Kind)
		require.Equal(t, 1, rows[0].Occurrences)
		requireJSONEqual(t, `{"refused":{"sample":900}}`, rows[0].Detail)
		require.True(t, rows[0].FirstReportedAt.Equal(first))
	})

	t.Run("the_same_kind_again_counts_it_and_keeps_the_newer_detail", func(t *testing.T) {
		session := seedSession(t, db, "RUNNING")
		fault := Fault{UserUUID: session.playerUUID, ParticipantID: "P1", Kind: "CONTAINER_VIOLATION", Detail: []byte(`{"n":1}`)}
		_, err := repo.RecordFault(ctx, session.uuid, fault, first)
		require.NoError(t, err)
		fault.Detail = []byte(`{"n":2}`)

		recorded, err := repo.RecordFault(ctx, session.uuid, fault, later)

		require.NoError(t, err)
		require.True(t, recorded)
		rows := faultsOf(t, db, session.id)
		require.Len(t, rows, 1)
		require.Equal(t, 2, rows[0].Occurrences)
		requireJSONEqual(t, `{"n":2}`, rows[0].Detail)
		require.True(t, rows[0].FirstReportedAt.Equal(first))
		require.True(t, rows[0].LastReportedAt.Equal(later))
	})

	t.Run("another_kind_is_another_record", func(t *testing.T) {
		session := seedSession(t, db, "RUNNING")
		for _, kind := range []string{"READY_TIMEOUT", "BRIDGE_ABUSE"} {
			_, err := repo.RecordFault(ctx, session.uuid, Fault{UserUUID: session.playerUUID, ParticipantID: "P1", Kind: kind, Detail: []byte(`{}`)}, first)
			require.NoError(t, err)
		}

		require.Len(t, faultsOf(t, db, session.id), 2)
	})

	t.Run("nothing_when_the_session_is_not_running", func(t *testing.T) {
		session := seedSession(t, db, "FINISHED")

		recorded, err := repo.RecordFault(ctx, session.uuid,
			Fault{UserUUID: session.playerUUID, ParticipantID: "P1", Kind: "BRIDGE_ABUSE", Detail: []byte(`{}`)}, first)

		require.NoError(t, err)
		require.False(t, recorded)
		require.Empty(t, faultsOf(t, db, session.id))
	})

	t.Run("nothing_from_someone_who_is_not_that_participant", func(t *testing.T) {
		session := seedSession(t, db, "RUNNING")

		for _, fault := range []Fault{
			{UserUUID: uuid.NewString(), ParticipantID: "P1", Kind: "BRIDGE_ABUSE", Detail: []byte(`{}`)},
			{UserUUID: session.playerUUID, ParticipantID: "P2", Kind: "BRIDGE_ABUSE", Detail: []byte(`{}`)},
		} {
			recorded, err := repo.RecordFault(ctx, session.uuid, fault, first)
			require.NoError(t, err)
			require.False(t, recorded)
		}
		require.Empty(t, faultsOf(t, db, session.id))
	})

	t.Run("nothing_for_an_unknown_session", func(t *testing.T) {
		recorded, err := repo.RecordFault(ctx, uuid.NewString(),
			Fault{UserUUID: uuid.NewString(), ParticipantID: "P1", Kind: "BRIDGE_ABUSE", Detail: []byte(`{}`)}, first)

		require.NoError(t, err)
		require.False(t, recorded)
	})
}
