package creategame

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/game/game"
	"gorm.io/gorm"
)

type repoAPI interface {
	createGameDraft(ctx context.Context, params createGameDraftParams) (*createdGameDraft, error)
}

type repo struct {
	db *gorm.DB
}

func newRepo(db *gorm.DB) *repo {
	return &repo{
		db: db,
	}
}

type createGameDraftParams struct {
	GameUUID       string
	DefinitionUUID string
	Name           string
	Description    string
	OwnerUUID      string
	LogoImageURL   string
	Script         string
}

type createdGameDraft struct {
	GameUUID       string
	DefinitionUUID string
	Name           string
	Description    string
	OwnerUUID      string
	LogoImageURL   string
}

type gameInsert struct {
	ID                  uint   `gorm:"column:id"`
	UUID                string `gorm:"column:uuid"`
	Name                string `gorm:"column:name"`
	Description         string `gorm:"column:description"`
	OwnerUUID           string `gorm:"column:owner_uuid"`
	CurrentDefinitionID *uint  `gorm:"column:current_definition_id"`
	LogoImageURL        string `gorm:"column:logo_image_url"`
	Visibility          string `gorm:"column:visibility"`
}

func (gameInsert) TableName() string {
	return "games"
}

type gameDefinitionInsert struct {
	ID            uint       `gorm:"column:id"`
	UUID          string     `gorm:"column:uuid"`
	GameID        uint       `gorm:"column:game_id"`
	VersionNumber uint       `gorm:"column:version_number"`
	Script        string     `gorm:"column:script"`
	PublishedAt   *time.Time `gorm:"column:published_at"`
}

func (gameDefinitionInsert) TableName() string {
	return "game_definitions"
}

func (r *repo) createGameDraft(ctx context.Context, params createGameDraftParams) (*createdGameDraft, error) {
	created := &createdGameDraft{
		GameUUID:       params.GameUUID,
		DefinitionUUID: params.DefinitionUUID,
		Name:           params.Name,
		Description:    params.Description,
		OwnerUUID:      params.OwnerUUID,
		LogoImageURL:   params.LogoImageURL,
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		gameRow := gameInsert{
			UUID:         params.GameUUID,
			Name:         params.Name,
			Description:  params.Description,
			OwnerUUID:    params.OwnerUUID,
			LogoImageURL: params.LogoImageURL,
			Visibility:   string(game.Draft),
		}
		if err := tx.Create(&gameRow).Error; err != nil {
			return fmt.Errorf("creating game row: %s", err)
		}

		definitionRow := gameDefinitionInsert{
			UUID:          params.DefinitionUUID,
			GameID:        gameRow.ID,
			VersionNumber: 1,
			Script:        params.Script,
		}
		if err := tx.Create(&definitionRow).Error; err != nil {
			return fmt.Errorf("creating game definition row: %s", err)
		}

		if err := tx.Exec(`
			UPDATE games
			SET current_definition_id = ?
			WHERE id = ?
		`, definitionRow.ID, gameRow.ID).Error; err != nil {
			return fmt.Errorf("setting current definition: %s", err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("creating game draft in transaction: %s", err)
	}

	return created, nil
}
