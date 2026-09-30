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
		migration20260924000000SessionInteractionsEngineInteractionID(),

		// session_timer_obligations - the durable Timer Obligation entity a
		// committed RuntimeTurn schedules/cancels/consumes.
		// session_runtime_turns.source_timer_obligation_id already existed
		// from migration20260919000000SessionRuntimeTurns and needs no
		// migration of its own.
		migration20260925000000SessionTimerObligations(),

		// session_runtime_failures - the durable fatal-diagnostic record,
		// populated by every RuntimeTurn-producing path's fatal branch.
		migration20260926000000SessionRuntimeFailures(),

		// sessions.activity_expires_at - the durable RUNNING-phase inactivity
		// deadline every active-dependent operation validates and may renew.
		migration20260926000001SessionsActivityExpiresAt(),

		// session_games/session_game_version_artifacts - Session Runtime's
		// own persisted copy of the Game Version Artifact, resolved by
		// Create instead of reading Game Management.
		migration20260928000000SessionGames(),
		migration20260928000001SessionGameVersionArtifacts(),

		// session_runtime_turns.new_state - the authoritative snapshot-based
		// persisted state, replacing live-path replay reconstruction.
		migration20260929000000SessionRuntimeTurnsNewState(),

		// session_game_version_artifacts.participant_min/participant_max -
		// structural Session admission/capacity metadata, enforced by
		// Join/Start before any authored script runs.
		migration20260929000002SessionGameVersionArtifactsParticipantConstraints(),

		// session_interactions retired outright: the platform's closed
		// Event/Command vocabulary has no OPEN_INTERACTION concept left,
		// every game-defined player action is a PLAYER_EVENT.
		migration20260929000003DropSessionInteractions(),

		// session_timer_obligations.engine_slot/engine_key renamed to
		// timer/data - a script addresses a timer by one opaque Timer
		// string with no separate key dimension.
		migration20260929000004SessionTimerObligationsTimerData(),

		// session_game_version_artifacts.projection_visibility - an
		// author's declared per-path privacy schema, consumed by a
		// capability-based filtering step before any authored project()
		// call.
		migration20260929000005SessionGameVersionArtifactsProjectionVisibility(),
	})

	return migrator.Migrate()
}
