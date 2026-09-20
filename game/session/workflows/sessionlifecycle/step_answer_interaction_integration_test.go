package sessionlifecycle

import (
	"context"
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

		result, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, engine.NumberValue{Value: 42})
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

	t.Run("rejects_answer_from_non_recipient", func(t *testing.T) {
		m, sessionUUID, _, interactionUUID := startedAnswerableSession(t, db, answerableDefinition(1, 4))

		otherUUID := uuid.NewString()
		testfixtures.SeedActiveParticipant(t, db, sessionIDForUUID(t, db, sessionUUID), otherUUID, "Not Recipient")

		result, err := m.AnswerInteraction(context.Background(), interactionUUID, UserUUID(otherUUID), engine.NumberValue{Value: 7})
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeRejected, result.Outcome)

		var turnCount int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCount).Error)
		require.Equal(t, int64(1), turnCount, "a rejected response must never commit a RuntimeTurn")
	})

	t.Run("reports_not_found_for_unknown_interaction_uuid", func(t *testing.T) {
		m := New(db, nil, stubStartPinnedGameReader{definition: answerableDefinition(1, 4)})

		_, err := m.AnswerInteraction(context.Background(), InteractionUUID(uuid.NewString()), UserUUID(uuid.NewString()), engine.NumberValue{Value: 1})
		require.ErrorIs(t, err, session.ErrInteractionNotFound)
	})

	t.Run("retried_equivalent_response_replays_without_second_engine_execution", func(t *testing.T) {
		m, sessionUUID, hostUUID, interactionUUID := startedAnswerableSession(t, db, answerableDefinition(1, 4))

		first, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, engine.NumberValue{Value: 9})
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, first.Outcome)

		var turnCountAfterFirst int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCountAfterFirst).Error)

		second, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, engine.NumberValue{Value: 9})
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, second.Outcome)

		var turnCountAfterSecond int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCountAfterSecond).Error)
		require.Equal(t, turnCountAfterFirst, turnCountAfterSecond, "an equivalent retry must not cause a second RuntimeTurn")
	})

	t.Run("conflicting_response_to_already_resolved_interaction_is_rejected", func(t *testing.T) {
		m, sessionUUID, hostUUID, interactionUUID := startedAnswerableSession(t, db, answerableDefinition(1, 4))

		first, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, engine.NumberValue{Value: 3})
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeAnswered, first.Outcome)

		var turnCountAfterFirst int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCountAfterFirst).Error)

		second, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, engine.NumberValue{Value: 4})
		require.NoError(t, err)
		require.Equal(t, AnswerInteractionOutcomeConflict, second.Outcome)

		var turnCountAfterSecond int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionIDForUUID(t, db, sessionUUID)).Scan(&turnCountAfterSecond).Error)
		require.Equal(t, turnCountAfterFirst, turnCountAfterSecond, "a conflicting response must never reach the engine")
	})

	t.Run("fatal_execution_failure_terminalizes_session_and_closes_active_interactions", func(t *testing.T) {
		m, sessionUUID, hostUUID, interactionUUID := startedAnswerableSession(t, db, answerableDefinitionWithFatalAnswer(1, 4))

		result, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, engine.NumberValue{Value: 5})
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
}

// sessionIDForUUID resolves sessionUUID's internal id, for assertions that
// need to query by it directly.
func sessionIDForUUID(t *testing.T, db *gorm.DB, sessionUUID SessionUUID) uint {
	t.Helper()
	var id uint
	require.NoError(t, db.Raw(`SELECT id FROM sessions WHERE uuid = ?`, string(sessionUUID)).Scan(&id).Error)
	return id
}
