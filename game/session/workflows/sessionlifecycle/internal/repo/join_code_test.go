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
		require.Nil(t, resolution.RevokedAt)
	})

	t.Run("resolves_revoked_code_with_revoked_at_set", func(t *testing.T) {
		// A revoked code still resolves - the Manager re-validates
		// admissibility under lock using RevokedAt, since a code revoked
		// concurrently by the same Session's own lazy lobby-expiration
		// materialization must surface as the ordinary LobbyExpired outcome
		// value rather than a hard "invalid code" error discovered here and
		// never re-checked under lock.
		resolution, err := r.ResolveSessionForJoinCode(context.Background(), 5252)
		require.NoError(t, err)
		require.NotNil(t, resolution)
		require.Equal(t, fx.SessionID, resolution.SessionID)
		require.NotNil(t, resolution.RevokedAt)
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
	require.Nil(t, resolution.RevokedAt)

	require.NoError(t, r.RevokeActiveJoinCode(context.Background(), db, fx.SessionID, time.Now().UTC()))

	resolution, err = r.ResolveSessionForJoinCode(context.Background(), code)
	require.NoError(t, err)
	require.NotNil(t, resolution, "a revoked join code still resolves - RevokedAt reports its status")
	require.NotNil(t, resolution.RevokedAt)
}
