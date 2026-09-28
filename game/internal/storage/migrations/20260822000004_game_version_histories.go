package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260822000004GameDefinitionHistories() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260822000004_game_definition_histories",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE game_definition_histories (
					id BIGSERIAL PRIMARY KEY,
					game_definition_id BIGINT NOT NULL,
					script JSONB NULL,
					published_at TIMESTAMPTZ NULL,
					disabled_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE game_definition_histories`).Error
		},
	}
}
