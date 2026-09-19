package repo

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/stretchr/testify/require"
)

func TestRepoResolveActiveSessionForJoinCode(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	r := New(db)

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 4242, false)
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 5252, true)

	t.Run("resolves_active_code", func(t *testing.T) {
		resolution, err := r.ResolveActiveSessionForJoinCode(context.Background(), 4242)
		require.NoError(t, err)
		require.NotNil(t, resolution)
		require.Equal(t, fx.SessionID, resolution.SessionID)
		require.Equal(t, fx.GameDefinitionUUID, resolution.GameDefinitionUUID)
	})

	t.Run("returns_nil_for_revoked_code", func(t *testing.T) {
		resolution, err := r.ResolveActiveSessionForJoinCode(context.Background(), 5252)
		require.NoError(t, err)
		require.Nil(t, resolution)
	})

	t.Run("returns_nil_for_unknown_code", func(t *testing.T) {
		resolution, err := r.ResolveActiveSessionForJoinCode(context.Background(), 9090)
		require.NoError(t, err)
		require.Nil(t, resolution)
	})
}

func TestRepoCreateAndRevokeJoinCode(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	r := New(db)
	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))

	code, err := r.CreateJoinCode(context.Background(), db, fx.SessionID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, code, uint(1000))
	require.LessOrEqual(t, code, uint(9999))

	resolution, err := r.ResolveActiveSessionForJoinCode(context.Background(), code)
	require.NoError(t, err)
	require.NotNil(t, resolution)
	require.Equal(t, fx.SessionID, resolution.SessionID)

	require.NoError(t, r.RevokeActiveJoinCode(context.Background(), db, fx.SessionID, time.Now().UTC()))

	resolution, err = r.ResolveActiveSessionForJoinCode(context.Background(), code)
	require.NoError(t, err)
	require.Nil(t, resolution, "a revoked join code must no longer resolve")
}
