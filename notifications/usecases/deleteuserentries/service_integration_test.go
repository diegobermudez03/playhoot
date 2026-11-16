package deleteuserentries

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/notifications/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestDeleteUserEntries(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := New(db)

	newEntry := func(t *testing.T, userUUID string) int64 {
		t.Helper()
		var entryID int64
		require.NoError(t, db.Raw(`
			INSERT INTO notifications_entries
				(entry_uuid, user_uuid, source_domain, type, external_token, idempotency_key, skeleton, send_at,
				 in_app, email_channel, necessary, created_at)
			VALUES (?, ?, 'identity', 'test-notice', ?, ?, '{}'::jsonb, now(), true, true, true, now())
			RETURNING id`,
			uuid.NewString(), userUUID, uuid.NewString(), uuid.NewString(),
		).Scan(&entryID).Error)
		require.NoError(t, db.Exec(`
			INSERT INTO notifications_email_deliveries (entry_id, provider, state, provider_message_id, detail, closes_email, created_at)
			VALUES (?, 'resend', 'sent', ?, '', false, now())`,
			entryID, uuid.NewString(),
		).Error)
		return entryID
	}

	newPreferences := func(t *testing.T, userUUID string) {
		t.Helper()
		require.NoError(t, db.Exec(`
			INSERT INTO notifications_preferences (user_uuid, ai_email, capacity_email, review_email, marketing_email, created_at)
			VALUES (?, false, false, true, false, now())`, userUUID).Error)
	}

	count := func(t *testing.T, query string, args ...any) int64 {
		t.Helper()
		var found int64
		require.NoError(t, db.Raw(query, args...).Scan(&found).Error)
		return found
	}

	t.Run("the entries and the preferences go, the suppression stays", func(t *testing.T) {
		gone, stays := uuid.NewString(), uuid.NewString()
		goneEntry := newEntry(t, gone)
		newEntry(t, gone)
		staysEntry := newEntry(t, stays)
		newPreferences(t, gone)
		newPreferences(t, stays)
		address := uuid.NewString() + "@example.test"
		require.NoError(t, db.Exec(`
			INSERT INTO notifications_suppressions (email, reason, origin, created_at) VALUES (?, 'bounce', 'provider', now())`,
			address,
		).Error)

		deleted, err := service.DeleteUserEntries(ctx, gone)
		require.NoError(t, err)
		require.Equal(t, int64(2), deleted)

		require.Zero(t, count(t, `SELECT COUNT(*) FROM notifications_entries WHERE user_uuid = ?`, gone))
		require.Zero(t, count(t, `SELECT COUNT(*) FROM notifications_preferences WHERE user_uuid = ?`, gone))
		require.Zero(t, count(t, `SELECT COUNT(*) FROM notifications_email_deliveries WHERE entry_id = ?`, goneEntry))

		require.Equal(t, int64(1), count(t, `SELECT COUNT(*) FROM notifications_entries WHERE user_uuid = ?`, stays))
		require.Equal(t, int64(1), count(t, `SELECT COUNT(*) FROM notifications_preferences WHERE user_uuid = ?`, stays))
		require.Equal(t, int64(1), count(t, `SELECT COUNT(*) FROM notifications_email_deliveries WHERE entry_id = ?`, staysEntry))
		require.Equal(t, int64(1), count(t, `SELECT COUNT(*) FROM notifications_suppressions WHERE email = ?`, address),
			"a suppression protects an address, not an account")
	})

	t.Run("running it again deletes nothing new", func(t *testing.T) {
		gone := uuid.NewString()
		newEntry(t, gone)

		_, err := service.DeleteUserEntries(ctx, gone)
		require.NoError(t, err)
		again, err := service.DeleteUserEntries(ctx, gone)
		require.NoError(t, err)
		require.Zero(t, again)
	})
}
