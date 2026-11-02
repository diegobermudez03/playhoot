package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261023000000PlayGameFaultReports keeps, to audit which games
// misbehave and which clients lie, every fault report of a Session's game: the
// faults players' clients report or the server sees (one row each), and the
// platform limits a user hit (one row per limit and audit flush, with how
// many times). Rows are only added (play/docs/DATA_MODEL.md).
func migration20261023000000PlayGameFaultReports() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261023000000_play_game_fault_reports",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE play_game_fault_reports (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					game_version_id BIGINT NOT NULL,
					user_uuid UUID NOT NULL,
					participant_id TEXT,
					source TEXT NOT NULL,
					kind TEXT NOT NULL,
					detail JSONB NOT NULL,
					occurrences INTEGER NOT NULL,
					reported_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX play_game_fault_reports_session_id_idx ON play_game_fault_reports (session_id)`,
				`CREATE INDEX play_game_fault_reports_game_version_id_kind_idx ON play_game_fault_reports (game_version_id, kind)`,
				`CREATE INDEX play_game_fault_reports_user_uuid_idx ON play_game_fault_reports (user_uuid)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE play_game_fault_reports`).Error
		},
	}
}
