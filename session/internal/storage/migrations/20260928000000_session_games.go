package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260928000000SessionGames creates session_games, Session
// Runtime's own record of a Game's current pinnable version - one row per
// Game public UUID, resolved by Create instead of reading Game Management.
// current_definition_uuid is nullable until a version has ever been
// published for this Game; Create treats a missing row and a null pointer
// identically (session.ErrGameNotFound).
func migration20260928000000SessionGames() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260928000000_session_games",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE session_games (
					id BIGSERIAL PRIMARY KEY,
					game_uuid UUID NOT NULL,
					current_definition_uuid UUID NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT session_games_game_uuid_key UNIQUE (game_uuid)
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE session_games`).Error
		},
	}
}
