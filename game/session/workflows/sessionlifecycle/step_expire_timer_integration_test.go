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
)

// timerObligationRow is this file's own minimal projection of a
// session_timer_obligations row, for assertions.
type timerObligationRow struct {
	UUID            string  `gorm:"column:uuid"`
	EngineSlot      string  `gorm:"column:engine_slot"`
	EngineKey       []byte  `gorm:"column:engine_key"`
	DelayMs         int64   `gorm:"column:delay_ms"`
	State           string  `gorm:"column:state"`
	ClosedByTurnID  *uint   `gorm:"column:closed_by_turn_id"`
	CreatedByTurnID uint    `gorm:"column:created_by_turn_id"`
	ClosureReason   *string `gorm:"column:closure_reason"`
}

func TestManagerExpireTimer_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	t.Run("scheduled_timer_obligation_is_durably_persisted_at_start", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: timerDefinition(1, 4, 5000)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		result, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, StartOutcomeStarted, result.Outcome)

		var row timerObligationRow
		require.NoError(t, db.Raw(`SELECT uuid, engine_slot, engine_key, delay_ms, state, created_by_turn_id, closed_by_turn_id FROM session_timer_obligations WHERE session_id = ?`, fx.SessionID).Scan(&row).Error)
		require.NotEmpty(t, row.UUID)
		require.Equal(t, timerSlotName, row.EngineSlot)
		require.Nil(t, row.EngineKey)
		require.Equal(t, int64(5000), row.DelayMs)
		require.Equal(t, session.TimerObligationStateActive, row.State)
		require.Nil(t, row.ClosedByTurnID)
	})

	t.Run("expiring_a_scheduled_timer_drives_a_new_runtime_turn_and_consumes_it", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: timerDefinition(1, 4, 5000)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		startResult, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, StartOutcomeStarted, startResult.Outcome)

		var obligationUUID string
		require.NoError(t, db.Raw(`SELECT uuid FROM session_timer_obligations WHERE session_id = ?`, fx.SessionID).Scan(&obligationUUID).Error)
		require.NotEmpty(t, obligationUUID)

		result, err := m.ExpireTimer(context.Background(), TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, ExpireTimerOutcomeExpired, result.Outcome)
		require.Equal(t, SessionUUID(fx.SessionUUID), result.SessionUUID)
		require.Len(t, result.Outputs, 1)
		effect, ok := result.Outputs[0].(EffectEmitted)
		require.True(t, ok, "%T", result.Outputs[0])
		require.Equal(t, timerEffectName, effect.Effect)

		var obligationRow timerObligationRow
		require.NoError(t, db.Raw(`SELECT state, closed_by_turn_id FROM session_timer_obligations WHERE uuid = ?`, obligationUUID).Scan(&obligationRow).Error)
		require.Equal(t, session.TimerObligationStateConsumed, obligationRow.State)
		require.NotNil(t, obligationRow.ClosedByTurnID)

		var turnRow struct {
			SourceKind              string `gorm:"column:source_kind"`
			SourceTimerObligationID *uint  `gorm:"column:source_timer_obligation_id"`
			Sequence                uint64 `gorm:"column:sequence"`
		}
		require.NoError(t, db.Raw(`SELECT source_kind, source_timer_obligation_id, sequence FROM session_runtime_turns WHERE id = ?`, obligationRow.ClosedByTurnID).Scan(&turnRow).Error)
		require.Equal(t, "TIMER_EXPIRED", turnRow.SourceKind)
		require.NotNil(t, turnRow.SourceTimerObligationID)
		require.Equal(t, uint64(2), turnRow.Sequence, "Start's own Turn is sequence 1")

		var currentTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE id = ?`, fx.SessionID).Scan(&currentTurnID).Error)
		require.Equal(t, *obligationRow.ClosedByTurnID, currentTurnID)
	})

	t.Run("expiring_an_already_consumed_obligation_is_stale_and_creates_no_second_turn", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: timerDefinition(1, 4, 5000)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		_, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)

		var obligationUUID string
		require.NoError(t, db.Raw(`SELECT uuid FROM session_timer_obligations WHERE session_id = ?`, fx.SessionID).Scan(&obligationUUID).Error)

		first, err := m.ExpireTimer(context.Background(), TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, ExpireTimerOutcomeExpired, first.Outcome)

		var turnCountAfterFirst int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turnCountAfterFirst).Error)

		second, err := m.ExpireTimer(context.Background(), TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, ExpireTimerOutcomeStale, second.Outcome)
		require.Empty(t, second.Outputs)

		var turnCountAfterSecond int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turnCountAfterSecond).Error)
		require.Equal(t, turnCountAfterFirst, turnCountAfterSecond, "a stale delivery must not create a second RuntimeTurn")
	})

	t.Run("expiring_an_unknown_obligation_uuid_reports_not_found", func(t *testing.T) {
		m := New(db, nil, nil)

		_, err := m.ExpireTimer(context.Background(), TimerObligationUUID(uuid.NewString()))
		require.ErrorIs(t, err, session.ErrTimerObligationNotFound)
	})

	t.Run("keyed_timers_under_the_same_slot_are_independent", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: keyedTimerDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		result, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, StartOutcomeStarted, result.Outcome)

		var rows []timerObligationRow
		require.NoError(t, db.Raw(`SELECT uuid, engine_key, state FROM session_timer_obligations WHERE session_id = ? ORDER BY id ASC`, fx.SessionID).Scan(&rows).Error)
		require.Len(t, rows, 2, "two independent keyed timer obligations must coexist under the same slot")
		require.NotEqual(t, rows[0].EngineKey, rows[1].EngineKey)
		for _, r := range rows {
			require.Equal(t, session.TimerObligationStateActive, r.State)
		}

		expireResult, err := m.ExpireTimer(context.Background(), TimerObligationUUID(rows[0].UUID))
		require.NoError(t, err)
		require.Equal(t, ExpireTimerOutcomeExpired, expireResult.Outcome)
		require.Len(t, expireResult.Outputs, 1)
		effect, ok := expireResult.Outputs[0].(EffectEmitted)
		require.True(t, ok, "%T", expireResult.Outputs[0])
		require.Equal(t, keyedTimerEffectName, effect.Effect)
		require.Len(t, effect.Arguments, 1)
		keyValue, ok := effect.Arguments[0].Value.(StringValue)
		require.True(t, ok)
		require.Contains(t, []string{"P0", "P1"}, keyValue.Value)

		// The other key's own obligation must remain untouched.
		var otherState string
		require.NoError(t, db.Raw(`SELECT state FROM session_timer_obligations WHERE uuid = ?`, rows[1].UUID).Scan(&otherState).Error)
		require.Equal(t, session.TimerObligationStateActive, otherState)
	})

	t.Run("cancelling_a_timer_marks_it_cancelled_and_a_later_expiry_is_stale", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: timerCancelledOnAnswerDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		_, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)

		var obligationUUID string
		require.NoError(t, db.Raw(`SELECT uuid FROM session_timer_obligations WHERE session_id = ?`, fx.SessionID).Scan(&obligationUUID).Error)
		var interactionUUID string
		require.NoError(t, db.Raw(`SELECT uuid FROM session_interactions WHERE session_id = ?`, fx.SessionID).Scan(&interactionUUID).Error)

		answerResult, err := m.AnswerInteraction(context.Background(), InteractionUUID(interactionUUID), UserUUID(hostUUID), numberAnswer(t, 1))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, answerResult.Outcome)

		var obligationRow timerObligationRow
		require.NoError(t, db.Raw(`SELECT state, closed_by_turn_id, closure_reason FROM session_timer_obligations WHERE uuid = ?`, obligationUUID).Scan(&obligationRow).Error)
		require.Equal(t, session.TimerObligationStateCancelled, obligationRow.State)
		require.NotNil(t, obligationRow.ClosedByTurnID, "cancelled by the answer's own Turn, not terminal cleanup")
		require.Nil(t, obligationRow.ClosureReason, "an authored cancel is not a terminal-cleanup closure")

		expireResult, err := m.ExpireTimer(context.Background(), TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, ExpireTimerOutcomeStale, expireResult.Outcome)
	})

	t.Run("scheduling_into_an_already_occupied_slot_fails_atomically_and_persists_no_obligation", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: doubleScheduleTimerDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		result, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, StartOutcomeRuntimeInitFailed, result.Outcome, "scheduling into an occupied slot is an engine execution error")

		var obligationCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_timer_obligations WHERE session_id = ?`, fx.SessionID).Scan(&obligationCount).Error)
		require.Zero(t, obligationCount, "the whole failed transition must leave no timer obligation behind")

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, fx.SessionID).Scan(&turnCount).Error)
		require.Zero(t, turnCount)
	})

	t.Run("session_terminating_cancels_a_still_active_timer_obligation", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: timerActiveAtTerminationDefinition(program.CompleteControl{Result: program.UnitLiteralExpression{}}, 1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		_, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)

		var obligationUUID string
		require.NoError(t, db.Raw(`SELECT uuid FROM session_timer_obligations WHERE session_id = ?`, fx.SessionID).Scan(&obligationUUID).Error)
		var interactionUUID string
		require.NoError(t, db.Raw(`SELECT uuid FROM session_interactions WHERE session_id = ?`, fx.SessionID).Scan(&interactionUUID).Error)

		answerResult, err := m.AnswerInteraction(context.Background(), InteractionUUID(interactionUUID), UserUUID(hostUUID), numberAnswer(t, 1))
		require.NoError(t, err)
		require.Equal(t, session.TerminalReasonGameCompleted, answerResult.TerminalReason)

		var obligationRow timerObligationRow
		require.NoError(t, db.Raw(`SELECT state, closed_by_turn_id, closure_reason FROM session_timer_obligations WHERE uuid = ?`, obligationUUID).Scan(&obligationRow).Error)
		require.Equal(t, session.TimerObligationStateCancelled, obligationRow.State)
		require.Nil(t, obligationRow.ClosedByTurnID, "terminal cleanup, not gameplay closure - mirrors session_interactions' own TERMINATED convention")
		require.NotNil(t, obligationRow.ClosureReason)
		require.Equal(t, session.TimerObligationClosureReasonSessionTerminated, *obligationRow.ClosureReason)

		expireResult, err := m.ExpireTimer(context.Background(), TimerObligationUUID(obligationUUID))
		require.NoError(t, err)
		require.Equal(t, ExpireTimerOutcomeStale, expireResult.Outcome, "a still-in-flight physical timer firing after the Session already terminated must be a harmless stale decline")
	})
}
