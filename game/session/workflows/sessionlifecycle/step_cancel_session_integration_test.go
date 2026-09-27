package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// startedCancelSession seeds a LOBBY Session with one active host
// Participant, starts it against definition, and returns the resulting
// Manager plus the Session's/host's identities - the common setup every
// CancelSession integration subtest below builds on, mirroring
// startedUserIntentSession exactly.
func startedCancelSession(t *testing.T, db *gorm.DB, definition program.Definition) (m *Manager, sessionUUID session.SessionUUID, hostUUID session.UserUUID) {
	t.Helper()

	m = New(db, nil, stubStartPinnedGameReader{definition: definition})

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

func TestManagerCancelSession_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	t.Run("accepted_and_game_reaches_terminal_run_status_reuses_game_cancelled_reason", func(t *testing.T) {
		// AC1
		m, sessionUUID, hostUUID := startedCancelSession(t, db, sessionCancelledDefinition(1, 4))

		result, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, result.Outcome)
		require.Equal(t, sessionUUID, result.SessionUUID)
		require.Equal(t, session.TerminalReasonGameCancelled, result.TerminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "the cancellation must commit a second RuntimeTurn")

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
		require.Equal(t, "SESSION_CANCELLED", row.SourceKind)
		require.NotNil(t, row.SourceCauseEventID, "the Turn's own source_cause_event_id must be backfilled")
		require.NotNil(t, row.ActorID)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)
	})

	t.Run("accepted_but_game_does_not_reach_terminal_run_status_is_forced_terminal", func(t *testing.T) {
		// AC2
		m, sessionUUID, hostUUID := startedCancelSession(t, db, sessionCancelledStaysDefinition(1, 4))

		result, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, result.Outcome)
		require.Equal(t, session.TerminalReasonSessionCancelledByHost, result.TerminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "the accepted-but-non-terminal transition must still commit a second RuntimeTurn")

		var phase, terminalReason string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)
		require.NoError(t, db.Raw(`SELECT terminal_reason FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&terminalReason).Error)
		require.Equal(t, session.TerminalReasonSessionCancelledByHost, terminalReason)

		// WORK-0014 AC6: a host cancellation is never a runtime failure -
		// no session_runtime_failures row, even though the Session
		// terminalized.
		var failureCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_failures WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&failureCount).Error)
		require.Zero(t, failureCount)
	})

	t.Run("rejected_by_the_engine_is_still_forced_terminal_with_no_runtime_turn", func(t *testing.T) {
		// AC3 - startableDefinition declares no transition matching
		// SessionCancelled at all, so the engine rejects it outright.
		m, sessionUUID, hostUUID := startedCancelSession(t, db, startableDefinition(1, 4))

		result, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, result.Outcome)
		require.Equal(t, session.TerminalReasonSessionCancelledByHost, result.TerminalReason)
		require.Empty(t, result.Outputs)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "an engine-rejected cancellation must not commit a Turn - only Start's own Turn remains")

		var phase, terminalReason string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)
		require.NoError(t, db.Raw(`SELECT terminal_reason FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&terminalReason).Error)
		require.Equal(t, session.TerminalReasonSessionCancelledByHost, terminalReason)

		// WORK-0014 AC6: an engine-rejected cancellation is still not a
		// runtime failure - no session_runtime_failures row.
		var failureCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_failures WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&failureCount).Error)
		require.Zero(t, failureCount)
	})

	t.Run("caller_not_the_host_is_rejected_without_reaching_the_engine", func(t *testing.T) {
		// AC4
		m, sessionUUID, _ := startedCancelSession(t, db, sessionCancelledDefinition(1, 4))

		result, err := m.CancelSession(context.Background(), sessionUUID, session.UserUUID(uuid.NewString()), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeNotHost, result.Outcome)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseRunning, phase, "a non-host caller must not terminalize the Session")
	})

	t.Run("still_lobby_is_rejected_without_reaching_the_engine", func(t *testing.T) {
		// AC5
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUIDStr := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUIDStr)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)

		m := New(db, nil, stubStartPinnedGameReader{definition: sessionCancelledDefinition(1, 4)})
		result, err := m.CancelSession(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUIDStr), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeNotRunning, result.Outcome)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(fx.SessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseLobby, phase)
	})

	t.Run("already_terminal_is_an_idempotent_no_op", func(t *testing.T) {
		// AC6
		m, sessionUUID, hostUUID := startedCancelSession(t, db, sessionCancelledDefinition(1, 4))

		first, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, first.Outcome)

		second, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeAlreadyTerminal, second.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "cancelling an already-TERMINAL Session must not commit a further Turn")
	})

	t.Run("retried_cancellation_with_same_idempotency_key_replays_without_a_second_effect", func(t *testing.T) {
		// AC7
		m, sessionUUID, hostUUID := startedCancelSession(t, db, sessionCancelledDefinition(1, 4))
		key := session.IdempotencyKey(uuid.NewString())

		first, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, key)
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, first.Outcome)

		second, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, key)
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, second.Outcome)
		require.Equal(t, first.TerminalReason, second.TerminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "a retried same-key cancellation must not commit a second Turn")
	})

	t.Run("rejects_conflicting_payload_under_same_token", func(t *testing.T) {
		// AC7 - the idempotency claim's own scoping identity is
		// (UserUUID, operation, IdempotencyKey) alone; SessionUUID is not
		// part of it, so the same host reusing the same key against a
		// second, different Session is a materially different request under
		// the same claim identity, mirroring Start's own
		// rejects_conflicting_payload_under_same_token exactly.
		m, sessionUUID, hostUUID := startedCancelSession(t, db, sessionCancelledDefinition(1, 4))
		key := session.IdempotencyKey(uuid.NewString())

		_, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, key)
		require.NoError(t, err)

		otherFx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		otherHostActorID := testfixtures.SeedActor(t, db, otherFx.SessionID, string(hostUUID))
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, otherHostActorID, otherFx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, otherHostActorID, "Host")
		otherStart, err := m.Start(context.Background(), session.SessionUUID(otherFx.SessionUUID), hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.StartOutcomeStarted, otherStart.Outcome)

		_, err = m.CancelSession(context.Background(), session.SessionUUID(otherFx.SessionUUID), hostUUID, key)
		require.ErrorIs(t, err, session.ErrIdempotencyConflict)
	})

	t.Run("already_terminal_for_a_different_reason_is_still_an_idempotent_no_op", func(t *testing.T) {
		// Complements AC6 with a Session already TERMINAL for a reason other
		// than a prior CancelSession call (here, nonStartableDefinition's own
		// Start-time fatal path) - CancelSession still reports
		// AlreadyTerminal rather than re-attempting engine execution.
		// AC8 itself (AdvanceTurn returning an error other than
		// ErrSignalRejected/ErrInputRejected) is not independently forceable
		// through a Definition fixture alone once past Start - the same
		// limitation every other capability's own fatal-path tests already
		// accept for this specific branch - so it is otherwise verified by
		// code review: terminalizeCancelSessionFatal is structurally
		// identical to the already-reviewed terminalizeSubmitUserIntentFatal/
		// terminalizeAnswerInteractionFatal.
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUIDStr := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUIDStr)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		m := New(db, nil, stubStartPinnedGameReader{definition: nonStartableDefinition(1, 4)})
		_, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUIDStr), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)

		result, err := m.CancelSession(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUIDStr), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeAlreadyTerminal, result.Outcome)
	})

	t.Run("stale_activity_deadline_materializes_inactivity_expiration_before_cancellation", func(t *testing.T) {
		// CancelSession is a RUNNING-phase lazy-materialization checkpoint
		// too (validation only, not a renewal trigger - it always
		// terminalizes the Session itself either way). A Session already
		// inactivity-expired must not have TerminalReasonSessionCancelledByHost
		// silently applied over it.
		m, sessionUUID, hostUUID := startedCancelSession(t, db, sessionCancelledStaysDefinition(1, 4))

		staleDeadline := time.Now().Add(-1 * time.Minute).UTC()
		require.NoError(t, db.Exec(`UPDATE sessions SET activity_expires_at = ? WHERE uuid = ?`, staleDeadline, string(sessionUUID)).Error)

		result, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeAlreadyTerminal, result.Outcome, "a materialized inactivity expiration leaves the existing already-TERMINAL decline path to apply")

		var row struct {
			Phase          string     `gorm:"column:phase"`
			TerminalReason *string    `gorm:"column:terminal_reason"`
			TerminalAt     *time.Time `gorm:"column:terminal_at"`
		}
		require.NoError(t, db.Raw(`SELECT phase, terminal_reason, terminal_at FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&row).Error)
		require.Equal(t, session.PhaseTerminal, row.Phase)
		require.NotNil(t, row.TerminalReason)
		require.Equal(t, session.TerminalReasonRuntimeInactivityExpired, *row.TerminalReason, "must not be overwritten by TerminalReasonSessionCancelledByHost")
		require.NotNil(t, row.TerminalAt)
		require.WithinDuration(t, staleDeadline, row.TerminalAt.UTC(), time.Second)

		var failureCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_failures WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&failureCount).Error)
		require.Equal(t, int64(0), failureCount)
	})

	t.Run("effect_outputs_from_an_accepted_transition_are_returned", func(t *testing.T) {
		// AC10 - reuses userIntentDefinition's own Effect-emitting shape by
		// having SessionCancelled itself emit one, proving mapOutputs/
		// clientoutputs.ClientFacing is exercised identically to every other
		// capability's accepted path.
		definition := sessionCancelledDefinition(1, 4)
		definition.Effects = []program.EffectDeclaration{{Name: userIntentEffectName}}
		definition.Workflows[0].States[0].Transitions[1] = program.TransitionDeclaration{
			Name:   "Cancelled",
			Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "SessionCancelled"}},
			Operations: program.Block{Operations: []program.Operation{
				program.EmitEffectOperation{
					Effect:     userIntentEffectName,
					Recipients: program.ReferenceExpression{Name: "players"},
				},
			}},
			Control: program.CancelControl{Reason: program.StringLiteralExpression{Value: sessionCancelledReasonArgName}},
		}

		m, sessionUUID, hostUUID := startedCancelSession(t, db, definition)
		result, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, result.Outcome)
		require.Len(t, result.Outputs, 1)
		effect, ok := result.Outputs[0].(session.EffectEmitted)
		require.True(t, ok, "expected a session.EffectEmitted, got %T", result.Outputs[0])
		require.Equal(t, userIntentEffectName, effect.Effect)
	})

	t.Run("session_not_found", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: sessionCancelledDefinition(1, 4)})
		_, err := m.CancelSession(context.Background(), session.SessionUUID(uuid.NewString()), session.UserUUID(uuid.NewString()), session.IdempotencyKey(uuid.NewString()))
		require.ErrorIs(t, err, session.ErrSessionNotFound)
	})
}
