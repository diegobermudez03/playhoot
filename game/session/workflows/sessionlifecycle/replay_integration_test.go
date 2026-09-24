package sessionlifecycle

import (
	"context"
	"strconv"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/runtimeturn"
	"github.com/stretchr/testify/require"
)

// TestReconstructCurrentSnapshot_Integration proves end to end that a
// Session's current authoritative engine.Snapshot is correctly
// reconstructable purely from durable state (Start's persisted
// Seed/RootParameters plus the ordered session_runtime_turns replay-input
// log), with no persisted Snapshot and no in-memory cache of any kind
// involved anywhere.
//
// It drives a real two-Turn Session (Start opens a question, AnswerInteraction
// answers and closes it) through the ordinary Manager, then independently
// replays the exact same durable inputs by hand - without calling
// reconstructCurrentSnapshot - to obtain the state the original live
// execution actually produced (no live Snapshot is retained anywhere for a
// direct diff, by design). It then calls reconstructCurrentSnapshot from two
// separate, freshly constructed Managers - simulating two different
// processes, neither sharing any in-memory state with the live execution or
// with each other - and asserts both reproduce that same state exactly.
// This also is this WORK's process-loss-recovery proof: reconstruction never
// depends on any particular process's memory, only on durable state.
func TestReconstructCurrentSnapshot_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	definition := answerableDefinition(1, 4)
	m, sessionUUID, hostUUID, interactionUUID := startedAnswerableSession(t, db, definition)

	answerResult, err := m.AnswerInteraction(context.Background(), interactionUUID, hostUUID, engine.NumberValue{Value: 42})
	require.NoError(t, err)
	require.Equal(t, AnswerInteractionOutcomeAnswered, answerResult.Outcome)

	sessionID := sessionIDForUUID(t, db, sessionUUID)

	// Read back exactly what a real process-loss recovery would have:
	// durable state only, nothing retained from the live execution above.
	var startRow struct {
		Seed           int64  `gorm:"column:seed"`
		RootParameters []byte `gorm:"column:root_parameters"`
	}
	require.NoError(t, db.Raw(`SELECT seed, root_parameters FROM session_runtime_starts WHERE session_id = ?`, sessionID).Scan(&startRow).Error)
	seed := uint64(startRow.Seed)
	rootParameters, err := decodeRootParameters(startRow.RootParameters)
	require.NoError(t, err)

	var interactionRow struct {
		EnginePath      []byte `gorm:"column:engine_path"`
		EngineSlot      string `gorm:"column:engine_slot"`
		Kind            string `gorm:"column:kind"`
		ResponsePayload []byte `gorm:"column:response_payload"`
	}
	require.NoError(t, db.Raw(`SELECT engine_path, engine_slot, kind, response_payload FROM session_interactions WHERE uuid = ?`, string(interactionUUID)).Scan(&interactionRow).Error)

	var respondentActorID uint
	require.NoError(t, db.Raw(`SELECT actor_id FROM session_runtime_turns WHERE session_id = ? AND sequence = 2`, sessionID).Scan(&respondentActorID).Error)
	require.NotZero(t, respondentActorID)

	compiledProgram, diagnostics := engineservice.Compile(definition)
	require.False(t, diagnostics.HasErrors())

	// Independently replay the same durable inputs by hand - this is
	// deliberately not a call to reconstructCurrentSnapshot - to obtain the
	// state the original live execution actually produced.
	snapshot, startSignal, err := engineservice.NewSnapshot(compiledProgram, engine.InitializationInput{RootParameters: rootParameters, Seed: seed})
	require.NoError(t, err)
	drain1 := runtimeturn.Drain(compiledProgram, snapshot, startSignal)
	require.NoError(t, drain1.Err)

	path, err := decodeEnginePath(interactionRow.EnginePath)
	require.NoError(t, err)
	signalKind, err := answerSignalKind(interactionRow.Kind)
	require.NoError(t, err)
	answerValue, err := engineservice.DecodeValue(interactionRow.ResponsePayload)
	require.NoError(t, err)
	answerSignal := engine.Signal{
		Kind:       signalKind,
		Path:       path,
		Slot:       interactionRow.EngineSlot,
		Respondent: engine.UserID(strconv.FormatUint(uint64(respondentActorID), 10)),
		Answer:     answerValue,
	}
	drain2 := runtimeturn.Drain(compiledProgram, drain1.Snapshot, answerSignal)
	require.NoError(t, drain2.Err)

	expectedEncoded, err := engineservice.EncodeSnapshot(drain2.Snapshot)
	require.NoError(t, err)

	// Two separate, freshly constructed Managers - neither sharing any
	// in-memory state with the live execution above or with each other -
	// each reconstruct purely from durable state.
	processA := New(db, nil, stubStartPinnedGameReader{definition: definition})
	actualA, err := processA.reconstructCurrentSnapshot(context.Background(), db, compiledProgram, sessionID)
	require.NoError(t, err)
	actualAEncoded, err := engineservice.EncodeSnapshot(actualA)
	require.NoError(t, err)
	require.JSONEq(t, string(expectedEncoded), string(actualAEncoded), "reconstruction must match the state produced by the original live execution")

	processB := New(db, nil, stubStartPinnedGameReader{definition: definition})
	actualB, err := processB.reconstructCurrentSnapshot(context.Background(), db, compiledProgram, sessionID)
	require.NoError(t, err)
	actualBEncoded, err := engineservice.EncodeSnapshot(actualB)
	require.NoError(t, err)
	require.JSONEq(t, string(expectedEncoded), string(actualBEncoded), "a second independent reconstruction (simulating process-loss recovery, no shared cache) must produce identical current state")
}
