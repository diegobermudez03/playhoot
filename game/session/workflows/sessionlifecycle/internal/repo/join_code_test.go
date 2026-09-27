package repo

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/stretchr/testify/require"
)

func TestRepoResolveSessionForJoinCode(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	r := New(db)

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 4242, false)
	testfixtures.SeedJoinCode(t, db, fx.SessionID, 5252, true)

	t.Run("resolves_active_code", func(t *testing.T) {
		resolution, err := r.ResolveSessionForJoinCode(context.Background(), 4242)
		require.NoError(t, err)
		require.NotNil(t, resolution)
		require.Equal(t, fx.SessionID, resolution.SessionID)
		require.Equal(t, fx.GameDefinitionUUID, resolution.GameDefinitionUUID)
	})

	t.Run("does_not_resolve_a_revoked_code", func(t *testing.T) {
		// A revoked assignment is indistinguishable from a code that never
		// existed - neither identifies an admissible lobby to join - so this
		// does not resolve, the same as returns_nil_for_unknown_code below.
		resolution, err := r.ResolveSessionForJoinCode(context.Background(), 5252)
		require.NoError(t, err)
		require.Nil(t, resolution)
	})

	t.Run("returns_nil_for_unknown_code", func(t *testing.T) {
		resolution, err := r.ResolveSessionForJoinCode(context.Background(), 9090)
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

	resolution, err := r.ResolveSessionForJoinCode(context.Background(), code)
	require.NoError(t, err)
	require.NotNil(t, resolution)
	require.Equal(t, fx.SessionID, resolution.SessionID)

	require.NoError(t, r.RevokeActiveJoinCode(context.Background(), db, fx.SessionID, time.Now().UTC()))

	resolution, err = r.ResolveSessionForJoinCode(context.Background(), code)
	require.NoError(t, err)
	require.Nil(t, resolution, "a revoked join code no longer resolves")
}
