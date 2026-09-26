package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/replay"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestReplayReconstructsTimerExpiredSignal_Integration proves
// internal/replay.LoadPriorSignals correctly reconstructs an
// engine.SignalKindTimerExpired signal from a durable TIMER_EXPIRED
// RuntimeTurn: a freshly constructed Manager, sharing no in-memory state
// with the live execution that actually produced the Turn, must load the
// same signal and reproduce the same Outputs from durable state alone - the
// same process-loss-recovery proof TestReconstructCurrentSnapshot_Integration
// already establishes for interaction-response causes, applied here to a
// timer's own expiration.
func TestReplayReconstructsTimerExpiredSignal_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	definition := timerDefinition(1, 4, 5000)
	m := New(db, nil, stubStartPinnedGameReader{definition: definition})

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

	expireResult, err := m.ExpireTimer(context.Background(), TimerObligationUUID(obligationUUID))
	require.NoError(t, err)
	require.Equal(t, ExpireTimerOutcomeExpired, expireResult.Outcome)
	require.Len(t, expireResult.Outputs, 1, "the live execution's own TimerFired effect")

	// A freshly constructed Manager, sharing no in-memory state with the live
	// execution above, independently reconstructs the durable signal log.
	process := New(db, nil, stubStartPinnedGameReader{definition: definition})
	input, priorSignals, err := replay.LoadPriorSignals(context.Background(), db, process.answerInteractionRepo, fx.SessionID)
	require.NoError(t, err)
	require.Len(t, priorSignals, 1, "the timer's own expiration is the only prior signal after Start")
	require.Equal(t, engine.Signal{Kind: engine.SignalKindTimerExpired, Slot: timerSlotName}, priorSignals[0])

	compiledProgram, diagnostics := engineservice.Compile(definition)
	require.False(t, diagnostics.HasErrors())

	// Reconstructing Turn 2 (the timer's expiration) independently: no prior
	// signal precedes it besides Start's own synthesized WorkflowStarted,
	// which AdvanceTurn reproduces internally and never needs supplied - the
	// same convention LoadPriorSignals itself documents.
	reconstructedOutputs, err := engineservice.AdvanceTurn(compiledProgram, input, nil, priorSignals[0], engine.DefaultLimits())
	require.NoError(t, err)
	require.Len(t, reconstructedOutputs, 1)
	effect, ok := reconstructedOutputs[0].(engine.EmitEffectOutput)
	require.True(t, ok, "%#v", reconstructedOutputs[0])
	require.Equal(t, timerEffectName, effect.Effect)
}

// TestReplayReconstructsKeyedTimerExpiredSignal_Integration is the keyed
// counterpart: proves LoadPriorSignals reconstructs the correct
// engine.SignalKindKeyedTimerExpired signal, carrying the authored key,
// from a durable TIMER_EXPIRED RuntimeTurn whose obligation has a non-nil
// EngineKey.
func TestReplayReconstructsKeyedTimerExpiredSignal_Integration(t *testing.T) {
	db := testdb.OpenSessionDB(t)

	definition := keyedTimerDefinition(1, 4)
	m := New(db, nil, stubStartPinnedGameReader{definition: definition})

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	hostUUID := uuid.NewString()
	hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUID)
	require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
	testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")

	startResult, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUID), IdempotencyKey(uuid.NewString()))
	require.NoError(t, err)
	require.Equal(t, StartOutcomeStarted, startResult.Outcome)

	var obligationUUID string
	require.NoError(t, db.Raw(`SELECT uuid FROM session_timer_obligations WHERE session_id = ? ORDER BY id ASC LIMIT 1`, fx.SessionID).Scan(&obligationUUID).Error)

	expireResult, err := m.ExpireTimer(context.Background(), TimerObligationUUID(obligationUUID))
	require.NoError(t, err)
	require.Equal(t, ExpireTimerOutcomeExpired, expireResult.Outcome)

	process := New(db, nil, stubStartPinnedGameReader{definition: definition})
	_, priorSignals, err := replay.LoadPriorSignals(context.Background(), db, process.answerInteractionRepo, fx.SessionID)
	require.NoError(t, err)
	require.Len(t, priorSignals, 1)
	require.Equal(t, engine.SignalKindKeyedTimerExpired, priorSignals[0].Kind)
	require.Equal(t, keyedTimerSlotName, priorSignals[0].Slot)
	require.Contains(t, []engine.Value{engine.StringValue{Value: "P0"}, engine.StringValue{Value: "P1"}}, priorSignals[0].Key)
}
