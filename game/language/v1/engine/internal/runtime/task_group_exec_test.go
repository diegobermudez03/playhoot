package runtime_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/runtime"
)

// taskGroupProgram builds a hand-assembled engine.Program (bypassing
// compiler.Compile, mirroring step_test.go's style): a "Worker" workflow taking
// one number parameter "amount" and returning it, reacting to three
// intents ("Succeed", "Fail", "SelfCancel") to reach each of the three
// terminal outcomes; and a "Main" root workflow with one number-keyed
// task-group slot "Workers" backed by it, whose "S" state can begin (with
// the given completion policy)+seal a fixed-size batch of tasks in one
// transition, finalize, or cancel the group, and join its completion
// signal with joinControl (bindings "taskKeys"->keys, "terminalKeys"->terminal,
// "results"->results, "failures"->failures, "cancellations"->cancellations,
// "unfinished"->unfinished).
func taskGroupProgram(completion engine.TaskGroupCompletionPolicy, taskAmounts []float64, resultType engine.Type, joinControl engine.WorkflowControl) engine.Program {
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

	beginOp := engine.BeginTaskGroupOperation{Slot: "Workers", Completion: completion}
	spawnOps := make([]engine.Operation, len(taskAmounts))
	for i, amount := range taskAmounts {
		spawnOps[i] = engine.SpawnTaskGroupChildOperation{
			Slot:      "Workers",
			Key:       engine.NumberLiteralExpression{Value: float64(i + 1)},
			Arguments: []engine.CallArgument{{Name: "amount", Value: engine.NumberLiteralExpression{Value: amount}}},
		}
	}
	sealOp := engine.SealTaskGroupOperation{Slot: "Workers"}
	finalizeOp := engine.FinalizeTaskGroupOperation{Slot: "Workers"}
	cancelOp := engine.CancelTaskGroupOperation{Slot: "Workers", Reason: engine.StringLiteralExpression{Value: "abandoned"}}

	beginAndSealOps := append(append([]engine.Operation{beginOp}, spawnOps...), sealOp)

	main := engine.Workflow{
		Name:           "Main",
		ResultType:     resultType,
		TaskGroupSlots: []engine.TaskGroupSlot{{Name: "Workers", Workflow: "Worker", KeyType: engine.NumberType{}}},
		InitialState:   "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "BeginAndSeal", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "BeginAndSeal"}}, Operations: engine.Block{Operations: beginAndSealOps}, Control: engine.StayControl{}},
					{Name: "Finalize", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Finalize"}}, Operations: engine.Block{Operations: []engine.Operation{finalizeOp}}, Control: engine.StayControl{}},
					{Name: "CancelGroup", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "CancelGroup"}}, Operations: engine.Block{Operations: []engine.Operation{cancelOp}}, Control: engine.StayControl{}},
					{
						Name: "Join",
						Signal: engine.SignalPattern{
							Source: engine.TaskGroupCompletedSignalSource{Slot: "Workers"},
							Bindings: []engine.SignalBinding{
								{Field: "taskKeys", Name: "keys"},
								{Field: "terminalKeys", Name: "terminal"},
								{Field: "results", Name: "results"},
								{Field: "failures", Name: "failures"},
								{Field: "cancellations", Name: "cancellations"},
								{Field: "unfinished", Name: "unfinished"},
							},
						},
						Control: joinControl,
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

func taskGroupSnapshot() engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root: engine.WorkflowInstance{
			Workflow:       "Main",
			State:          "S",
			LocalState:     engine.RecordValue{TypeName: "local"},
			TaskGroupSlots: []engine.TaskGroupSlotInstance{{Name: "Workers"}},
		},
	}
}

func taskPath(key float64) []engine.PathStep {
	return []engine.PathStep{{Slot: "Workers", TaskKey: engine.NumberValue{Value: key}}}
}

func terminateTask(t *testing.T, p engine.Program, snap engine.Snapshot, key float64, intent string) engine.Snapshot {
	t.Helper()
	commit, err := runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Path: taskPath(key), Intent: intent}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error terminating task %v via %q: %v", key, intent, err)
	}
	return commit.Snapshot
}

func TestExec_BeginAndSealTaskGroupCreatesRunningTasks(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, []float64{10, 20, 30}, engine.UnitType{}, engine.StayControl{})
	snap := taskGroupSnapshot()

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	group := commit.Snapshot.Root.TaskGroupSlots[0].Group
	if group == nil || group.Phase != engine.TaskGroupPhaseRunning || len(group.Tasks) != 3 {
		t.Fatalf("got %+v", group)
	}
	if len(commit.InternalSignals) != 3 {
		t.Fatalf("got %d internal signals, want 3", len(commit.InternalSignals))
	}
	for _, s := range commit.InternalSignals {
		if s.Name != "WorkflowStarted" || len(s.Path) != 1 || s.Path[0].Slot != "Workers" {
			t.Fatalf("got %+v", s)
		}
	}
}

func TestExec_BeginIntoOccupiedSlotFailsAtomically(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, []float64{10}, engine.UnitType{}, engine.StayControl{})
	snap := taskGroupSnapshot()
	commit, _ := runtime.Step(p, snap, engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())

	_, err := runtime.Step(p, commit.Snapshot, engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorSlotOccupied {
		t.Fatalf("expected runtime.ExecutionErrorSlotOccupied, got %v", err)
	}
}

func TestExec_AllTerminalPolicy_CompletesWhenEveryTaskTerminates(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, []float64{10, 20, 30}, engine.UnitType{}, engine.StayControl{})
	commit, _ := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	snap := commit.Snapshot

	snap = terminateTask(t, p, snap, 1, "Succeed")
	if snap.Root.TaskGroupSlots[0].Group.Phase == engine.TaskGroupPhaseCompleted {
		t.Fatal("expected the group to still be running after one of three tasks terminated")
	}
	snap = terminateTask(t, p, snap, 2, "Fail")
	if snap.Root.TaskGroupSlots[0].Group.Phase == engine.TaskGroupPhaseCompleted {
		t.Fatal("expected the group to still be running after two of three tasks terminated")
	}
	snap = terminateTask(t, p, snap, 3, "SelfCancel")

	group := snap.Root.TaskGroupSlots[0].Group
	if group.Phase != engine.TaskGroupPhaseCompleted {
		t.Fatal("expected the group to complete once every task terminated")
	}
	want := []float64{1, 2, 3}
	for i, w := range want {
		if group.TerminalOrder[i].(engine.NumberValue).Value != w {
			t.Fatalf("expected terminal-order [1,2,3] (acceptance order), got %+v", group.TerminalOrder)
		}
	}
}

func TestExec_AllTerminalPolicy_EmptyGroupCompletesAtSeal(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, nil, engine.UnitType{}, engine.StayControl{})
	commit, err := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	group := commit.Snapshot.Root.TaskGroupSlots[0].Group
	if group == nil || group.Phase != engine.TaskGroupPhaseCompleted {
		t.Fatalf("expected an all-terminal group with no tasks to complete immediately at seal, got %+v", group)
	}
}

func TestExec_FirstTerminalPolicy_CompletesOnFirstTerminationAndOthersUnfinished(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupFirstTerminalPolicy{}, []float64{10, 20, 30}, engine.UnitType{}, engine.StayControl{})
	commit, _ := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	snap := commit.Snapshot

	snap = terminateTask(t, p, snap, 2, "Succeed")
	group := snap.Root.TaskGroupSlots[0].Group
	if group.Phase != engine.TaskGroupPhaseCompleted {
		t.Fatal("expected first-terminal to complete on the first termination")
	}
	if len(group.TerminalOrder) != 1 || group.TerminalOrder[0].(engine.NumberValue).Value != 2 {
		t.Fatalf("got %+v", group.TerminalOrder)
	}

	// Tasks 1 and 3 are now permanently unaddressable.
	_, err := runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Path: taskPath(1), Intent: "Succeed"}, engine.DefaultLimits())
	if err != runtime.ErrSignalRejected {
		t.Fatalf("expected a structurally cancelled task to reject further signals, got %v", err)
	}
}

func TestExec_FirstTerminalPolicy_NoTasksIsExecutionErrorAtSeal(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupFirstTerminalPolicy{}, nil, engine.UnitType{}, engine.StayControl{})
	_, err := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorInvalidQuorum {
		t.Fatalf("expected runtime.ExecutionErrorInvalidQuorum, got %v", err)
	}
}

func TestExec_QuorumTerminalPolicy_CompletesAtCountAndRestUnfinished(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupQuorumTerminalPolicy{Count: engine.NumberLiteralExpression{Value: 2}}, []float64{10, 20, 30}, engine.UnitType{}, engine.StayControl{})
	commit, _ := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	snap := commit.Snapshot

	snap = terminateTask(t, p, snap, 1, "Succeed")
	if snap.Root.TaskGroupSlots[0].Group.Phase == engine.TaskGroupPhaseCompleted {
		t.Fatal("expected quorum of 2 to still be running after 1 termination")
	}
	snap = terminateTask(t, p, snap, 3, "Fail")
	group := snap.Root.TaskGroupSlots[0].Group
	if group.Phase != engine.TaskGroupPhaseCompleted {
		t.Fatal("expected quorum of 2 to complete after 2 terminations")
	}
	if len(group.TerminalOrder) != 2 {
		t.Fatalf("got %+v", group.TerminalOrder)
	}
}

func TestExec_QuorumTerminalPolicy_CountExceedsTaskCountIsExecutionErrorAtSeal(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupQuorumTerminalPolicy{Count: engine.NumberLiteralExpression{Value: 5}}, []float64{10, 20, 30}, engine.UnitType{}, engine.StayControl{})
	_, err := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorInvalidQuorum {
		t.Fatalf("expected runtime.ExecutionErrorInvalidQuorum, got %v", err)
	}
}

func TestExec_DuplicateTaskKeyIsRejected(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, nil, engine.UnitType{}, engine.StayControl{})
	// Build manually with a duplicate key.
	worker := p.Workflows["Worker"]
	main := p.Workflows["Main"]
	main.States[0].Transitions[0].Operations.Operations = []engine.Operation{
		engine.BeginTaskGroupOperation{Slot: "Workers", Completion: engine.TaskGroupAllTerminalPolicy{}},
		engine.SpawnTaskGroupChildOperation{Slot: "Workers", Key: engine.NumberLiteralExpression{Value: 1}, Arguments: []engine.CallArgument{{Name: "amount", Value: engine.NumberLiteralExpression{Value: 10}}}},
		engine.SpawnTaskGroupChildOperation{Slot: "Workers", Key: engine.NumberLiteralExpression{Value: 1}, Arguments: []engine.CallArgument{{Name: "amount", Value: engine.NumberLiteralExpression{Value: 20}}}},
		engine.SealTaskGroupOperation{Slot: "Workers"},
	}
	p.Workflows["Main"] = main
	p.Workflows["Worker"] = worker

	_, err := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorDuplicateTaskKey {
		t.Fatalf("expected runtime.ExecutionErrorDuplicateTaskKey, got %v", err)
	}
}

func TestExec_TaskGroupLeftBuildingIsRejectedAtomically(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, nil, engine.UnitType{}, engine.StayControl{})
	main := p.Workflows["Main"]
	main.States[0].Transitions[0].Operations.Operations = []engine.Operation{
		engine.BeginTaskGroupOperation{Slot: "Workers", Completion: engine.TaskGroupAllTerminalPolicy{}},
		// no Seal or Cancel
	}
	p.Workflows["Main"] = main

	snap := taskGroupSnapshot()
	_, err := runtime.Step(p, snap, engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorTaskGroupLeftBuilding {
		t.Fatalf("expected runtime.ExecutionErrorTaskGroupLeftBuilding, got %v", err)
	}
	if snap.Root.TaskGroupSlots[0].Group != nil {
		t.Fatal("original snapshot's task-group slot must remain untouched")
	}
}

func TestExec_FinalizeRunningGroupCompletesWithPartialResultsAndUnfinished(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, []float64{10, 20, 30}, engine.UnitType{}, engine.StayControl{})
	commit, _ := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	snap := terminateTask(t, p, commit.Snapshot, 1, "Succeed")

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Finalize"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	group := commit.Snapshot.Root.TaskGroupSlots[0].Group
	if group.Phase != engine.TaskGroupPhaseCompleted {
		t.Fatal("expected finalize to complete the group")
	}
	if len(group.TerminalOrder) != 1 || group.TerminalOrder[0].(engine.NumberValue).Value != 1 {
		t.Fatalf("got %+v", group.TerminalOrder)
	}
}

func TestExec_TaskGroupFinalizeAlreadyCompletedIsIdempotentNoOp(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, []float64{10}, engine.UnitType{}, engine.StayControl{})
	commit, _ := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	snap := terminateTask(t, p, commit.Snapshot, 1, "Succeed") // completes naturally (all-terminal, 1 task)
	before := snap.Root.TaskGroupSlots[0].Group

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Finalize"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error on a finalize-vs-natural-completion race: %v", err)
	}
	after := commit.Snapshot.Root.TaskGroupSlots[0].Group
	if len(after.TerminalOrder) != len(before.TerminalOrder) {
		t.Fatalf("expected finalize on an already-completed group to be a no-op, got %+v", after)
	}
}

func TestExec_TaskGroupFinalizeEmptyOrBuildingSlotIsExecutionError(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, nil, engine.UnitType{}, engine.StayControl{})
	_, err := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "Finalize"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorUnknown {
		t.Fatalf("expected an execution error for finalizing an empty slot, got %v", err)
	}
}

func TestExec_CancelBuildingOrRunningGroupDiscardsTasks(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, []float64{10, 20}, engine.UnitType{}, engine.StayControl{})
	commit, _ := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	snap := terminateTask(t, p, commit.Snapshot, 1, "Succeed")

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "CancelGroup"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.TaskGroupSlots[0].Group != nil {
		t.Fatal("expected cancellation to clear the slot")
	}
}

func TestExec_TaskGroupCancelEmptySlotIsNoOp(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, nil, engine.UnitType{}, engine.StayControl{})
	commit, err := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "CancelGroup"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.TaskGroupSlots[0].Group != nil {
		t.Fatal("expected the slot to remain empty")
	}
}

func TestExec_TaskGroupCancelCompletedAwaitingJoinIsExecutionErrorAtomically(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, []float64{10}, engine.UnitType{}, engine.StayControl{})
	commit, _ := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	snap := terminateTask(t, p, commit.Snapshot, 1, "Succeed")

	_, err := runtime.Step(p, snap, engine.Signal{Name: "CancelGroup"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorTaskGroupNotJoined {
		t.Fatalf("expected runtime.ExecutionErrorTaskGroupNotJoined, got %v", err)
	}
	if snap.Root.TaskGroupSlots[0].Group == nil {
		t.Fatal("the completed-awaiting-join group must remain untouched")
	}
}

func TestExec_JoinBindsAggregatedFieldsAndClearsSlot(t *testing.T) {
	resultType := engine.MapType{Key: engine.NumberType{}, Value: engine.NumberType{}}
	joinControl := engine.CompleteControl{Result: engine.ReferenceExpression{Name: "results"}}
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, []float64{10, 20, 30}, resultType, joinControl)
	commit, _ := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	snap := commit.Snapshot

	snap = terminateTask(t, p, snap, 1, "Succeed")
	snap = terminateTask(t, p, snap, 2, "Fail")
	snap = terminateTask(t, p, snap, 3, "SelfCancel")

	commit, err := runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindTaskGroupCompleted, Slot: "Workers"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error joining: %v", err)
	}
	results := commit.Snapshot.Root.Outcome.Result.(engine.MapValue).Entries
	if len(results) != 1 || !results[0].Key.Equal(engine.NumberValue{Value: 1}) || results[0].Value.(engine.NumberValue).Value != 10 {
		t.Fatalf("got results %+v", results)
	}
	if commit.Snapshot.Root.TaskGroupSlots[0].Group != nil {
		t.Fatal("expected the slot to be cleared after joining")
	}
}

func TestExec_TaskGroupJoinStaleOrDuplicateRejected(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, []float64{10}, engine.UnitType{}, engine.StayControl{})
	snap := taskGroupSnapshot()

	_, err := runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindTaskGroupCompleted, Slot: "Workers"}, engine.DefaultLimits())
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected joining an empty slot, got %v", err)
	}

	commit, _ := runtime.Step(p, snap, engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	_, err = runtime.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindTaskGroupCompleted, Slot: "Workers"}, engine.DefaultLimits())
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected joining a still-running group, got %v", err)
	}

	snap = terminateTask(t, p, commit.Snapshot, 1, "Succeed") // completes naturally
	commit, err = runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindTaskGroupCompleted, Slot: "Workers"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = runtime.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindTaskGroupCompleted, Slot: "Workers"}, engine.DefaultLimits())
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected for a duplicate join, got %v", err)
	}
}

func TestExec_RecursiveCleanupOnParentTerminationClearsTaskGroup(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupAllTerminalPolicy{}, []float64{10, 20}, engine.UnitType{}, engine.StayControl{})
	main := p.Workflows["Main"]
	main.States[0].Transitions = append(main.States[0].Transitions, engine.Transition{
		Name:    "Terminate",
		Signal:  engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Terminate"}},
		Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}},
	})
	p.Workflows["Main"] = main

	commit, _ := runtime.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	commit, err := runtime.Step(p, commit.Snapshot, engine.Signal{Name: "Terminate"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.TaskGroupSlots[0].Group != nil {
		t.Fatalf("expected the terminating parent's task-group slot to be recursively cleared, got %+v", commit.Snapshot.Root.TaskGroupSlots[0].Group)
	}
}
