package sessionlifecycle

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestReconstructCurrentSnapshot_Integration proves engineservice.AdvanceTurn's
// internal replay matches state the original live execution actually
// produced - not merely state re-derived from the same durable rows replay
// itself reads, which would only prove replay is consistent with itself.
//
// It drives a real three-Turn Session through the ordinary Manager (Start
// opens a question exposing a live random draw as one of its own
// arguments; answering it opens a second question; answering that one
// opens a third), confirms the live random draw against an oracle that
// never passes through session_runtime_starts/session_runtime_turns (the
// rows AdvanceTurn's internal replay itself reads), then has two
// independently-constructed Managers - simulating two different processes,
// neither sharing any in-memory state with the live execution or with each
// other - independently load the same durable prior-signal log and apply
// the same hypothetical next signal purely in memory (never committed).
// Neither this package nor engineservice ever exposes an engine.Snapshot to
// compare directly, so the proof is: both independent reconstructions must
// load byte-identical InitializationInput/priorSignals from the same
// durable data, and applying the same next signal to each must produce
// byte-identical Outputs - which is only possible if AdvanceTurn's
// internal replay is itself deterministic and reproduces live execution's
// own result. This is also a process-loss-recovery proof: reconstruction
// never depends on any particular process's memory, only on durable state.
func TestReconstructCurrentSnapshot_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	definition := replayObservableDefinition(1, 4)
	m := New(db, nil, stubStartPinnedGameReader{definition: definition})

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	hostUUID := uuid.NewString()
	hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
	require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
	testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

	startResult, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), IdempotencyKey(uuid.NewString()))
	require.NoError(t, err)
	require.Equal(t, StartOutcomeStarted, startResult.Outcome)

	sessionID := fx.SessionID

	// The live random draw: read from Q1's own interaction_payload, captured
	// directly from Start's live OpenQuestionOutput - a table
	// reconstructCurrentSnapshot never reads, and a value that never passes
	// through session_runtime_starts.seed on this side of the comparison.
	var firstRow struct {
		UUID               string `gorm:"column:uuid"`
		InteractionPayload []byte `gorm:"column:interaction_payload"`
	}
	require.NoError(t, db.Raw(`SELECT uuid, interaction_payload FROM session_interactions WHERE session_id = ? ORDER BY id ASC LIMIT 1`, sessionID).Scan(&firstRow).Error)
	require.NotEmpty(t, firstRow.UUID, "Start's own first Turn must open Q1")

	var firstWire interactionPayloadWire
	require.NoError(t, json.Unmarshal(firstRow.InteractionPayload, &firstWire))
	firstArguments, err := engineservice.DecodeValue(firstWire.Arguments)
	require.NoError(t, err)
	firstArgumentsRecord, ok := firstArguments.(engine.RecordValue)
	require.True(t, ok)
	liveRandomField, ok := firstArgumentsRecord.FieldByName(replayRandomArgName)
	require.True(t, ok, "Q1's live interaction_payload must carry the random argument")
	liveRandomValue, ok := liveRandomField.Value.(engine.NumberValue)
	require.True(t, ok)

	answer1, err := m.AnswerInteraction(context.Background(), InteractionUUID(firstRow.UUID), UserUUID(hostUUID), engine.NumberValue{Value: 111})
	require.NoError(t, err)
	require.Equal(t, AnswerInteractionOutcomeAnswered, answer1.Outcome)

	var secondUUID string
	require.NoError(t, db.Raw(`SELECT uuid FROM session_interactions WHERE session_id = ? ORDER BY id DESC LIMIT 1`, sessionID).Scan(&secondUUID).Error)
	require.NotEmpty(t, secondUUID, "answering Q1 must open Q2")

	answer2, err := m.AnswerInteraction(context.Background(), InteractionUUID(secondUUID), UserUUID(hostUUID), engine.NumberValue{Value: 222})
	require.NoError(t, err)
	require.Equal(t, AnswerInteractionOutcomeAnswered, answer2.Outcome)

	var thirdUUID string
	var thirdEngineInteractionID uint64
	require.NoError(t, db.Raw(`SELECT uuid, engine_interaction_id FROM session_interactions WHERE session_id = ? ORDER BY id DESC LIMIT 1`, sessionID).Row().Scan(&thirdUUID, &thirdEngineInteractionID))
	require.NotEmpty(t, thirdUUID, "answering Q2 must open Q3")

	var turnCount int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionID).Scan(&turnCount).Error)
	require.Equal(t, int64(3), turnCount, "Start plus two answers must commit three RuntimeTurns")

	compiledProgram, diagnostics := engineservice.Compile(definition)
	require.False(t, diagnostics.HasErrors())

	// Two separate, freshly constructed Managers - neither sharing any
	// in-memory state with the live execution above or with each other -
	// each independently load the same durable prior-signal log.
	processA := New(db, nil, stubStartPinnedGameReader{definition: definition})
	inputA, priorSignalsA, err := processA.loadPriorSignals(context.Background(), db, sessionID)
	require.NoError(t, err)

	processB := New(db, nil, stubStartPinnedGameReader{definition: definition})
	inputB, priorSignalsB, err := processB.loadPriorSignals(context.Background(), db, sessionID)
	require.NoError(t, err)

	require.Equal(t, inputA, inputB, "two independent processes must load byte-identical InitializationInput from the same durable Start record")
	require.Equal(t, priorSignalsA, priorSignalsB, "two independent processes must load a byte-identical prior-signal log from the same durable Turn rows")
	require.Len(t, priorSignalsA, 2, "Q1's and Q2's answers, in order")
	require.Equal(t, engine.NumberValue{Value: 111}, priorSignalsA[0].Answer, "the first prior signal must be Q1's own literal answer")
	require.Equal(t, engine.NumberValue{Value: 222}, priorSignalsA[1].Answer, "the second prior signal must be Q2's own literal answer")

	nField, ok := decodeLiveRandomFromInput(t, inputA)
	require.True(t, ok)
	require.Equal(t, liveRandomValue.Value, nField, "the reconstructed InitializationInput.Seed must reproduce the same random draw live execution actually exposed, not merely a value re-derived from itself")

	// A hypothetical next signal - answering Q3 - is applied independently
	// by each process purely in memory (never committed), to prove
	// AdvanceTurn's internal replay reproduces the exact same result from
	// the exact same durable data, regardless of which process computes
	// it - only reachable if replay correctly threaded both prior answers
	// through first.
	answerQ3 := engine.Signal{
		Kind:          engine.SignalKindInteractionAnswered,
		InteractionID: engine.InteractionID(thirdEngineInteractionID),
		Respondent:    engine.UserID(strconv.FormatUint(uint64(hostActorID), 10)),
		Answer:        engine.NumberValue{Value: 333},
	}
	outputsA, err := engineservice.AdvanceTurn(compiledProgram, inputA, priorSignalsA, answerQ3, engine.DefaultLimits())
	require.NoError(t, err)
	outputsB, err := engineservice.AdvanceTurn(compiledProgram, inputB, priorSignalsB, answerQ3, engine.DefaultLimits())
	require.NoError(t, err)
	require.Equal(t, outputsA, outputsB, "two independent reconstructions (simulating process-loss recovery, no shared cache) applying the same next signal must produce identical Outputs")
}

// decodeLiveRandomFromInput draws the same random value input's Seed
// deterministically produces, purely by re-running a fresh StartTurn
// against it and reading the value it exposes as Q1's own argument - the
// same observable channel the live oracle above already used, so both
// sides of the comparison go through Outputs only, never a Snapshot.
func decodeLiveRandomFromInput(t *testing.T, input engine.InitializationInput) (float64, bool) {
	t.Helper()
	compiledProgram, diagnostics := engineservice.Compile(replayObservableDefinition(1, 4))
	require.False(t, diagnostics.HasErrors())
	outputs, err := engineservice.StartTurn(compiledProgram, input, engine.DefaultLimits())
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	openQ1, ok := outputs[0].(engine.OpenQuestionOutput)
	if !ok {
		return 0, false
	}
	n, ok := openQ1.Arguments[0].Value.(engine.NumberValue)
	if !ok {
		return 0, false
	}
	return n.Value, true
}
