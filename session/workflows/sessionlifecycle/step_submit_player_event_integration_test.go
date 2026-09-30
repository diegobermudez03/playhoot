package sessionlifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/executor"
	"github.com/diegobermudez03/playhoot/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/platform"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// startPassthroughThenExec builds an executor.Fake that returns startState
// unconditionally for the SESSION_STARTED call Start makes, then delegates
// to afterStart for every subsequent call - letting a test control
// SubmitPlayerEvent's own Execute outcome independently of Start's.
func startPassthroughThenExec(t *testing.T, startState json.RawMessage, afterStart func(in executor.ExecutionInput) (executor.ExecutionOutput, error)) *executor.Fake {
	return &executor.Fake{
		ExecuteFunc: func(ctx context.Context, in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			if eventKind(t, in.Event) == platform.EventKindSessionStarted {
				return executor.ExecutionOutput{NewState: startState}, nil
			}
			return afterStart(in)
		},
	}
}

func TestManagerSubmitPlayerEvent_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	t.Run("accepted_event_commits_second_turn_with_new_state_and_cause_event", func(t *testing.T) {
		nextState := stateJSON(t, map[string]any{"guesses": []any{42}})
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: nextState}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", stateJSON(t, map[string]any{"value": float64(42)}), session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, result.Outcome)
		require.Equal(t, sessionUUID, result.SessionUUID)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "the event must commit a second RuntimeTurn")

		var row struct {
			SourceKind         string `gorm:"column:source_kind"`
			SourceCauseEventID *uint  `gorm:"column:source_cause_event_id"`
			ActorID            *uint  `gorm:"column:actor_id"`
			NewState           []byte `gorm:"column:new_state"`
		}
		require.NoError(t, db.Raw(`
			SELECT source_kind, source_cause_event_id, actor_id, new_state
			FROM session_runtime_turns
			WHERE session_id = ?
			ORDER BY sequence DESC LIMIT 1
		`, sessionIDForUUID(t, db, sessionUUID)).Scan(&row).Error)
		require.Equal(t, playerEventSourceKind, row.SourceKind)
		require.NotNil(t, row.SourceCauseEventID, "the Turn's own source_cause_event_id must be backfilled")
		require.NotNil(t, row.ActorID)
		require.JSONEq(t, string(nextState), string(row.NewState), "no silent drift between what was persisted and what the Executor returned")

		var causeEventCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_cause_events WHERE runtime_turn_id = (SELECT id FROM session_runtime_turns WHERE session_id = ? ORDER BY sequence DESC LIMIT 1)`, sessionIDForUUID(t, db, sessionUUID)).Scan(&causeEventCount).Error)
		require.Equal(t, int64(1), causeEventCount)
	})

	t.Run("accepted_event_renews_activity_deadline", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		before := activityExpiresAtForUUID(t, db, sessionUUID)
		require.NotNil(t, before)

		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, result.Outcome)

		after := activityExpiresAtForUUID(t, db, sessionUUID)
		require.NotNil(t, after)
		require.True(t, after.After(*before), "a Turn-committing event must renew activity_expires_at")
	})

	t.Run("accepted_event_requesting_send_event_returns_it_in_memory_in_order", func(t *testing.T) {
		var hostRef string
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			payload1 := stateJSON(t, map[string]any{"n": float64(1)})
			payload2 := stateJSON(t, map[string]any{"n": float64(2)})
			return executor.ExecutionOutput{
				NewState: stateJSON(t, map[string]any{}),
				RequestedCommands: []json.RawMessage{
					sendEventCommand(t, []string{hostRef}, "first", payload1),
					sendEventCommand(t, []string{hostRef}, "second", payload2),
				},
			}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		hostRef = string(actorRefForActorID(actorIDForUUID(t, db, sessionUUID, hostUUID)))

		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, result.Outcome)
		require.Equal(t, []session.OutboundEvent{
			{ID: "2:0", Recipients: []session.ActorRef{session.ActorRef(hostRef)}, Name: "first", Payload: stateJSON(t, map[string]any{"n": float64(1)}), Revision: 2},
			{ID: "2:1", Recipients: []session.ActorRef{session.ActorRef(hostRef)}, Name: "second", Payload: stateJSON(t, map[string]any{"n": float64(2)}), Revision: 2},
		}, result.Events)
	})

	t.Run("retried_submission_with_same_idempotency_key_returns_nil_events_on_replay", func(t *testing.T) {
		var hostRef string
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{
				NewState:          stateJSON(t, map[string]any{}),
				RequestedCommands: []json.RawMessage{sendEventCommand(t, []string{hostRef}, "first", stateJSON(t, map[string]any{}))},
			}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		hostRef = string(actorRefForActorID(actorIDForUUID(t, db, sessionUUID, hostUUID)))
		key := session.IdempotencyKey(uuid.NewString())

		first, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, key)
		require.NoError(t, err)
		require.NotEmpty(t, first.Events, "the original call must have actually collected the requested event")

		second, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, key)
		require.NoError(t, err)
		require.Nil(t, second.Events, "a replayed idempotency claim must never reconstruct/re-emit the original call's transient events")
	})

	t.Run("stale_activity_deadline_materializes_inactivity_expiration_and_declines", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		staleDeadline := time.Now().Add(-1 * time.Minute).UTC()
		require.NoError(t, db.Exec(`UPDATE sessions SET activity_expires_at = ? WHERE uuid = ?`, staleDeadline, string(sessionUUID)).Error)

		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeRejected, result.Outcome, "a materialized inactivity expiration leaves the existing non-RUNNING decline path to apply")

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

	t.Run("retried_submission_with_same_idempotency_key_replays_without_a_second_turn", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		key := session.IdempotencyKey(uuid.NewString())

		first, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, key)
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, first.Outcome)

		second, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, key)
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, second.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "a retried same-key submission must not commit a second Turn")
	})

	t.Run("conflicting_retry_with_same_idempotency_key_is_rejected", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		key := session.IdempotencyKey(uuid.NewString())

		_, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, key)
		require.NoError(t, err)

		_, err = m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", stateJSON(t, map[string]any{"value": float64(1)}), key)
		require.ErrorIs(t, err, session.ErrIdempotencyConflict)
	})

	t.Run("script_rejects_event_without_committing_a_turn", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{}, &executor.ScriptRejectedError{Reason: "no matching reaction"}
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Unknown", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeRejected, result.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "only Start's own Turn - no Turn from a rejected event")
	})

	t.Run("unresolvable_actor_is_rejected", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, _ := startedSessionWithExecutor(t, db, exec)

		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, session.UserUUID(uuid.NewString()), "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeRejected, result.Outcome)
	})

	t.Run("session_complete_command_terminalizes_the_session", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{"done": true}), RequestedCommands: []json.RawMessage{sessionCompleteCommand(t)}}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, result.Outcome)
		require.Equal(t, session.TerminalReasonGameCompleted, result.TerminalReason)

		var phase string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)

		var failureCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_failures WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&failureCount).Error)
		require.Zero(t, failureCount, "an authored completion outcome is not a runtime failure")
	})

	t.Run("executor_error_terminalizes_the_session_and_replays_without_re_executing", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{}, &executor.ExecutorError{Reason: "executor unreachable"}
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		key := session.IdempotencyKey(uuid.NewString())

		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, key)
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeRuntimeExecutionFailed, result.Outcome)

		var phase, terminalReason string
		require.NoError(t, db.Raw(`SELECT phase FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&phase).Error)
		require.Equal(t, session.PhaseTerminal, phase)
		require.NoError(t, db.Raw(`SELECT terminal_reason FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&terminalReason).Error)
		require.Equal(t, session.TerminalReasonRuntimeExecutionFailed, terminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "a fatal execution must not persist a partial RuntimeTurn - only Start's own Turn remains")

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
			FROM session_runtime_failures WHERE session_id = ?
		`, sessionIDForUUID(t, db, sessionUUID)).Scan(&failureRow).Error)
		require.Equal(t, RuntimeFailureKindExecution, failureRow.FailureKind)
		require.NotNil(t, failureRow.BaseTurnID, "Start's own Turn already committed before this fatal failure")
		require.Equal(t, uint64(2), failureRow.AttemptedSequence)
		require.Equal(t, playerEventSourceKind, failureRow.SourceKind)
		require.NotNil(t, failureRow.ActorID)

		result2, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, key)
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeRuntimeExecutionFailed, result2.Outcome)

		result3, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeRejected, result3.Outcome, "a genuinely new submission against an already-TERMINAL Session must be declined by the phase check, never re-attempt execution")

		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "no further Turn may ever commit against a TERMINAL Session")
	})

	t.Run("schedule_timer_command_persists_an_active_obligation", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{}), RequestedCommands: []json.RawMessage{scheduleTimerCommand(t, "round_timer", 5000)}}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, result.Outcome)

		var row struct {
			Timer   string `gorm:"column:timer"`
			DelayMs int64  `gorm:"column:delay_ms"`
			State   string `gorm:"column:state"`
		}
		require.NoError(t, db.Raw(`SELECT timer, delay_ms, state FROM session_timer_obligations WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&row).Error)
		require.Equal(t, "round_timer", row.Timer)
		require.Equal(t, int64(5000), row.DelayMs)
		require.Equal(t, session.TimerObligationStateActive, row.State)
	})

	t.Run("cancel_timer_command_cancels_the_matching_active_obligation", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{}), RequestedCommands: []json.RawMessage{scheduleTimerCommand(t, "round_timer", 5000)}}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		_, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)

		exec.ExecuteFunc = func(ctx context.Context, in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{}), RequestedCommands: []json.RawMessage{cancelTimerCommand(t, "round_timer")}}, nil
		}
		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, result.Outcome)

		var state string
		require.NoError(t, db.Raw(`SELECT state FROM session_timer_obligations WHERE session_id = ? AND timer = ?`, sessionIDForUUID(t, db, sessionUUID), "round_timer").Scan(&state).Error)
		require.Equal(t, session.TimerObligationStateCancelled, state)
	})

	t.Run("cancel_timer_command_with_no_matching_obligation_is_a_no_op", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{}), RequestedCommands: []json.RawMessage{cancelTimerCommand(t, "never_scheduled")}}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, result.Outcome, "a CANCEL_TIMER with nothing to cancel must not be rejected")

		var count int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_timer_obligations WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&count).Error)
		require.Equal(t, int64(0), count)
	})

	t.Run("rescheduling_an_already_active_timer_replaces_it", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{}), RequestedCommands: []json.RawMessage{scheduleTimerCommand(t, "round_timer", 5000)}}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		_, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)

		exec.ExecuteFunc = func(ctx context.Context, in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{}), RequestedCommands: []json.RawMessage{scheduleTimerCommand(t, "round_timer", 9000)}}, nil
		}
		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, result.Outcome)

		var rows []struct {
			DelayMs int64  `gorm:"column:delay_ms"`
			State   string `gorm:"column:state"`
		}
		require.NoError(t, db.Raw(`SELECT delay_ms, state FROM session_timer_obligations WHERE session_id = ? AND timer = ? ORDER BY id`, sessionIDForUUID(t, db, sessionUUID), "round_timer").Scan(&rows).Error)
		require.Len(t, rows, 2, "the old row is cancelled, not deleted, and a new row is created")
		require.Equal(t, session.TimerObligationStateCancelled, rows[0].State)
		require.Equal(t, int64(5000), rows[0].DelayMs)
		require.Equal(t, session.TimerObligationStateActive, rows[1].State)
		require.Equal(t, int64(9000), rows[1].DelayMs)
	})

	t.Run("a_turn_that_both_schedules_a_timer_and_completes_leaves_no_active_obligation", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{
				NewState:          stateJSON(t, map[string]any{}),
				RequestedCommands: []json.RawMessage{scheduleTimerCommand(t, "round_timer", 5000), sessionCompleteCommand(t)},
			}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		result, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, result.Outcome)
		require.Equal(t, session.TerminalReasonGameCompleted, result.TerminalReason)

		var count int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_timer_obligations WHERE session_id = ? AND state = ?`, sessionIDForUUID(t, db, sessionUUID), session.TimerObligationStateActive).Scan(&count).Error)
		require.Equal(t, int64(0), count, "terminal cleanup must cancel the obligation the same Turn just created")
	})

	t.Run("session_not_found", func(t *testing.T) {
		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))
		_, err := m.SubmitPlayerEvent(context.Background(), session.SessionUUID(uuid.NewString()), session.UserUUID(uuid.NewString()), "Guess", nil, session.IdempotencyKey(uuid.NewString()))
		require.ErrorIs(t, err, session.ErrSessionNotFound)
	})
}

func TestManagerGetSubmitPlayerEventOutcome_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	// fault_injection_recovers_a_dropped_accepted_response: SubmitPlayerEvent
	// already durably commits its own outcome the moment its transaction
	// commits - this test proves that a caller who never actually used that
	// first return value (modeling a response lost after commit, before the
	// client ever saw it) can still recover the exact same outcome
	// afterward, without triggering a second execution.
	t.Run("fault_injection_recovers_a_dropped_accepted_response", func(t *testing.T) {
		nextState := stateJSON(t, map[string]any{"guesses": []any{42}})
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: nextState}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		key := session.IdempotencyKey(uuid.NewString())

		original, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", stateJSON(t, map[string]any{"value": float64(42)}), key)
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeAccepted, original.Outcome, "the original call must have actually committed, for this fault-injection scenario to mean anything")
		// original is deliberately not consulted again below - modeling the
		// caller never having received it.

		recovered, err := m.GetSubmitPlayerEventOutcome(context.Background(), sessionUUID, hostUUID, key)
		require.NoError(t, err)
		require.Equal(t, session.GetSubmitPlayerEventOutcomeFound, recovered.Outcome)
		require.Equal(t, original, recovered.Result, "the recovered outcome must exactly match what the original call itself committed")

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "reading the outcome back must never re-execute or commit a further Turn")
	})

	t.Run("recovers_a_rejected_outcome", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{}, &executor.ScriptRejectedError{Reason: "no reaction"}
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)
		key := session.IdempotencyKey(uuid.NewString())

		original, err := m.SubmitPlayerEvent(context.Background(), sessionUUID, hostUUID, "Guess", nil, key)
		require.NoError(t, err)
		require.Equal(t, session.SubmitPlayerEventOutcomeRejected, original.Outcome)

		recovered, err := m.GetSubmitPlayerEventOutcome(context.Background(), sessionUUID, hostUUID, key)
		require.NoError(t, err)
		require.Equal(t, session.GetSubmitPlayerEventOutcomeFound, recovered.Outcome)
		require.Equal(t, session.SubmitPlayerEventOutcomeRejected, recovered.Result.Outcome)
	})

	t.Run("not_found_for_a_key_that_was_never_submitted", func(t *testing.T) {
		exec := startPassthroughThenExec(t, stateJSON(t, map[string]any{}), func(in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: stateJSON(t, map[string]any{})}, nil
		})
		m, sessionUUID, hostUUID := startedSessionWithExecutor(t, db, exec)

		result, err := m.GetSubmitPlayerEventOutcome(context.Background(), sessionUUID, hostUUID, session.IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, session.GetSubmitPlayerEventOutcomeNotFound, result.Outcome)
	})

	// The "a key completed under a different session must not leak its
	// outcome here" defensive branch is covered precisely at the unit level
	// (TestManagerGetSubmitPlayerEventOutcome's own
	// not_found_when_request_belongs_to_a_different_session case) - building
	// it here would require two real Sessions sharing one host identity
	// purely to exercise one already-precisely-covered branch, disproportionate
	// to what a real-Postgres test needs to additionally prove.

	t.Run("session_not_found", func(t *testing.T) {
		m := New(db, fakeExecutorAlwaysReturning(stateJSON(t, map[string]any{})))
		_, err := m.GetSubmitPlayerEventOutcome(context.Background(), session.SessionUUID(uuid.NewString()), session.UserUUID(uuid.NewString()), session.IdempotencyKey(uuid.NewString()))
		require.ErrorIs(t, err, session.ErrSessionNotFound)
	})
}
