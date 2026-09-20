package runtimeturn

import (
	"errors"
	"fmt"
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

// oversizedSpawnProgram builds a "Main" root workflow whose only transition
// spawns spawnCount children in one Step call, each into its own declared
// child slot running "Leaf" - a workflow that just stays on its own
// WorkflowStarted, producing no further InternalSignals. This lets a test
// force the RuntimeTurn's total Step count arbitrarily high (1 initial +
// spawnCount children draining their own WorkflowStarted) while every spawn
// stays at child depth 1, never tripping engine.Limits.MaxWorkflowDepth -
// isolating MaxSteps's own bound from every other engine limit.
func oversizedSpawnProgram(spawnCount int) engine.Program {
	childSlots := make([]engine.ChildWorkflowSlot, spawnCount)
	spawnOps := make([]engine.Operation, spawnCount)
	for i := 0; i < spawnCount; i++ {
		slot := fmt.Sprintf("Child%d", i)
		childSlots[i] = engine.ChildWorkflowSlot{Name: slot, Workflow: "Leaf"}
		spawnOps[i] = engine.SpawnChildWorkflowOperation{Slot: slot}
	}

	return engine.Program{
		RootWorkflow: "Main",
		Workflows: map[string]engine.Workflow{
			"Main": {
				Name:         "Main",
				InitialState: "Start",
				ChildSlots:   childSlots,
				States: []engine.WorkflowState{
					{
						Name: "Start",
						Transitions: []engine.Transition{
							{
								Name:       "Spawn",
								Signal:     engine.SignalPattern{Source: engine.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: engine.Block{Operations: spawnOps},
								Control:    engine.StayControl{},
							},
						},
					},
				},
			},
			"Leaf": {
				Name:         "Leaf",
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

	t.Run("exceeds_step_bound", func(t *testing.T) {
		// 1 initial Step + 25 children draining their own WorkflowStarted =
		// 26 total Step calls, exceeding MaxSteps (20).
		p := oversizedSpawnProgram(25)
		snap, signal := newSnapshotAndSignal(t, p)

		result := Drain(p, snap, signal)
		require.ErrorIs(t, result.Err, ErrStepBoundExceeded)
		require.Nil(t, result.Steps)
	})

	t.Run("commits_exactly_at_bound", func(t *testing.T) {
		// 1 initial Step + 19 children = 20 total Step calls, exactly at
		// MaxSteps - must still commit, not overflow.
		p := oversizedSpawnProgram(19)
		snap, signal := newSnapshotAndSignal(t, p)

		result := Drain(p, snap, signal)
		require.NoError(t, result.Err)
		require.Len(t, result.Steps, 20)
		require.Equal(t, "Main", result.Snapshot.Root.Workflow)
	})
}
