package getgame

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/game"
	"github.com/diegobermudez03/playhoot/game/game/internal/businessservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
	"github.com/diegobermudez03/playhoot/logging"
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

func (c *UseCase) GetGameWithCurrentVersion(ctx context.Context, gameUUID string) (*game.Game, error) {
	defer logging.Step(ctx, "GetGameWithCurrentVersion").Close()
	logging.LogFields(ctx, logging.Field("game_uuid", gameUUID))
	g, err := c.repo.getGameCurrentVersion(ctx, gameUUID)
	if err != nil {
		return nil, fmt.Errorf("fetching game in service: %s", err)
	}
	// not found or non existent
	if g == nil {
		return nil, nil
	}

	var programDefinition *program.Definition
	if g.Script != "" {
		programDefinition, err = gameservice.DecodeJSON([]byte(g.Script))
		if err != nil {
			logging.LogError(ctx, err)
		}
	}

	visbility, ok := businessservice.ValidateVisibility(g.Visibility)
	if !ok {
		message := fmt.Sprintf("invalid visibility %s", g.Visibility)
		logging.LogError(ctx, fmt.Errorf("%s", message))
		panic(message)
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
