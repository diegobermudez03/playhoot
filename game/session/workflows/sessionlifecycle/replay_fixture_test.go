package sessionlifecycle

import (
	"strconv"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/runtimeturn"
	"github.com/stretchr/testify/require"
)

// TestReplayObservableDefinitionFixture proves replayObservableDefinition
// itself behaves the way TestReconstructCurrentSnapshot_Integration relies
// on: the random draw at Start is both exposed as Q1's own argument and
// stored into global state under the same value, and each answer threads
// into global state in turn, opening the next question only when expected.
// It needs no database - it drives the fixture directly through
// engineservice/internal/runtimeturn - so it exercises the fixture's
// mechanics even in an environment with no reachable Postgres.
func TestReplayObservableDefinitionFixture(t *testing.T) {
	def := replayObservableDefinition(1, 1)
	compiledProgram, diagnostics := engineservice.Compile(def)
	require.False(t, diagnostics.HasErrors(), "%v", diagnostics)

	players := []engine.Value{engine.UserValue{ID: "1"}}
	snapshot, startSignal, err := engineservice.NewSnapshot(compiledProgram, engine.InitializationInput{
		RootParameters: map[string]engine.Value{"players": engine.ListValue{ElementType: engine.UserType{}, Elements: players}},
		Seed:           12345,
	})
	require.NoError(t, err)

	drain1 := runtimeturn.Drain(compiledProgram, snapshot, startSignal)
	require.NoError(t, drain1.Err)
	require.Len(t, drain1.Steps, 1)
	require.Len(t, drain1.Steps[0].Outputs, 1)
	openQ1, ok := drain1.Steps[0].Outputs[0].(engine.OpenQuestionOutput)
	require.True(t, ok, "%#v", drain1.Steps[0].Outputs[0])
	require.Equal(t, replayObservableSlot, openQ1.Slot)
	require.Len(t, openQ1.Arguments, 1)
	require.Equal(t, replayRandomArgName, openQ1.Arguments[0].Name)
	nValue, ok := openQ1.Arguments[0].Value.(engine.NumberValue)
	require.True(t, ok)
	t.Logf("drawn n = %v", nValue.Value)

	nField, ok := drain1.Snapshot.GlobalState.FieldByName("n")
	require.True(t, ok)
	require.Equal(t, nValue.Value, nField.Value.(engine.NumberValue).Value, "global.n must equal the same drawn value exposed as the question argument")

	answerSignal1 := engine.Signal{
		Kind:       engine.SignalKindQuestionAnswered,
		Path:       drain1.Steps[0].Path,
		Slot:       replayObservableSlot,
		Respondent: engine.UserID(strconv.FormatUint(1, 10)),
		Answer:     engine.NumberValue{Value: 111},
	}
	drain2 := runtimeturn.Drain(compiledProgram, drain1.Snapshot, answerSignal1)
	require.NoError(t, drain2.Err, "%v", drain2.Err)
	require.Len(t, drain2.Steps, 1)
	require.Len(t, drain2.Steps[0].Outputs, 1)
	openQ2, ok := drain2.Steps[0].Outputs[0].(engine.OpenQuestionOutput)
	require.True(t, ok, "%#v", drain2.Steps[0].Outputs[0])
	require.Equal(t, replayObservableSlot2, openQ2.Slot)
	require.Len(t, openQ2.Arguments, 1)
	require.Equal(t, float64(111), openQ2.Arguments[0].Value.(engine.NumberValue).Value)

	aField, ok := drain2.Snapshot.GlobalState.FieldByName("a")
	require.True(t, ok)
	require.Equal(t, float64(111), aField.Value.(engine.NumberValue).Value)

	answerSignal2 := engine.Signal{
		Kind:       engine.SignalKindQuestionAnswered,
		Path:       drain2.Steps[0].Path,
		Slot:       replayObservableSlot2,
		Respondent: engine.UserID(strconv.FormatUint(1, 10)),
		Answer:     engine.NumberValue{Value: 222},
	}
	drain3 := runtimeturn.Drain(compiledProgram, drain2.Snapshot, answerSignal2)
	require.NoError(t, drain3.Err, "%v", drain3.Err)
	require.Empty(t, drain3.Steps[0].Outputs, "answering Q2 must not open a further question")

	bField, ok := drain3.Snapshot.GlobalState.FieldByName("b")
	require.True(t, ok)
	require.Equal(t, float64(222), bField.Value.(engine.NumberValue).Value)
}
