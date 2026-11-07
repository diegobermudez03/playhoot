package repo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/game/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestCreateGameRequests(t *testing.T) {
	db := testdb.OpenGameDB(t)
	repo := New(db)
	ctx := context.Background()

	t.Run("claim_then_complete_then_read_back", func(t *testing.T) {
		owner := uuid.NewString()

		requestID, existing, err := repo.ClaimCreateGameRequest(ctx, db, owner, "key-1", "Memoria", "")
		require.NoError(t, err)
		require.NotZero(t, requestID)
		require.Nil(t, existing)
		require.NoError(t, repo.CompleteCreateGameRequest(ctx, db, requestID, 77))

		againID, again, err := repo.ClaimCreateGameRequest(ctx, db, owner, "key-1", "Memoria", "")
		require.NoError(t, err)
		require.Zero(t, againID)
		gameID := uint(77)
		name := "Memoria"
		require.Equal(t, &CreateGameRequest{ID: requestID, GameID: &gameID, Name: &name}, again)
	})

	t.Run("a_completed_request_is_never_completed_again", func(t *testing.T) {
		requestID, _, err := repo.ClaimCreateGameRequest(ctx, db, uuid.NewString(), "key-1", "Memoria", "")
		require.NoError(t, err)
		require.NoError(t, repo.CompleteCreateGameRequest(ctx, db, requestID, 1))

		require.Error(t, repo.CompleteCreateGameRequest(ctx, db, requestID, 2))
	})
}

func TestCreateGameAndEdition(t *testing.T) {
	db := testdb.OpenGameDB(t)
	repo := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	newGame := func(t *testing.T) (uint, NewGame) {
		t.Helper()
		game := NewGame{UUID: uuid.NewString(), OwnerUUID: uuid.NewString(), Visibility: "private", CreatedAt: now}
		id, err := repo.CreateGame(ctx, db, game)
		require.NoError(t, err)
		return id, game
	}

	t.Run("creates_the_game_and_its_first_edition", func(t *testing.T) {
		gameID, game := newGame(t)
		editionUUID := uuid.NewString()

		require.NoError(t, repo.CreateEdition(ctx, db, gameID, editionUUID, "EDITING", nil, now))

		created, err := repo.FindCreatedGame(ctx, db, gameID)
		require.NoError(t, err)
		require.Equal(t, &CreatedGame{GameUUID: game.UUID, EditionVersionUUID: editionUUID, CreatedAt: now}, created)
		var history struct {
			Name        string
			Visibility  string
			IsPublished bool
		}
		require.NoError(t, db.Raw(`SELECT name, visibility, is_published FROM game_histories WHERE game_id = ?`, gameID).Scan(&history).Error)
		require.Equal(t, "private", history.Visibility)
		require.False(t, history.IsPublished)
	})

	t.Run("a_game_has_at_most_one_editing_edition", func(t *testing.T) {
		gameID, _ := newGame(t)
		require.NoError(t, repo.CreateEdition(ctx, db, gameID, uuid.NewString(), "EDITING", nil, now))

		require.Error(t, repo.CreateEdition(ctx, db, gameID, uuid.NewString(), "EDITING", nil, now))
		require.NoError(t, repo.CreateEdition(ctx, db, gameID, uuid.NewString(), "PUBLISHED", nil, now))
	})

	t.Run("an_unknown_game_is_not_found", func(t *testing.T) {
		created, err := repo.FindCreatedGame(ctx, db, 987654321)
		require.NoError(t, err)
		require.Nil(t, created)
	})
}
