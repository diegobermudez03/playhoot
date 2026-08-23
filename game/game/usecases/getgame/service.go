package getgame

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/game"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
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
			obser
		}
	}

}
