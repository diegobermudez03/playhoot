package expireguests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
	"github.com/diegobermudez03/playhoot/identity/usecases/deleteuser"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestExpire(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	queue := txqueue.NewRegistry(db, txqueue.DefaultWorkerConfig).Queue(identity.QueueDomain)
	service := New(db, deleteuser.New(db, queue))

	// newUser makes a User created createdAgo, with one session last used
	// usedAgo (never, when usedAgo is zero).
	newUser := func(t *testing.T, account bool, createdAgo time.Duration, usedAgo time.Duration) string {
		t.Helper()
		userUUID := uuid.NewString()
		var userID int64
		require.NoError(t, db.Raw(`
			INSERT INTO identity_users (user_uuid, display_name, created_at) VALUES (?, ?, now() - ?::interval) RETURNING id`,
			userUUID, "Ana", createdAgo.String(),
		).Scan(&userID).Error)
		var lastUsed *time.Time
		if usedAgo > 0 {
			at := time.Now().UTC().Add(-usedAgo)
			lastUsed = &at
		}
		require.NoError(t, db.Exec(`
			INSERT INTO identity_sessions (session_uuid, user_id, refresh_token, issued_at, started_at, last_used_at)
			VALUES (?, ?, ?, now(), now(), ?)`,
			uuid.NewString(), userID, "phr_"+uuid.NewString(), lastUsed,
		).Error)
		if account {
			require.NoError(t, db.Exec(`INSERT INTO identity_accounts (user_id) VALUES (?)`, userID).Error)
		}
		return userUUID
	}

	deleted := func(t *testing.T, userUUID string) bool {
		t.Helper()
		var found int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM identity_users WHERE user_uuid = ? AND deleted_at IS NOT NULL`, userUUID).Scan(&found).Error)
		return found == 1
	}

	t.Run("only the guests nobody has used for the whole term expire", func(t *testing.T) {
		longGone := newUser(t, false, Term+30*24*time.Hour, Term+time.Hour)
		neverUsedAndOld := newUser(t, false, Term+time.Hour, 0)
		usedRecently := newUser(t, false, Term+30*24*time.Hour, time.Hour)
		youngGuest := newUser(t, false, time.Hour, 0)
		upgraded := newUser(t, true, Term+30*24*time.Hour, Term+time.Hour)

		expired, err := service.Expire(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, expired, int64(2))

		require.True(t, deleted(t, longGone))
		require.True(t, deleted(t, neverUsedAndOld), "a guest who never renewed is measured from their creation")
		require.False(t, deleted(t, usedRecently))
		require.False(t, deleted(t, youngGuest))
		require.False(t, deleted(t, upgraded), "an upgraded guest is an account and never expires here")

		var origin string
		require.NoError(t, db.Raw(`SELECT deleted_origin FROM identity_users WHERE user_uuid = ?`, longGone).Scan(&origin).Error)
		require.Equal(t, string(identity.DeleteExpiry), origin)

		var tasks int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM identity_queue_tasks WHERE type = ? AND idempotency_key = ?`,
			txqueue.TypeAccountCleanup, longGone).Scan(&tasks).Error)
		require.Equal(t, int64(1), tasks, "an expired guest leaves its own cleanup")
	})

	t.Run("a second sweep expires nobody again", func(t *testing.T) {
		newUser(t, false, Term+30*24*time.Hour, Term+time.Hour)
		_, err := service.Expire(ctx)
		require.NoError(t, err)

		again, err := service.Expire(ctx)
		require.NoError(t, err)
		require.Zero(t, again)
	})
}
