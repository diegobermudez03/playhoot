package engineservice_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// keyedSnapshotProgram builds, through the real engineservice.Compile
// pipeline, a "Main" workflow with one keyed slot of each family (a
// keyed question "Q", a keyed ask group "G", and a keyed timer "T", all
// keyed by string), driven by user intents so a test can open several
// simultaneous (slot, key) occurrences before round-tripping the
// resulting Snapshot.
func keyedSnapshotProgram() engine.Program {
	def := program.Definition{
		Questions: []program.QuestionDeclaration{{Name: "Confirm", ResponseType: boolType()}},
		UserIntents: []program.UserIntentDeclaration{
			{Name: "OpenQ", Parameters: []program.FieldDeclaration{{Name: "key", Type: stringType()}, {Name: "recipient", Type: userType()}}},
			{Name: "OpenG", Parameters: []program.FieldDeclaration{{Name: "key", Type: stringType()}, {Name: "recipients", Type: program.ListTypeReference{Element: userType()}}}},
			{Name: "ScheduleT", Parameters: []program.FieldDeclaration{{Name: "key", Type: stringType()}}},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:               "Main",
				ResultType:         program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				KeyedQuestionSlots: []program.KeyedQuestionSlotDeclaration{{Name: "Q", Question: "Confirm", KeyType: stringType()}},
				KeyedAskGroupSlots: []program.KeyedAskGroupSlotDeclaration{{Name: "G", Question: "Confirm", KeyType: stringType()}},
				KeyedTimerSlots:    []program.KeyedTimerSlotDeclaration{{Name: "T", KeyType: stringType()}},
				InitialState:       "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Control: program.StayControl{}},
							{
								Signal: program.SignalPattern{
									Source:   program.UserIntentSignalSource{Intent: "OpenQ"},
									Bindings: []program.SignalBinding{{Field: "key", Name: "key"}, {Field: "recipient", Name: "recipient"}},
								},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenKeyedQuestionOperation{Slot: "Q", Key: program.ReferenceExpression{Name: "key"}, Recipient: program.ReferenceExpression{Name: "recipient"}},
								}},
								Control: program.StayControl{},
							},
							{
								Signal: program.SignalPattern{
									Source:   program.UserIntentSignalSource{Intent: "OpenG"},
									Bindings: []program.SignalBinding{{Field: "key", Name: "key"}, {Field: "recipients", Name: "recipients"}},
								},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenKeyedAskGroupOperation{Slot: "G", Key: program.ReferenceExpression{Name: "key"}, Recipients: program.ReferenceExpression{Name: "recipients"}, Completion: program.AskGroupAllResponsesPolicy{}},
								}},
								Control: program.StayControl{},
							},
							{
								Signal: program.SignalPattern{
									Source:   program.UserIntentSignalSource{Intent: "ScheduleT"},
									Bindings: []program.SignalBinding{{Field: "key", Name: "key"}},
								},
								Operations: program.Block{Operations: []program.Operation{
									program.ScheduleKeyedTimerOperation{Slot: "T", Key: program.ReferenceExpression{Name: "key"}, DelayMilliseconds: program.NumberLiteralExpression{Value: "5000"}},
								}},
								Control: program.StayControl{},
							},
							{
								Signal:  program.SignalPattern{Source: program.KeyedQuestionAnsweredSignalSource{Slot: "Q"}},
								Control: program.StayControl{},
							},
							{
								Signal:  program.SignalPattern{Source: program.KeyedTimerExpiredSignalSource{Slot: "T"}},
								Control: program.StayControl{},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}

	p, diags := engineservice.Compile(def)
	if diags.HasErrors() {
		panic(diags)
	}
	return p
}

// TestCodec_KeyedSlotsRoundTrip proves engineservice.EncodeSnapshot /
// DecodeSnapshot preserve every keyed-family occurrence: two
// simultaneously pending keyed questions, a keyed ask-group occurrence
// with one already-accepted response (still collecting), and a pending
// keyed timer. It then continues execution directly against the decoded
// Snapshot - answering a keyed question, completing the keyed ask group
// with its second answer, and expiring the keyed timer - to prove the
// restored state is not just byte-identical but actually usable, not
// only inspectable.
func TestCodec_KeyedSlotsRoundTrip(t *testing.T) {
	p := keyedSnapshotProgram()
	snap, startSignal, err := engineservice.NewSnapshot(p, engine.InitializationInput{Seed: 1})
	if err != nil {
		t.Fatalf("unexpected NewSnapshot error: %v", err)
	}
	commit, err := engineservice.Step(p, snap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error applying WorkflowStarted: %v", err)
	}
	snap = commit.Snapshot

	const userA = engine.UserID("user-a")
	const userB = engine.UserID("user-b")

	step := func(s engine.Signal) {
		t.Helper()
		commit, err = engineservice.Step(p, snap, s, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error applying %+v: %v", s, err)
		}
		snap = commit.Snapshot
	}

	step(engine.Signal{Kind: engine.SignalKindIntent, Intent: "OpenQ", Fields: map[string]engine.Value{"key": engine.StringValue{Value: "k1"}, "recipient": engine.UserValue{ID: userA}}})
	step(engine.Signal{Kind: engine.SignalKindIntent, Intent: "OpenQ", Fields: map[string]engine.Value{"key": engine.StringValue{Value: "k2"}, "recipient": engine.UserValue{ID: userB}}})
	step(engine.Signal{Kind: engine.SignalKindIntent, Intent: "OpenG", Fields: map[string]engine.Value{
		"key":        engine.StringValue{Value: "team1"},
		"recipients": engine.ListValue{ElementType: engine.UserType{}, Elements: []engine.Value{engine.UserValue{ID: userA}, engine.UserValue{ID: userB}}},
	}})
	commit, err = engineservice.Step(p, snap, engine.Signal{
		Kind: engine.SignalKindKeyedAskGroupAnswered, Slot: "G", Key: engine.StringValue{Value: "team1"}, Respondent: userA, Answer: engine.BoolValue{Value: true},
	}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error answering the ask group's first response: %v", err)
	}
	snap = commit.Snapshot
	step(engine.Signal{Kind: engine.SignalKindIntent, Intent: "ScheduleT", Fields: map[string]engine.Value{"key": engine.StringValue{Value: "k1"}}})

	// Sanity-check the pre-round-trip state before proving it survives
	// encode/decode.
	qSlot := snap.Root.KeyedQuestionSlots[0]
	if len(qSlot.Pending) != 2 {
		t.Fatalf("expected 2 pending keyed questions before round trip, got %+v", qSlot)
	}
	gSlot := snap.Root.KeyedAskGroupSlots[0]
	if len(gSlot.Pending) != 1 || gSlot.Pending[0].Completed || len(gSlot.Pending[0].Responses) != 1 {
		t.Fatalf("expected one still-collecting keyed ask group with 1 response before round trip, got %+v", gSlot)
	}
	tSlot := snap.Root.KeyedTimerSlots[0]
	if len(tSlot.Pending) != 1 {
		t.Fatalf("expected 1 pending keyed timer before round trip, got %+v", tSlot)
	}

	decoded := roundTripSnapshot(t, snap)
	assertSnapshotsEqual(t, snap, decoded)

	dq := decoded.Root.KeyedQuestionSlots[0]
	if len(dq.Pending) != 2 {
		t.Fatalf("keyed question state lost across round trip: %+v", dq)
	}
	dg := decoded.Root.KeyedAskGroupSlots[0]
	if len(dg.Pending) != 1 || dg.Pending[0].Completed || len(dg.Pending[0].Responses) != 1 || dg.Pending[0].Responses[0].Respondent != userA {
		t.Fatalf("keyed ask-group state lost across round trip: %+v", dg)
	}
	dt := decoded.Root.KeyedTimerSlots[0]
	if len(dt.Pending) != 1 {
		t.Fatalf("keyed timer state lost across round trip: %+v", dt)
	}

	// The restored state must be genuinely usable, not merely
	// inspectable: continue driving the game directly from decoded.
	snap = decoded
	step(engine.Signal{Kind: engine.SignalKindKeyedQuestionAnswered, Slot: "Q", Key: engine.StringValue{Value: "k2"}, Respondent: userB, Answer: engine.BoolValue{Value: true}})
	qSlotAfter := snap.Root.KeyedQuestionSlots[0]
	if len(qSlotAfter.Pending) != 1 || qSlotAfter.Pending[0].Key.(engine.StringValue).Value != "k1" {
		t.Fatalf("expected only k1 to remain pending after answering k2 post-decode, got %+v", qSlotAfter)
	}

	commit, err = engineservice.Step(p, snap, engine.Signal{
		Kind: engine.SignalKindKeyedAskGroupAnswered, Slot: "G", Key: engine.StringValue{Value: "team1"}, Respondent: userB, Answer: engine.BoolValue{Value: false},
	}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error completing the ask group post-decode: %v", err)
	}
	snap = commit.Snapshot
	gSlotAfter := snap.Root.KeyedAskGroupSlots[0]
	if len(gSlotAfter.Pending) != 1 || !gSlotAfter.Pending[0].Completed {
		t.Fatalf("expected team1 to complete once its second recipient answered post-decode, got %+v", gSlotAfter)
	}

	step(engine.Signal{Kind: engine.SignalKindKeyedTimerExpired, Slot: "T", Key: engine.StringValue{Value: "k1"}})
	tSlotAfter := snap.Root.KeyedTimerSlots[0]
	if len(tSlotAfter.Pending) != 0 {
		t.Fatalf("expected k1's timer to be cleared after expiring post-decode, got %+v", tSlotAfter)
	}
}
