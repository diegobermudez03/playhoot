package signinmethods

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

// account creates an account with one method per issuer and subject given.
func account(t *testing.T, db *gorm.DB, methods ...identity.Provider) string {
	t.Helper()
	userUUID := uuid.NewString()
	var userID int64
	require.NoError(t, db.Raw(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, ?) RETURNING id`, userUUID, "Ana").Scan(&userID).Error)
	require.NoError(t, db.Exec(`INSERT INTO identity_accounts (user_id) VALUES (?)`, userID).Error)
	for _, issuer := range methods {
		subject := uuid.NewString()
		if issuer == identity.ProviderEmail {
			subject += "@example.com"
		}
		require.NoError(t, db.Exec(`
			INSERT INTO identity_external_identities (issuer, subject, user_id, email, name, created_at)
			VALUES (?, ?, ?, ?, ?, now())`,
			string(issuer), subject, userID, subject, "Ana").Error)
	}
	return userUUID
}

func TestSignInMethods(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	service := New(db)

	tests := map[string]func(t *testing.T){
		"the methods are listed oldest first": func(t *testing.T) {
			user := account(t, db, identity.ProviderGoogle, identity.ProviderEmail)
			methods, err := service.ListMethods(ctx, user)
			require.NoError(t, err)
			require.Len(t, methods, 2)
			require.Equal(t, identity.ProviderGoogle, methods[0].Issuer)
			require.Equal(t, identity.ProviderEmail, methods[1].Issuer)
			require.Contains(t, methods[1].Address, "@example.com")
			require.NotEmpty(t, methods[0].MethodID)
			require.False(t, methods[0].AddedAt.IsZero())
		},
		"one of several methods is removed and stops being listed": func(t *testing.T) {
			user := account(t, db, identity.ProviderGoogle, identity.ProviderEmail)
			methods, err := service.ListMethods(ctx, user)
			require.NoError(t, err)
			outcome, err := service.RemoveMethod(ctx, user, methods[0].MethodID)
			require.NoError(t, err)
			require.Equal(t, identity.MethodRemoved, outcome)
			left, err := service.ListMethods(ctx, user)
			require.NoError(t, err)
			require.Len(t, left, 1)
			require.Equal(t, methods[1].MethodID, left[0].MethodID)
		},
		"the last method cannot be removed": func(t *testing.T) {
			user := account(t, db, identity.ProviderGoogle)
			methods, err := service.ListMethods(ctx, user)
			require.NoError(t, err)
			outcome, err := service.RemoveMethod(ctx, user, methods[0].MethodID)
			require.NoError(t, err)
			require.Equal(t, identity.MethodLastOne, outcome)
			left, err := service.ListMethods(ctx, user)
			require.NoError(t, err)
			require.Len(t, left, 1)
		},
		"removing the same method twice changes nothing": func(t *testing.T) {
			user := account(t, db, identity.ProviderGoogle, identity.ProviderEmail)
			methods, err := service.ListMethods(ctx, user)
			require.NoError(t, err)
			_, err = service.RemoveMethod(ctx, user, methods[0].MethodID)
			require.NoError(t, err)
			again, err := service.RemoveMethod(ctx, user, methods[0].MethodID)
			require.NoError(t, err)
			require.Equal(t, identity.MethodUnknown, again)
		},
		"another account's method is not the caller's to remove": func(t *testing.T) {
			mine := account(t, db, identity.ProviderGoogle, identity.ProviderEmail)
			theirs := account(t, db, identity.ProviderGoogle, identity.ProviderEmail)
			methods, err := service.ListMethods(ctx, theirs)
			require.NoError(t, err)
			outcome, err := service.RemoveMethod(ctx, mine, methods[0].MethodID)
			require.NoError(t, err)
			require.Equal(t, identity.MethodUnknown, outcome)
		},
		"an identifier that is not one is unknown": func(t *testing.T) {
			user := account(t, db, identity.ProviderGoogle, identity.ProviderEmail)
			outcome, err := service.RemoveMethod(ctx, user, "not-a-method")
			require.NoError(t, err)
			require.Equal(t, identity.MethodUnknown, outcome)
		},
		"a removed address can be linked again": func(t *testing.T) {
			user := account(t, db, identity.ProviderGoogle, identity.ProviderEmail)
			methods, err := service.ListMethods(ctx, user)
			require.NoError(t, err)
			removed := methods[1]
			_, err = service.RemoveMethod(ctx, user, removed.MethodID)
			require.NoError(t, err)
			other := account(t, db, identity.ProviderGoogle)
			var otherID int64
			require.NoError(t, db.Raw(`SELECT id FROM identity_users WHERE user_uuid = ?`, other).Scan(&otherID).Error)
			require.NoError(t, db.Exec(`
				INSERT INTO identity_external_identities (issuer, subject, user_id, email, name, created_at)
				VALUES (?, ?, ?, ?, ?, now())`,
				string(identity.ProviderEmail), removed.Address, otherID, removed.Address, "Ana").Error)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
