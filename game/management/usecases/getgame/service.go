package getgame

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/management"
	"github.com/diegobermudez03/playhoot/game/management/internal/businessservice"
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

// GetPlayableGameWithCurrentVersion returns the game identified by gameUUID
// together with its current definition, or (nil, nil) if the game does not
// exist. It fails with ErrNonPlayableGame if the game's visibility does not
// allow play, and with ErrBrokenGame if its stored data is invalid.
func (c *UseCase) GetPlayableGameWithCurrentVersion(ctx context.Context, gameUUID string) (*management.Game, error) {
	defer logging.Step(ctx, "GetGameWithCurrentVersion").Close()
	logging.LogFields(ctx, logging.Field("game_uuid", gameUUID))

	g, err := c.repo.getGameCurrentVersion(ctx, gameUUID)
	if err != nil {
		return nil, fmt.Errorf("fetching game in service: %s", err)
	}
	logging.LogFields(ctx, logging.Field("found", g != nil))

	if g == nil {
		return nil, nil
	}

	logging.LogFields(ctx, logging.Field("visibility", g.Visibility))
	visbility, ok := businessservice.ValidateVisibility(g.Visibility)
	if !ok {
		monitoring.Alert(ctx, fmt.Sprintf("invalid visibility %s", g.Visibility))
		return nil, management.ErrBrokenGame
	}

	if !businessservice.IsPlayableVisibility(visbility) {
		return nil, management.ErrNonPlayableGame
	}

	programDefinition, err := gameservice.DecodeJSON([]byte(g.Script))
	if err != nil {
		monitoring.Alert(ctx, "playable game with invalid script")
		return nil, management.ErrBrokenGame
	}

	return &management.Game{
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
