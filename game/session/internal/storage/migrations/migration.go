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

		// session_runtime_turns (the ordered replay-input envelope every
		// committed RuntimeTurn is recorded against) and the
		// sessions.current_turn_id pointer to the current one.
		migration20260919000000SessionRuntimeTurns(),
		migration20260919000002SessionsCurrentTurnID(),

		// session_interactions - the durable Interaction entity a committed
		// RuntimeTurn opens (from an engine.OpenQuestionOutput) and resolves.
		migration20260919000003SessionInteractions(),

		// Start's durable Seed/RootParameters, and the shared
		// session_cause_events satellite table plus
		// session_runtime_turns.source_cause_event_id for a future
		// RuntimeTurn cause with no existing normalized home.
		migration20260922000000SessionRuntimeStarts(),
		migration20260922000001SessionCauseEvents(),

		// Replaces session_interactions' (engine_path, engine_slot) identity
		// with the Game Language engine's own assigned engine_interaction_id.
		// session_timer_obligations is unaffected - Timer occurrences keep
		// engine_path/engine_slot addressing unchanged.
		migration20260924000000SessionInteractionsEngineInteractionID(),
	})

	return migrator.Migrate()
}
