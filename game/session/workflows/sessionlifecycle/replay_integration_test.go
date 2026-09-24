package sessionlifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestReconstructCurrentSnapshot_Integration proves replay reconstruction
// matches state the original live execution actually produced - not merely
// state re-derived from the same durable rows replay itself reads, which
// would only prove replay is consistent with itself.
//
// It drives a real three-Turn Session through the ordinary Manager (Start
// opens a question exposing a live random draw as one of its own
// arguments; answering it opens a second question; answering that one
// closes it), then reconstructs current state from two
// independently-constructed Managers - simulating two different processes,
// neither sharing any in-memory state with the live execution or with each
// other - and checks the reconstructed global state against two oracles
// that never pass through session_runtime_starts/session_runtime_turns, the
// rows reconstruction itself reads:
//   - the live random draw, read from session_interactions.interaction_payload
//     (written directly from Start's own live OpenQuestionOutput, never read
//     by reconstructCurrentSnapshot);
//   - the plain Go answer values this test itself passed to AnswerInteraction,
//     not anything decoded from a persisted row.
//
// The third Turn's expected value is only reachable if replay correctly
// threaded the second Turn's answer through first, so this also covers a
// later live AnswerInteraction depending on correct replay of an earlier
// one. This is also this WORK's process-loss-recovery proof: reconstruction
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

	var turnCount int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM session_runtime_turns WHERE session_id = ?`, sessionID).Scan(&turnCount).Error)
	require.Equal(t, int64(3), turnCount, "Start plus two answers must commit three RuntimeTurns")

	compiledProgram, diagnostics := engineservice.Compile(definition)
	require.False(t, diagnostics.HasErrors())

	assertReconstructedState := func(t *testing.T, snapshot engine.Snapshot) {
		t.Helper()

		nField, ok := snapshot.GlobalState.FieldByName("n")
		require.True(t, ok)
		nValue, ok := nField.Value.(engine.NumberValue)
		require.True(t, ok)
		require.Equal(t, liveRandomValue.Value, nValue.Value, "reconstructed global.n must match the value live execution actually drew and exposed, not merely a value re-derived from the persisted seed by the same code path replay itself uses")

		aField, ok := snapshot.GlobalState.FieldByName("a")
		require.True(t, ok)
		aValue, ok := aField.Value.(engine.NumberValue)
		require.True(t, ok)
		require.Equal(t, float64(111), aValue.Value, "reconstructed global.a must match the literal answer this test submitted for Q1")

		bField, ok := snapshot.GlobalState.FieldByName("b")
		require.True(t, ok)
		bValue, ok := bField.Value.(engine.NumberValue)
		require.True(t, ok)
		require.Equal(t, float64(222), bValue.Value, "reconstructed global.b must match the literal answer this test submitted for Q2 - only reachable if replay correctly threaded Q1's answer through first")
	}

	// Two separate, freshly constructed Managers - neither sharing any
	// in-memory state with the live execution above or with each other -
	// each reconstruct purely from durable state.
	processA := New(db, nil, stubStartPinnedGameReader{definition: definition})
	snapshotA, err := processA.reconstructCurrentSnapshot(context.Background(), db, compiledProgram, sessionID)
	require.NoError(t, err)
	assertReconstructedState(t, snapshotA)

	processB := New(db, nil, stubStartPinnedGameReader{definition: definition})
	snapshotB, err := processB.reconstructCurrentSnapshot(context.Background(), db, compiledProgram, sessionID)
	require.NoError(t, err)
	assertReconstructedState(t, snapshotB)

	encodedA, err := engineservice.EncodeSnapshot(snapshotA)
	require.NoError(t, err)
	encodedB, err := engineservice.EncodeSnapshot(snapshotB)
	require.NoError(t, err)
	require.JSONEq(t, string(encodedA), string(encodedB), "two independent reconstructions (simulating process-loss recovery, no shared cache) must produce identical current state")
}
