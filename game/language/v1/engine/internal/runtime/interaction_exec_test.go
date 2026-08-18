package runtime_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/runtime"
)

// findInstanceQuestionSlot is a small test-only lookup helper: the real
// runtime keeps its own equivalent private to engine/internal/runtime,
// since it's an implementation detail, not part of the public contract
// these tests exercise.
func findInstanceQuestionSlot(instance engine.WorkflowInstance, name string) (engine.QuestionSlotInstance, bool) {
	for _, s := range instance.QuestionSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.QuestionSlotInstance{}, false
}

func findInstanceTimerSlot(instance engine.WorkflowInstance, name string) (engine.TimerSlotInstance, bool) {
	for _, s := range instance.TimerSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.TimerSlotInstance{}, false
}

// questionDemoProgram builds a hand-assembled engine.Program (bypassing
// compiler.Compile, mirroring step_test.go's style) for a single workflow "QDemo"
// with one question slot ("Ask", backed by question "Confirm") and one
// timer slot ("Deadline"). Its "S" state has transitions to open/close
// the question, schedule/cancel the timer, emit an effect, and a
// QuestionAnswered/TimerExpired-sourced transition ("Answered"/"Expired")
// that completes the workflow so dispatch can be observed.
func questionDemoProgram() engine.Program {
	openOp := engine.OpenQuestionOperation{
		Slot:      "Ask",
		Recipient: engine.ReferenceExpression{Name: "player"},
		Arguments: []engine.CallArgument{{Name: "prompt", Value: engine.StringLiteralExpression{Value: "Ready?"}}},
	}
	closeOp := engine.CloseQuestionOperation{Slot: "Ask"}
	scheduleOp := engine.ScheduleTimerOperation{Slot: "Deadline", DelayMilliseconds: engine.NumberLiteralExpression{Value: 5000}}
	cancelOp := engine.CancelTimerOperation{Slot: "Deadline"}
	emitOp := engine.EmitEffectOperation{
		Effect:     "Confetti",
		Recipients: engine.ReferenceExpression{Name: "recipients"},
		Arguments:  []engine.CallArgument{{Name: "amount", Value: engine.NumberLiteralExpression{Value: 10}}},
	}

	workflow := engine.Workflow{
		Name:          "QDemo",
		Parameters:    []engine.FieldType{{Name: "player", Type: engine.UserType{}}, {Name: "recipients", Type: engine.ListType{Element: engine.UserType{}}}},
		ResultType:    engine.UnitType{},
		QuestionSlots: []engine.QuestionSlot{{Name: "Ask", Question: "Confirm"}},
		TimerSlots:    []string{"Deadline"},
		InitialState:  "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "Open", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Open"}}, Operations: engine.Block{Operations: []engine.Operation{openOp}}, Control: engine.StayControl{}},
					{Name: "Close", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Close"}}, Operations: engine.Block{Operations: []engine.Operation{closeOp}}, Control: engine.StayControl{}},
					{Name: "Schedule", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Schedule"}}, Operations: engine.Block{Operations: []engine.Operation{scheduleOp}}, Control: engine.StayControl{}},
					{Name: "Cancel", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Cancel"}}, Operations: engine.Block{Operations: []engine.Operation{cancelOp}}, Control: engine.StayControl{}},
					{Name: "Emit", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Emit"}}, Operations: engine.Block{Operations: []engine.Operation{emitOp}}, Control: engine.StayControl{}},
					{
						Name:    "Answered",
						Signal:  engine.SignalPattern{Source: engine.QuestionAnsweredSignalSource{Slot: "Ask"}, Bindings: []engine.SignalBinding{{Field: "answer", Name: "a"}}},
						Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}},
					},
					{
						Name:    "Expired",
						Signal:  engine.SignalPattern{Source: engine.TimerExpiredSignalSource{Slot: "Deadline"}},
						Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}},
					},
				},
			},
		},
	}

	return engine.Program{
		RootWorkflow: "QDemo",
		Workflows:    map[string]engine.Workflow{"QDemo": workflow},
		Questions: map[string]engine.Question{
			"Confirm": {
				Name:         "Confirm",
				Parameters:   []engine.FieldType{{Name: "prompt", Type: engine.StringType{}}},
				ResponseType: engine.BoolType{},
			},
		},
		Effects: map[string]engine.Effect{
			"Confetti": {Name: "Confetti", Parameters: []engine.FieldType{{Name: "amount", Type: engine.NumberType{}}}},
		},
	}
}

const player = engine.UserID("player-1")
const other = engine.UserID("other-1")

func questionDemoSnapshot() engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root: engine.WorkflowInstance{
			Workflow: "QDemo",
			State:    "S",
			Parameters: []engine.FieldValue{
				{Name: "player", Value: engine.UserValue{ID: player}},
				{Name: "recipients", Value: engine.ListValue{ElementType: engine.UserType{}, Elements: []engine.Value{engine.UserValue{ID: player}}}},
			},
			LocalState:    engine.RecordValue{TypeName: "local"},
			QuestionSlots: []engine.QuestionSlotInstance{{Name: "Ask"}},
			TimerSlots:    []engine.TimerSlotInstance{{Name: "Deadline"}},
		},
	}
}

func TestExec_OpenQuestionOccupiesSlotAndProducesOutput(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	slot, ok := findInstanceQuestionSlot(commit.Snapshot.Root, "Ask")
	if !ok || slot.Pending == nil || slot.Pending.Recipient != player {
		t.Fatalf("got %+v", slot)
	}
	if len(commit.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(commit.Outputs))
	}
	out, ok := commit.Outputs[0].(engine.OpenQuestionOutput)
	if !ok || out.Slot != "Ask" || out.Recipient != player || out.Question != "Confirm" {
		t.Fatalf("got %+v", commit.Outputs[0])
	}

	// The original snapshot's slot must remain untouched.
	origSlot, _ := findInstanceQuestionSlot(snap.Root, "Ask")
	if origSlot.Pending != nil {
		t.Fatal("original snapshot's question slot was mutated")
	}
}

func TestExec_OpenQuestionOnOccupiedSlotFailsAtomically(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()
	snap.Root.QuestionSlots[0] = engine.QuestionSlotInstance{Name: "Ask", Pending: &engine.PendingQuestion{Recipient: other}}

	_, err := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorSlotOccupied {
		t.Fatalf("expected runtime.ExecutionErrorSlotOccupied, got %v", err)
	}
}

func TestExec_CloseQuestionClearsOccupiedSlotAndOutput(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()
	snap.Root.QuestionSlots[0] = engine.QuestionSlotInstance{Name: "Ask", Pending: &engine.PendingQuestion{Recipient: player}}

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Close"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	slot, _ := findInstanceQuestionSlot(commit.Snapshot.Root, "Ask")
	if slot.Pending != nil {
		t.Fatalf("expected the slot to be cleared, got %+v", slot)
	}
	if len(commit.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(commit.Outputs))
	}
	if out, ok := commit.Outputs[0].(engine.CloseQuestionOutput); !ok || out.Slot != "Ask" || out.Recipient != player {
		t.Fatalf("got %+v", commit.Outputs[0])
	}
}

func TestExec_CloseQuestionOnEmptySlotIsNoOp(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot() // Ask starts empty

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Close"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commit.Outputs) != 0 {
		t.Fatalf("expected no output for closing an already-empty slot, got %+v", commit.Outputs)
	}
}

func TestExec_QuestionAnsweredAcceptedClearsSlotAndDispatches(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()
	snap.Root.QuestionSlots[0] = engine.QuestionSlotInstance{Name: "Ask", Pending: &engine.PendingQuestion{Recipient: player}}

	commit, err := runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindQuestionAnswered, Slot: "Ask", Respondent: player, Answer: engine.BoolValue{Value: true}}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.Outcome == nil || commit.Snapshot.Root.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("expected the Answered transition to run and complete, got %+v", commit.Snapshot.Root.Outcome)
	}
	slot, _ := findInstanceQuestionSlot(commit.Snapshot.Root, "Ask")
	if slot.Pending != nil {
		t.Fatalf("expected an accepted answer to clear its slot, got %+v", slot)
	}
}

func TestExec_QuestionAnsweredWrongRespondentRejected(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()
	snap.Root.QuestionSlots[0] = engine.QuestionSlotInstance{Name: "Ask", Pending: &engine.PendingQuestion{Recipient: player}}

	_, err := runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindQuestionAnswered, Slot: "Ask", Respondent: other, Answer: engine.BoolValue{Value: true}}, engine.DefaultLimits())
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected for an unauthorized respondent, got %v", err)
	}
}

func TestExec_QuestionAnsweredStaleOrDuplicateRejected(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot() // Ask starts empty: this answer is stale/duplicate

	_, err := runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindQuestionAnswered, Slot: "Ask", Respondent: player, Answer: engine.BoolValue{Value: true}}, engine.DefaultLimits())
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected for a stale/duplicate answer, got %v", err)
	}
}

func TestExec_QuestionAnsweredDuplicateAfterAcceptanceRejected(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()
	snap.Root.QuestionSlots[0] = engine.QuestionSlotInstance{Name: "Ask", Pending: &engine.PendingQuestion{Recipient: player}}

	answer := engine.Signal{Kind: engine.SignalKindQuestionAnswered, Slot: "Ask", Respondent: player, Answer: engine.BoolValue{Value: true}}
	commit, err := runtime.Step(p, snap, answer, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error on first delivery: %v", err)
	}

	// Deliver the exact same answer again against the resulting snapshot.
	_, err = runtime.Step(p, commit.Snapshot, answer, engine.DefaultLimits())
	if err != runtime.ErrSignalRejected && err != runtime.ErrInputRejected {
		t.Fatalf("expected the duplicate answer to be rejected, got %v", err)
	}
}

func TestExec_QuestionAnsweredFailingValidationRejected(t *testing.T) {
	p := questionDemoProgram()
	q := p.Questions["Confirm"]
	q.Validation = engine.ReferenceExpression{Name: "answer"} // must be true
	p.Questions["Confirm"] = q

	snap := questionDemoSnapshot()
	snap.Root.QuestionSlots[0] = engine.QuestionSlotInstance{Name: "Ask", Pending: &engine.PendingQuestion{Recipient: player}}

	_, err := runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindQuestionAnswered, Slot: "Ask", Respondent: player, Answer: engine.BoolValue{Value: false}}, engine.DefaultLimits())
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected for an answer failing Validation, got %v", err)
	}
}

func TestExec_ScheduleTimerOccupiesSlotAndProducesOutput(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Schedule"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	slot, _ := findInstanceTimerSlot(commit.Snapshot.Root, "Deadline")
	if !slot.Pending {
		t.Fatalf("got %+v", slot)
	}
	if out, ok := commit.Outputs[0].(engine.ScheduleTimerOutput); !ok || out.Slot != "Deadline" || out.DelayMilliseconds != 5000 {
		t.Fatalf("got %+v", commit.Outputs[0])
	}
}

func TestExec_ScheduleTimerOnOccupiedSlotFails(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()
	snap.Root.TimerSlots[0] = engine.TimerSlotInstance{Name: "Deadline", Pending: true}

	_, err := runtime.Step(p, snap, engine.Signal{Name: "Schedule"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorSlotOccupied {
		t.Fatalf("expected runtime.ExecutionErrorSlotOccupied, got %v", err)
	}
}

func TestExec_CancelTimerClearsOccupiedSlotAndOutput(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()
	snap.Root.TimerSlots[0] = engine.TimerSlotInstance{Name: "Deadline", Pending: true}

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Cancel"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	slot, _ := findInstanceTimerSlot(commit.Snapshot.Root, "Deadline")
	if slot.Pending {
		t.Fatal("expected the timer slot to be cleared")
	}
	if _, ok := commit.Outputs[0].(engine.CancelTimerOutput); !ok {
		t.Fatalf("got %+v", commit.Outputs[0])
	}
}

func TestExec_CancelTimerOnEmptySlotIsNoOp(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Cancel"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commit.Outputs) != 0 {
		t.Fatalf("expected no output, got %+v", commit.Outputs)
	}
}

func TestExec_TimerExpiredAcceptedClearsSlotAndDispatches(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()
	snap.Root.TimerSlots[0] = engine.TimerSlotInstance{Name: "Deadline", Pending: true}

	commit, err := runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindTimerExpired, Slot: "Deadline"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.Outcome == nil || commit.Snapshot.Root.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("expected the Expired transition to run and complete, got %+v", commit.Snapshot.Root.Outcome)
	}
	slot, _ := findInstanceTimerSlot(commit.Snapshot.Root, "Deadline")
	if slot.Pending {
		t.Fatal("expected the accepted expiration to clear its slot")
	}
}

func TestExec_TimerExpiredNotPendingRejected(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot() // Deadline starts empty

	_, err := runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindTimerExpired, Slot: "Deadline"}, engine.DefaultLimits())
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected for a not-pending timer, got %v", err)
	}
}

func TestExec_EmitEffectProducesOutput(t *testing.T) {
	p := questionDemoProgram()
	snap := questionDemoSnapshot()

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Emit"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commit.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(commit.Outputs))
	}
	out, ok := commit.Outputs[0].(engine.EmitEffectOutput)
	if !ok || out.Effect != "Confetti" || len(out.Recipients) != 1 || out.Recipients[0] != player {
		t.Fatalf("got %+v", commit.Outputs[0])
	}
	if len(out.Arguments) != 1 || out.Arguments[0].Name != "amount" || out.Arguments[0].Value.(engine.NumberValue).Value != 10 {
		t.Fatalf("got %+v", out.Arguments)
	}
}

func TestExec_InvariantViolationAfterOpenQuestionLeavesSlotsUntouched(t *testing.T) {
	p := questionDemoProgram()
	p.Invariants = []engine.Invariant{
		{Name: "NeverOpen", Condition: engine.BoolLiteralExpression{Value: false}},
	}
	snap := questionDemoSnapshot()

	_, err := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorInvariantViolation {
		t.Fatalf("expected runtime.ExecutionErrorInvariantViolation, got %v", err)
	}

	slot, _ := findInstanceQuestionSlot(snap.Root, "Ask")
	if slot.Pending != nil {
		t.Fatal("original snapshot's question slot must remain untouched after a failed step")
	}
}
