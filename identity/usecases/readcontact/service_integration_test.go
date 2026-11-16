package readcontact

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestReadContact(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	service := New(db)
	newUser := func(account bool, emails ...string) (string, int64) {
		userUUID := uuid.NewString()
		var userID int64
		require.NoError(t, db.Raw(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, ?) RETURNING id`, userUUID, "Ana").Scan(&userID).Error)
		if account {
			require.NoError(t, db.Exec(`INSERT INTO identity_accounts (user_id) VALUES (?)`, userID).Error)
		}
		for _, email := range emails {
			require.NoError(t, db.Exec(`INSERT INTO identity_external_identities (issuer, subject, user_id, email, name) VALUES (?, ?, ?, ?, ?)`, "local", uuid.NewString(), userID, email, "Ana").Error)
		}
		return userUUID, userID
	}
	tests := map[string]func(t *testing.T){
		"an account is reached at its latest observed address": func(t *testing.T) {
			user, _ := newUser(true, "old@x.com", "new@x.com")
			contact, err := service.ReadContact(ctx, user)
			require.NoError(t, err)
			require.Equal(t, identity.Contact{Found: true, Account: true, Email: "new@x.com"}, contact)
		},
		"an email goes out in the language the account chose": func(t *testing.T) {
			user, userID := newUser(true, "ana@x.com")
			require.NoError(t, db.Exec(`UPDATE identity_users SET interface_language = ? WHERE id = ?`, "pt", userID).Error)

			contact, err := service.ReadContact(ctx, user)
			require.NoError(t, err)
			require.Equal(t, "pt", contact.Language)
		},
		"an account that chose none leaves the language to the default": func(t *testing.T) {
			user, _ := newUser(true, "bea@x.com")

			contact, err := service.ReadContact(ctx, user)
			require.NoError(t, err)
			require.Empty(t, contact.Language)
		},
		"a guest has no address and an unknown user is not found": func(t *testing.T) {
			guest, _ := newUser(false)
			contact, err := service.ReadContact(ctx, guest)
			require.NoError(t, err)
			require.Equal(t, identity.Contact{Found: true}, contact)
			contact, err = service.ReadContact(ctx, uuid.NewString())
			require.NoError(t, err)
			require.False(t, contact.Found)
		},
		"a removed method is no longer where an account is reached": func(t *testing.T) {
			user, userID := newUser(true, "old@x.com", "new@x.com")
			require.NoError(t, db.Exec(`UPDATE identity_external_identities SET removed_at = now()
				WHERE user_id = ? AND email = ?`, userID, "new@x.com").Error)
			contact, err := service.ReadContact(ctx, user)
			require.NoError(t, err)
			require.Equal(t, "old@x.com", contact.Email)
		},
		"an email code attempt is reached at the address it was asked for": func(t *testing.T) {
			attempt := uuid.NewString()
			require.NoError(t, db.Exec(`
				INSERT INTO identity_email_codes (attempt_uuid, email, intent, code_sha256, language, expires_at, created_at)
				VALUES (?, ?, ?, ?, ?, now() + interval '10 minutes', now())`,
				attempt, "nobody@x.com", "sign-in", "a-hash", "pt").Error)
			contact, err := service.ReadContact(ctx, attempt)
			require.NoError(t, err)
			require.Equal(t, identity.Contact{Found: true, Email: "nobody@x.com", Language: "pt"}, contact)
		},
		"the administrator is the active grant": func(t *testing.T) {
			require.NoError(t, db.Exec(`UPDATE identity_admin_grants SET revoked_at = now() WHERE revoked_at IS NULL`).Error)
			_, found, err := service.AdminUserUUID(ctx)
			require.NoError(t, err)
			require.False(t, found)
			admin, adminID := newUser(true, "admin@x.com")
			require.NoError(t, db.Exec(`INSERT INTO identity_admin_grants (user_id, granted_at, note) VALUES (?, now(), ?)`, adminID, "test").Error)
			got, found, err := service.AdminUserUUID(ctx)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, admin, got)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
