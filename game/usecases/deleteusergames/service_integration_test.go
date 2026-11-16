package deleteusergames

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/game/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestDeleteUserGames(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenGameDB(t)
	service := New(db)

	newGame := func(t *testing.T, ownerUUID string) string {
		t.Helper()
		gameUUID := uuid.NewString()
		require.NoError(t, db.Exec(`
			INSERT INTO games (uuid, name, description, owner_uuid, visibility) VALUES (?, ?, '', ?, 'draft')`,
			gameUUID, "A game", ownerUUID,
		).Error)
		return gameUUID
	}

	live := func(t *testing.T, gameUUID string) bool {
		t.Helper()
		var found int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM games WHERE uuid = ? AND deleted_at IS NULL`, gameUUID).Scan(&found).Error)
		return found == 1
	}

	t.Run("every game of the deleted owner goes, and nobody else's", func(t *testing.T) {
		owner, other := uuid.NewString(), uuid.NewString()
		first, second := newGame(t, owner), newGame(t, owner)
		theirs := newGame(t, other)

		deleted, err := service.DeleteUserGames(ctx, owner)
		require.NoError(t, err)
		require.Equal(t, int64(2), deleted)

		require.False(t, live(t, first))
		require.False(t, live(t, second))
		require.True(t, live(t, theirs))
	})

	t.Run("running it again deletes nothing new", func(t *testing.T) {
		owner := uuid.NewString()
		newGame(t, owner)

		_, err := service.DeleteUserGames(ctx, owner)
		require.NoError(t, err)
		again, err := service.DeleteUserGames(ctx, owner)
		require.NoError(t, err)
		require.Zero(t, again)
	})
}
