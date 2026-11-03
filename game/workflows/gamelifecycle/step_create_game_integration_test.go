package gamelifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/testdb"
)

// Integration tests run CreateGame end to end against the shared disposable
// database. Every case uses its own random owner.

type persistedGame struct {
	ID          uint
	OwnerUUID   string
	Name        string
	Visibility  string
	PublishedAt *time.Time
}

type persistedEdition struct {
	ID     uint
	UUID   string
	Status string
}

func TestCreateGameIntegration(t *testing.T) {
	db := testdb.OpenGameDB(t)
	manager := New(db)
	ctx := context.Background()

	readGame := func(t *testing.T, gameUUID string) persistedGame {
		t.Helper()
		var row persistedGame
		require.NoError(t, db.Raw(`SELECT id, owner_uuid, name, visibility, published_at FROM games WHERE uuid = ?`, gameUUID).Scan(&row).Error)
		return row
	}
	readEditions := func(t *testing.T, gameID uint) []persistedEdition {
		t.Helper()
		var rows []persistedEdition
		require.NoError(t, db.Raw(`SELECT id, uuid, status FROM game_editions WHERE game_id = ? ORDER BY id`, gameID).Scan(&rows).Error)
		return rows
	}

	t.Run("the name is required, and a key keeps the name it was used with", func(t *testing.T) {
		owner := uuid.NewString()
		_, err := manager.CreateGame(ctx, owner, "   ", "key-1")
		require.ErrorIs(t, err, game.ErrInvalidCard)
		_, err = manager.CreateGame(ctx, owner, "Memoria", "key-1")
		require.NoError(t, err)
		_, err = manager.CreateGame(ctx, owner, "Otro", "key-1")
		require.ErrorIs(t, err, game.ErrIdempotencyConflict)
	})

	t.Run("creates_an_unpublished_private_game_with_its_open_edition", func(t *testing.T) {
		owner := uuid.NewString()

		created, err := manager.CreateGame(ctx, owner, "Memoria", "key-1")

		require.NoError(t, err)
		persisted := readGame(t, created.GameUUID)
		require.Equal(t, owner, persisted.OwnerUUID)
		require.Equal(t, "Memoria", persisted.Name)
		require.Equal(t, string(game.Private), persisted.Visibility)
		require.Nil(t, persisted.PublishedAt)
		editions := readEditions(t, persisted.ID)
		require.Equal(t, []persistedEdition{{ID: editions[0].ID, UUID: created.EditionVersionUUID, Status: "EDITING"}}, editions)
		var changes []string
		require.NoError(t, db.Raw(`SELECT to_status FROM game_edition_status_changes WHERE edition_id = ? AND from_status IS NULL`, editions[0].ID).Scan(&changes).Error)
		require.Equal(t, []string{"EDITING"}, changes)
		var published []bool
		require.NoError(t, db.Raw(`SELECT is_published FROM game_histories WHERE game_id = ?`, persisted.ID).Scan(&published).Error)
		require.Equal(t, []bool{false}, published)
	})

	t.Run("a_retry_answers_the_same_game_and_a_new_key_another", func(t *testing.T) {
		owner := uuid.NewString()
		first, err := manager.CreateGame(ctx, owner, "Memoria", "key-1")
		require.NoError(t, err)

		retried, err := manager.CreateGame(ctx, owner, "Memoria", "key-1")
		require.NoError(t, err)
		other, err := manager.CreateGame(ctx, owner, "Memoria", "key-2")
		require.NoError(t, err)

		require.Equal(t, first, retried)
		require.NotEqual(t, first.GameUUID, other.GameUUID)
		require.NotEqual(t, first.EditionVersionUUID, other.EditionVersionUUID)
	})

	t.Run("concurrent_requests_with_one_key_create_one_game", func(t *testing.T) {
		owner := uuid.NewString()
		const requests = 5
		results := make([]game.CreatedGame, requests)
		errs := make([]error, requests)
		var wg sync.WaitGroup
		for i := range requests {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i], errs[i] = manager.CreateGame(ctx, owner, "Memoria", "same-key")
			}()
		}
		wg.Wait()

		for i := range requests {
			require.NoError(t, errs[i])
			require.Equal(t, results[0], results[i])
		}
		var count int
		require.NoError(t, db.Raw(`SELECT count(*) FROM games WHERE owner_uuid = ?`, owner).Scan(&count).Error)
		require.Equal(t, 1, count)
	})
}
