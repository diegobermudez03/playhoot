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

		// Replaces the pre-lobby scaffolding schema above with the
		// Session/Actor/Participant/lobby identity model.
		migration20260908000000DropLegacySessionSchema(),
		migration20260908000001Sessions(),
		migration20260908000002SessionActors(),
		migration20260908000003SessionParticipants(),
		migration20260908000004JoinCodes(),
		migration20260908000005SessionRequests(),

		// RuntimeTurn/RuntimeStep tables and the sessions.current_turn_id
		// pointer to the current one.
		migration20260919000000SessionRuntimeTurns(),
		migration20260919000001SessionRuntimeSteps(),
		migration20260919000002SessionsCurrentTurnID(),

		// session_interactions - the durable Interaction entity a committed
		// RuntimeTurn opens (from an engine.OpenQuestionOutput) and resolves.
		migration20260919000003SessionInteractions(),
	})

	return migrator.Migrate()
}
