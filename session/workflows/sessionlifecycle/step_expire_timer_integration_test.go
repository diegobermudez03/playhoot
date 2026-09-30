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

func TestManagerExpireTimer_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	t.Run("expiring_a_scheduled_timer_drives_a_new_runtime_turn_and_consumes_it", func(t *testing.T) {
		nextState := stateJSON(t, map[string]any{"fired": true})
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: nextState}, nil
		})
		m, sessionUUID, _ := startedSessionWithExecutor(t, db, exec)

		var currentTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&currentTurnID).Error)
		obligationUUID := testfixtures.SeedTimerObligation(t, db, sessionIDForUUID(t, db, sessionUUID), "T", 5000, currentTurnID)

		result, err := m.ExpireTimer(context.Background(), session.TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, session.ExpireTimerOutcomeExpired, result.Outcome)
		require.Equal(t, sessionUUID, result.SessionUUID)

		var obligationRow struct {
			State          string `gorm:"column:state"`
			ClosedByTurnID *uint  `gorm:"column:closed_by_turn_id"`
		}
		require.NoError(t, db.Raw(`SELECT state, closed_by_turn_id FROM session_timer_obligations WHERE uuid = ?`, obligationUUID).Scan(&obligationRow).Error)
		require.Equal(t, session.TimerObligationStateConsumed, obligationRow.State)
		require.NotNil(t, obligationRow.ClosedByTurnID)

		var turnRow struct {
			SourceKind              string `gorm:"column:source_kind"`
			SourceTimerObligationID *uint  `gorm:"column:source_timer_obligation_id"`
			Sequence                uint64 `gorm:"column:sequence"`
			NewState                []byte `gorm:"column:new_state"`
		}
		require.NoError(t, db.Raw(`SELECT source_kind, source_timer_obligation_id, sequence, new_state FROM session_runtime_turns WHERE id = ?`, obligationRow.ClosedByTurnID).Scan(&turnRow).Error)
		require.Equal(t, timerExpiredSourceKind, turnRow.SourceKind)
		require.NotNil(t, turnRow.SourceTimerObligationID)
		require.Equal(t, uint64(2), turnRow.Sequence, "Start's own Turn is sequence 1")
		require.JSONEq(t, string(nextState), string(turnRow.NewState))

		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&currentTurnID).Error)
		require.Equal(t, *obligationRow.ClosedByTurnID, currentTurnID)
	})

	t.Run("expiring_a_scheduled_timer_renews_activity_deadline", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, _ := startedSessionWithExecutor(t, db, exec)

		var currentTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&currentTurnID).Error)
		obligationUUID := testfixtures.SeedTimerObligation(t, db, sessionIDForUUID(t, db, sessionUUID), "T", 5000, currentTurnID)

		before := activityExpiresAtForUUID(t, db, sessionUUID)
		require.NotNil(t, before)

		result, err := m.ExpireTimer(context.Background(), session.TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, session.ExpireTimerOutcomeExpired, result.Outcome)

		after := activityExpiresAtForUUID(t, db, sessionUUID)
		require.NotNil(t, after)
		require.True(t, after.After(*before), "a Turn-committing expiration must renew activity_expires_at")
	})

	t.Run("expiring_a_scheduled_timer_requesting_send_event_returns_it_in_memory", func(t *testing.T) {
		var hostRef string
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{
				NewState:          stateJSON(t, map[string]any{}),
				RequestedCommands: []json.RawMessage{sendEventCommand(t, []string{hostRef}, "ding", stateJSON(t, map[string]any{}))},
			}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		hostRef = string(actorRefForActorID(actorIDForUUID(t, db, sessionUUID, hostUUID)))

		var currentTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&currentTurnID).Error)
		obligationUUID := testfixtures.SeedTimerObligation(t, db, sessionIDForUUID(t, db, sessionUUID), "T", 5000, currentTurnID)

		result, err := m.ExpireTimer(context.Background(), session.TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, session.ExpireTimerOutcomeExpired, result.Outcome)
		require.Equal(t, []session.OutboundEvent{
			{Recipients: []session.ActorRef{session.ActorRef(hostRef)}, Name: "ding", Payload: stateJSON(t, map[string]any{})},
		}, result.Events)
	})

	t.Run("stale_activity_deadline_materializes_inactivity_expiration_and_declines", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, _ := startedSessionWithExecutor(t, db, exec)

		var currentTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&currentTurnID).Error)
		obligationUUID := testfixtures.SeedTimerObligation(t, db, sessionIDForUUID(t, db, sessionUUID), "T", 5000, currentTurnID)

		staleDeadline := time.Now().Add(-1 * time.Minute).UTC()
		require.NoError(t, db.Exec(`UPDATE sessions SET activity_expires_at = ? WHERE uuid = ?`, staleDeadline, string(sessionUUID)).Error)

		result, err := m.ExpireTimer(context.Background(), session.TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, session.ExpireTimerOutcomeStale, result.Outcome, "a materialized inactivity expiration cancels the obligation, so the existing stale-obligation decline path applies")

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

		var obligationState string
		require.NoError(t, db.Raw(`SELECT state FROM session_timer_obligations WHERE uuid = ?`, obligationUUID).Scan(&obligationState).Error)
		require.Equal(t, session.TimerObligationStateCancelled, obligationState, "SESSION-ADR-0018 terminal cleanup must cancel the still-ACTIVE obligation")

		var failureCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_failures WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&failureCount).Error)
		require.Equal(t, int64(0), failureCount)
	})

	t.Run("expiring_an_already_consumed_obligation_is_stale_and_creates_no_second_turn", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, _ := startedSessionWithExecutor(t, db, exec)

		var currentTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&currentTurnID).Error)
		obligationUUID := testfixtures.SeedTimerObligation(t, db, sessionIDForUUID(t, db, sessionUUID), "T", 5000, currentTurnID)

		first, err := m.ExpireTimer(context.Background(), session.TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, session.ExpireTimerOutcomeExpired, first.Outcome)

		var turnCountAfterFirst int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCountAfterFirst).Error)

		second, err := m.ExpireTimer(context.Background(), session.TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, session.ExpireTimerOutcomeStale, second.Outcome)

		var turnCountAfterSecond int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCountAfterSecond).Error)
		require.Equal(t, turnCountAfterFirst, turnCountAfterSecond, "a stale delivery must not create a second RuntimeTurn")
	})

	t.Run("script_rejects_expiration_as_stale_or_cancelled", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{}, &executor.ScriptRejectedError{Reason: "no reaction to this timer"}
		})
		m, sessionUUID, _ := startedSessionWithExecutor(t, db, exec)

		var currentTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&currentTurnID).Error)
		obligationUUID := testfixtures.SeedTimerObligation(t, db, sessionIDForUUID(t, db, sessionUUID), "T", 5000, currentTurnID)

		result, err := m.ExpireTimer(context.Background(), session.TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, session.ExpireTimerOutcomeRejected, result.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "a rejected expiration must not commit a Turn")
	})

	t.Run("executor_error_terminalizes_the_session", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{}, &executor.ExecutorError{Reason: "executor unreachable"}
		})
		m, sessionUUID, _ := startedSessionWithExecutor(t, db, exec)

		var currentTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&currentTurnID).Error)
		obligationUUID := testfixtures.SeedTimerObligation(t, db, sessionIDForUUID(t, db, sessionUUID), "T", 5000, currentTurnID)

		result, err := m.ExpireTimer(context.Background(), session.TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, session.ExpireTimerOutcomeRuntimeExecutionFailed, result.Outcome)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)

		var failureRow struct {
			FailureKind             string `gorm:"column:failure_kind"`
			SourceTimerObligationID *uint  `gorm:"column:source_timer_obligation_id"`
		}
		require.NoError(t, db.Raw(`SELECT failure_kind, source_timer_obligation_id FROM session_runtime_failures WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&failureRow).Error)
		require.Equal(t, RuntimeFailureKindExecution, failureRow.FailureKind)
		require.NotNil(t, failureRow.SourceTimerObligationID)
	})

	t.Run("expiring_a_timer_can_chain_into_scheduling_the_next_one", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{
				NewState:          stateJSON(t, map[string]any{"round": 2}),
				RequestedCommands: []json.RawMessage{scheduleTimerCommand(t, "round_timer", 5000)},
			}, nil
		})
		m, sessionUUID, _ := startedSessionWithExecutor(t, db, exec)

		var currentTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&currentTurnID).Error)
		obligationUUID := testfixtures.SeedTimerObligation(t, db, sessionIDForUUID(t, db, sessionUUID), "round_timer", 5000, currentTurnID)

		result, err := m.ExpireTimer(context.Background(), session.TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, session.ExpireTimerOutcomeExpired, result.Outcome)

		var rows []struct {
			State string `gorm:"column:state"`
		}
		require.NoError(t, db.Raw(`SELECT state FROM session_timer_obligations WHERE session_id = ? AND timer = ? ORDER BY id`, sessionIDForUUID(t, db, sessionUUID), "round_timer").Scan(&rows).Error)
		require.Len(t, rows, 2, "the expired obligation stays CONSUMED, and a new one is scheduled for the next round")
		require.Equal(t, session.TimerObligationStateConsumed, rows[0].State)
		require.Equal(t, session.TimerObligationStateActive, rows[1].State)
	})

	t.Run("expiring_an_unknown_obligation_uuid_reports_not_found", func(t *testing.T) {
		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))

		_, err := m.ExpireTimer(context.Background(), session.TimerObligationUUID(uuid.NewString()))
		require.ErrorIs(t, err, session.ErrTimerObligationNotFound)
	})
}
