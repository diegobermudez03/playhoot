package runtime_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/runtime"
)

const (
	keyedPlayerA = engine.UserID("player-a")
	keyedPlayerB = engine.UserID("player-b")
)

func findInstanceKeyedQuestionSlot(instance engine.WorkflowInstance, name string) (engine.KeyedQuestionSlotInstance, bool) {
	for _, s := range instance.KeyedQuestionSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.KeyedQuestionSlotInstance{}, false
}

func findKeyedQuestionPendingByKey(pending []engine.KeyedPendingQuestion, key string) (engine.KeyedPendingQuestion, bool) {
	for _, p := range pending {
		if p.Key.(engine.StringValue).Value == key {
			return p, true
		}
	}
	return engine.KeyedPendingQuestion{}, false
}

func findInstanceKeyedTimerSlot(instance engine.WorkflowInstance, name string) (engine.KeyedTimerSlotInstance, bool) {
	for _, s := range instance.KeyedTimerSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.KeyedTimerSlotInstance{}, false
}

func hasKeyedTimerPending(pending []engine.KeyedPendingTimer, key string) bool {
	for _, p := range pending {
		if p.Key.(engine.StringValue).Value == key {
			return true
		}
	}
	return false
}

// keyedInteractionDemoProgram builds a hand-assembled engine.Program
// (bypassing compiler.Compile, mirroring interaction_exec_test.go's
// style) for a workflow "KDemo" with one keyed question slot ("Quiz")
// and one keyed timer slot ("Deadline"), both keyed by string. Every
// operation's Key/Recipient come from a UserIntentSignalSource's bound
// fields ("key"/"recipient"), rather than from fixed workflow
// parameters, so each signal delivery can target a different key — the
// dynamic-per-delivery addressing every keyed operation is designed
// for. The Answered/Expired transitions record the bound "key" (and,
// for Answered, "answer") into global state via SetOperation and stay
// in place (never complete), so a test can deliver several signals in
// sequence and inspect the resulting global state and slot occupancy
// after each one.
func keyedInteractionDemoProgram() engine.Program {
	keyRef := engine.ReferenceExpression{Name: "key"}
	openOp := engine.OpenKeyedQuestionOperation{
		Slot:      "Quiz",
		Key:       keyRef,
		Recipient: engine.ReferenceExpression{Name: "recipient"},
		Arguments: []engine.CallArgument{{Name: "prompt", Value: engine.StringLiteralExpression{Value: "Ready?"}}},
	}
	closeOp := engine.CloseKeyedQuestionOperation{Slot: "Quiz", Key: keyRef}
	scheduleOp := engine.ScheduleKeyedTimerOperation{Slot: "Deadline", Key: keyRef, DelayMilliseconds: engine.NumberLiteralExpression{Value: 5000}}
	cancelOp := engine.CancelKeyedTimerOperation{Slot: "Deadline", Key: keyRef}

	intentSignal := func(intent string, bindings ...engine.SignalBinding) engine.SignalPattern {
		return engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: intent}, Bindings: bindings}
	}
	setGlobal := func(field string, value engine.Expression) engine.Operation {
		return engine.SetOperation{
			Target: engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: field},
			Value:  value,
		}
	}

	workflow := engine.Workflow{
		Name:               "KDemo",
		ResultType:         engine.UnitType{},
		KeyedQuestionSlots: []engine.KeyedQuestionSlot{{Name: "Quiz", Question: "Confirm", KeyType: engine.StringType{}}},
		KeyedTimerSlots:    []engine.KeyedTimerSlot{{Name: "Deadline", KeyType: engine.StringType{}}},
		InitialState:       "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Name:       "Open",
						Signal:     intentSignal("Open", engine.SignalBinding{Field: "key", Name: "key"}, engine.SignalBinding{Field: "recipient", Name: "recipient"}),
						Operations: engine.Block{Operations: []engine.Operation{openOp}},
						Control:    engine.StayControl{},
					},
					{
						Name:       "Close",
						Signal:     intentSignal("Close", engine.SignalBinding{Field: "key", Name: "key"}),
						Operations: engine.Block{Operations: []engine.Operation{closeOp}},
						Control:    engine.StayControl{},
					},
					{
						Name:       "Schedule",
						Signal:     intentSignal("Schedule", engine.SignalBinding{Field: "key", Name: "key"}),
						Operations: engine.Block{Operations: []engine.Operation{scheduleOp}},
						Control:    engine.StayControl{},
					},
					{
						Name:       "CancelTimer",
						Signal:     intentSignal("CancelTimer", engine.SignalBinding{Field: "key", Name: "key"}),
						Operations: engine.Block{Operations: []engine.Operation{cancelOp}},
						Control:    engine.StayControl{},
					},
					{
						Name: "Answered",
						Signal: engine.SignalPattern{
							Source: engine.KeyedQuestionAnsweredSignalSource{Slot: "Quiz"},
							Bindings: []engine.SignalBinding{
								{Field: "key", Name: "k"},
								{Field: "answer", Name: "a"},
							},
						},
						Operations: engine.Block{Operations: []engine.Operation{
							setGlobal("lastKey", engine.ReferenceExpression{Name: "k"}),
							setGlobal("lastAnswer", engine.ReferenceExpression{Name: "a"}),
						}},
						Control: engine.StayControl{},
					},
					{
						Name: "Expired",
						Signal: engine.SignalPattern{
							Source:   engine.KeyedTimerExpiredSignalSource{Slot: "Deadline"},
							Bindings: []engine.SignalBinding{{Field: "key", Name: "k"}},
						},
						Operations: engine.Block{Operations: []engine.Operation{setGlobal("lastKey", engine.ReferenceExpression{Name: "k"})}},
						Control:    engine.StayControl{},
					},
				},
			},
		},
	}

	return engine.Program{
		RootWorkflow: "KDemo",
		Workflows:    map[string]engine.Workflow{"KDemo": workflow},
		GlobalState: []engine.StateField{
			{Name: "lastKey", Type: engine.StringType{}, Initializer: engine.StringLiteralExpression{Value: ""}},
			{Name: "lastAnswer", Type: engine.BoolType{}, Initializer: engine.BoolLiteralExpression{Value: false}},
		},
		Questions: map[string]engine.Question{
			"Confirm": {
				Name:         "Confirm",
				Parameters:   []engine.FieldType{{Name: "prompt", Type: engine.StringType{}}},
				ResponseType: engine.BoolType{},
			},
		},
	}
}

func keyedInteractionDemoSnapshot() engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global", Fields: []engine.FieldValue{
			{Name: "lastKey", Value: engine.StringValue{Value: ""}},
			{Name: "lastAnswer", Value: engine.BoolValue{Value: false}},
		}},
		Root: engine.WorkflowInstance{
			Workflow:           "KDemo",
			State:              "S",
			LocalState:         engine.RecordValue{TypeName: "local"},
			KeyedQuestionSlots: []engine.KeyedQuestionSlotInstance{{Name: "Quiz"}},
			KeyedTimerSlots:    []engine.KeyedTimerSlotInstance{{Name: "Deadline"}},
		},
	}
}

func openKeyedQuestion(p engine.Program, snap engine.Snapshot, key string, recipient engine.UserID) (engine.Commit, error) {
	return runtime.Step(p, snap, engine.Signal{
		Kind:   engine.SignalKindIntent,
		Intent: "Open",
		Fields: map[string]engine.Value{"key": engine.StringValue{Value: key}, "recipient": engine.UserValue{ID: recipient}},
	}, engine.DefaultLimits())
}

func answerKeyedQuestion(p engine.Program, snap engine.Snapshot, key string, respondent engine.UserID, answer bool) (engine.Commit, error) {
	return runtime.Step(p, snap, engine.Signal{
		Kind:       engine.SignalKindKeyedQuestionAnswered,
		Slot:       "Quiz",
		Key:        engine.StringValue{Value: key},
		Respondent: respondent,
		Answer:     engine.BoolValue{Value: answer},
	}, engine.DefaultLimits())
}

func TestExec_OpenKeyedQuestion_DifferentKeysAreIndependentlyPending(t *testing.T) {
	p := keyedInteractionDemoProgram()
	snap := keyedInteractionDemoSnapshot()

	commit, err := openKeyedQuestion(p, snap, "A", keyedPlayerA)
	if err != nil {
		t.Fatalf("unexpected error opening key A: %v", err)
	}
	commit, err = openKeyedQuestion(p, commit.Snapshot, "B", keyedPlayerB)
	if err != nil {
		t.Fatalf("unexpected error opening key B (must not contend with key A): %v", err)
	}

	slot, ok := findInstanceKeyedQuestionSlot(commit.Snapshot.Root, "Quiz")
	if !ok || len(slot.Pending) != 2 {
		t.Fatalf("expected both keys simultaneously pending, got %+v", slot)
	}
	a, aOK := findKeyedQuestionPendingByKey(slot.Pending, "A")
	b, bOK := findKeyedQuestionPendingByKey(slot.Pending, "B")
	if !aOK || a.Recipient != keyedPlayerA {
		t.Fatalf("expected key A pending for keyedPlayerA, got %+v", a)
	}
	if !bOK || b.Recipient != keyedPlayerB {
		t.Fatalf("expected key B pending for keyedPlayerB, got %+v", b)
	}
}

func TestExec_OpenKeyedQuestion_SameKeyOccupiedFailsAtomically(t *testing.T) {
	p := keyedInteractionDemoProgram()
	snap := keyedInteractionDemoSnapshot()
	commit, err := openKeyedQuestion(p, snap, "A", keyedPlayerA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	before := commit.Snapshot

	_, err = openKeyedQuestion(p, before, "A", keyedPlayerB)
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorSlotOccupied {
		t.Fatalf("expected runtime.ExecutionErrorSlotOccupied opening an already-occupied key, got %v", err)
	}

	// Atomic failure: the occupied key's own pending occurrence (and
	// everything else about the snapshot) must be entirely untouched.
	slot, _ := findInstanceKeyedQuestionSlot(before.Root, "Quiz")
	if len(slot.Pending) != 1 {
		t.Fatalf("original snapshot's keyed question slot was mutated: %+v", slot)
	}
}

// keyedOccupiedFailureAtomicityProgram builds a one-transition program
// whose single "OpenTwo" operation block first mutates global state and
// opens a free key, then attempts a second key: when that second key is
// occupied, the whole transition must fail without keeping any of the
// earlier operations' effects — not just "nothing happened because there
// was nothing to undo".
func keyedOccupiedFailureAtomicityProgram() engine.Program {
	openFree := engine.OpenKeyedQuestionOperation{Slot: "Quiz", Key: engine.StringLiteralExpression{Value: "free"}, Recipient: engine.ReferenceExpression{Name: "recipient"}}
	openTarget := engine.OpenKeyedQuestionOperation{Slot: "Quiz", Key: engine.ReferenceExpression{Name: "target"}, Recipient: engine.ReferenceExpression{Name: "recipient"}}
	setMarker := engine.SetOperation{
		Target: engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: "marker"},
		Value:  engine.BoolLiteralExpression{Value: true},
	}

	workflow := engine.Workflow{
		Name:               "KAtomic",
		Parameters:         []engine.FieldType{{Name: "recipient", Type: engine.UserType{}}, {Name: "target", Type: engine.StringType{}}},
		ResultType:         engine.UnitType{},
		KeyedQuestionSlots: []engine.KeyedQuestionSlot{{Name: "Quiz", Question: "Confirm", KeyType: engine.StringType{}}},
		InitialState:       "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Name:       "OpenTwo",
						Signal:     engine.SignalPattern{Source: engine.NamedSignalSource{Name: "OpenTwo"}},
						Operations: engine.Block{Operations: []engine.Operation{setMarker, openFree, openTarget}},
						Control:    engine.StayControl{},
					},
				},
			},
		},
	}

	return engine.Program{
		RootWorkflow: "KAtomic",
		Workflows:    map[string]engine.Workflow{"KAtomic": workflow},
		GlobalState:  []engine.StateField{{Name: "marker", Type: engine.BoolType{}, Initializer: engine.BoolLiteralExpression{Value: false}}},
		Questions:    map[string]engine.Question{"Confirm": {Name: "Confirm", ResponseType: engine.BoolType{}}},
	}
}

func keyedOccupiedFailureAtomicitySnapshot(recipient engine.UserID, target string) engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global", Fields: []engine.FieldValue{{Name: "marker", Value: engine.BoolValue{Value: false}}}},
		Root: engine.WorkflowInstance{
			Workflow: "KAtomic",
			State:    "S",
			Parameters: []engine.FieldValue{
				{Name: "recipient", Value: engine.UserValue{ID: recipient}},
				{Name: "target", Value: engine.StringValue{Value: target}},
			},
			LocalState:         engine.RecordValue{TypeName: "local"},
			KeyedQuestionSlots: []engine.KeyedQuestionSlotInstance{{Name: "Quiz"}},
		},
	}
}

func TestExec_OccupiedKeyFailsAtomically_DiscardsEarlierOperationsInSameTransition(t *testing.T) {
	p := keyedOccupiedFailureAtomicityProgram()

	// Control: with a free target key, the same operation sequence
	// (mutate global, open "free", open target) genuinely succeeds and
	// genuinely mutates state - establishing that these operations are
	// not no-ops to begin with, so their absence below is meaningful.
	control := keyedOccupiedFailureAtomicitySnapshot(keyedPlayerA, "also_free")
	commit, err := runtime.Step(p, control, engine.Signal{Name: "OpenTwo"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error on the uninterrupted control case: %v", err)
	}
	if marker, _ := commit.Snapshot.GlobalState.FieldByName("marker"); !marker.Value.(engine.BoolValue).Value {
		t.Fatal("control case: expected the marker mutation to commit when nothing fails")
	}
	slot, _ := findInstanceKeyedQuestionSlot(commit.Snapshot.Root, "Quiz")
	if len(slot.Pending) != 2 {
		t.Fatalf("control case: expected both keys pending when nothing fails, got %+v", slot)
	}

	// Now pre-occupy "free" (the operation sequence's first, always-
	// unconditional open) so the transition's *second* open - the one
	// targeting an already-occupied key - is what actually fails, after
	// the marker mutation and the first open have already run inside
	// this same transition.
	snap := keyedOccupiedFailureAtomicitySnapshot(keyedPlayerA, "free")
	snap.Root.KeyedQuestionSlots[0] = engine.KeyedQuestionSlotInstance{
		Name:    "Quiz",
		Pending: []engine.KeyedPendingQuestion{{Key: engine.StringValue{Value: "free"}, Recipient: keyedPlayerB}},
	}
	before := snap

	_, err = runtime.Step(p, snap, engine.Signal{Name: "OpenTwo"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorSlotOccupied {
		t.Fatalf("expected runtime.ExecutionErrorSlotOccupied, got %v", err)
	}

	// The marker mutation and the first (successful, in isolation) open
	// must both have been discarded along with the whole failed
	// transition - not just "nothing changed because nothing ran".
	if marker, _ := before.GlobalState.FieldByName("marker"); marker.Value.(engine.BoolValue).Value {
		t.Fatal("expected the marker mutation from earlier in the same failed transition to be discarded, not committed")
	}
	slotAfter, _ := findInstanceKeyedQuestionSlot(before.Root, "Quiz")
	if len(slotAfter.Pending) != 1 || slotAfter.Pending[0].Key.(engine.StringValue).Value != "free" {
		t.Fatalf("expected only the pre-existing occupant of key %q to remain, with no new key opened by the failed transition, got %+v", "free", slotAfter)
	}
}

func TestExec_CloseKeyedQuestion_ScopedToOneKey(t *testing.T) {
	p := keyedInteractionDemoProgram()
	snap := keyedInteractionDemoSnapshot()
	commit, _ := openKeyedQuestion(p, snap, "A", keyedPlayerA)
	commit, err := openKeyedQuestion(p, commit.Snapshot, "B", keyedPlayerB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commit, err = runtime.Step(p, commit.Snapshot, engine.Signal{
		Kind: engine.SignalKindIntent, Intent: "Close", Fields: map[string]engine.Value{"key": engine.StringValue{Value: "A"}},
	}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error closing key A: %v", err)
	}

	slot, _ := findInstanceKeyedQuestionSlot(commit.Snapshot.Root, "Quiz")
	if _, ok := findKeyedQuestionPendingByKey(slot.Pending, "A"); ok {
		t.Fatal("expected key A to be closed")
	}
	if _, ok := findKeyedQuestionPendingByKey(slot.Pending, "B"); !ok {
		t.Fatal("expected key B to remain pending, untouched by closing key A")
	}
}

func TestExec_KeyedQuestionAnswered_RoutesToTheCorrectKeyOnly(t *testing.T) {
	p := keyedInteractionDemoProgram()
	snap := keyedInteractionDemoSnapshot()
	commit, _ := openKeyedQuestion(p, snap, "A", keyedPlayerA)
	commit, err := openKeyedQuestion(p, commit.Snapshot, "B", keyedPlayerB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commit, err = answerKeyedQuestion(p, commit.Snapshot, "A", keyedPlayerA, true)
	if err != nil {
		t.Fatalf("unexpected error answering key A: %v", err)
	}
	global := commit.Snapshot.GlobalState
	if lastKey, _ := global.FieldByName("lastKey"); lastKey.Value.(engine.StringValue).Value != "A" {
		t.Fatalf("expected the Answered transition to bind key %q, got %+v", "A", lastKey)
	}
	slot, _ := findInstanceKeyedQuestionSlot(commit.Snapshot.Root, "Quiz")
	if _, ok := findKeyedQuestionPendingByKey(slot.Pending, "A"); ok {
		t.Fatal("expected key A to be cleared once its answer was accepted")
	}
	if _, ok := findKeyedQuestionPendingByKey(slot.Pending, "B"); !ok {
		t.Fatal("expected key B to remain pending, unaffected by key A's answer")
	}

	// Key B is still independently answerable afterward.
	commit, err = answerKeyedQuestion(p, commit.Snapshot, "B", keyedPlayerB, false)
	if err != nil {
		t.Fatalf("unexpected error answering key B: %v", err)
	}
	global = commit.Snapshot.GlobalState
	if lastKey, _ := global.FieldByName("lastKey"); lastKey.Value.(engine.StringValue).Value != "B" {
		t.Fatalf("expected the Answered transition to now bind key %q, got %+v", "B", lastKey)
	}
	if lastAnswer, _ := global.FieldByName("lastAnswer"); lastAnswer.Value.(engine.BoolValue).Value != false {
		t.Fatalf("expected the bound answer to be false, got %+v", lastAnswer)
	}
}

func TestExec_KeyedQuestionAnswered_WrongRespondentRejected(t *testing.T) {
	p := keyedInteractionDemoProgram()
	snap := keyedInteractionDemoSnapshot()
	commit, err := openKeyedQuestion(p, snap, "A", keyedPlayerA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = answerKeyedQuestion(p, commit.Snapshot, "A", keyedPlayerB, true)
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected for an unauthorized respondent, got %v", err)
	}
}

func TestExec_KeyedQuestionAnswered_StaleOrUnknownKeyRejected(t *testing.T) {
	p := keyedInteractionDemoProgram()
	snap := keyedInteractionDemoSnapshot()

	_, err := answerKeyedQuestion(p, snap, "A", keyedPlayerA, true)
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected answering a key that was never opened, got %v", err)
	}
}

func TestExec_ScheduleKeyedTimer_DifferentKeysAreIndependentlyPending(t *testing.T) {
	p := keyedInteractionDemoProgram()
	snap := keyedInteractionDemoSnapshot()

	schedule := func(snap engine.Snapshot, key string) (engine.Commit, error) {
		return runtime.Step(p, snap, engine.Signal{
			Kind: engine.SignalKindIntent, Intent: "Schedule", Fields: map[string]engine.Value{"key": engine.StringValue{Value: key}},
		}, engine.DefaultLimits())
	}

	commit, err := schedule(snap, "A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	commit, err = schedule(commit.Snapshot, "B")
	if err != nil {
		t.Fatalf("unexpected error scheduling key B (must not contend with key A): %v", err)
	}
	slot, _ := findInstanceKeyedTimerSlot(commit.Snapshot.Root, "Deadline")
	if !hasKeyedTimerPending(slot.Pending, "A") || !hasKeyedTimerPending(slot.Pending, "B") {
		t.Fatalf("expected both keys simultaneously pending, got %+v", slot)
	}

	_, err = schedule(commit.Snapshot, "A")
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorSlotOccupied {
		t.Fatalf("expected runtime.ExecutionErrorSlotOccupied scheduling an already-occupied key, got %v", err)
	}
}

func TestExec_KeyedTimerExpired_RoutesToTheCorrectKeyOnly(t *testing.T) {
	p := keyedInteractionDemoProgram()
	snap := keyedInteractionDemoSnapshot()
	schedule := func(snap engine.Snapshot, key string) engine.Commit {
		c, err := runtime.Step(p, snap, engine.Signal{
			Kind: engine.SignalKindIntent, Intent: "Schedule", Fields: map[string]engine.Value{"key": engine.StringValue{Value: key}},
		}, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error scheduling %q: %v", key, err)
		}
		return c
	}
	commit := schedule(snap, "A")
	commit = schedule(commit.Snapshot, "B")

	commit, err := runtime.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindKeyedTimerExpired, Slot: "Deadline", Key: engine.StringValue{Value: "A"}}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error expiring key A: %v", err)
	}
	if lastKey, _ := commit.Snapshot.GlobalState.FieldByName("lastKey"); lastKey.Value.(engine.StringValue).Value != "A" {
		t.Fatalf("expected the Expired transition to bind key %q, got %+v", "A", lastKey)
	}
	slot, _ := findInstanceKeyedTimerSlot(commit.Snapshot.Root, "Deadline")
	if hasKeyedTimerPending(slot.Pending, "A") {
		t.Fatal("expected key A's timer to be cleared once its expiration was accepted")
	}
	if !hasKeyedTimerPending(slot.Pending, "B") {
		t.Fatal("expected key B's timer to remain pending, unaffected by key A's expiration")
	}

	_, err = runtime.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindKeyedTimerExpired, Slot: "Deadline", Key: engine.StringValue{Value: "A"}}, engine.DefaultLimits())
	if err != runtime.ErrInputRejected {
		t.Fatalf("expected runtime.ErrInputRejected for a stale/duplicate expiration of key A, got %v", err)
	}
}

func TestExec_CancelKeyedTimer_ScopedToOneKey(t *testing.T) {
	p := keyedInteractionDemoProgram()
	snap := keyedInteractionDemoSnapshot()
	schedule := func(snap engine.Snapshot, key string) engine.Commit {
		c, err := runtime.Step(p, snap, engine.Signal{
			Kind: engine.SignalKindIntent, Intent: "Schedule", Fields: map[string]engine.Value{"key": engine.StringValue{Value: key}},
		}, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error scheduling %q: %v", key, err)
		}
		return c
	}
	commit := schedule(snap, "A")
	commit = schedule(commit.Snapshot, "B")

	commit, err := runtime.Step(p, commit.Snapshot, engine.Signal{
		Kind: engine.SignalKindIntent, Intent: "CancelTimer", Fields: map[string]engine.Value{"key": engine.StringValue{Value: "A"}},
	}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error cancelling key A: %v", err)
	}
	slot, _ := findInstanceKeyedTimerSlot(commit.Snapshot.Root, "Deadline")
	if hasKeyedTimerPending(slot.Pending, "A") {
		t.Fatal("expected key A to be cancelled")
	}
	if !hasKeyedTimerPending(slot.Pending, "B") {
		t.Fatal("expected key B to remain pending, untouched by cancelling key A")
	}
}

func TestExec_KeyedActiveSlotLimit_CountsEveryOccupiedKey(t *testing.T) {
	p := keyedInteractionDemoProgram()
	snap := keyedInteractionDemoSnapshot()
	commit, err := openKeyedQuestion(p, snap, "A", keyedPlayerA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	limits := engine.DefaultLimits()
	limits.MaxActiveSlotsPerInstance = 1
	_, err = runtime.Step(p, commit.Snapshot, engine.Signal{
		Kind: engine.SignalKindIntent, Intent: "Open", Fields: map[string]engine.Value{"key": engine.StringValue{Value: "B"}, "recipient": engine.UserValue{ID: keyedPlayerB}},
	}, limits)
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorActiveSlotLimitExceeded {
		t.Fatalf("expected runtime.ExecutionErrorActiveSlotLimitExceeded once the one already-occupied key exhausts a limit of 1, got %v", err)
	}
}

// keyedQuestionPresentationProgram builds a hand-assembled engine.Program
// (bypassing compiler.Compile, mirroring presentationProgram()'s style):
// a "Main" workflow with one keyed question slot ("Quiz", keyed by
// string) whose Presentation projects both the implicit "recipient" and
// "key" bindings into its model, proving the "key" binding
// KeyedQuestionSlotDeclaration's own doc comment documents is actually
// available and threaded through by the runtime, not just the compiler.
func keyedQuestionPresentationProgram() engine.Program {
	proj := engine.Projection{
		Name:       "KeyProj",
		Parameters: []engine.FieldType{{Name: "k", Type: engine.StringType{}}},
		ResultType: engine.StringType{},
		Body:       engine.ReferenceExpression{Name: "k"},
	}
	view := engine.View{Name: "KeyView", ModelType: engine.StringType{}, Root: engine.EmptyElement{}}

	openOp := engine.OpenKeyedQuestionOperation{Slot: "Quiz", Key: engine.ReferenceExpression{Name: "key"}, Recipient: engine.ReferenceExpression{Name: "recipient"}}
	closeOp := engine.CloseKeyedQuestionOperation{Slot: "Quiz", Key: engine.ReferenceExpression{Name: "key"}}

	main := engine.Workflow{
		Name: "Main",
		KeyedQuestionSlots: []engine.KeyedQuestionSlot{{
			Name: "Quiz", Question: "Confirm", KeyType: engine.StringType{},
			Presentation: &engine.QuestionPresentation{
				Slot: "modal", Projection: "KeyProj",
				ProjectionArguments: []engine.CallArgument{{Name: "k", Value: engine.ReferenceExpression{Name: "key"}}},
				View:                "KeyView",
			},
		}},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{
						Name:       "Open",
						Signal:     engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: "Open"}, Bindings: []engine.SignalBinding{{Field: "key", Name: "key"}, {Field: "recipient", Name: "recipient"}}},
						Operations: engine.Block{Operations: []engine.Operation{openOp}},
						Control:    engine.StayControl{},
					},
					{
						Name:       "Close",
						Signal:     engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: "Close"}, Bindings: []engine.SignalBinding{{Field: "key", Name: "key"}}},
						Operations: engine.Block{Operations: []engine.Operation{closeOp}},
						Control:    engine.StayControl{},
					},
				},
			},
		},
	}

	return engine.Program{
		RootWorkflow: "Main",
		Workflows:    map[string]engine.Workflow{"Main": main},
		Projections:  map[string]engine.Projection{"KeyProj": proj},
		Views:        map[string]engine.View{"KeyView": view},
		Questions:    map[string]engine.Question{"Confirm": {Name: "Confirm", ResponseType: engine.BoolType{}}},
	}
}

func keyedQuestionPresentationSnapshot() engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root: engine.WorkflowInstance{
			Workflow:           "Main",
			State:              "S",
			LocalState:         engine.RecordValue{TypeName: "local"},
			KeyedQuestionSlots: []engine.KeyedQuestionSlotInstance{{Name: "Quiz"}},
		},
	}
}

func TestExec_KeyedQuestionPresentation_MountsIndependentlyPerKeyWithKeyBinding(t *testing.T) {
	p := keyedQuestionPresentationProgram()
	snap := keyedQuestionPresentationSnapshot()

	open := func(snap engine.Snapshot, key string, recipient engine.UserID) engine.Commit {
		c, err := runtime.Step(p, snap, engine.Signal{
			Kind: engine.SignalKindIntent, Intent: "Open",
			Fields: map[string]engine.Value{"key": engine.StringValue{Value: key}, "recipient": engine.UserValue{ID: recipient}},
		}, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error opening key %q: %v", key, err)
		}
		return c
	}

	commit := open(snap, "A", keyedPlayerA)
	var activateA *engine.ActivatePresentationOutput
	for _, o := range commit.Outputs {
		if a, ok := o.(engine.ActivatePresentationOutput); ok && a.Slot == "modal" {
			activateA = &a
		}
	}
	if activateA == nil || activateA.Recipient != keyedPlayerA || activateA.Model.(engine.StringValue).Value != "A" {
		t.Fatalf("expected an ActivatePresentationOutput for key A with model bound to its own key, got %+v", commit.Outputs)
	}

	// A second, different key for a different recipient mounts its own
	// independent presentation, with its own key bound into the model -
	// proving per-(slot, key) mounting, not a single shared one.
	commit = open(commit.Snapshot, "B", keyedPlayerB)
	var activateB *engine.ActivatePresentationOutput
	for _, o := range commit.Outputs {
		if a, ok := o.(engine.ActivatePresentationOutput); ok && a.Slot == "modal" {
			activateB = &a
		}
	}
	if activateB == nil || activateB.Recipient != keyedPlayerB || activateB.Model.(engine.StringValue).Value != "B" {
		t.Fatalf("expected an independent ActivatePresentationOutput for key B with model bound to its own key, got %+v", commit.Outputs)
	}

	// Closing key A's occurrence unmounts only key A's presentation; key
	// B's remains untouched.
	commit, err := runtime.Step(p, commit.Snapshot, engine.Signal{
		Kind: engine.SignalKindIntent, Intent: "Close", Fields: map[string]engine.Value{"key": engine.StringValue{Value: "A"}},
	}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error closing key A: %v", err)
	}
	removedA := false
	for _, o := range commit.Outputs {
		if r, ok := o.(engine.RemovePresentationOutput); ok && r.Slot == "modal" && r.Recipient == keyedPlayerA {
			removedA = true
		}
		if _, ok := o.(engine.ActivatePresentationOutput); ok {
			t.Fatalf("closing key A must not re-activate or otherwise touch key B's presentation, got %+v", commit.Outputs)
		}
	}
	if !removedA {
		t.Fatalf("expected key A's presentation to unmount on close, got %+v", commit.Outputs)
	}
}
