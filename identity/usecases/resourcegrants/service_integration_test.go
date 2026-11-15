package resourcegrants

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/resourcegrant"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestResourceGrants(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	newUser := func() string {
		userUUID := uuid.NewString()
		require.NoError(t, db.Exec(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, ?)`, userUUID, "Ana").Error)
		return userUUID
	}
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	service := New(db)
	service.now = func() time.Time { return now }

	t.Run("a grant is claimed once, released, and claimed again", func(t *testing.T) {
		owner := newUser()
		created, err := service.CreateResourceGrant(ctx, owner, "game", "g-1", "asset_upload mcp cardBack")
		require.NoError(t, err)
		require.True(t, resourcegrant.WellFormed(created.Token))
		require.Equal(t, now.Add(resourcegrant.Lifetime), created.ExpiresAt)

		var stored int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM identity_resource_grants WHERE token_sha256 = ?`, created.Token).Scan(&stored).Error)
		require.Zero(t, stored, "only the token's hash is stored")

		claimed, err := service.ClaimResourceGrant(ctx, created.Token)
		require.NoError(t, err)
		require.Equal(t, identity.ClaimedResourceGrant{Outcome: identity.ResourceGrantClaimed, GrantUUID: created.GrantUUID, UserUUID: owner, Kind: "game", ResourceID: "g-1", Scope: "asset_upload mcp cardBack"}, claimed)

		again, err := service.ClaimResourceGrant(ctx, created.Token)
		require.NoError(t, err)
		require.Equal(t, identity.ResourceGrantUsed, again.Outcome)

		require.NoError(t, service.ReleaseResourceGrant(ctx, created.GrantUUID))
		released, err := service.ClaimResourceGrant(ctx, created.Token)
		require.NoError(t, err)
		require.Equal(t, identity.ResourceGrantClaimed, released.Outcome)
	})

	t.Run("an expired grant is not claimed", func(t *testing.T) {
		created, err := service.CreateResourceGrant(ctx, newUser(), "game", "g-2", "s")
		require.NoError(t, err)
		later := New(db)
		later.now = func() time.Time { return now.Add(resourcegrant.Lifetime) }
		claimed, err := later.ClaimResourceGrant(ctx, created.Token)
		require.NoError(t, err)
		require.Equal(t, identity.ResourceGrantExpired, claimed.Outcome)
	})

	t.Run("a user holds a bounded number of live grants", func(t *testing.T) {
		owner := newUser()
		for range resourcegrant.MaxLive {
			_, err := service.CreateResourceGrant(ctx, owner, "game", "g-3", "s")
			require.NoError(t, err)
		}
		_, err := service.CreateResourceGrant(ctx, owner, "game", "g-3", "s")
		require.ErrorIs(t, err, identity.ErrTooManyResourceGrants)
	})

	t.Run("an unknown user has no grants", func(t *testing.T) {
		_, err := service.CreateResourceGrant(ctx, uuid.NewString(), "game", "g", "s")
		require.ErrorIs(t, err, identity.ErrUserNotFound)
	})
}
