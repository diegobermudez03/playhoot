package grantadmin

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

func TestEnsureAdmin(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	newUser := func(account bool) string {
		userUUID := uuid.NewString()
		var userID int64
		require.NoError(t, db.Raw(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, ?) RETURNING id`, userUUID, "Ana").Scan(&userID).Error)
		if account {
			require.NoError(t, db.Exec(`INSERT INTO identity_accounts (user_id) VALUES (?)`, userID).Error)
		}
		return userUUID
	}
	activeGrants := func(userUUID string) int64 {
		var count int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM identity_admin_grants g JOIN identity_users u ON u.id = g.user_id WHERE u.user_uuid = ? AND g.revoked_at IS NULL`, userUUID).Scan(&count).Error)
		return count
	}
	service := New(db)

	first := newUser(true)
	outcome, err := service.EnsureAdmin(ctx, first)
	require.NoError(t, err)
	require.Equal(t, identity.AdminGrantCreated, outcome)

	outcome, err = service.EnsureAdmin(ctx, first)
	require.NoError(t, err)
	require.Equal(t, identity.AdminGrantKept, outcome)
	require.EqualValues(t, 1, activeGrants(first))

	outcome, err = service.EnsureAdmin(ctx, newUser(false))
	require.NoError(t, err)
	require.Equal(t, identity.AdminGrantNotAccount, outcome, "a guest is never the administrator")
	outcome, err = service.EnsureAdmin(ctx, uuid.NewString())
	require.NoError(t, err)
	require.Equal(t, identity.AdminGrantNotAccount, outcome)
	require.EqualValues(t, 1, activeGrants(first))

	second := newUser(true)
	outcome, err = service.EnsureAdmin(ctx, second)
	require.NoError(t, err)
	require.Equal(t, identity.AdminGrantCreated, outcome)
	require.Zero(t, activeGrants(first), "there is one administrator")
	require.EqualValues(t, 1, activeGrants(second))
}
