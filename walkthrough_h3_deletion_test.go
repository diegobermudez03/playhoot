package main

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game/usecases/deleteusergames"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/usecases/deleteuser"
	"github.com/diegobermudez03/playhoot/identity/usecases/refreshcredentials"
	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/notifications/emailproviders"
	"github.com/diegobermudez03/playhoot/notifications/usecases/deleteuserentries"
	"github.com/diegobermudez03/playhoot/orchestrator/accountcleanup"
	"github.com/diegobermudez03/playhoot/play/session/usecases/anonymizeuser"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// TestH3Deletion walks one person out of Playhoot: they ask to be deleted,
// access ends immediately, the queued cleanup clears what names them across
// the domains, and the game they played with somebody else still reads with
// that person's part of it intact.
func TestH3Deletion(t *testing.T) {
	ctx := context.Background()
	db := walkthroughDB.Open(t)
	s := wireH1(t, db, emailproviders.NewResend(walkthroughWebhook), emailproviders.NewSES(walkthroughWebhook))

	deletion := deleteuser.New(db, s.identityQ)
	accountcleanup.New(deleteusergames.New(db), anonymizeuser.New(db), deleteuserentries.New(db)).Register(s.identityQ)

	leaving := s.signIn(t, "Ana")
	staying := s.signIn(t, "Bea")

	// What the person leaves behind: a game of their own, a shared game's
	// roster, and the notices they were sent.
	leavingGame := seedGame(t, db, leaving)
	stayingGame := seedGame(t, db, staying)
	sessionID, sessionUUID := seedFinishedSession(t, db, staying)
	seedParticipant(t, db, sessionID, leaving, "P1", "Ana")
	seedParticipant(t, db, sessionID, staying, "P2", "Bea")
	s.notify(t, leaving, notifications.TypeCapacityWarning, uuid.NewString())
	s.notify(t, staying, notifications.TypeCapacityWarning, uuid.NewString())
	s.drain(t)
	suppressed := uuid.NewString() + "@example.test"
	require.NoError(t, db.Exec(`
		INSERT INTO notifications_suppressions (email, reason, origin, created_at) VALUES (?, 'bounce', 'provider', now())`,
		suppressed,
	).Error)

	refreshToken := activeRefreshToken(t, db, leaving)
	refreshes, err := refreshcredentials.New(db, walkthroughAccessKey)
	require.NoError(t, err)

	t.Run("deleting withdraws access before the call returns", func(t *testing.T) {
		outcome, err := deletion.DeleteUser(ctx, leaving, identity.DeleteRequested)
		require.NoError(t, err)
		require.Equal(t, identity.UserDeleted, outcome)

		renewed, err := refreshes.Refresh(ctx, refreshToken)
		require.NoError(t, err)
		require.Equal(t, identity.RefreshInvalid, renewed.Outcome, "the credential stopped working with the deletion")
	})

	t.Run("the queued cleanup clears the other domains", func(t *testing.T) {
		s.drain(t)

		require.Zero(t, countOf(t, db, `SELECT COUNT(*) FROM games WHERE uuid = ? AND deleted_at IS NULL`, leavingGame))
		require.Equal(t, int64(1), countOf(t, db, `SELECT COUNT(*) FROM games WHERE uuid = ? AND deleted_at IS NULL`, stayingGame),
			"nobody else's games are touched")

		require.Zero(t, countOf(t, db, `SELECT COUNT(*) FROM notifications_entries WHERE user_uuid = ?`, leaving))
		require.Equal(t, int64(1), countOf(t, db, `SELECT COUNT(*) FROM notifications_entries WHERE user_uuid = ?`, staying))
		require.Equal(t, int64(1), countOf(t, db, `SELECT COUNT(*) FROM notifications_suppressions WHERE email = ?`, suppressed),
			"a suppression protects an address and outlives the account")

		require.Zero(t, countOf(t, db, `SELECT COUNT(*) FROM identity_queue_tasks WHERE type = ?`, txqueue.TypeAccountCleanup),
			"a completed cleanup is deleted, not kept")
	})

	t.Run("the shared game still reads with the deleted person in it", func(t *testing.T) {
		require.Equal(t, int64(1), countOf(t, db, `SELECT COUNT(*) FROM play_sessions WHERE uuid = ?`, sessionUUID))

		var roster []struct {
			ParticipantID string
			DisplayName   string
		}
		require.NoError(t, db.Raw(`
			SELECT participant_id, display_name FROM play_session_participants WHERE session_id = ? ORDER BY participant_id`,
			sessionID,
		).Scan(&roster).Error)
		require.Len(t, roster, 2, "the deleted person keeps their place in the roster")
		require.Equal(t, "P1", roster[0].ParticipantID)
		require.Empty(t, roster[0].DisplayName, "their name is gone; readers show the neutral reference")
		require.Equal(t, "Bea", roster[1].DisplayName)
	})

	t.Run("compliance evidence is the exception and stays", func(t *testing.T) {
		require.Equal(t, int64(1), countOf(t, db, `SELECT COUNT(*) FROM identity_users WHERE user_uuid = ? AND deleted_at IS NOT NULL`, leaving),
			"the uuid keeps resolving, to nobody in particular")
	})

	t.Run("deleting again changes nothing", func(t *testing.T) {
		outcome, err := deletion.DeleteUser(ctx, leaving, identity.DeleteRequested)
		require.NoError(t, err)
		require.Equal(t, identity.UserAlreadyDeleted, outcome)
		s.drain(t)
	})
}

func countOf(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var found int64
	require.NoError(t, db.Raw(query, args...).Scan(&found).Error)
	return found
}

func seedGame(t *testing.T, db *gorm.DB, ownerUUID string) string {
	t.Helper()
	gameUUID := uuid.NewString()
	require.NoError(t, db.Exec(`
		INSERT INTO games (uuid, name, description, owner_uuid, visibility) VALUES (?, 'A game', '', ?, 'draft')`,
		gameUUID, ownerUUID,
	).Error)
	return gameUUID
}

func seedFinishedSession(t *testing.T, db *gorm.DB, creatorUUID string) (uint, string) {
	t.Helper()
	sessionUUID := uuid.NewString()
	var sessionID uint
	require.NoError(t, db.Raw(`
		INSERT INTO play_sessions (uuid, game_version_id, created_by_user_uuid, status, lobby_expires_at, expires_at)
		VALUES (?, ?, ?, 'FINISHED', now(), now() + interval '1 hour') RETURNING id`,
		sessionUUID, uint(time.Now().UnixNano()%1_000_000), creatorUUID,
	).Scan(&sessionID).Error)
	return sessionID, sessionUUID
}

func seedParticipant(t *testing.T, db *gorm.DB, sessionID uint, userUUID string, participantID string, name string) {
	t.Helper()
	require.NoError(t, db.Exec(`
		INSERT INTO play_session_participants (session_id, user_uuid, participant_id, display_name) VALUES (?, ?, ?, ?)`,
		sessionID, userUUID, participantID, name,
	).Error)
}

// activeRefreshToken is the token the User's live session renews with.
func activeRefreshToken(t *testing.T, db *gorm.DB, userUUID string) string {
	t.Helper()
	var token string
	require.NoError(t, db.Raw(`
		SELECT session.refresh_token FROM identity_sessions session
		JOIN identity_users users ON users.id = session.user_id
		WHERE users.user_uuid = ? AND session.deactivated_at IS NULL`,
		userUUID,
	).Scan(&token).Error)
	require.NotEmpty(t, token)
	return token
}
