package sessionlifecycle

import (
	"strconv"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/stretchr/testify/require"
)

// TestReplayObservableDefinitionFixture proves replayObservableDefinition
// itself behaves the way TestReconstructCurrentSnapshot_Integration relies
// on: the random draw at Start is both exposed as Q1's own argument and
// threaded into Q2's own argument once answered, and each subsequent
// answer threads into the next opened question's argument in turn - all
// observable purely through engineservice.StartTurn/AdvanceTurn's own
// Outputs, since neither hands a caller an engine.Snapshot to inspect
// directly. It needs no database - it drives the fixture directly through
// engineservice - so it exercises the fixture's mechanics even in an
// environment with no reachable Postgres.
func TestReplayObservableDefinitionFixture(t *testing.T) {
	def := replayObservableDefinition(1, 1)
	compiledProgram, diagnostics := engineservice.Compile(def)
	require.False(t, diagnostics.HasErrors(), "%v", diagnostics)

	players := []engine.Value{engine.UserValue{ID: "1"}}
	start := engine.InitializationInput{
		RootParameters: map[string]engine.Value{"players": engine.ListValue{ElementType: engine.UserType{}, Elements: players}},
		Seed:           12345,
	}

	outputs1, err := engineservice.StartTurn(compiledProgram, start, engine.DefaultLimits())
	require.NoError(t, err)
	require.Len(t, outputs1, 1)
	openQ1, ok := outputs1[0].(engine.OpenQuestionOutput)
	require.True(t, ok, "%#v", outputs1[0])
	require.Equal(t, replayObservableSlot, openQ1.Slot)
	require.Len(t, openQ1.Arguments, 1)
	require.Equal(t, replayRandomArgName, openQ1.Arguments[0].Name)
	nValue, ok := openQ1.Arguments[0].Value.(engine.NumberValue)
	require.True(t, ok)
	t.Logf("drawn n = %v", nValue.Value)

	answerSignal1 := engine.Signal{
		Kind:          engine.SignalKindInteractionAnswered,
		InteractionID: openQ1.InteractionID,
		Respondent:    engine.UserID(strconv.FormatUint(1, 10)),
		Answer:        engine.NumberValue{Value: 111},
	}
	outputs2, err := engineservice.AdvanceTurn(compiledProgram, start, nil, answerSignal1, engine.DefaultLimits())
	require.NoError(t, err, "%v", err)
	require.Len(t, outputs2, 1)
	openQ2, ok := outputs2[0].(engine.OpenQuestionOutput)
	require.True(t, ok, "%#v", outputs2[0])
	require.Equal(t, replayObservableSlot2, openQ2.Slot)
	require.Len(t, openQ2.Arguments, 1)
	require.Equal(t, float64(111), openQ2.Arguments[0].Value.(engine.NumberValue).Value, "global.a must equal the literal answer submitted for Q1, threaded into Q2's own argument")

	answerSignal2 := engine.Signal{
		Kind:          engine.SignalKindInteractionAnswered,
		InteractionID: openQ2.InteractionID,
		Respondent:    engine.UserID(strconv.FormatUint(1, 10)),
		Answer:        engine.NumberValue{Value: 222},
	}
	outputs3, err := engineservice.AdvanceTurn(compiledProgram, start, []engine.Signal{answerSignal1}, answerSignal2, engine.DefaultLimits())
	require.NoError(t, err, "%v", err)
	require.Len(t, outputs3, 1, "answering Q2 must open Q3")
	openQ3, ok := outputs3[0].(engine.OpenQuestionOutput)
	require.True(t, ok, "%#v", outputs3[0])
	require.Equal(t, replayObservableSlot3, openQ3.Slot)
	require.Len(t, openQ3.Arguments, 1)
	require.Equal(t, float64(222), openQ3.Arguments[0].Value.(engine.NumberValue).Value, "global.b must equal the literal answer submitted for Q2, threaded into Q3's own argument - only reachable if AdvanceTurn's internal replay correctly threaded Q1's answer through first")
}
