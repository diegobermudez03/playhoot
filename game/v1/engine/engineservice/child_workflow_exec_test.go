package engineservice

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

// childWorkflowProgram builds a hand-assembled engine.Program (bypassing
// Compile, mirroring step_test.go's style): a "Worker" workflow taking
// one number parameter "amount" and returning it, reacting to three
// intents ("Succeed", "Fail", "SelfCancel") to reach each of the three
// terminal outcomes; and a "Main" root workflow with one child slot "W"
// declared against "Worker", whose "S" state can spawn into "W", cancel
// it, join each of its three possible outcomes, or terminate itself.
func childWorkflowProgram() engine.Program {
	worker := engine.Workflow{
		Name:         "Worker",
		Parameters:   []engine.FieldType{{Name: "amount", Type: engine.NumberType{}}},
		ResultType:   engine.NumberType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "Started", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "WorkflowStarted"}}, Control: engine.StayControl{}},
					{Name: "Succeed", Signal: engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: "Succeed"}}, Control: engine.CompleteControl{Result: engine.ReferenceExpression{Name: "amount"}}},
					{Name: "Fail", Signal: engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: "Fail"}}, Control: engine.FailControl{Error: engine.StringLiteralExpression{Value: "boom"}}},
					{Name: "SelfCancel", Signal: engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: "SelfCancel"}}, Control: engine.CancelControl{Reason: engine.StringLiteralExpression{Value: "nope"}}},
				},
			},
		},
	}

	spawnOp := engine.SpawnChildWorkflowOperation{Slot: "W", Arguments: []engine.CallArgument{{Name: "amount", Value: engine.NumberLiteralExpression{Value: 42}}}}
	cancelOp := engine.CancelChildWorkflowOperation{Slot: "W", Reason: engine.StringLiteralExpression{Value: "parent cancelled"}}

	main := engine.Workflow{
		Name:         "Main",
		ResultType:   engine.UnitType{},
		ChildSlots:   []engine.ChildWorkflowSlot{{Name: "W", Workflow: "Worker"}},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "Spawn", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Spawn"}}, Operations: engine.Block{Operations: []engine.Operation{spawnOp}}, Control: engine.StayControl{}},
					{Name: "CancelChild", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "CancelChild"}}, Operations: engine.Block{Operations: []engine.Operation{cancelOp}}, Control: engine.StayControl{}},
					{Name: "Terminate", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Terminate"}}, Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}}},
					{
						Name:    "JoinCompleted",
						Signal:  engine.SignalPattern{Source: engine.ChildCompletedSignalSource{Slot: "W"}, Bindings: []engine.SignalBinding{{Field: "result", Name: "r"}}},
						Control: engine.StayControl{},
					},
					{
						Name:    "JoinFailed",
						Signal:  engine.SignalPattern{Source: engine.ChildFailedSignalSource{Slot: "W"}, Bindings: []engine.SignalBinding{{Field: "error", Name: "e"}}},
						Control: engine.StayControl{},
					},
					{
						Name:    "JoinCancelled",
						Signal:  engine.SignalPattern{Source: engine.ChildCancelledSignalSource{Slot: "W"}, Bindings: []engine.SignalBinding{{Field: "reason", Name: "c"}}},
						Control: engine.StayControl{},
					},
				},
			},
		},
	}

	return engine.Program{
		RootWorkflow: "Main",
		Workflows:    map[string]engine.Workflow{"Main": main, "Worker": worker},
	}
}

func mainSnapshot() engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root: engine.WorkflowInstance{
			Workflow:   "Main",
			State:      "S",
			LocalState: engine.RecordValue{TypeName: "local"},
			ChildSlots: []engine.ChildWorkflowSlotInstance{{Name: "W"}},
		},
	}
}

// spawnAndStart spawns a child in "W" and applies its WorkflowStarted
// internal signal, returning the resulting snapshot.
func spawnAndStart(t *testing.T, p engine.Program, snap engine.Snapshot) engine.Snapshot {
	t.Helper()
	commit, err := Step(p, snap, engine.Signal{Name: "Spawn"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error spawning: %v", err)
	}
	if len(commit.InternalSignals) != 1 {
		t.Fatalf("got %d internal signals, want 1", len(commit.InternalSignals))
	}
	started := commit.InternalSignals[0]
	if started.Name != "WorkflowStarted" || len(started.Path) != 1 || started.Path[0].Slot != "W" {
		t.Fatalf("got %+v", started)
	}
	commit, err = Step(p, commit.Snapshot, started, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error starting child: %v", err)
	}
	return commit.Snapshot
}

func TestExec_SpawnChildOccupiesSlot(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot())

	child := snap.Root.ChildSlots[0].Child
	if child == nil || child.Workflow != "Worker" || child.State != "S" {
		t.Fatalf("got %+v", child)
	}
	amount, _ := child.Parameters[0], true
	if amount.Name != "amount" || amount.Value.(engine.NumberValue).Value != 42 {
		t.Fatalf("got %+v", child.Parameters)
	}
}

func TestExec_SpawnIntoOccupiedSlotFailsAtomically(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot())

	_, err := Step(p, snap, engine.Signal{Name: "Spawn"}, engine.DefaultLimits())
	if e, ok := err.(*ExecutionError); !ok || e.Code != ExecutionErrorSlotOccupied {
		t.Fatalf("expected ExecutionErrorSlotOccupied, got %v", err)
	}
}

func TestExec_ChildCompletesAndParentJoins(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot())

	commit, err := Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Path: []engine.PathStep{{Slot: "W"}}, Intent: "Succeed"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error completing child: %v", err)
	}
	snap = commit.Snapshot
	child := snap.Root.ChildSlots[0].Child
	if child == nil || child.Outcome == nil || child.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("expected the child to be completed-awaiting-join, got %+v", child)
	}
	if child.Outcome.Result.(engine.NumberValue).Value != 42 {
		t.Fatalf("got result %v, want 42", child.Outcome.Result)
	}

	commit, err = Step(p, snap, engine.Signal{Kind: engine.SignalKindChildCompleted, Slot: "W"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error joining: %v", err)
	}
	if commit.Snapshot.Root.ChildSlots[0].Child != nil {
		t.Fatalf("expected the slot to be cleared after joining, got %+v", commit.Snapshot.Root.ChildSlots[0].Child)
	}

	// A duplicate join delivery must be rejected once the slot is empty.
	_, err = Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindChildCompleted, Slot: "W"}, engine.DefaultLimits())
	if err != ErrInputRejected {
		t.Fatalf("expected ErrInputRejected for a duplicate join, got %v", err)
	}
}

func TestExec_ChildFailsAndParentJoins(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot())

	commit, err := Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Path: []engine.PathStep{{Slot: "W"}}, Intent: "Fail"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error failing child: %v", err)
	}
	snap = commit.Snapshot
	if snap.Root.ChildSlots[0].Child.Outcome.Kind != engine.WorkflowOutcomeFailed {
		t.Fatalf("got %+v", snap.Root.ChildSlots[0].Child.Outcome)
	}

	commit, err = Step(p, snap, engine.Signal{Kind: engine.SignalKindChildFailed, Slot: "W"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error joining a failed child: %v", err)
	}
	if commit.Snapshot.Root.ChildSlots[0].Child != nil {
		t.Fatal("expected the slot to be cleared after joining")
	}
}

func TestExec_ChildSelfCancelsAndParentJoins(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot())

	commit, err := Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Path: []engine.PathStep{{Slot: "W"}}, Intent: "SelfCancel"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error self-cancelling child: %v", err)
	}
	snap = commit.Snapshot
	if snap.Root.ChildSlots[0].Child.Outcome.Kind != engine.WorkflowOutcomeCancelled {
		t.Fatalf("got %+v", snap.Root.ChildSlots[0].Child.Outcome)
	}

	commit, err = Step(p, snap, engine.Signal{Kind: engine.SignalKindChildCancelled, Slot: "W"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error joining a self-cancelled child: %v", err)
	}
	if commit.Snapshot.Root.ChildSlots[0].Child != nil {
		t.Fatal("expected the slot to be cleared after joining")
	}
}

func TestExec_ChildOutcomeWrongKindRejected(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot())

	commit, err := Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Path: []engine.PathStep{{Slot: "W"}}, Intent: "Succeed"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The child completed, not failed or cancelled: joining it as a
	// failure or cancellation must be rejected, not silently accepted.
	_, err = Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindChildFailed, Slot: "W"}, engine.DefaultLimits())
	if err != ErrInputRejected {
		t.Fatalf("expected the mismatched-outcome join to be rejected, got %v", err)
	}
}

func TestExec_ParentDrivenCancelDiscardsRunningChild(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot())

	commit, err := Step(p, snap, engine.Signal{Name: "CancelChild"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.ChildSlots[0].Child != nil {
		t.Fatalf("expected parent-driven cancellation to discard the running child, got %+v", commit.Snapshot.Root.ChildSlots[0].Child)
	}
	if len(commit.InternalSignals) != 0 {
		t.Fatalf("parent-driven cancellation must not produce a child-outcome signal, got %+v", commit.InternalSignals)
	}
}

func TestExec_ParentDrivenCancelOnEmptySlotIsNoOp(t *testing.T) {
	p := childWorkflowProgram()
	snap := mainSnapshot() // slot "W" starts empty

	commit, err := Step(p, snap, engine.Signal{Name: "CancelChild"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.ChildSlots[0].Child != nil {
		t.Fatal("expected the slot to remain empty")
	}
}

func TestExec_ParentDrivenCancelOnTerminalAwaitingJoinFailsAtomically(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot())
	commit, err := Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Path: []engine.PathStep{{Slot: "W"}}, Intent: "Succeed"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap = commit.Snapshot

	_, err = Step(p, snap, engine.Signal{Name: "CancelChild"}, engine.DefaultLimits())
	if e, ok := err.(*ExecutionError); !ok || e.Code != ExecutionErrorChildOutcomeNotJoined {
		t.Fatalf("expected ExecutionErrorChildOutcomeNotJoined, got %v", err)
	}

	// Atomicity: the completed-awaiting-join child must remain exactly
	// as it was, not silently discarded.
	if snap.Root.ChildSlots[0].Child == nil || snap.Root.ChildSlots[0].Child.Outcome == nil {
		t.Fatal("original snapshot's awaiting-join child must remain untouched")
	}
}

func TestExec_RecursiveCleanupOnParentTermination_RunningChild(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot()) // child is running, never joined

	commit, err := Step(p, snap, engine.Signal{Name: "Terminate"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.Outcome == nil || commit.Snapshot.Root.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("got %+v", commit.Snapshot.Root.Outcome)
	}
	if commit.Snapshot.Root.ChildSlots[0].Child != nil {
		t.Fatalf("expected the terminating parent's child slot to be recursively cleared, got %+v", commit.Snapshot.Root.ChildSlots[0].Child)
	}
}

func TestExec_RecursiveCleanupOnParentTermination_AwaitingJoinChild(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot())
	commit, err := Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Path: []engine.PathStep{{Slot: "W"}}, Intent: "Succeed"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap = commit.Snapshot // child is completed, awaiting join, never joined

	commit, err = Step(p, snap, engine.Signal{Name: "Terminate"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.ChildSlots[0].Child != nil {
		t.Fatalf("expected the terminating parent's awaiting-join child slot to be recursively cleared too, got %+v", commit.Snapshot.Root.ChildSlots[0].Child)
	}
}

func TestExec_SignalToNonexistentChildPathIsRejected(t *testing.T) {
	p := childWorkflowProgram()
	snap := mainSnapshot() // slot "W" is empty: no child to address

	_, err := Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Path: []engine.PathStep{{Slot: "W"}}, Intent: "Succeed"}, engine.DefaultLimits())
	if err != ErrSignalRejected {
		t.Fatalf("expected ErrSignalRejected for a signal addressed to a nonexistent child, got %v", err)
	}
}
