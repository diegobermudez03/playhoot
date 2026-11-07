package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game/internal/business/edition"
)

// migration20261101000000GameEditions adds what creating and publishing a
// Game needs (game creation project): its editions (each the Play version it
// is built in) with their status history, and the idempotency requests of
// Game creation.
func migration20261101000000GameEditions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261101000000_game_editions",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				// uuid is also the Play version's uuid the edition is built in;
				// a first edition started from a template names it.
				`CREATE TABLE game_editions (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_id BIGINT NOT NULL,
					status TEXT NOT NULL,
					template_game_uuid UUID NULL,
					template_version_uuid UUID NULL,
					created_at TIMESTAMPTZ NOT NULL,
					published_at TIMESTAMPTZ NULL
				)`,
				`CREATE INDEX game_editions_game_id_idx ON game_editions (game_id)`,
				// A Game has at most one edition open for editing.
				fmt.Sprintf(`CREATE UNIQUE INDEX game_editions_one_editing_idx ON game_editions (game_id) WHERE status = '%s'`, edition.Editing),

				`CREATE TABLE game_edition_status_changes (
					id BIGSERIAL PRIMARY KEY,
					edition_id BIGINT NOT NULL,
					from_status TEXT NULL,
					to_status TEXT NOT NULL,
					changed_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX game_edition_status_changes_edition_id_idx ON game_edition_status_changes (edition_id)`,

				// name and template_game_uuid are what the creation was asked
				// with, to tell a retry from a different request; game_id is written once, by the creation
				// that claimed the request.
				`CREATE TABLE game_create_requests (
					id BIGSERIAL PRIMARY KEY,
					owner_uuid UUID NOT NULL,
					idempotency_key TEXT NOT NULL,
					name TEXT NOT NULL,
					template_game_uuid UUID NULL,
					game_id BIGINT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE (owner_uuid, idempotency_key)
				)`,
				`CREATE INDEX game_create_requests_game_id_idx ON game_create_requests (game_id)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE game_create_requests, game_edition_status_changes, game_editions`).Error
		},
	}
}
