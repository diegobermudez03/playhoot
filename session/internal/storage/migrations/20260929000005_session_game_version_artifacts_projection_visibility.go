package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260929000005SessionGameVersionArtifactsProjectionVisibility
// adds ProjectionVisibility (session/docs/GAME_VERSION_ARTIFACT_MODEL.md's
// own addendum) to session_game_version_artifacts: an author's declared
// per-path visibility schema (public/private/server_only plus declassified
// derivatives) that Session Runtime's own visibility-filtering step uses
// to construct a viewer-scoped projection input *before* any authored
// project() call - never a game-rule mechanism the authored script itself
// interprets. NULL means no schema declared - the filtering step treats
// that the same as an explicitly empty schema, so every path falls back to
// its own safe default (server_only) and nothing is exposed.
func migration20260929000005SessionGameVersionArtifactsProjectionVisibility() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260929000005_session_game_version_artifacts_projection_visibility",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE session_game_version_artifacts
					ADD COLUMN projection_visibility JSONB NULL
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE session_game_version_artifacts
					DROP COLUMN projection_visibility
			`).Error
		},
	}
}
