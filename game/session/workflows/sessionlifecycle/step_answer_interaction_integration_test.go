package sessionlifecycle

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// startedAnswerableSession seeds a LOBBY Session with one active host
// Participant, starts it against definition (expected to open a "Q"
// question at players[0], the host - answerableDefinition's/
// answerableDefinitionWithFatalAnswer's shape), and returns the resulting
// Manager, the Session's/host's identities, and the opened interaction's
// public UUID - the common setup every AnswerInteraction integration
// subtest below builds on.
func startedAnswerableSession(t *testing.T, db *gorm.DB, definition program.Definition) (m *Manager, sessionUUID SessionUUID, hostUUID UserUUID, interactionUUID InteractionUUID) {
	t.Helper()

	m = New(db, nil, stubStartPinnedGameReader{definition: definition})

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	hostUUIDStr := uuid.NewString()
	hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUIDStr)
	require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
	testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

	startResult, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUIDStr), IdempotencyKey(uuid.NewString()))
	require.NoError(t, err)
	require.Equal(t, StartOutcomeStarted, startResult.Outcome)

	var interactionUUIDStr string
	require.NoError(t, db.Raw(`
		SELECT uuid FROM session_interactions WHERE session_id = ? AND state = ?
	`, fx.SessionID, session.InteractionStateActive).Scan(&interactionUUIDStr).Error)
	require.NotEmpty(t, interactionUUIDStr, "Start's own first Turn must open and persist the Q interaction")

	return m, SessionUUID(fx.SessionUUID), UserUUID(hostUUIDStr), InteractionUUID(interactionUUIDStr)
}

func TestManagerAnswerInteraction_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	t.Run("accepted_answer_commits_second_turn_and_resolves_interaction", func(t *testing.T) {
		m, sessionUUID, hostUUID, interactionUUID := startedAnswerableSession(t, db, answerableDefinition(1, 4))

		result, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, numberAnswer(t, 42))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, result.Outcome)
		require.Equal(t, sessionUUID, result.SessionUUID)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "the response must commit a second RuntimeTurn")

		var row struct {
			State           string `gorm:"column:state"`
			ResponsePayload []byte `gorm:"column:response_payload"`
			ClosedByTurnID  *uint  `gorm:"column:closed_by_turn_id"`
		}
		require.NoError(t, db.Raw(`SELECT state, response_payload, closed_by_turn_id FROM session_interactions WHERE uuid = ?`, string(interactionUUID)).Scan(&row).Error)
		require.Equal(t, session.InteractionStateClosed, row.State)
		require.NotEmpty(t, row.ResponsePayload)
		require.NotNil(t, row.ClosedByTurnID)

		var currentTurnID, closedByTurnID uint
		require.NoError(t, db.Raw(`SELECT current_turn_id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&currentTurnID).Error)
		require.NoError(t, db.Raw(`SELECT closed_by_turn_id FROM session_interactions WHERE uuid = ?`, string(interactionUUID)).Scan(&closedByTurnID).Error)
		require.Equal(t, currentTurnID, closedByTurnID, "current_turn_id must advance to the Turn that closed the interaction")
	})

	t.Run("accepted_answer_returns_effect_and_updated_presentation_outputs", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: presentationEffectDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUIDStr := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUIDStr)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")
		playerTwoActorID := testfixtures.SeedActiveParticipant(t, db, fx.SessionID, uuid.NewString(), "Player Two")

		startResult, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUIDStr), IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, StartOutcomeStarted, startResult.Outcome)

		var interactionUUIDStr string
		require.NoError(t, db.Raw(`SELECT uuid FROM session_interactions WHERE session_id = ? AND state = ?`, fx.SessionID, session.InteractionStateActive).Scan(&interactionUUIDStr).Error)
		require.NotEmpty(t, interactionUUIDStr)

		result, err := m.AnswerInteraction(context.Background(), InteractionUUID(interactionUUIDStr), UserUUID(hostUUIDStr), numberAnswer(t, 42))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, result.Outcome)

		var effects []engine.EmitEffectOutput
		var updates []engine.UpdatePresentationOutput
		for _, o := range result.Outputs {
			switch v := o.(type) {
			case engine.EmitEffectOutput:
				effects = append(effects, v)
			case engine.UpdatePresentationOutput:
				updates = append(updates, v)
			default:
				t.Fatalf("unexpected Output kind returned: %T", o)
			}
		}
		require.Len(t, effects, 1, "one EmitEffectOutput addressed to both players via a single Recipients list")
		require.ElementsMatch(t, []engine.UserID{
			engine.UserID(strconv.FormatUint(uint64(hostActorID), 10)),
			engine.UserID(strconv.FormatUint(uint64(playerTwoActorID), 10)),
		}, effects[0].Recipients)
		require.Equal(t, presentationEffectName, effects[0].Effect)

		require.Len(t, updates, 2, "one UpdatePresentationOutput per mounted Hud presentation")
		for _, u := range updates {
			require.Equal(t, presentationEffectHudSlot, u.Slot)
			require.Equal(t, engine.NumberValue{Value: 42}, u.Model)
		}
	})

	t.Run("answer_completing_the_game_terminalizes_session_and_closes_bystander_interaction", func(t *testing.T) {
		m, sessionUUID, hostUUID, primaryInteractionUUID, bystanderInteractionUUID := startedTerminationControlSession(t, db, program.CompleteControl{Result: program.UnitLiteralExpression{}})

		result, err := m.AnswerInteraction(context.Background(), primaryInteractionUUID, hostUUID, numberAnswer(t, 1))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, result.Outcome)
		require.Equal(t, session.TerminalReasonGameCompleted, result.TerminalReason)

		var row struct {
			Phase          string  `gorm:"column:phase"`
			TerminalReason *string `gorm:"column:terminal_reason"`
		}
		require.NoError(t, db.Raw(`SELECT phase, terminal_reason FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&row).Error)
		require.Equal(t, session.PhaseTerminal, row.Phase)
		require.NotNil(t, row.TerminalReason)
		require.Equal(t, session.TerminalReasonGameCompleted, *row.TerminalReason)

		var bystanderState string
		require.NoError(t, db.Raw(`SELECT state FROM session_interactions WHERE uuid = ?`, string(bystanderInteractionUUID)).Scan(&bystanderState).Error)
		require.Equal(t, session.InteractionStateTerminated, bystanderState, "the bystander's still-ACTIVE interaction must close via terminal cleanup, not gameplay closure")
	})

	t.Run("retried_equivalent_answer_completing_the_game_still_reports_terminal_reason", func(t *testing.T) {
		m, _, hostUUID, primaryInteractionUUID, _ := startedTerminationControlSession(t, db, program.CompleteControl{Result: program.UnitLiteralExpression{}})

		first, err := m.AnswerInteraction(context.Background(), primaryInteractionUUID, hostUUID, numberAnswer(t, 1))
		require.NoError(t, err)
		require.Equal(t, session.TerminalReasonGameCompleted, first.TerminalReason)

		second, err := m.AnswerInteraction(context.Background(), primaryInteractionUUID, hostUUID, numberAnswer(t, 1))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, second.Outcome)
		require.Equal(t, session.TerminalReasonGameCompleted, second.TerminalReason, "a replayed retry of the game-ending answer must still report why the Session ended")
	})

	t.Run("answer_failing_the_game_terminalizes_session_with_game_failed_reason", func(t *testing.T) {
		m, sessionUUID, hostUUID, primaryInteractionUUID, _ := startedTerminationControlSession(t, db, program.FailControl{Error: program.StringLiteralExpression{Value: "unwinnable"}})

		result, err := m.AnswerInteraction(context.Background(), primaryInteractionUUID, hostUUID, numberAnswer(t, 1))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, result.Outcome)
		require.Equal(t, session.TerminalReasonGameFailed, result.TerminalReason)

		var row struct {
			Phase          string  `gorm:"column:phase"`
			TerminalReason *string `gorm:"column:terminal_reason"`
		}
		require.NoError(t, db.Raw(`SELECT phase, terminal_reason FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&row).Error)
		require.Equal(t, session.PhaseTerminal, row.Phase)
		require.Equal(t, session.TerminalReasonGameFailed, *row.TerminalReason)
	})

	t.Run("answer_cancelling_the_game_terminalizes_session_with_game_cancelled_reason", func(t *testing.T) {
		m, sessionUUID, hostUUID, primaryInteractionUUID, _ := startedTerminationControlSession(t, db, program.CancelControl{Reason: program.StringLiteralExpression{Value: "abandoned"}})

		result, err := m.AnswerInteraction(context.Background(), primaryInteractionUUID, hostUUID, numberAnswer(t, 1))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, result.Outcome)
		require.Equal(t, session.TerminalReasonGameCancelled, result.TerminalReason)

		var row struct {
			Phase          string  `gorm:"column:phase"`
			TerminalReason *string `gorm:"column:terminal_reason"`
		}
		require.NoError(t, db.Raw(`SELECT phase, terminal_reason FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&row).Error)
		require.Equal(t, session.PhaseTerminal, row.Phase)
		require.Equal(t, session.TerminalReasonGameCancelled, *row.TerminalReason)
	})

	t.Run("rejects_answer_from_non_recipient", func(t *testing.T) {
		m, sessionUUID, _, interactionUUID := startedAnswerableSession(t, db, answerableDefinition(1, 4))

		otherUUID := uuid.NewString()
		testfixtures.SeedActiveParticipant(t, db, sessionIDForUUID(t, db, sessionUUID), otherUUID, "Not Recipient")

		result, err := m.AnswerInteraction(context.Background(), interactionUUID, UserUUID(otherUUID), numberAnswer(t, 7))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeRejected, result.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "a rejected response must never commit a RuntimeTurn")
	})

	t.Run("reports_not_found_for_unknown_interaction_uuid", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: answerableDefinition(1, 4)})

		_, err := m.AnswerInteraction(context.Background(), InteractionUUID(uuid.NewString()), UserUUID(uuid.NewString()), numberAnswer(t, 1))
		require.ErrorIs(t, err, session.ErrInteractionNotFound)
	})

	t.Run("retried_equivalent_response_replays_without_second_engine_execution", func(t *testing.T) {
		m, sessionUUID, hostUUID, interactionUUID := startedAnswerableSession(t, db, answerableDefinition(1, 4))

		first, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, numberAnswer(t, 9))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, first.Outcome)

		var turnCountAfterFirst int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCountAfterFirst).Error)

		second, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, numberAnswer(t, 9))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, second.Outcome)

		var turnCountAfterSecond int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCountAfterSecond).Error)
		require.Equal(t, turnCountAfterFirst, turnCountAfterSecond, "an equivalent retry must not cause a second RuntimeTurn")
	})

	t.Run("conflicting_response_to_already_resolved_interaction_is_rejected", func(t *testing.T) {
		m, sessionUUID, hostUUID, interactionUUID := startedAnswerableSession(t, db, answerableDefinition(1, 4))

		first, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, numberAnswer(t, 3))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, first.Outcome)

		var turnCountAfterFirst int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCountAfterFirst).Error)

		second, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, numberAnswer(t, 4))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeConflict, second.Outcome)

		var turnCountAfterSecond int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCountAfterSecond).Error)
		require.Equal(t, turnCountAfterFirst, turnCountAfterSecond, "a conflicting response must never reach the engine")
	})

	t.Run("fatal_execution_failure_terminalizes_session_and_closes_active_interactions", func(t *testing.T) {
		m, sessionUUID, hostUUID, interactionUUID := startedAnswerableSession(t, db, answerableDefinitionWithFatalAnswer(1, 4))

		result, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, numberAnswer(t, 5))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeRuntimeExecutionFailed, result.Outcome)

		var row struct {
			Phase          string  `gorm:"column:phase"`
			TerminalReason *string `gorm:"column:terminal_reason"`
		}
		require.NoError(t, db.Raw(`SELECT phase, terminal_reason FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&row).Error)
		require.Equal(t, session.PhaseTerminal, row.Phase)
		require.NotNil(t, row.TerminalReason)
		require.Equal(t, session.TerminalReasonRuntimeExecutionFailed, *row.TerminalReason)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "a fatal failure must never persist a partial second RuntimeTurn")

		var interactionState string
		require.NoError(t, db.Raw(`SELECT state FROM session_interactions WHERE uuid = ?`, string(interactionUUID)).Scan(&interactionState).Error)
		require.Equal(t, session.InteractionStateTerminated, interactionState, "a fatal failure must close the still-ACTIVE interaction it was processing")
	})

	t.Run("accepted_answer_to_ask_group_question_resolves_interaction", func(t *testing.T) {
		m, sessionUUID, hostUUID, interactionUUID := startedAnswerableSession(t, db, askGroupAnswerableDefinition(1, 4))

		var kind string
		require.NoError(t, db.Raw(`SELECT kind FROM session_interactions WHERE uuid = ?`, string(interactionUUID)).Scan(&kind).Error)
		require.Equal(t, session.InteractionKindAskGroup, kind)

		result, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, numberAnswer(t, 21))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, result.Outcome)
		require.Equal(t, sessionUUID, result.SessionUUID)

		var row struct {
			State           string `gorm:"column:state"`
			ResponsePayload []byte `gorm:"column:response_payload"`
		}
		require.NoError(t, db.Raw(`SELECT state, response_payload FROM session_interactions WHERE uuid = ?`, string(interactionUUID)).Scan(&row).Error)
		require.Equal(t, session.InteractionStateClosed, row.State)
		require.NotEmpty(t, row.ResponsePayload)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "the response must commit a second RuntimeTurn")
	})

	t.Run("closing_a_different_pending_slot_while_answering_is_captured", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: dualQuestionDefinition(1, 4)})

		fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
		hostUUID := uuid.NewString()
		hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
		require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
		testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

		startResult, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), IdempotencyKey(uuid.NewString()))
		require.NoError(t, err)
		require.Equal(t, StartOutcomeStarted, startResult.Outcome)

		// dualQuestionDefinition's "Started" transition opens Q1 (dualPrimarySlot)
		// then Q2 (dualSecondarySlot) as two sequential OpenQuestionOutputs
		// within the same Step, so captureInteractions persists them in that
		// same order - the first-created row is Q1, the second is Q2.
		var interactionUUIDs []string
		require.NoError(t, db.Raw(`SELECT uuid FROM session_interactions WHERE session_id = ? ORDER BY id ASC`, fx.SessionID).Scan(&interactionUUIDs).Error)
		require.Len(t, interactionUUIDs, 2, "Start must open both Q1 and Q2")
		primaryUUID, secondaryUUID := interactionUUIDs[0], interactionUUIDs[1]
		require.NotEmpty(t, primaryUUID)
		require.NotEmpty(t, secondaryUUID)

		result, err := m.AnswerInteraction(context.Background(), InteractionUUID(primaryUUID), UserUUID(hostUUID), numberAnswer(t, 5))
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, result.Outcome)

		var primaryRow struct {
			State           string `gorm:"column:state"`
			ResponsePayload []byte `gorm:"column:response_payload"`
			ClosedByTurnID  *uint  `gorm:"column:closed_by_turn_id"`
		}
		require.NoError(t, db.Raw(`SELECT state, response_payload, closed_by_turn_id FROM session_interactions WHERE uuid = ?`, primaryUUID).Scan(&primaryRow).Error)
		require.Equal(t, session.InteractionStateClosed, primaryRow.State)
		require.NotEmpty(t, primaryRow.ResponsePayload)
		require.NotNil(t, primaryRow.ClosedByTurnID)

		var secondaryRow struct {
			State           string `gorm:"column:state"`
			ResponsePayload []byte `gorm:"column:response_payload"`
			ClosedByTurnID  *uint  `gorm:"column:closed_by_turn_id"`
		}
		require.NoError(t, db.Raw(`SELECT state, response_payload, closed_by_turn_id FROM session_interactions WHERE uuid = ?`, secondaryUUID).Scan(&secondaryRow).Error)
		require.Equal(t, session.InteractionStateClosed, secondaryRow.State, "the authored CloseQuestionOperation's CloseQuestionOutput must be captured generically")
		require.Empty(t, secondaryRow.ResponsePayload, "Q2 was never answered, only closed")
		require.Equal(t, primaryRow.ClosedByTurnID, secondaryRow.ClosedByTurnID, "both closures happen in the same RuntimeTurn")
	})

	t.Run("two_concurrent_answers_to_the_same_interaction_never_both_execute_a_runtime_turn", func(t *testing.T) {
		m, sessionUUID, hostUUID, interactionUUID := startedAnswerableSession(t, db, answerableDefinition(1, 4))

		type outcome struct {
			result AnswerInteractionResult
			err    error
		}
		results := make(chan outcome, 2)
		answer := func() {
			result, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, numberAnswer(t, 11))
			results <- outcome{result: result, err: err}
		}
		go answer()
		go answer()

		first := <-results
		second := <-results
		require.NoError(t, first.err)
		require.NoError(t, second.err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, first.result.Outcome)
		require.Equal(t, AnswerInteractionOutcomeAnswered, second.result.Outcome, "the loser must replay the winner's outcome, not fail or reject")

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(2), turnCount, "exactly one RuntimeTurn must ever be produced for this response, on top of Start's own first Turn")
	})
}

// sessionIDForUUID resolves sessionUUID's internal id, for assertions that
// need to query by it directly.
func sessionIDForUUID(t *testing.T, db *gorm.DB, sessionUUID SessionUUID) uint {
	t.Helper()
	var id uint
	require.NoError(t, db.Raw(`SELECT id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&id).Error)
	return id
}
