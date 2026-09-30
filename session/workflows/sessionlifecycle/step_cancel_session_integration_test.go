package sessionlifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"github.com/diegobermudez03/playhoot/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestManagerCancelSession_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	t.Run("accepted_and_script_requests_completion_reuses_game_completed_reason", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{}), RequestedCommands: []json.RawMessage{sessionCompleteCommand(t)}}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		result, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, result.Outcome)
		require.Equal(t, sessionUUID, result.SessionUUID)
		require.Equal(t, session.TerminalReasonGameCompleted, result.TerminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "the cancellation must commit a second RuntimeTurn")

		var row struct {
			SourceKind         string `gorm:"column:source_kind"`
			SourceCauseEventID *uint  `gorm:"column:source_cause_event_id"`
			ActorID            *uint  `gorm:"column:actor_id"`
		}
		require.NoError(t, db.Raw(`
			SELECT source_kind, source_cause_event_id, actor_id
			FROM session_runtime_turns
			WHERE session_id = ?
			ORDER BY sequence DESC LIMIT 1
		`, sessionIDForUUID(t, db, sessionUUID)).Scan(&row).Error)
		require.Equal(t, sessionCancelledSourceKind, row.SourceKind)
		require.NotNil(t, row.SourceCauseEventID, "the Turn's own source_cause_event_id must be backfilled")
		require.NotNil(t, row.ActorID)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)
	})

	t.Run("accepted_but_script_does_not_request_termination_is_forced_terminal", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		result, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, result.Outcome)
		require.Equal(t, session.TerminalReasonSessionCancelledByHost, result.TerminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "the accepted-but-non-terminal reaction must still commit a second RuntimeTurn")

		var phase, terminalReason string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)
		require.NoError(t, db.Raw(`SELECT terminal_reason FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&terminalReason).Error)
		require.Equal(t, session.TerminalReasonSessionCancelledByHost, terminalReason)

		var failureCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_failures WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&failureCount).Error)
		require.Zero(t, failureCount)
	})

	t.Run("rejected_by_the_script_is_still_forced_terminal_with_no_runtime_turn", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{}, &executor.ScriptRejectedError{Reason: "no reaction to SESSION_CANCELLED"}
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		result, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, result.Outcome)
		require.Equal(t, session.TerminalReasonSessionCancelledByHost, result.TerminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "a script-rejected cancellation must not commit a Turn - only Start's own Turn remains")

		var phase, terminalReason string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)
		require.NoError(t, db.Raw(`SELECT terminal_reason FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&terminalReason).Error)
		require.Equal(t, session.TerminalReasonSessionCancelledByHost, terminalReason)

		var failureCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_failures WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&failureCount).Error)
		require.Zero(t, failureCount)
	})

	t.Run("accepted_and_script_requests_send_event_returns_it_in_memory", func(t *testing.T) {
		var hostRef string
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{
				NewState:          stateJSON(t, map[string]any{}),
				RequestedCommands: []json.RawMessage{sendEventCommand(t, []string{hostRef}, "farewell", stateJSON(t, map[string]any{}))},
			}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		hostRef = string(actorRefForActorID(actorIDForUUID(t, db, sessionUUID, hostUUID)))

		result, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, result.Outcome)
		require.Equal(t, []session.OutboundEvent{
			{ID: "2:0", Recipients: []session.ActorRef{session.ActorRef(hostRef)}, Name: "farewell", Payload: stateJSON(t, map[string]any{}), Revision: 2},
		}, result.Events)
	})

	t.Run("rejected_by_the_script_returns_nil_events_since_no_execute_pass_ran", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{}, &executor.ScriptRejectedError{Reason: "no reaction to SESSION_CANCELLED"}
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		result, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, result.Outcome)
		require.Nil(t, result.Events)
	})

	t.Run("retried_cancellation_with_same_idempotency_key_returns_nil_events_on_replay", func(t *testing.T) {
		var hostRef string
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{
				NewState:          stateJSON(t, map[string]any{}),
				RequestedCommands: []json.RawMessage{sendEventCommand(t, []string{hostRef}, "farewell", stateJSON(t, map[string]any{}))},
			}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		hostRef = string(actorRefForActorID(actorIDForUUID(t, db, sessionUUID, hostUUID)))
		key := session.IdempotencyKey(uuid.NewString())

		first, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, key)
		require.NoError(t, err)
		require.NotEmpty(t, first.Events, "the original call must have actually collected the requested event")

		second, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, key)
		require.NoError(t, err)
		require.Nil(t, second.Events, "a replayed idempotency claim must never reconstruct/re-emit the original call's transient events")
	})

	t.Run("caller_not_the_host_is_rejected_without_reaching_the_executor", func(t *testing.T) {
		exec := fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{}))
		m, sessionUUID, _ := startedSessionWithExecutor(t, db, exec)

		result, err := m.CancelSession(context.Background(), sessionUUID, session.UserUUID(uuid.NewString()), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeNotHost, result.Outcome)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseRunning, phase, "a non-host caller must not terminalize the Session")
	})

	t.Run("still_lobby_is_rejected_without_reaching_the_executor", func(t *testing.T) {
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUIDStr := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUIDStr)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)

		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))
		result, err := m.CancelSession(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUIDStr), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeNotRunning, result.Outcome)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(fx.SessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseLobby, phase)
	})

	t.Run("already_terminal_is_an_idempotent_no_op", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		first, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, first.Outcome)

		second, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeAlreadyTerminal, second.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "cancelling an already-TERMINAL Session must not commit a further Turn")
	})

	t.Run("retried_cancellation_with_same_idempotency_key_replays_without_a_second_effect", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		key := session.IdempotencyKey(uuid.NewString())

		first, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, key)
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, first.Outcome)

		second, err := m.CancelSession(context.Background(), sessionUUID, hostUUID, key)
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeCancelled, second.Outcome)
		require.Equal(t, first.TerminalReason, second.TerminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "a retried same-key cancellation must not commit a second Turn")
	})

	t.Run("already_terminal_for_a_different_reason_is_still_an_idempotent_no_op", func(t *testing.T) {
		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUIDStr := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUIDStr)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")
		gv := testfixtures.SeedCurrentGameVersion(t, db, "function backend() {}", "function frontend() {}", 1, nil)
		require.NoError(t, db.Exec(`UPDATE sessions SET game_definition_uuid = ? WHERE id = ?`, gv.DefinitionUUID, fx.SessionID).Error)

		m := New(db, fakeExecutorFailing("executor unreachable"))
		_, err := m.Start(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUIDStr), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)

		result, err := m.CancelSession(context.Background(), session.SessionUUID(fx.SessionUUID), session.UserUUID(hostUUIDStr), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.CancelSessionOutcomeAlreadyTerminal, result.Outcome)
	})

	t.Run("stale_activity_deadline_materializes_inactivity_expiration_before_cancellation", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

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

	t.Run("session_not_found", func(t *testing.T) {
		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))
		_, err := m.CancelSession(context.Background(), session.SessionUUID(uuid.NewString()), session.UserUUID(uuid.NewString()), session.IdempotencyKey(uuid.NewString()))
		require.ErrorIs(t, err, session.ErrSessionNotFound)
	})
}
