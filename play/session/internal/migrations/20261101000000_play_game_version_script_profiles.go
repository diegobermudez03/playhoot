package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261101000000PlayGameVersionScriptProfiles adds the profile a
// game version's backend script is checked and run under
// (play/runtime/sandbox/scriptcheck.Profile). Every version before it is
// handwritten, and so is what the earlier seed migrations insert after it on
// a new database, hence the default.
func migration20261101000000PlayGameVersionScriptProfiles() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261101000000_play_game_version_script_profiles",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE play_game_versions ADD COLUMN script_profile TEXT NOT NULL DEFAULT 'handwritten'`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE play_game_versions DROP COLUMN script_profile`).Error
		},
	}
}
