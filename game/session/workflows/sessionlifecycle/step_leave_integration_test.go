package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestManagerLeave_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	m := New(db, nil, nil)

	t.Run("active_participant_leaves_and_slot_is_released", func(t *testing.T) {
		userUUID := uuid.NewString()
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		testfixtures.SeedActiveParticipant(t, db, fx.SessionID, userUUID, "User One")

		result, err := m.Leave(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(userUUID), "leave-key-1")
		require.NoError(t, err)
		require.Equal(t, session.SessionUUID(fx.SessionUUID), result.SessionUUID)

		var active bool
		require.NoError(t, db.Raw(`
			SELECT p.active FROM session_participants p
			INNER JOIN session_actors a ON a.id = p.session_actor_id
			WHERE a.user_uuid = ?
		`, userUUID).Scan(&active).Error)
		require.False(t, active)

		var actorCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_actors WHERE user_uuid = ?`, userUUID).Scan(&actorCount).Error)
		require.Equal(t, int64(1), actorCount, "SessionActor must remain durable after Leave")
	})

	t.Run("host_retains_host_authority_after_leaving", func(t *testing.T) {
		hostUUID := uuid.NewString()
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		_, err := m.Leave(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUID), "leave-key-host")
		require.NoError(t, err)

		var hostActorIDAfter *uint
		require.NoError(t, db.Raw(`SELECT host_actor_id FROM sessions WHERE uuid = ?`, fx.SessionUUID).Scan(&hostActorIDAfter).Error)
		require.NotNil(t, hostActorIDAfter)
	})

	t.Run("retried_leave_with_same_idempotency_key_is_idempotent", func(t *testing.T) {
		userUUID := uuid.NewString()
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		testfixtures.SeedActiveParticipant(t, db, fx.SessionID, userUUID, "User Two")

		_, err := m.Leave(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(userUUID), "leave-key-retry")
		require.NoError(t, err)

		result, err := m.Leave(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(userUUID), "leave-key-retry")
		require.NoError(t, err)
		require.Equal(t, session.SessionUUID(fx.SessionUUID), result.SessionUUID)
	})

	t.Run("rejects_leave_when_session_already_terminal", func(t *testing.T) {
		userUUID := uuid.NewString()
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		testfixtures.SeedActiveParticipant(t, db, fx.SessionID, userUUID, "User Three")
		require.NoError(t, db.Exec(`UPDATE sessions SET phase = 'TERMINAL' WHERE id = ?`, fx.SessionID).Error)

		result, err := m.Leave(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(userUUID), "leave-key-terminal")
		require.NoError(t, err)
		require.Equal(t, session.LeaveOutcomeNotInLobby, result.Outcome)
	})

	t.Run("lazily_materializes_expired_lobby_and_rejects", func(t *testing.T) {
		userUUID := uuid.NewString()
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(-1*time.Minute))
		testfixtures.SeedActiveParticipant(t, db, fx.SessionID, userUUID, "User Four")

		result, err := m.Leave(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(userUUID), "leave-key-expired")
		require.NoError(t, err)
		require.Equal(t, session.LeaveOutcomeNotInLobby, result.Outcome)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, fx.SessionUUID).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)
	})

	t.Run("rejects_when_actor_never_joined", func(t *testing.T) {
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))

		result, err := m.Leave(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(uuid.NewString()), "leave-key-missing")
		require.NoError(t, err)
		require.Equal(t, session.LeaveOutcomeActorNotFound, result.Outcome)
	})
}
