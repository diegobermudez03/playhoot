package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260928000001SessionGameVersionArtifacts creates
// session_game_version_artifacts, Session Runtime's own persisted copy of
// the executable Game Version Artifact
// (session/docs/GAME_VERSION_ARTIFACT_MODEL.md) for one immutable Game
// definition/version - the same identity sessions.game_definition_uuid
// pins to. backend_script/frontend_script are authored source, never
// parsed/executed by Playhoot's own backend for frontend_script and only
// ever handed opaque to the separately deployed JavaScript Executor for
// backend_script; game_contract/assets/platform_contract_version are the
// artifact's optional fields. game_uuid correlates back to session_games -
// Session Runtime's own addition beyond the artifact model itself, needed
// to resolve "current version for this Game" without any Game Management
// call. The FK pair between this table and session_games (added here, once
// both tables exist) guarantees session_games.current_definition_uuid can
// never point at a version with no artifact content.
func migration20260928000001SessionGameVersionArtifacts() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260928000001_session_game_version_artifacts",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE session_game_version_artifacts (
					id BIGSERIAL PRIMARY KEY,
					definition_uuid UUID NOT NULL,
					game_uuid UUID NOT NULL,
					backend_script TEXT NOT NULL,
					frontend_script TEXT NOT NULL,
					game_contract JSONB NULL,
					assets JSONB NULL,
					platform_contract_version TEXT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT session_game_version_artifacts_definition_uuid_key UNIQUE (definition_uuid),
					CONSTRAINT session_game_version_artifacts_game_uuid_fkey
						FOREIGN KEY (game_uuid) REFERENCES session_games (game_uuid)
				)
			`).Error; err != nil {
				return err
			}
			return tx.Exec(`
				ALTER TABLE session_games
					ADD CONSTRAINT session_games_current_definition_uuid_fkey
					FOREIGN KEY (current_definition_uuid) REFERENCES session_game_version_artifacts (definition_uuid)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE session_games DROP CONSTRAINT session_games_current_definition_uuid_fkey`).Error; err != nil {
				return err
			}
			return tx.Exec(`DROP TABLE session_game_version_artifacts`).Error
		},
	}
}
