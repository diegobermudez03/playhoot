package engineservice_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// counterProgramDefinition is a program.Definition-level counterpart to
// step_test.go's hand-built engine-IR counterProgram, for tests that
// need to go through the real engineservice.Compile pipeline.
func counterProgramDefinition() program.Definition {
	globalCount := program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "count"}
	return program.Definition{
		GlobalState: program.StateDeclaration{Fields: []program.StateFieldDeclaration{
			{Name: "count", Type: numberType(), Initializer: program.NumberLiteralExpression{Value: "0"}},
		}},
		UserIntents: []program.UserIntentDeclaration{
			{Name: "Increment"},
			{Name: "Finish"},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Counter",
				ResultType:   numberType(),
				InitialState: "Running",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Running",
						Transitions: []program.TransitionDeclaration{
							{Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Control: program.StayControl{}},
							{
								Signal: program.SignalPattern{Source: program.UserIntentSignalSource{Intent: "Increment"}},
								Operations: program.Block{Operations: []program.Operation{
									program.SetOperation{
										Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "count"},
										Value:  program.BinaryExpression{Operator: program.BinaryOperatorAdd, Left: globalCount, Right: program.NumberLiteralExpression{Value: "1"}},
									},
								}},
								Control: program.StayControl{},
							},
							{
								Signal:  program.SignalPattern{Source: program.UserIntentSignalSource{Intent: "Finish"}},
								Guard:   program.BinaryExpression{Operator: program.BinaryOperatorGreaterOrEqual, Left: globalCount, Right: program.NumberLiteralExpression{Value: "3"}},
								Control: program.CompleteControl{Result: globalCount},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Counter",
	}
}

func roundTripSnapshot(t *testing.T, snap engine.Snapshot) engine.Snapshot {
	t.Helper()
	data, err := engineservice.EncodeSnapshot(snap)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	decoded, err := engineservice.DecodeSnapshot(data)
	if err != nil {
		t.Fatalf("unexpected decode error: %v (data: %s)", err, data)
	}
	return decoded
}

// assertSnapshotsEqual compares a and b by their own wire encoding
// rather than reflect.DeepEqual on the decoded Go structs directly:
// several construction paths elsewhere in this package build an empty
// slice as make([]T, 0, n) (non-nil) rather than leaving it nil, and
// engineservice.EncodeSnapshot already treats "nil" and "empty" identically (both hit
// omitempty) — so encoded-byte equality is the meaningful notion of
// "the same Snapshot" here, and DeepEqual on the raw structs would
// otherwise fail on an incidental, semantically irrelevant difference.
func assertSnapshotsEqual(t *testing.T, a, b engine.Snapshot) {
	t.Helper()
	aBytes, err := engineservice.EncodeSnapshot(a)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	bBytes, err := engineservice.EncodeSnapshot(b)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	if string(aBytes) != string(bBytes) {
		t.Fatalf("snapshots differ:\n%s\nvs\n%s", aBytes, bBytes)
	}
}

func TestCodec_SimpleSnapshotRoundTrips(t *testing.T) {
	snap := counterSnapshot(2)
	decoded := roundTripSnapshot(t, snap)
	assertSnapshotsEqual(t, snap, decoded)
}

func TestCodec_ChildWorkflowTreeRoundTrips(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot())
	decoded := roundTripSnapshot(t, snap)
	assertSnapshotsEqual(t, snap, decoded)
	if decoded.Root.ChildSlots[0].Child == nil || decoded.Root.ChildSlots[0].Child.Workflow != "Worker" {
		t.Fatalf("child instance lost across round trip: %+v", decoded.Root.ChildSlots[0])
	}
}

func TestCodec_TaskGroupRoundTrips(t *testing.T) {
	p := taskGroupProgram(engine.TaskGroupQuorumTerminalPolicy{Count: engine.NumberLiteralExpression{Value: 2}}, []float64{10, 20, 30}, engine.UnitType{}, engine.StayControl{})
	commit, err := engineservice.Step(p, taskGroupSnapshot(), engine.Signal{Name: "BeginAndSeal"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := terminateTask(t, p, commit.Snapshot, 1, "Succeed")
	snap = terminateTask(t, p, snap, 3, "Fail")

	decoded := roundTripSnapshot(t, snap)
	assertSnapshotsEqual(t, snap, decoded)
	group := decoded.Root.TaskGroupSlots[0].Group
	if group == nil || group.Phase != engine.TaskGroupPhaseCompleted || len(group.TerminalOrder) != 2 {
		t.Fatalf("task group state lost across round trip: %+v", group)
	}
}

func TestCodec_AskGroupRoundTrips(t *testing.T) {
	p := askGroupProgram(engine.AskGroupAllResponsesPolicy{}, engine.UnitType{}, engine.StayControl{})
	commit, _ := engineservice.Step(p, askGroupSnapshot([]engine.UserID{askAlice, askBob}), engine.Signal{Name: "Open"}, engine.DefaultLimits())
	commit, err := answerAskGroup(p, commit.Snapshot, askAlice, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded := roundTripSnapshot(t, commit.Snapshot)
	assertSnapshotsEqual(t, commit.Snapshot, decoded)
	pending := decoded.Root.AskGroupSlots[0].Pending
	if pending == nil || len(pending.Responses) != 1 || pending.Responses[0].Respondent != askAlice {
		t.Fatalf("ask-group state lost across round trip: %+v", pending)
	}
}

func TestCodec_PendingQuestionRoundTrips(t *testing.T) {
	p := questionDemoProgram()
	commit, err := engineservice.Step(p, questionDemoSnapshot(), engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	decoded := roundTripSnapshot(t, commit.Snapshot)
	assertSnapshotsEqual(t, commit.Snapshot, decoded)
	pending := decoded.Root.QuestionSlots[0].Pending
	if pending == nil || pending.Recipient != player {
		t.Fatalf("pending question lost across round trip: %+v", pending)
	}
}

func TestCodec_TerminalOutcomeRoundTrips(t *testing.T) {
	p := counterProgram()
	commit, err := engineservice.Step(p, counterSnapshot(3), engine.Signal{Name: "Finish"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	decoded := roundTripSnapshot(t, commit.Snapshot)
	assertSnapshotsEqual(t, commit.Snapshot, decoded)
	if decoded.Root.Outcome == nil || decoded.Root.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("outcome lost across round trip: %+v", decoded.Root.Outcome)
	}
}

// TestCodec_EveryValueKindRoundTrips exercises all twelve engine.Value
// variants directly (no Program needed), including a nested
// NewTypeValue, a some/none OptionalValue pair, and non-empty
// list/map values.
func TestCodec_EveryValueKindRoundTrips(t *testing.T) {
	samples := []engine.Value{
		engine.UnitValue{},
		engine.BoolValue{Value: true},
		engine.NumberValue{Value: 3.5},
		engine.StringValue{Value: "hello"},
		engine.UserValue{ID: engine.UserID("u1")},
		engine.EnumValue{TypeName: "Color", ValueName: "Red"},
		engine.RecordValue{TypeName: "Point", Fields: []engine.FieldValue{
			{Name: "x", Value: engine.NumberValue{Value: 1}},
			{Name: "y", Value: engine.NumberValue{Value: 2}},
		}},
		engine.UnionValue{TypeName: "Outcome", VariantName: "Won", Fields: []engine.FieldValue{
			{Name: "score", Value: engine.NumberValue{Value: 10}},
		}},
		engine.NewTypeValue{TypeName: "Score", Underlying: engine.NumberValue{Value: 42}},
		engine.OptionalValue{ElementType: engine.NumberType{}, Value: engine.NumberValue{Value: 7}},
		engine.OptionalValue{ElementType: engine.NumberType{}, Value: nil},
		engine.ListValue{ElementType: engine.NumberType{}, Elements: []engine.Value{
			engine.NumberValue{Value: 1}, engine.NumberValue{Value: 2},
		}},
		engine.MapValue{KeyType: engine.StringType{}, ValueType: engine.NumberType{}, Entries: []engine.MapEntry{
			{Key: engine.StringValue{Value: "a"}, Value: engine.NumberValue{Value: 1}},
		}},
	}

	for _, v := range samples {
		snap := engine.Snapshot{
			GlobalState: engine.RecordValue{TypeName: "global"},
			Root: engine.WorkflowInstance{
				Workflow:   "Main",
				State:      "S",
				LocalState: engine.RecordValue{TypeName: "local"},
				Parameters: []engine.FieldValue{{Name: "v", Value: v}},
			},
		}
		decoded := roundTripSnapshot(t, snap)
		got := decoded.Root.Parameters[0].Value
		if !got.Equal(v) {
			t.Fatalf("value %+v did not round-trip: got %+v", v, got)
		}
	}
}

func TestCodec_DecodeMalformedJSONReturnsPathAwareError(t *testing.T) {
	_, err := engineservice.DecodeSnapshot([]byte(`{"root": {`))
	if err == nil {
		t.Fatal("expected a decode error")
	}
	var decErr *engineservice.DecodeError
	if de, ok := err.(*engineservice.DecodeError); ok {
		decErr = de
	}
	if decErr == nil {
		t.Fatalf("expected a *engineservice.DecodeError, got %T", err)
	}
	if decErr.Path == "" {
		t.Fatal("expected a non-empty path")
	}
}

func TestCodec_DecodeUnknownFieldRejected(t *testing.T) {
	valid, err := engineservice.EncodeSnapshot(counterSnapshot(0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tampered := append(valid[:len(valid)-1], []byte(`,"bogus_field":1}`)...)
	_, err = engineservice.DecodeSnapshot(tampered)
	if err == nil {
		t.Fatal("expected an unknown-field decode error")
	}
}

func TestCodec_CheckSnapshotCompatibility(t *testing.T) {
	p := counterProgram()
	snap := counterSnapshot(0)
	if err := engineservice.CheckSnapshotCompatibility(p, snap); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mismatched := snap
	mismatched.Root.Workflow = "SomeOtherWorkflow"
	err := engineservice.CheckSnapshotCompatibility(p, mismatched)
	if e, ok := err.(*engineservice.ExecutionError); !ok || e.Code != engineservice.ExecutionErrorSnapshotProgramMismatch {
		t.Fatalf("expected engineservice.ExecutionErrorSnapshotProgramMismatch, got %v", err)
	}
}

func TestCodec_CheckSnapshotCompatibilityDetectsMissingChildWorkflow(t *testing.T) {
	p := childWorkflowProgram()
	snap := spawnAndStart(t, p, mainSnapshot())

	// Simulate resuming against a newer program version that dropped "Worker".
	trimmed := engine.Program{RootWorkflow: p.RootWorkflow, Workflows: map[string]engine.Workflow{"Main": p.Workflows["Main"]}}
	err := engineservice.CheckSnapshotCompatibility(trimmed, snap)
	if e, ok := err.(*engineservice.ExecutionError); !ok || e.Code != engineservice.ExecutionErrorSnapshotProgramMismatch {
		t.Fatalf("expected engineservice.ExecutionErrorSnapshotProgramMismatch, got %v", err)
	}
}

// TestIntegration_PersistRestoreContinueMatchesUninterruptedExecution is
// the "complete compile -> initialize -> step -> persist -> restore ->
// continue" test: it drives the same game two ways — persisting and
// restoring the Snapshot between every step, versus never persisting at
// all — and asserts both reach an identical final Snapshot.
func TestIntegration_PersistRestoreContinueMatchesUninterruptedExecution(t *testing.T) {
	def := counterProgramDefinition()
	p, diags := engineservice.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	if err := engineservice.CheckSnapshotCompatibility(p, engine.Snapshot{Root: engine.WorkflowInstance{Workflow: p.RootWorkflow}}); err != nil {
		t.Fatalf("unexpected incompatibility on a freshly compiled program: %v", err)
	}

	snapDirect, startSignal, err := engineservice.NewSnapshot(p, engine.InitializationInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snapPersisted := snapDirect

	increment := engine.Signal{Kind: engine.SignalKindIntent, Intent: "Increment"}
	finish := engine.Signal{Kind: engine.SignalKindIntent, Intent: "Finish"}
	signals := []engine.Signal{startSignal, increment, increment, increment, finish}
	for _, sig := range signals {
		commit, err := engineservice.Step(p, snapDirect, sig, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error (direct): %v", err)
		}
		snapDirect = commit.Snapshot

		commit, err = engineservice.Step(p, snapPersisted, sig, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error (persisted): %v", err)
		}
		data, err := engineservice.EncodeSnapshot(commit.Snapshot)
		if err != nil {
			t.Fatalf("unexpected encode error: %v", err)
		}
		snapPersisted, err = engineservice.DecodeSnapshot(data)
		if err != nil {
			t.Fatalf("unexpected decode error: %v", err)
		}
	}

	assertSnapshotsEqual(t, snapDirect, snapPersisted)
	if snapDirect.Root.Outcome == nil || snapDirect.Root.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("expected the workflow to complete, got %+v", snapDirect.Root.Outcome)
	}
}
