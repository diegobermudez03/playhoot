package deleteuser

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/setting"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

// person is a User as these tests build them, holding everything a deletion
// has to reach.
type person struct {
	userUUID string
	userID   int64
}

func TestDeleteUser(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	queue := txqueue.NewRegistry(db, txqueue.DefaultWorkerConfig).Queue(identity.QueueDomain)
	service := New(db, queue)

	newPerson := func(t *testing.T, account bool) person {
		t.Helper()
		userUUID := uuid.NewString()
		var userID int64
		require.NoError(t, db.Raw(`
			INSERT INTO identity_users (user_uuid, display_name, interface_language) VALUES (?, ?, ?) RETURNING id`,
			userUUID, "Ana", string(identity.LanguageSpanish),
		).Scan(&userID).Error)
		require.NoError(t, db.Exec(`
			INSERT INTO identity_sessions (session_uuid, user_id, refresh_token, issued_at, started_at)
			VALUES (?, ?, ?, now(), now())`,
			uuid.NewString(), userID, "phr_"+uuid.NewString(),
		).Error)
		require.NoError(t, db.Exec(`
			INSERT INTO identity_resource_links (user_id, resource_kind, resource_id, token_sha256, created_at, expires_at)
			VALUES (?, 'game', ?, ?, now(), now() + interval '1 day')`,
			userID, uuid.NewString(), uuid.NewString(),
		).Error)
		require.NoError(t, db.Exec(`
			INSERT INTO identity_resource_grants (uuid, user_id, resource_kind, resource_id, scope, token_sha256, created_at, expires_at)
			VALUES (?, ?, 'game', ?, 'read', ?, now(), now() + interval '1 day')`,
			uuid.NewString(), userID, uuid.NewString(), uuid.NewString(),
		).Error)
		if account {
			require.NoError(t, db.Exec(`INSERT INTO identity_accounts (user_id, mode) VALUES (?, ?)`, userID, string(identity.ModeTeacher)).Error)
			require.NoError(t, db.Exec(`
				INSERT INTO identity_external_identities (issuer, subject, user_id, email, name)
				VALUES (?, ?, ?, ?, ?)`,
				string(identity.ProviderLocal), uuid.NewString(), userID, "ana@example.test", "Ana",
			).Error)
		} else {
			require.NoError(t, db.Exec(`
				INSERT INTO identity_guest_requests (idempotency_key, display_name, user_uuid, session_uuid, requested_at)
				VALUES (?, ?, ?, ?, now())`,
				uuid.NewString(), "Ana", userUUID, uuid.NewString(),
			).Error)
		}
		return person{userUUID: userUUID, userID: userID}
	}

	count := func(t *testing.T, query string, args ...any) int64 {
		t.Helper()
		var found int64
		require.NoError(t, db.Raw(query, args...).Scan(&found).Error)
		return found
	}

	tests := map[string]func(t *testing.T){
		"deleting an account ends its sessions, its methods and everything it holds": func(t *testing.T) {
			who := newPerson(t, true)

			outcome, err := service.DeleteUser(ctx, who.userUUID, identity.DeleteRequested)
			require.NoError(t, err)
			require.Equal(t, identity.UserDeleted, outcome)

			require.Zero(t, count(t, `SELECT COUNT(*) FROM identity_sessions WHERE user_id = ? AND deactivated_at IS NULL`, who.userID),
				"no session of a deleted account may still renew")
			require.Zero(t, count(t, `SELECT COUNT(*) FROM identity_external_identities WHERE user_id = ? AND removed_at IS NULL`, who.userID))
			require.Zero(t, count(t, `SELECT COUNT(*) FROM identity_external_identities
				WHERE user_id = ? AND (email IS NOT NULL OR name IS NOT NULL OR subject <> '')`, who.userID),
				"a removed method keeps no address and no name")
			require.Zero(t, count(t, `SELECT COUNT(*) FROM identity_resource_links WHERE user_id = ? AND revoked_at IS NULL`, who.userID))
			require.Zero(t, count(t, `SELECT COUNT(*) FROM identity_resource_grants WHERE user_id = ?`, who.userID))

			var row struct {
				DisplayName   string
				Language      string
				DeletedOrigin string
				DeletedAt     *time.Time
			}
			require.NoError(t, db.Raw(`
				SELECT display_name, interface_language AS language, deleted_origin, deleted_at
				FROM identity_users WHERE id = ?`, who.userID).Scan(&row).Error)
			require.Empty(t, row.DisplayName)
			require.Empty(t, row.Language)
			require.Equal(t, string(identity.DeleteRequested), row.DeletedOrigin)
			require.NotNil(t, row.DeletedAt)

			require.Equal(t, int64(1), count(t, `SELECT COUNT(*) FROM identity_accounts WHERE user_id = ?`, who.userID),
				"the account row stays: it is what says this User was never a guest")
			var mode string
			require.NoError(t, db.Raw(`SELECT mode FROM identity_accounts WHERE user_id = ?`, who.userID).Scan(&mode).Error)
			require.Equal(t, string(identity.ModeNormal), mode)

			require.Equal(t, int64(1), count(t, `SELECT COUNT(*) FROM identity_display_name_changes WHERE user_id = ? AND to_name = ''`, who.userID))
			require.Equal(t, int64(1), count(t, `SELECT COUNT(*) FROM identity_setting_changes WHERE user_id = ? AND setting = ?`, who.userID, setting.Language))
			require.Equal(t, int64(1), count(t, `SELECT COUNT(*) FROM identity_setting_changes WHERE user_id = ? AND setting = ?`, who.userID, setting.Mode))

			require.Equal(t, int64(1), count(t, `SELECT COUNT(*) FROM identity_queue_tasks WHERE type = ? AND idempotency_key = ?`,
				txqueue.TypeAccountCleanup, who.userUUID), "the cleanup of the other domains is queued")
		},
		"deleting a guest clears the name kept with its creation request": func(t *testing.T) {
			who := newPerson(t, false)

			outcome, err := service.DeleteUser(ctx, who.userUUID, identity.DeleteRequested)
			require.NoError(t, err)
			require.Equal(t, identity.UserDeleted, outcome)

			require.Zero(t, count(t, `SELECT COUNT(*) FROM identity_guest_requests WHERE user_uuid = ? AND display_name <> ''`, who.userUUID))
			require.Zero(t, count(t, `SELECT COUNT(*) FROM identity_sessions WHERE user_id = ? AND deactivated_at IS NULL`, who.userID))
			require.Zero(t, count(t, `SELECT COUNT(*) FROM identity_accounts WHERE user_id = ?`, who.userID))
		},
		"deleting twice changes nothing and leaves one cleanup": func(t *testing.T) {
			who := newPerson(t, true)

			first, err := service.DeleteUser(ctx, who.userUUID, identity.DeleteRequested)
			require.NoError(t, err)
			require.Equal(t, identity.UserDeleted, first)
			var deletedAt time.Time
			require.NoError(t, db.Raw(`SELECT deleted_at FROM identity_users WHERE id = ?`, who.userID).Scan(&deletedAt).Error)

			second, err := service.DeleteUser(ctx, who.userUUID, identity.DeleteExpiry)
			require.NoError(t, err)
			require.Equal(t, identity.UserAlreadyDeleted, second)

			var after time.Time
			require.NoError(t, db.Raw(`SELECT deleted_at FROM identity_users WHERE id = ?`, who.userID).Scan(&after).Error)
			require.Equal(t, deletedAt.UTC(), after.UTC(), "the moment of deletion is written once")
			require.Equal(t, int64(1), count(t, `SELECT COUNT(*) FROM identity_queue_tasks WHERE type = ? AND idempotency_key = ?`,
				txqueue.TypeAccountCleanup, who.userUUID))
			require.Equal(t, int64(2), count(t, `SELECT COUNT(*) FROM identity_setting_changes WHERE user_id = ?`, who.userID),
				"the second deletion records no further change")
		},
		"a User nobody has is not deleted": func(t *testing.T) {
			_, err := service.DeleteUser(ctx, uuid.NewString(), identity.DeleteRequested)
			require.ErrorIs(t, err, identity.ErrUserNotFound)
		},
	}

	for name, run := range tests {
		t.Run(name, func(t *testing.T) { run(t) })
	}
}
