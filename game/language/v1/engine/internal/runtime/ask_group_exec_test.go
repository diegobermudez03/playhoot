package runtime_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/runtime"
)

const (
	askAlice = engine.UserID("alice")
	askBob   = engine.UserID("bob")
	askCarol = engine.UserID("carol")
)

// askGroupProgram builds a hand-assembled engine.Program (bypassing
// compiler.Compile, mirroring step_test.go's style): a "Main" workflow taking a
// list<user> parameter "recipients", with one ask-group slot "Ask"
// backed by a bool-answering question "Confirm", able to open (with the
// given completion policy), finalize, or cancel the group, and join its
// completion signal with joinControl (bindings "responses"->r,
// "respondents"->resp, "missing"->m).
func askGroupProgram(completion engine.AskGroupCompletionPolicy, resultType engine.Type, joinControl engine.WorkflowControl) engine.Program {
	openOp := engine.OpenAskGroupOperation{Slot: "Ask", Recipients: engine.ReferenceExpression{Name: "recipients"}, Completion: completion}
	finalizeOp := engine.FinalizeAskGroupOperation{Slot: "Ask"}
	cancelOp := engine.CancelAskGroupOperation{Slot: "Ask"}

	main := engine.Workflow{
		Name:          "Main",
		Parameters:    []engine.FieldType{{Name: "recipients", Type: engine.ListType{Element: engine.UserType{}}}},
		ResultType:    resultType,
		AskGroupSlots: []engine.AskGroupSlot{{Name: "Ask", Question: "Confirm"}},
		InitialState:  "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "Open", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Open"}}, Operations: engine.Block{Operations: []engine.Operation{openOp}}, Control: engine.StayControl{}},
					{Name: "Finalize", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Finalize"}}, Operations: engine.Block{Operations: []engine.Operation{finalizeOp}}, Control: engine.StayControl{}},
					{Name: "CancelAsk", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "CancelAsk"}}, Operations: engine.Block{Operations: []engine.Operation{cancelOp}}, Control: engine.StayControl{}},
					{
						Name: "Join",
						Signal: engine.SignalPattern{
							Source: engine.AskGroupCompletedSignalSource{Slot: "Ask"},
							Bindings: []engine.SignalBinding{
								{Field: "responses", Name: "r"},
								{Field: "respondents", Name: "resp"},
								{Field: "missing", Name: "m"},
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
		Workflows:    map[string]engine.Workflow{"Main": main},
		Questions: map[string]engine.Question{
			"Confirm": {Name: "Confirm", ResponseType: engine.BoolType{}},
		},
	}
}

func askGroupSnapshot(recipients []engine.UserID) engine.Snapshot {
	elements := make([]engine.Value, len(recipients))
	for i, r := range recipients {
		elements[i] = engine.UserValue{ID: r}
	}
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root: engine.WorkflowInstance{
			Workflow:      "Main",
			State:         "S",
			Parameters:    []engine.FieldValue{{Name: "recipients", Value: engine.ListValue{ElementType: engine.UserType{}, Elements: elements}}},
			LocalState:    engine.RecordValue{TypeName: "local"},
			AskGroupSlots: []engine.AskGroupSlotInstance{{Name: "Ask"}},
		},
	}
}

func answerAskGroup(p engine.Program, snap engine.Snapshot, respondent engine.UserID, answer bool) (engine.Commit, error) {
	return runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindAskGroupAnswered, Slot: "Ask", Respondent: respondent, Answer: engine.BoolValue{Value: answer}}, engine.DefaultLimits())
}

func TestExec_OpenAskGroupAllResponses_CompletesWhenEveryoneAnswers(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice, askBob, askCarol})

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error opening: %v", err)
	}
	if len(commit.Outputs) != 3 {
		t.Fatalf("got %d outputs, want 3", len(commit.Outputs))
	}
	snap = commit.Snapshot

	commit, err = answerAskGroup(p, snap, askAlice, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap = commit.Snapshot
	if snap.Root.AskGroupSlots[0].Pending.Completed {
		t.Fatal("expected the group to still be collecting after one of three answers")
	}

	commit, err = answerAskGroup(p, snap, askBob, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap = commit.Snapshot
	if snap.Root.AskGroupSlots[0].Pending.Completed {
		t.Fatal("expected the group to still be collecting after two of three answers")
	}

	commit, err = answerAskGroup(p, snap, askCarol, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap = commit.Snapshot
	pending := snap.Root.AskGroupSlots[0].Pending
	if !pending.Completed {
		t.Fatal("expected the group to complete once everyone answered")
	}
	if len(pending.Responses) != 3 || pending.Responses[0].Respondent != askAlice || pending.Responses[1].Respondent != askBob || pending.Responses[2].Respondent != askCarol {
		t.Fatalf("expected acceptance-order responses, got %+v", pending.Responses)
	}

	commit, err = runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindAskGroupCompleted, Slot: "Ask"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error joining: %v", err)
	}
	if commit.Snapshot.Root.AskGroupSlots[0].Pending != nil {
		t.Fatal("expected the slot to be cleared after joining")
	}
}

func TestExec_OpenAskGroupAllResponses_EmptyRecipientsCompletesImmediately(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot(nil)

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pending := commit.Snapshot.Root.AskGroupSlots[0].Pending
	if pending == nil || !pending.Completed {
		t.Fatalf("expected an all-responses group with no recipients to complete immediately, got %+v", pending)
	}

	_, err = runtime.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindAskGroupCompleted, Slot: "Ask"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error joining: %v", err)
	}
}

func TestExec_OpenAskGroupFirstResponse_CompletesOnFirstAnswer(t *testing.T) {
	p := askGroupProgram(engine.AskGroupFirstResponsePolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice, askBob, askCarol})

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap = commit.Snapshot

	commit, err = answerAskGroup(p, snap, askBob, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pending := commit.Snapshot.Root.AskGroupSlots[0].Pending
	if !pending.Completed {
		t.Fatal("expected first-response to complete on the first answer")
	}
	if len(pending.Responses) != 1 || pending.Responses[0].Respondent != askBob {
		t.Fatalf("got %+v", pending.Responses)
	}

	// A second answer must now be rejected: the group already completed.
	_, err = answerAskGroup(p, commit.Snapshot, askAlice, true)
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected for an answer after first-response completion, got %v", err)
	}
}

func TestExec_OpenAskGroupFirstResponse_NoRecipientsIsExecutionError(t *testing.T) {
	p := askGroupProgram(engine.AskGroupFirstResponsePolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot(nil)

	_, err := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorInvalidQuorum {
		t.Fatalf("expected runtime.ExecutionErrorInvalidQuorum, got %v", err)
	}
}

func TestExec_OpenAskGroupQuorum_CompletesAtCount(t *testing.T) {
	p := askGroupProgram(engine.AskGroupQuorumPolicy{Count: engine.NumberLiteralExpression{Value: 2}}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice, askBob, askCarol})

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap = commit.Snapshot

	commit, err = answerAskGroup(p, snap, askAlice, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.AskGroupSlots[0].Pending.Completed {
		t.Fatal("expected quorum of 2 to still be collecting after 1 answer")
	}

	commit, err = answerAskGroup(p, commit.Snapshot, askCarol, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pending := commit.Snapshot.Root.AskGroupSlots[0].Pending
	if !pending.Completed {
		t.Fatal("expected quorum of 2 to complete after 2 answers")
	}
	if len(pending.Responses) != 2 {
		t.Fatalf("got %+v", pending.Responses)
	}
}

func TestExec_OpenAskGroupQuorum_InvalidCountIsExecutionErrorAtomically(t *testing.T) {
	p := askGroupProgram(engine.AskGroupQuorumPolicy{Count: engine.NumberLiteralExpression{Value: 5}}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice, askBob, askCarol})

	_, err := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorInvalidQuorum {
		t.Fatalf("expected runtime.ExecutionErrorInvalidQuorum, got %v", err)
	}
	if snap.Root.AskGroupSlots[0].Pending != nil {
		t.Fatal("original snapshot's ask-group slot must remain untouched")
	}
}

func TestExec_AcceptedResponseOrderingIsAcceptanceOrderNotRecipientOrder(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice, askBob, askCarol})

	commit, _ := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	snap = commit.Snapshot
	commit, _ = answerAskGroup(p, snap, askCarol, true)
	snap = commit.Snapshot
	commit, _ = answerAskGroup(p, snap, askAlice, true)
	snap = commit.Snapshot
	commit, err := answerAskGroup(p, snap, askBob, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pending := commit.Snapshot.Root.AskGroupSlots[0].Pending
	want := []engine.UserID{askCarol, askAlice, askBob}
	for i, w := range want {
		if pending.Responses[i].Respondent != w {
			t.Fatalf("got order %+v, want %v", pending.Responses, want)
		}
	}
}

func TestExec_DuplicateRecipientInOpenIsRejected(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice, askAlice})

	_, err := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorDuplicateRecipient {
		t.Fatalf("expected runtime.ExecutionErrorDuplicateRecipient, got %v", err)
	}
}

func TestExec_AnswerFromNonRecipientRejected(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice, askBob})
	commit, _ := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())

	_, err := answerAskGroup(p, commit.Snapshot, askCarol, true)
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected for a non-recipient answer, got %v", err)
	}
}

func TestExec_DuplicateAnswerFromSameRespondentRejected(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice, askBob})
	commit, _ := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	commit, err := answerAskGroup(p, commit.Snapshot, askAlice, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = answerAskGroup(p, commit.Snapshot, askAlice, false)
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected for a duplicate answer, got %v", err)
	}
}

func TestExec_FinalizeCollectingGroupCompletesWithPartialResponses(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice, askBob, askCarol})
	commit, _ := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	commit, err := answerAskGroup(p, commit.Snapshot, askAlice, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commit, err = runtime.Step(p, commit.Snapshot, engine.Signal{Name: "Finalize"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error finalizing: %v", err)
	}
	pending := commit.Snapshot.Root.AskGroupSlots[0].Pending
	if !pending.Completed {
		t.Fatal("expected finalize to complete the group")
	}
	if len(pending.Responses) != 1 || pending.Responses[0].Respondent != askAlice {
		t.Fatalf("got %+v", pending.Responses)
	}
	if len(commit.Outputs) != 2 {
		t.Fatalf("expected a CloseQuestionOutput for each of the 2 missing recipients, got %+v", commit.Outputs)
	}
}

func TestExec_FinalizeAlreadyCompletedIsIdempotentNoOp(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice})
	commit, _ := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	commit, err := answerAskGroup(p, commit.Snapshot, askAlice, true) // completes naturally
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pendingBefore := commit.Snapshot.Root.AskGroupSlots[0].Pending

	commit, err = runtime.Step(p, commit.Snapshot, engine.Signal{Name: "Finalize"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error on a finalize-vs-answer race: %v", err)
	}
	if len(commit.Outputs) != 0 {
		t.Fatalf("expected finalize on an already-completed group to be a no-op, got outputs %+v", commit.Outputs)
	}
	pendingAfter := commit.Snapshot.Root.AskGroupSlots[0].Pending
	if len(pendingAfter.Responses) != len(pendingBefore.Responses) {
		t.Fatalf("expected responses to be unchanged, got %+v", pendingAfter.Responses)
	}
}

func TestExec_FinalizeEmptySlotIsExecutionError(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice})

	_, err := runtime.Step(p, snap, engine.Signal{Name: "Finalize"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorUnknown {
		t.Fatalf("expected an execution error for finalizing an empty slot, got %v", err)
	}
}

func TestExec_CancelCollectingGroupDiscardsResponses(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice, askBob, askCarol})
	commit, _ := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	commit, err := answerAskGroup(p, commit.Snapshot, askAlice, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commit, err = runtime.Step(p, commit.Snapshot, engine.Signal{Name: "CancelAsk"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.AskGroupSlots[0].Pending != nil {
		t.Fatal("expected cancellation to clear the slot")
	}
	if len(commit.Outputs) != 2 {
		t.Fatalf("expected a CloseQuestionOutput for each of the 2 never-answered recipients, got %+v", commit.Outputs)
	}
}

func TestExec_CancelEmptySlotIsNoOp(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice})

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "CancelAsk"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commit.Outputs) != 0 {
		t.Fatalf("expected no outputs, got %+v", commit.Outputs)
	}
}

func TestExec_CancelCompletedAwaitingJoinIsExecutionErrorAtomically(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice})
	commit, _ := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	commit, err := answerAskGroup(p, commit.Snapshot, askAlice, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap = commit.Snapshot

	_, err = runtime.Step(p, snap, engine.Signal{Name: "CancelAsk"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorAskGroupNotJoined {
		t.Fatalf("expected runtime.ExecutionErrorAskGroupNotJoined, got %v", err)
	}
	if snap.Root.AskGroupSlots[0].Pending == nil {
		t.Fatal("the completed-awaiting-join group must remain untouched")
	}
}

func TestExec_JoinBindsResponsesRespondentsAndMissing(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.ListType{Element: engine.UserType{}}, engine.CompleteControl{Result: engine.ReferenceExpression{Name: "m"}})
	snap := askGroupSnapshot([]engine.UserID{askAlice, askBob, askCarol})
	commit, _ := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	commit, _ = answerAskGroup(p, commit.Snapshot, askAlice, true)
	commit, err := runtime.Step(p, commit.Snapshot, engine.Signal{Name: "Finalize"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commit, err = runtime.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindAskGroupCompleted, Slot: "Ask"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error joining: %v", err)
	}
	missing := commit.Snapshot.Root.Outcome.Result.(engine.ListValue).Elements
	if len(missing) != 2 || missing[0].(engine.UserValue).ID != askBob || missing[1].(engine.UserValue).ID != askCarol {
		t.Fatalf("got missing %+v, want [bob, carol] in original recipient order", missing)
	}
}

func TestExec_JoinStaleOrDuplicateRejected(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	snap := askGroupSnapshot([]engine.UserID{askAlice})

	// Not even opened yet.
	_, err := runtime.Step(p, snap, engine.Signal{Kind: engine.SignalKindAskGroupCompleted, Slot: "Ask"}, engine.DefaultLimits())
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected joining an unopened slot, got %v", err)
	}

	commit, _ := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	// Collecting, not yet completed.
	_, err = runtime.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindAskGroupCompleted, Slot: "Ask"}, engine.DefaultLimits())
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected joining a still-collecting slot, got %v", err)
	}

	commit, _ = answerAskGroup(p, commit.Snapshot, askAlice, true) // completes naturally
	commit, err = runtime.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindAskGroupCompleted, Slot: "Ask"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = runtime.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindAskGroupCompleted, Slot: "Ask"}, engine.DefaultLimits())
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected for a duplicate join, got %v", err)
	}
}
