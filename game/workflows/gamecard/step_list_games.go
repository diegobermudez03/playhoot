package gamecard

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/game/internal/business/edition"
	internalrepo "github.com/diegobermudez03/playhoot/game/workflows/gamecard/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
)

//go:generate mockgen -package=gamecard -destination=step_list_games_mock_test.go . listGamesRepo

// listGamesRepo is ListGames' own narrow persistence contract.
type listGamesRepo interface {
	ListOwnedGames(ctx context.Context, tx *gorm.DB, ownerUUID string, editingStatus string) ([]internalrepo.OwnedGame, error)
}

// ListGames returns ownerUUID's Games, the most recently changed first.
func (m *Manager) ListGames(ctx context.Context, ownerUUID string) ([]game.GameSummary, error) {
	defer logging.Step(ctx, "gamecard.list_games").Close()

	owner, err := uuid.Parse(ownerUUID)
	if err != nil {
		return []game.GameSummary{}, nil
	}
	found, err := m.listGamesRepo.ListOwnedGames(ctx, m.dbServicer.GetDB(), owner.String(), string(edition.Editing))
	if err != nil {
		return nil, err
	}
	games := make([]game.GameSummary, len(found))
	for i, owned := range found {
		games[i] = game.GameSummary{
			GameUUID: owned.UUID, Name: owned.Name, Published: owned.PublishedAt != nil,
			Visibility: game.VisibilityType(owned.Visibility), Editing: owned.Editing, Publishing: owned.Publishing,
			UpdatedAt: owned.UpdatedAt.UTC(),
		}
	}
	logging.LogFields(ctx, logging.Field("games", len(games)))
	return games, nil
}
