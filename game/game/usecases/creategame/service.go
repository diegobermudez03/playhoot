package creategame

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/diegobermudez03/playhoot/game/game"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
	"github.com/diegobermudez03/playhoot/logging"
	"gorm.io/gorm"
)

type Params struct {
	Name         string
	Description  string
	OwnerUUID    string
	LogoImageURL string
	Definition   program.Definition
}

type UseCase struct {
	repo    repoAPI
	newUUID func() (string, error)
}

func New(db *gorm.DB) *UseCase {
	return &UseCase{
		repo:    newRepo(db),
		newUUID: newUUID,
	}
}

func (c *UseCase) CreateGameDraft(ctx context.Context, params Params) (*game.Game, error) {
	defer logging.Step(ctx, "CreateGameDraft").Close()
	logging.LogFields(ctx, logging.Field("owner_uuid", params.OwnerUUID))

	gameUUID, err := c.newUUID()
	if err != nil {
		return nil, fmt.Errorf("generating game uuid in service: %s", err)
	}

	definitionUUID, err := c.newUUID()
	if err != nil {
		return nil, fmt.Errorf("generating definition uuid in service: %s", err)
	}

	script, err := gameservice.EncodeJSON(params.Definition)
	if err != nil {
		return nil, fmt.Errorf("encoding definition in service: %s", err)
	}

	created, err := c.repo.createGameDraft(ctx, createGameDraftParams{
		GameUUID:       gameUUID,
		DefinitionUUID: definitionUUID,
		Name:           params.Name,
		Description:    params.Description,
		OwnerUUID:      params.OwnerUUID,
		LogoImageURL:   params.LogoImageURL,
		Script:         string(script),
	})
	if err != nil {
		return nil, fmt.Errorf("creating game draft in service: %s", err)
	}

	return &game.Game{
		UUID:         created.GameUUID,
		Name:         created.Name,
		Description:  created.Description,
		OwnerUUID:    created.OwnerUUID,
		LogoImageURL: created.LogoImageURL,
		Visibility:   game.Draft,
		VersionUUID:  created.DefinitionUUID,
		Definition:   params.Definition,
	}, nil
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("reading random bytes: %s", err)
	}

	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
