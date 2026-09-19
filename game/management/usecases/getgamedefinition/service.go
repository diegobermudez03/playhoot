package getgamedefinition

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
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

// GetGameDefinition loads the immutable Game Definition identified by its own
// Definition/Version UUID.
//
// Unlike getgame.GetPlayableGameWithCurrentVersion, this never resolves
// through a Game's current_definition_id: it is the capability a caller uses
// after it has already pinned a specific version (for example a Session
// created against a Game's then-current Definition), so it keeps observing
// that exact version even after the Game's author publishes a newer one.
//
// Returns (nil, nil) when no Definition exists for gameDefinitionUUID.
func (c *UseCase) GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error) {
	defer logging.Step(ctx, "GetGameDefinition").Close()
	logging.LogFields(ctx, logging.Field("game_definition_uuid", gameDefinitionUUID))

	d, err := c.repo.getGameDefinitionByUUID(ctx, gameDefinitionUUID)
	if err != nil {
		return nil, fmt.Errorf("fetching game definition in service: %s", err)
	}
	logging.LogFields(ctx, logging.Field("found", d != nil))

	if d == nil {
		return nil, nil
	}

	definition, err := gameservice.DecodeJSON([]byte(d.Script))
	if err != nil {
		monitoring.Alert(ctx, "pinned game definition with invalid script")
		return nil, fmt.Errorf("decoding pinned game definition script: %s", err)
	}

	return definition, nil
}
