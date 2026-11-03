package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261103000000GamePublishing adds what a Game's card and its
// publication need: card images with their object registry and upload
// requests, releases, and edition requests
// (game/docs/DATA_MODEL.md).
func migration20261103000000GamePublishing() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261103000000_game_publishing",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				// One row per image shown on a card; removed_at is written once.
				`CREATE TABLE game_card_images (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_id BIGINT NOT NULL,
					kind TEXT NOT NULL,
					sha256 TEXT NOT NULL,
					size BIGINT NOT NULL,
					mime_type TEXT NOT NULL,
					object_key TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					removed_at TIMESTAMPTZ NULL
				)`,
				`CREATE INDEX game_card_images_game_id_idx ON game_card_images (game_id) WHERE removed_at IS NULL`,

				// Every card image object stored, bound or not (object-storage.md).
				`CREATE TABLE game_card_objects (
					id BIGSERIAL PRIMARY KEY,
					sha256 TEXT NOT NULL UNIQUE,
					size BIGINT NOT NULL,
					mime_type TEXT NOT NULL,
					object_key TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					registered_at TIMESTAMPTZ NOT NULL,
					deleted_at TIMESTAMPTZ NULL
				)`,

				// image_id is written once, by the upload that claimed the key.
				`CREATE TABLE game_card_image_requests (
					id BIGSERIAL PRIMARY KEY,
					game_id BIGINT NOT NULL,
					idempotency_key TEXT NOT NULL,
					kind TEXT NOT NULL,
					sha256 TEXT NOT NULL,
					image_id BIGINT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					UNIQUE (game_id, idempotency_key)
				)`,

				// completed_at and abandoned_at are written once; a release with
				// neither is still being published.
				`CREATE TABLE game_releases (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_id BIGINT NOT NULL,
					edition_id BIGINT NOT NULL,
					sequence BIGINT NOT NULL,
					revision BIGINT NOT NULL,
					visibility TEXT NOT NULL,
					owner_uuid UUID NOT NULL,
					idempotency_key TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					completed_at TIMESTAMPTZ NULL,
					abandoned_at TIMESTAMPTZ NULL,
					abandon_reason TEXT NULL,
					UNIQUE (game_id, sequence),
					UNIQUE (game_id, idempotency_key)
				)`,
				// A Game has at most one release being published.
				`CREATE UNIQUE INDEX game_releases_one_pending_idx ON game_releases (game_id) WHERE completed_at IS NULL AND abandoned_at IS NULL`,

				// edition_id is written once, by the request that opened it.
				`CREATE TABLE game_edition_requests (
					id BIGSERIAL PRIMARY KEY,
					game_id BIGINT NOT NULL,
					idempotency_key TEXT NOT NULL,
					edition_id BIGINT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					UNIQUE (game_id, idempotency_key)
				)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE game_edition_requests, game_releases, game_card_image_requests, game_card_objects, game_card_images`).Error
		},
	}
}
