package runtime_test

import (
	"reflect"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/runtime"
)

// threeTimerSlotsProgram builds a workflow declaring three independent
// timer slots and a single transition that schedules all three in one
// step — used to exercise Limits.MaxActiveSlotsPerInstance.
func threeTimerSlotsProgram() engine.Program {
	schedule := func(slot string) engine.Operation {
		return engine.ScheduleTimerOperation{Slot: slot, DelayMilliseconds: engine.NumberLiteralExpression{Value: 1000}}
	}
	wf := engine.Workflow{
		Name:         "Timers",
		ResultType:   engine.UnitType{},
		TimerSlots:   []string{"T1", "T2", "T3"},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "ScheduleAll", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "ScheduleAll"}}, Operations: engine.Block{Operations: []engine.Operation{
						schedule("T1"), schedule("T2"), schedule("T3"),
					}}, Control: engine.StayControl{}},
				},
			},
		},
	}
	return engine.Program{RootWorkflow: "Timers", Workflows: map[string]engine.Workflow{"Timers": wf}}
}

func threeTimerSlotsSnapshot() engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root: engine.WorkflowInstance{
			Workflow:   "Timers",
			State:      "S",
			LocalState: engine.RecordValue{TypeName: "local"},
			TimerSlots: []engine.TimerSlotInstance{{Name: "T1"}, {Name: "T2"}, {Name: "T3"}},
		},
	}
}

func TestExec_ActiveSlotLimitExceeded(t *testing.T) {
	p := threeTimerSlotsProgram()
	limits := engine.Limits{MaxOperations: 100, MaxLoopIterations: 100, MaxActiveSlotsPerInstance: 2}
	snap := threeTimerSlotsSnapshot()

	_, err := runtime.Step(p, snap, engine.Signal{Name: "ScheduleAll"}, limits)
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorActiveSlotLimitExceeded {
		t.Fatalf("expected runtime.ExecutionErrorActiveSlotLimitExceeded, got %v", err)
	}

	// Atomicity: the original snapshot's timer slots must remain empty.
	for _, s := range snap.Root.TimerSlots {
		if s.Pending {
			t.Fatal("original snapshot must remain untouched")
		}
	}

	// A batch within the limit succeeds.
	within := engine.Limits{MaxOperations: 100, MaxLoopIterations: 100, MaxActiveSlotsPerInstance: 3}
	commit, err := runtime.Step(p, snap, engine.Signal{Name: "ScheduleAll"}, within)
	if err != nil {
		t.Fatalf("unexpected error for a batch within the limit: %v", err)
	}
	for _, s := range commit.Snapshot.Root.TimerSlots {
		if !s.Pending {
			t.Fatalf("got %+v, want all timer slots pending", commit.Snapshot.Root.TimerSlots)
		}
	}
}

func TestExec_TraceRecordsTransitionGuardStateAndOutputs(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(0)

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Increment"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tr := commit.Trace
	if tr.Workflow != "Counter" {
		t.Fatalf("got workflow %q", tr.Workflow)
	}
	if tr.TransitionName != "Increment" {
		t.Fatalf("got transition %q", tr.TransitionName)
	}
	if tr.GuardEvaluated {
		t.Fatalf("expected no guard on Increment, got GuardEvaluated=true")
	}
	if tr.StateBefore != "Running" || tr.StateAfter != "Running" {
		t.Fatalf("got before=%q after=%q", tr.StateBefore, tr.StateAfter)
	}
	if tr.Outcome != nil {
		t.Fatalf("got outcome %+v, want nil", tr.Outcome)
	}
	if tr.OperationCount != 1 {
		t.Fatalf("got operation count %d, want 1", tr.OperationCount)
	}

	// Finish requires count >= 3 and has a guard; drive it there.
	for i := 0; i < 2; i++ {
		commit, err = runtime.Step(p, commit.Snapshot, engine.Signal{Name: "Increment"}, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	commit, err = runtime.Step(p, commit.Snapshot, engine.Signal{Name: "Finish"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tr = commit.Trace
	if !tr.GuardEvaluated || !tr.GuardResult {
		t.Fatalf("got GuardEvaluated=%v GuardResult=%v, want true/true", tr.GuardEvaluated, tr.GuardResult)
	}
	if tr.Outcome == nil || tr.Outcome.Kind != engine.RunOutcomeCompleted {
		t.Fatalf("got outcome %+v", tr.Outcome)
	}
	if !reflect.DeepEqual(tr.Outputs, commit.Outputs) {
		t.Fatalf("trace outputs %+v do not match commit outputs %+v", tr.Outputs, commit.Outputs)
	}
}

func TestExec_ReplayIsDeterministic(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(1)
	signal := engine.Signal{Name: "Increment"}

	commit1, err1 := runtime.Step(p, snap, signal, engine.DefaultLimits())
	commit2, err2 := runtime.Step(p, snap, signal, engine.DefaultLimits())
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if !reflect.DeepEqual(commit1, commit2) {
		t.Fatalf("replaying the same (program, snapshot, signal, limits) produced different commits:\n%+v\nvs\n%+v", commit1, commit2)
	}

	// The original snapshot is unaffected by either call, so a third
	// replay from scratch still reproduces the same result.
	commit3, err3 := runtime.Step(p, snap, signal, engine.DefaultLimits())
	if err3 != nil {
		t.Fatalf("unexpected error: %v", err3)
	}
	if !reflect.DeepEqual(commit1, commit3) {
		t.Fatal("a third replay diverged from the first two")
	}
}

func TestExec_RetryAfterFailureIsSafeAndReproducible(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(0)
	// Finish's guard requires count >= 3; with count 0 this always rejects.
	signal := engine.Signal{Name: "Finish"}

	_, err1 := runtime.Step(p, snap, signal, engine.DefaultLimits())
	_, err2 := runtime.Step(p, snap, signal, engine.DefaultLimits())
	if err1 != runtime.ErrSignalRejected || err2 != runtime.ErrSignalRejected {
		t.Fatalf("expected both attempts to reject identically, got %v, %v", err1, err2)
	}

	count, _ := snap.GlobalState.FieldByName("count")
	if count.Value.(engine.NumberValue).Value != 0 {
		t.Fatal("a rejected retry must never mutate the original snapshot")
	}
}

func TestExec_RetryAfterSuccessReproducesIdenticalCommit(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(0)
	signal := engine.Signal{Name: "Increment"}

	commitA, err := runtime.Step(p, snap, signal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A caller unsure whether commitA was durably persisted can safely
	// recompute it from the same original, untouched snapshot.
	commitB, err := runtime.Step(p, snap, signal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(commitA, commitB) {
		t.Fatal("retrying a succeeded step from its original snapshot must reproduce an identical commit")
	}
}

func TestExec_ExecutionErrorCodeStringIsExhaustiveAndStable(t *testing.T) {
	codes := []runtime.ExecutionErrorCode{
		runtime.ExecutionErrorUnknown, runtime.ExecutionErrorUndefinedReference,
		runtime.ExecutionErrorDivisionByZero, runtime.ExecutionErrorIndexOutOfRange, runtime.ExecutionErrorKeyNotFound,
		runtime.ExecutionErrorNoMatchingCase, runtime.ExecutionErrorInvalidInitialState, runtime.ExecutionErrorInvariantViolation,
		runtime.ExecutionErrorSnapshotProgramMismatch, runtime.ExecutionErrorSignalRejected, runtime.ExecutionErrorBudgetExceeded,
		runtime.ExecutionErrorLoopLimitExceeded, runtime.ExecutionErrorInvalidRandomRange, runtime.ExecutionErrorEmptyRandomCollection,
		runtime.ExecutionErrorSlotOccupied, runtime.ExecutionErrorInvalidTimerDelay, runtime.ExecutionErrorInputRejected,
		runtime.ExecutionErrorDuplicateRecipient, runtime.ExecutionErrorInvalidQuorum,
		runtime.ExecutionErrorAskGroupNotJoined, runtime.ExecutionErrorPresentationSlotOccupied,
		runtime.ExecutionErrorActiveSlotLimitExceeded,
	}
	seen := make(map[string]runtime.ExecutionErrorCode, len(codes))
	for _, c := range codes {
		name := c.String()
		if name == "" {
			t.Fatalf("code %d has an empty String()", c)
		}
		if c != runtime.ExecutionErrorUnknown && name == "unknown" {
			t.Fatalf("code %d falls through to the default \"unknown\" name — missing a String() case", c)
		}
		if other, dup := seen[name]; dup && other != c {
			t.Fatalf("codes %d and %d share the name %q", other, c, name)
		}
		seen[name] = c
	}
}

func TestExec_InvariantFailureAfterTransitionIsAtomicAndReproducible(t *testing.T) {
	p := counterProgram()
	p.Invariants = []engine.Invariant{
		{Name: "CountBelowTwo", Condition: engine.BinaryExpression{
			Operator: engine.BinaryOperatorLess,
			Left:     engine.FieldExpression{Target: engine.ReferenceExpression{Name: "global"}, Field: "count"},
			Right:    engine.NumberLiteralExpression{Value: 2},
		}},
	}
	snap := counterSnapshot(1)

	_, err1 := runtime.Step(p, snap, engine.Signal{Name: "Increment"}, engine.DefaultLimits())
	_, err2 := runtime.Step(p, snap, engine.Signal{Name: "Increment"}, engine.DefaultLimits())
	e1, ok1 := err1.(*runtime.ExecutionError)
	e2, ok2 := err2.(*runtime.ExecutionError)
	if !ok1 || !ok2 || e1.Code != runtime.ExecutionErrorInvariantViolation || e2.Code != runtime.ExecutionErrorInvariantViolation {
		t.Fatalf("expected two identical runtime.ExecutionErrorInvariantViolation, got %v, %v", err1, err2)
	}
	count, _ := snap.GlobalState.FieldByName("count")
	if count.Value.(engine.NumberValue).Value != 1 {
		t.Fatal("an invariant-violating step must never mutate the original snapshot")
	}
}
