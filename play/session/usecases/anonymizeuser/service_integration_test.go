package anonymizeuser

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

type roster struct {
	sessionID   uint
	sessionUUID string
}

func TestAnonymizeUser(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenSessionDB(t)
	service := New(db)

	newSession := func(t *testing.T) roster {
		t.Helper()
		seeded := roster{sessionUUID: uuid.NewString()}
		require.NoError(t, db.Raw(`
			INSERT INTO play_sessions (uuid, game_version_id, created_by_user_uuid, status, lobby_expires_at, expires_at)
			VALUES (?, ?, ?, 'FINISHED', NOW(), NOW() + INTERVAL '1 hour') RETURNING id`,
			seeded.sessionUUID, uint(time.Now().UnixNano()%1_000_000), uuid.NewString(),
		).Scan(&seeded.sessionID).Error)
		return seeded
	}

	join := func(t *testing.T, seeded roster, userUUID string, participantID string, name string) {
		t.Helper()
		require.NoError(t, db.Exec(`
			INSERT INTO play_session_participants (session_id, user_uuid, participant_id, display_name) VALUES (?, ?, ?, ?)`,
			seeded.sessionID, userUUID, participantID, name,
		).Error)
	}

	nameOf := func(t *testing.T, seeded roster, participantID string) string {
		t.Helper()
		var name string
		require.NoError(t, db.Raw(`
			SELECT display_name FROM play_session_participants WHERE session_id = ? AND participant_id = ?`,
			seeded.sessionID, participantID,
		).Scan(&name).Error)
		return name
	}

	t.Run("the names of the deleted person go and the rosters stay readable", func(t *testing.T) {
		seeded := newSession(t)
		gone, alsoGone, stays := uuid.NewString(), uuid.NewString(), uuid.NewString()
		join(t, seeded, gone, "P1", "Ana")
		join(t, seeded, alsoGone, "P2", "Bea")
		join(t, seeded, stays, "P3", "Cris")

		cleared, err := service.AnonymizeUser(ctx, gone)
		require.NoError(t, err)
		require.Equal(t, int64(1), cleared)
		cleared, err = service.AnonymizeUser(ctx, alsoGone)
		require.NoError(t, err)
		require.Equal(t, int64(1), cleared)

		require.Empty(t, nameOf(t, seeded, "P1"))
		require.Empty(t, nameOf(t, seeded, "P2"))
		require.Equal(t, "Cris", nameOf(t, seeded, "P3"), "nobody else's name is touched")

		var rows int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM play_session_participants WHERE session_id = ?`, seeded.sessionID).Scan(&rows).Error)
		require.Equal(t, int64(3), rows, "the rows stay: two deleted players are still told apart by participant id")

		var participants []string
		require.NoError(t, db.Raw(`
			SELECT participant_id FROM play_session_participants WHERE session_id = ? AND display_name = '' ORDER BY participant_id`,
			seeded.sessionID,
		).Scan(&participants).Error)
		require.Equal(t, []string{"P1", "P2"}, participants)

		var sessions int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM play_sessions WHERE uuid = ?`, seeded.sessionUUID).Scan(&sessions).Error)
		require.Equal(t, int64(1), sessions, "the Session itself still reads")
	})

	t.Run("running it again clears nothing new", func(t *testing.T) {
		seeded := newSession(t)
		gone := uuid.NewString()
		join(t, seeded, gone, "P1", "Ana")

		_, err := service.AnonymizeUser(ctx, gone)
		require.NoError(t, err)
		again, err := service.AnonymizeUser(ctx, gone)
		require.NoError(t, err)
		require.Zero(t, again)
	})
}
