package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260929000002SessionGameVersionArtifactsParticipantConstraints
// adds ParticipantConstraints (session/docs/GAME_VERSION_ARTIFACT_MODEL.md)
// to session_game_version_artifacts: structural Session admission/capacity
// metadata Playhoot itself enforces (Join rejects beyond participant_max,
// Start rejects below participant_min) independently of executing the
// authored backend script, not a new game-rule mechanism. participant_max
// NULL means unlimited.
func migration20260929000002SessionGameVersionArtifactsParticipantConstraints() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260929000002_session_game_version_artifacts_participant_constraints",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE session_game_version_artifacts
					ADD COLUMN participant_min INTEGER NOT NULL DEFAULT 1,
					ADD COLUMN participant_max INTEGER NULL
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE session_game_version_artifacts
					DROP COLUMN participant_min,
					DROP COLUMN participant_max
			`).Error
		},
	}
}
