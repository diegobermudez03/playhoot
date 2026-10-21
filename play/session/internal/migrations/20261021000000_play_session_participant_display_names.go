package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261021000000PlaySessionParticipantDisplayNames keeps each
// participant's display name as it was when the game started, so a game
// recovered on another instance still shows the other players who they play
// with. Participants recorded before it have an empty name.
func migration20261021000000PlaySessionParticipantDisplayNames() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261021000000_play_session_participant_display_names",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE play_session_participants ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE play_session_participants DROP COLUMN display_name`).Error
		},
	}
}
