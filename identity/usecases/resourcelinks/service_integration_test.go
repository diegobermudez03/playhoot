package resourcelinks

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/resourcelink"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestResourceLinks(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	newUser := func(name string) string {
		userUUID := uuid.NewString()
		require.NoError(t, db.Exec(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, ?)`, userUUID, name).Error)
		return userUUID
	}
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	service := New(db)
	service.now = func() time.Time { return now }
	gameID := uuid.NewString()

	t.Run("a link resolves to its user and resource, and only its hash is stored", func(t *testing.T) {
		owner := newUser("Ana")
		created, err := service.CreateResourceLink(ctx, owner, "game", gameID)
		require.NoError(t, err)
		require.True(t, resourcelink.WellFormed(created.Token))
		require.Equal(t, now.Add(resourcelink.Lifetime), created.ExpiresAt)

		var stored []string
		require.NoError(t, db.Raw(`SELECT token_sha256 FROM identity_resource_links l JOIN identity_users u ON u.id = l.user_id WHERE u.user_uuid = ?`, owner).Scan(&stored).Error)
		require.Equal(t, []string{resourcelink.Hash(created.Token)}, stored)

		resolved, err := service.ResolveResourceLink(ctx, created.Token)
		require.NoError(t, err)
		require.Equal(t, identity.ResolvedResourceLink{Outcome: identity.ResourceLinkValid, UserUUID: owner, Kind: "game", ResourceID: gameID, ExpiresAt: created.ExpiresAt}, resolved)

		status, err := service.ResourceLinkStatus(ctx, owner, "game", gameID)
		require.NoError(t, err)
		require.Equal(t, identity.ResourceLinkStatus{Active: true, CreatedAt: created.CreatedAt, ExpiresAt: created.ExpiresAt}, status)
	})

	t.Run("a new link replaces the open one", func(t *testing.T) {
		owner := newUser("Bea")
		first, err := service.CreateResourceLink(ctx, owner, "game", gameID)
		require.NoError(t, err)
		second, err := service.CreateResourceLink(ctx, owner, "game", gameID)
		require.NoError(t, err)

		old, err := service.ResolveResourceLink(ctx, first.Token)
		require.NoError(t, err)
		require.Equal(t, identity.ResourceLinkRevoked, old.Outcome)
		current, err := service.ResolveResourceLink(ctx, second.Token)
		require.NoError(t, err)
		require.Equal(t, identity.ResourceLinkValid, current.Outcome)

		other, err := service.CreateResourceLink(ctx, owner, "game", uuid.NewString())
		require.NoError(t, err)
		stillValid, err := service.ResolveResourceLink(ctx, second.Token)
		require.NoError(t, err)
		require.Equal(t, identity.ResourceLinkValid, stillValid.Outcome, "a link to another resource replaces nothing")
		require.NotEqual(t, second.Token, other.Token)
	})

	t.Run("revoking ends the link and revoking again does nothing", func(t *testing.T) {
		owner := newUser("Caro")
		created, err := service.CreateResourceLink(ctx, owner, "game", gameID)
		require.NoError(t, err)
		require.NoError(t, service.RevokeResourceLink(ctx, owner, "game", gameID))
		require.NoError(t, service.RevokeResourceLink(ctx, owner, "game", gameID))

		resolved, err := service.ResolveResourceLink(ctx, created.Token)
		require.NoError(t, err)
		require.Equal(t, identity.ResolvedResourceLink{Outcome: identity.ResourceLinkRevoked}, resolved)
		status, err := service.ResourceLinkStatus(ctx, owner, "game", gameID)
		require.NoError(t, err)
		require.False(t, status.Active)
	})

	t.Run("a link stops working when it expires", func(t *testing.T) {
		owner := newUser("Dani")
		created, err := service.CreateResourceLink(ctx, owner, "game", gameID)
		require.NoError(t, err)
		later := New(db)
		later.now = func() time.Time { return created.ExpiresAt }

		resolved, err := later.ResolveResourceLink(ctx, created.Token)
		require.NoError(t, err)
		require.Equal(t, identity.ResourceLinkExpired, resolved.Outcome)
		status, err := later.ResourceLinkStatus(ctx, owner, "game", gameID)
		require.NoError(t, err)
		require.False(t, status.Active)
	})

	t.Run("unknown and malformed tokens resolve to nobody", func(t *testing.T) {
		token, err := resourcelink.NewToken()
		require.NoError(t, err)
		for _, unknown := range []string{token, "not-a-token", ""} {
			resolved, err := service.ResolveResourceLink(ctx, unknown)
			require.NoError(t, err)
			require.Equal(t, identity.ResolvedResourceLink{Outcome: identity.ResourceLinkUnknown}, resolved)
		}
	})

	t.Run("a link for nobody or no resource is refused", func(t *testing.T) {
		_, err := service.CreateResourceLink(ctx, uuid.NewString(), "game", gameID)
		require.ErrorIs(t, err, identity.ErrUserNotFound)
		_, err = service.CreateResourceLink(ctx, newUser("Eva"), "game", "")
		require.Error(t, err)
	})
}
