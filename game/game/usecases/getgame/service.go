package getgame

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/game"
	"github.com/diegobermudez03/playhoot/game/game/internal/businessservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"gorm.io/gorm"
)

type UseCase struct {
	repo repoAPI
}

func New(db *gorm.DB) *UseCase {
	return &UseCase{
		repo: newRepo(db),
	}
}

// GetPlayableGameWithCurrentVersion returns the playable game by its uuid
// - Playable game means that its visibility is playable
func (c *UseCase) GetPlayableGameWithCurrentVersion(ctx context.Context, gameUUID string) (*game.Game, error) {
	defer logging.Step(ctx, "GetGameWithCurrentVersion").Close()
	logging.LogFields(ctx, logging.Field("game_uuid", gameUUID))

	g, err := c.repo.getGameCurrentVersion(ctx, gameUUID)
	if err != nil {
		return nil, fmt.Errorf("fetching game in service: %s", err)
	}
	logging.LogFields(ctx, logging.Field("found", g != nil))

	// not found or non existent
	if g == nil {
		return nil, nil
	}

	logging.LogFields(ctx, logging.Field("visibility", g.Visibility))
	visbility, ok := businessservice.ValidateVisibility(g.Visibility)
	if !ok {
		monitoring.Alert(ctx, fmt.Sprintf("invalid visibility %s", g.Visibility))
		return nil, game.ErrBrokenGame
	}

	if !businessservice.IsPlayableVisibility(visbility) {
		return nil, game.ErrNonPlayableGame
	}

	programDefinition, err := gameservice.DecodeJSON([]byte(g.Script))
	if err != nil {
		monitoring.Alert(ctx, "playable game with invalid script")
		return nil, game.ErrBrokenGame
	}

	return &game.Game{
		UUID:         g.UUID,
		Definition:   *programDefinition,
		Name:         g.Name,
		Description:  g.Description,
		OwnerUUID:    g.OwnerUUID,
		LogoImageURL: g.LogoImageURL,
		Visibility:   visbility,
		VersionUUID:  g.VersionUUID,
	}, nil
}
