package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/session/internal/testfixtures"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/replay"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// startedUserIntentSession seeds a LOBBY Session with one active host
// Participant, starts it against definition, and returns the resulting
// Manager plus the Session's/host's identities - the common setup every
// SubmitUserIntent integration subtest below builds on.
func startedUserIntentSession(t *testing.T, db *gorm.DB, definition program.Definition) (m *Manager, sessionUUID session.SessionUUID, hostUUID session.UserUUID) {
	t.Helper()

	m = New(db, stubStartPinnedGameReader{definition: definition})

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	hostUUIDStr := uuid.NewString()
	hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUIDStr)
	require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
	testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

	startResult, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUIDStr), session.IdempotencyKey(uuid.NewString()))
	require.NoError(t, err)
	require.Equal(t, session.StartOutcomeStarted, startResult.Outcome)

	return m, session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUIDStr)
}

func TestManagerSubmitUserIntent_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	t.Run("accepted_intent_commits_second_turn_and_returns_effect_output", func(t *testing.T) {
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentDefinition(1, 4))

		result, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 42), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeAccepted, result.Outcome)
		require.Equal(t, sessionUUID, result.SessionUUID)
		require.Len(t, result.Outputs, 1)
		effect, ok := result.Outputs[0].(session.EffectEmitted)
		require.True(t, ok, "expected a session.EffectEmitted, got %T", result.Outputs[0])
		require.Equal(t, userIntentEffectName, effect.Effect)
		require.Equal(t, []session.UserUUID{hostUUID}, effect.Recipients)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "the intent must commit a second RuntimeTurn")

		var row struct {
			SourceKind         string `gorm:"column:source_kind"`
			SourceCauseEventID *uint  `gorm:"column:source_cause_event_id"`
			ActorID            *uint  `gorm:"column:actor_id"`
		}
		require.NoError(t, db.Raw(`
			SELECT source_kind, source_cause_event_id, actor_id
			FROM session_runtime_turns
			WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)
			ORDER BY sequence DESC LIMIT 1
		`, string(sessionUUID)).Scan(&row).Error)
		require.Equal(t, "USER_INTENT", row.SourceKind)
		require.NotNil(t, row.SourceCauseEventID, "the Turn's own source_cause_event_id must be backfilled")
		require.NotNil(t, row.ActorID)

		var causeEventCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_cause_events WHERE runtime_turn_id = (SELECT id FROM session_runtime_turns WHERE id = (SELECT MAX(id) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)))`, string(sessionUUID)).Scan(&causeEventCount).Error)
		require.Equal(t, int64(1), causeEventCount)
	})

	t.Run("accepted_intent_renews_activity_deadline", func(t *testing.T) {
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentDefinition(1, 4))

		before := activityExpiresAtForUUID(t, db, sessionUUID)
		require.NotNil(t, before)

		result, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 42), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeAccepted, result.Outcome)

		after := activityExpiresAtForUUID(t, db, sessionUUID)
		require.NotNil(t, after)
		require.True(t, after.After(*before), "a Turn-committing intent must renew activity_expires_at")
	})

	t.Run("stale_activity_deadline_materializes_inactivity_expiration_and_declines", func(t *testing.T) {
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentDefinition(1, 4))

		staleDeadline := time.Now().Add(-1 * time.Minute).UTC()
		require.NoError(t, db.Exec(`UPDATE sessions SET activity_expires_at = ? WHERE uuid = ?`, staleDeadline, string(sessionUUID)).Error)

		result, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 42), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeRejected, result.Outcome, "a materialized inactivity expiration leaves the existing non-RUNNING decline path to apply")

		var row struct {
			Phase          string     `gorm:"column:phase"`
			TerminalReason *string    `gorm:"column:terminal_reason"`
			TerminalAt     *time.Time `gorm:"column:terminal_at"`
		}
		require.NoError(t, db.Raw(`SELECT phase, terminal_reason, terminal_at FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&row).Error)
		require.Equal(t, session.PhaseTerminal, row.Phase)
		require.NotNil(t, row.TerminalReason)
		require.Equal(t, session.TerminalReasonRuntimeInactivityExpired, *row.TerminalReason)
		require.NotNil(t, row.TerminalAt)
		require.WithinDuration(t, staleDeadline, row.TerminalAt.UTC(), time.Second)

		var failureCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_failures WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&failureCount).Error)
		require.Equal(t, int64(0), failureCount)
	})

	t.Run("second_submission_replays_correctly_via_replay_reconstruction", func(t *testing.T) {
		// Proves USER_INTENT's replay wiring: a second SubmitUserIntent call
		// forces replay.LoadPriorSignals to reconstruct the first
		// SubmitUserIntent-driven Turn from session_cause_events against
		// real Postgres before advancing further - the same style
		// AnswerInteraction/Start's own multi-turn integration tests already
		// rely on replay to reconstruct Start's own Turn.
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentDefinition(1, 4))

		first, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 1), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeAccepted, first.Outcome)

		second, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 2), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeAccepted, second.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(3), turnCount, "Start + two accepted intents")
	})

	t.Run("retried_submission_with_same_idempotency_key_replays_without_a_second_turn", func(t *testing.T) {
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentDefinition(1, 4))
		key := session.IdempotencyKey(uuid.NewString())

		first, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 42), key)
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeAccepted, first.Outcome)

		second, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 42), key)
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeAccepted, second.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "a retried same-key submission must not commit a second Turn")
	})

	t.Run("conflicting_retry_with_same_idempotency_key_is_rejected", func(t *testing.T) {
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentDefinition(1, 4))
		key := session.IdempotencyKey(uuid.NewString())

		_, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 42), key)
		require.NoError(t, err)

		_, err = m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 43), key)
		require.ErrorIs(t, err, session.ErrIdempotencyConflict)
	})

	t.Run("unknown_intent_name_is_rejected_without_reaching_the_engine", func(t *testing.T) {
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentDefinition(1, 4))

		result, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, "NotDeclared", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeRejected, result.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "only Start's own Turn - no Turn from a rejected unknown intent")
	})

	t.Run("missing_declared_argument_is_rejected_without_reaching_the_engine", func(t *testing.T) {
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentDefinition(1, 4))

		result, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, userIntentArguments(t), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeRejected, result.Outcome)
	})

	t.Run("wrong_typed_argument_is_rejected_without_reaching_the_engine", func(t *testing.T) {
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentDefinition(1, 4))

		wrongTyped := userIntentArguments(t, engine.FieldValue{Name: "value", Value: engine.StringValue{Value: "not-a-number"}})
		result, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, wrongTyped, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeRejected, result.Outcome)
	})

	t.Run("engine_rejects_unmatched_declared_intent", func(t *testing.T) {
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentDefinition(1, 4))

		result, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentIgnoredName, nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeRejected, result.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "an engine-rejected unmatched signal must not commit a Turn")
	})

	t.Run("unresolvable_actor_is_rejected", func(t *testing.T) {
		m, sessionUUID, _ := startedUserIntentSession(t, db, userIntentDefinition(1, 4))

		result, err := m.SubmitUserIntent(context.Background(), sessionUUID, session.UserUUID(uuid.NewString()), userIntentGuessName, guessArguments(t, 1), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeRejected, result.Outcome)
	})

	t.Run("game_completion_terminalizes_the_session", func(t *testing.T) {
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentTriggersTerminationDefinition(1, 4))

		result, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 1), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeAccepted, result.Outcome)
		require.Equal(t, session.TerminalReasonGameCompleted, result.TerminalReason)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)

		// WORK-0014 AC6: an authored game-completion terminal outcome is not
		// a runtime failure - no session_runtime_failures row.
		var failureCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_failures WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&failureCount).Error)
		require.Zero(t, failureCount)
	})

	t.Run("deterministic_engine_failure_terminalizes_the_session", func(t *testing.T) {
		m, sessionUUID, hostUUID := startedUserIntentSession(t, db, userIntentDefinitionWithFatalGuess(1, 4))
		key := session.IdempotencyKey(uuid.NewString())

		result, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 1), key)
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeRuntimeExecutionFailed, result.Outcome)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)
		var terminalReason string
		require.NoError(t, db.Raw(`SELECT terminal_reason FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&terminalReason).Error)
		require.Equal(t, session.TerminalReasonRuntimeExecutionFailed, terminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "a fatal execution must not persist a partial RuntimeTurn - only Start's own Turn remains")

		// WORK-0014: the same atomic materialization also persists a
		// session_runtime_failures diagnostic row.
		var failureRow struct {
			FailureKind       string `gorm:"column:failure_kind"`
			ErrorCode         string `gorm:"column:error_code"`
			BaseTurnID        *uint  `gorm:"column:base_turn_id"`
			AttemptedSequence uint64 `gorm:"column:attempted_sequence"`
			SourceKind        string `gorm:"column:source_kind"`
			ActorID           *uint  `gorm:"column:actor_id"`
		}
		require.NoError(t, db.Raw(`
			SELECT failure_kind, error_code, base_turn_id, attempted_sequence, source_kind, actor_id
			FROM session_runtime_failures WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)
		`, string(sessionUUID)).Scan(&failureRow).Error)
		require.Equal(t, RuntimeFailureKindExecution, failureRow.FailureKind)
		require.Equal(t, RuntimeFailureErrorCodeDivisionByZero, failureRow.ErrorCode)
		require.NotNil(t, failureRow.BaseTurnID, "Start's own Turn already committed before this fatal failure")
		require.Equal(t, uint64(2), failureRow.AttemptedSequence)
		require.Equal(t, replay.UserIntentSourceKind, failureRow.SourceKind)
		require.NotNil(t, failureRow.ActorID)

		var failureCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_failures WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&failureCount).Error)
		require.Equal(t, int64(1), failureCount, "a same-key replay below must not persist a second failure row")

		// A retry under the same idempotency key must replay the already-
		// recorded fatal outcome from session_requests, not re-attempt a
		// doomed execution against an already-TERMINAL Session.
		result2, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 1), key)
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeRuntimeExecutionFailed, result2.Outcome)

		// A genuinely new submission (a fresh idempotency key) against the
		// now-TERMINAL Session must be rejected by the explicit phase check
		// before ever reaching the engine again - not re-attempt (and
		// re-fail) the same doomed execution. Rejected, not
		// RuntimeExecutionFailed, is the tell: reaching AdvanceTurn again
		// would deterministically reproduce the same division-by-zero
		// failure instead.
		result3, err := m.SubmitUserIntent(context.Background(), sessionUUID, hostUUID, userIntentGuessName, guessArguments(t, 1), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitUserIntentOutcomeRejected, result3.Outcome)

		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "no further Turn may ever commit against a TERMINAL Session")
	})

	t.Run("session_not_found", func(t *testing.T) {
		m := New(db, stubStartPinnedGameReader{definition: userIntentDefinition(1, 4)})
		_, err := m.SubmitUserIntent(context.Background(), session.SessionUUID(uuid.NewString()), session.UserUUID(uuid.NewString()), userIntentGuessName, guessArguments(t, 1), session.IdempotencyKey(uuid.NewString()))
		require.ErrorIs(t, err, session.ErrSessionNotFound)
	})
}
