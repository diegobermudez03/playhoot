package runtimeturn

import (
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/stretchr/testify/require"
)

// stayProgram builds a minimal hand-assembled engine.Program (bypassing
// engineservice.Compile): a "Main" root workflow whose only transition
// consumes WorkflowStarted and stays, producing no InternalSignals - the
// simplest possible RuntimeTurn, exactly one Step call.
func stayProgram() engine.Program {
	return engine.Program{
		RootWorkflow: "Main",
		Workflows: map[string]engine.Workflow{
			"Main": {
				Name:         "Main",
				InitialState: "Start",
				States: []engine.WorkflowState{
					{
						Name: "Start",
						Transitions: []engine.Transition{
							{Name: "Started", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "WorkflowStarted"}}, Control: engine.StayControl{}},
						},
					},
				},
			},
		},
	}
}

// rejectingProgram builds a "Main" root workflow declaring no transition at
// all for WorkflowStarted, so its own mandatory first Step call is an
// outright ErrSignalRejected.
func rejectingProgram() engine.Program {
	return engine.Program{
		RootWorkflow: "Main",
		Workflows: map[string]engine.Workflow{
			"Main": {
				Name:         "Main",
				InitialState: "Start",
				States:       []engine.WorkflowState{{Name: "Start"}},
			},
		},
	}
}

// divideByZeroProgram builds a "Main" root workflow whose only transition
// deterministically fails its very first Step call with a non-rejection
// ExecutionError (division by zero).
func divideByZeroProgram() engine.Program {
	return engine.Program{
		RootWorkflow: "Main",
		GlobalState:  []engine.StateField{{Name: "n", Type: engine.NumberType{}, Initializer: engine.NumberLiteralExpression{Value: 0}}},
		Workflows: map[string]engine.Workflow{
			"Main": {
				Name:         "Main",
				InitialState: "Start",
				States: []engine.WorkflowState{
					{
						Name: "Start",
						Transitions: []engine.Transition{
							{
								Name:   "Started",
								Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: engine.Block{Operations: []engine.Operation{
									engine.SetOperation{
										Target: engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: "n"},
										Value: engine.BinaryExpression{
											Operator: engine.BinaryOperatorDivide,
											Left:     engine.NumberLiteralExpression{Value: 1},
											Right:    engine.NumberLiteralExpression{Value: 0},
										},
									},
								}},
								Control: engine.StayControl{},
							},
						},
					},
				},
			},
		},
	}
}

// newSnapshotAndSignal is a small test helper: builds p's initial Snapshot
// and first Signal through the real engineservice.NewSnapshot, failing the
// test immediately if initialization itself fails - every fixture above is
// expected to initialize successfully; what differs is what happens once
// Drain actually starts stepping it.
func newSnapshotAndSignal(t *testing.T, p engine.Program) (engine.Snapshot, engine.Signal) {
	snap, signal, err := engineservice.NewSnapshot(p, engine.InitializationInput{})
	require.NoError(t, err)
	return snap, signal
}

// TestDrain exercises Drain's Step-bound enforcement and atomicity as a
// pure function over engine/engineservice, with no persistence/transaction
// dependency.
func TestDrain(t *testing.T) {
	t.Run("commits_within_bound", func(t *testing.T) {
		p := stayProgram()
		snap, signal := newSnapshotAndSignal(t, p)

		result := Drain(p, snap, signal)
		require.NoError(t, result.Err)
		require.Len(t, result.Steps, 1)
		require.Equal(t, "Main", result.Snapshot.Root.Workflow)
	})

	t.Run("rejects_initial_signal", func(t *testing.T) {
		p := rejectingProgram()
		snap, signal := newSnapshotAndSignal(t, p)

		result := Drain(p, snap, signal)
		require.Error(t, result.Err)
		require.True(t, result.FailedOnInitialSignal)
		require.True(t, errors.Is(result.Err, engineservice.ErrSignalRejected))
		require.Nil(t, result.Steps)
	})

	t.Run("fails_on_first_transition_execution_error", func(t *testing.T) {
		p := divideByZeroProgram()
		snap, signal := newSnapshotAndSignal(t, p)

		result := Drain(p, snap, signal)
		require.Error(t, result.Err)
		require.True(t, result.FailedOnInitialSignal)
		require.False(t, errors.Is(result.Err, engineservice.ErrSignalRejected))
		require.False(t, errors.Is(result.Err, engineservice.ErrInputRejected))
		require.Nil(t, result.Steps)
	})

	// Drain's MaxSteps bound (a cascade of several actual Step calls
	// within one RuntimeTurn) has no fixture left to construct it with:
	// its only source, spawning a child/task-group workflow whose own
	// WorkflowStarted signal comes back as an InternalSignal, was
	// removed - a Session now runs exactly one workflow instance, and
	// nothing in the engine produces an InternalSignal any more. This is
	// a known, accepted coverage gap, not a silently dropped case.
}
