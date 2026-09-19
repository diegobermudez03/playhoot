package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func MigrateTables(db *gorm.DB) error {
	migrator := gormigrate.New(db, &gormigrate.Options{
		TableName:      "migrations",
		IDColumnName:   "id",
		IDColumnSize:   255,
		UseTransaction: true,
		// Other domain packages store their migration IDs in this same table.
		ValidateUnknownMigrations: false,
	}, []*gormigrate.Migration{
		// sessions must exist before session_states can add its foreign key.
		migration20260817000001Sessions(),
		migration20260817000000SessionStates(),

		// Slice 1 (WORK-0001): replaces the pre-lobby scaffolding schema
		// above with the accepted Session/Actor/Participant/lobby identity
		// model. See game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md.
		migration20260908000000DropLegacySessionSchema(),
		migration20260908000001Sessions(),
		migration20260908000002SessionActors(),
		migration20260908000003SessionParticipants(),
		migration20260908000004JoinCodes(),
		migration20260908000005SessionRequests(),

		// Slice 2 (WORK-0003): RuntimeTurn/RuntimeStep tables and the
		// sessions.current_turn_id pointer Start's inline RuntimeTurn
		// execution logic persists to. See
		// game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md's Runtime History
		// Tables and GAME-ADR-0023 (current_turn_id lives on sessions, not
		// a separate session_runtime_state table).
		migration20260919000000SessionRuntimeTurns(),
		migration20260919000001SessionRuntimeSteps(),
		migration20260919000002SessionsCurrentTurnID(),
	})

	return migrator.Migrate()
}
