package gamelifecycle

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/business/edition"
	"github.com/diegobermudez03/playhoot/game/internal/testdb"
)

func TestPublicationIntegration(t *testing.T) {
	db := testdb.OpenGameDB(t)
	manager := New(db)
	ctx := context.Background()

	named := func(t *testing.T) (string, game.CreatedGame) {
		t.Helper()
		owner := uuid.NewString()
		created, err := manager.CreateGame(ctx, owner, "Memoria", "create")
		require.NoError(t, err)
		require.NoError(t, db.Exec(`UPDATE games SET name = ? WHERE uuid = ?`, "Memoria", created.GameUUID).Error)
		return owner, created
	}

	t.Run("a card without a name is not ready and records no release", func(t *testing.T) {
		owner := uuid.NewString()
		created, err := manager.CreateGame(ctx, owner, "Memoria", "create")
		require.NoError(t, err)
		// Unreachable for games named at creation; kept as a defense.
		require.NoError(t, db.Exec(`UPDATE games SET name = ? WHERE uuid = ?`, "", created.GameUUID).Error)
		started, err := manager.BeginPublication(ctx, owner, created.GameUUID, 1, game.Private, "publish")
		require.NoError(t, err)
		require.Equal(t, game.Publication{Outcome: game.PublicationNotReady, Blockers: []string{"name_required"}}, started)
		pending, err := manager.FindPendingPublication(ctx, owner, created.GameUUID)
		require.NoError(t, err)
		require.Nil(t, pending)
	})

	t.Run("a publication is recorded, completed once and published", func(t *testing.T) {
		owner, created := named(t)
		started, err := manager.BeginPublication(ctx, owner, created.GameUUID, 3, game.Private, "publish")
		require.NoError(t, err)
		require.Equal(t, game.PublicationStarted, started.Outcome)
		require.Equal(t, created.EditionVersionUUID, started.Release.VersionUUID)
		require.Equal(t, int64(1), started.Release.Sequence)

		retried, err := manager.BeginPublication(ctx, owner, created.GameUUID, 3, game.Private, "publish")
		require.NoError(t, err)
		require.Equal(t, started.Release.UUID, retried.Release.UUID)
		_, err = manager.BeginPublication(ctx, owner, created.GameUUID, 3, game.Public, "publish")
		require.ErrorIs(t, err, game.ErrIdempotencyConflict)
		other, err := manager.BeginPublication(ctx, owner, created.GameUUID, 3, game.Public, "another")
		require.NoError(t, err)
		require.Equal(t, game.PublicationPending, other.Outcome)
		require.Equal(t, started.Release.UUID, other.Release.UUID)

		pending, err := manager.FindPendingPublication(ctx, owner, created.GameUUID)
		require.NoError(t, err)
		require.Equal(t, started.Release.UUID, pending.UUID)

		completed, err := manager.CompletePublication(ctx, started.Release.UUID)
		require.NoError(t, err)
		require.NotNil(t, completed.CompletedAt)
		again, err := manager.CompletePublication(ctx, started.Release.UUID)
		require.NoError(t, err)
		require.Equal(t, completed.CompletedAt.UTC(), again.CompletedAt.UTC())

		var row struct {
			Visibility    string
			Published     bool
			EditionStatus string
		}
		require.NoError(t, db.Raw(`
			SELECT g.visibility, g.published_at IS NOT NULL AS published, e.status AS edition_status
			FROM games g JOIN game_editions e ON e.game_id = g.id WHERE g.uuid = ?
		`, created.GameUUID).Scan(&row).Error)
		require.Equal(t, string(game.Private), row.Visibility)
		require.True(t, row.Published)
		require.Equal(t, string(edition.Published), row.EditionStatus)

		found, err := manager.FindPublication(ctx, owner, created.GameUUID, "publish")
		require.NoError(t, err)
		require.Equal(t, game.PublicationCompleted, found.Outcome)
		none, err := manager.FindPublication(ctx, owner, created.GameUUID, "never-used")
		require.NoError(t, err)
		require.Nil(t, none)

		replayed, err := manager.BeginPublication(ctx, owner, created.GameUUID, 3, game.Private, "publish")
		require.NoError(t, err)
		require.Equal(t, game.PublicationCompleted, replayed.Outcome)
		closed, err := manager.BeginPublication(ctx, owner, created.GameUUID, 3, game.Private, "later")
		require.NoError(t, err)
		require.Equal(t, game.PublicationNotEditable, closed.Outcome)

		t.Run("a new edition opens once and starts from the published version", func(t *testing.T) {
			stranger, err := manager.OpenEdition(ctx, uuid.NewString(), created.GameUUID, "edit")
			require.NoError(t, err)
			require.Equal(t, game.EditionNotFound, stranger.Outcome)

			opened, err := manager.OpenEdition(ctx, owner, created.GameUUID, "edit")
			require.NoError(t, err)
			require.Equal(t, game.EditionOpened, opened.Outcome)
			require.Equal(t, created.EditionVersionUUID, opened.BaseVersionUUID)
			require.NotEqual(t, created.EditionVersionUUID, opened.VersionUUID)
			retried, err := manager.OpenEdition(ctx, owner, created.GameUUID, "edit")
			require.NoError(t, err)
			require.Equal(t, opened, retried)
			second, err := manager.OpenEdition(ctx, owner, created.GameUUID, "edit-again")
			require.NoError(t, err)
			require.Equal(t, game.EditionAlreadyOpen, second.Outcome)

			republished, err := manager.BeginPublication(ctx, owner, created.GameUUID, 1, game.Public, "republish")
			require.NoError(t, err)
			require.Equal(t, int64(2), republished.Release.Sequence)
			require.Equal(t, opened.VersionUUID, republished.Release.VersionUUID)
		})
	})

	t.Run("an abandoned release frees the game for another publication", func(t *testing.T) {
		owner, created := named(t)
		started, err := manager.BeginPublication(ctx, owner, created.GameUUID, 2, game.Public, "publish")
		require.NoError(t, err)
		abandoned, err := manager.AbandonPublication(ctx, started.Release.UUID, "revision_changed")
		require.NoError(t, err)
		require.NotNil(t, abandoned.AbandonedAt)
		replayed, err := manager.BeginPublication(ctx, owner, created.GameUUID, 2, game.Public, "publish")
		require.NoError(t, err)
		require.Equal(t, game.PublicationAbandoned, replayed.Outcome)
		retried, err := manager.BeginPublication(ctx, owner, created.GameUUID, 3, game.Public, "publish-again")
		require.NoError(t, err)
		require.Equal(t, game.PublicationStarted, retried.Outcome)
		require.Equal(t, int64(2), retried.Release.Sequence)
	})

	t.Run("an unpublished game opens no new edition", func(t *testing.T) {
		owner, created := named(t)
		opened, err := manager.OpenEdition(ctx, owner, created.GameUUID, "edit")
		require.NoError(t, err)
		require.Equal(t, game.EditionNotPublished, opened.Outcome)
	})
}
