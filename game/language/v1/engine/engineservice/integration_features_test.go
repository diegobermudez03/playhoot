package engineservice_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// TestIntegration_CounterWorkflow drives counterProgramDefinition
// through the real engineservice.Compile -> engineservice.NewSnapshot -> engineservice.Step pipeline, as the
// "simple counter workflow" integration test.
func TestIntegration_CounterWorkflow(t *testing.T) {
	p, diags := engineservice.Compile(counterProgramDefinition())
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	snap, startSignal, err := engineservice.NewSnapshot(p, engine.InitializationInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	commit, err := engineservice.Step(p, snap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap = commit.Snapshot

	for i := 0; i < 3; i++ {
		commit, err = engineservice.Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Intent: "Increment"}, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error incrementing: %v", err)
		}
		snap = commit.Snapshot
	}

	commit, err = engineservice.Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Intent: "Finish"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Snapshot.Root.Outcome == nil || commit.Snapshot.Root.Outcome.Kind != engine.WorkflowOutcomeCompleted {
		t.Fatalf("expected completion, got %+v", commit.Snapshot.Root.Outcome)
	}
	if commit.Snapshot.Root.Outcome.Result.(engine.NumberValue).Value != 3 {
		t.Fatalf("got result %v, want 3", commit.Snapshot.Root.Outcome.Result)
	}
}

// questionWithTimeoutDefinition compiles a "Main" workflow that opens a
// question and schedules a timer in the same transition, then reaches
// one of two distinct terminal results depending on whether the answer
// or the timer wins the race.
func questionWithTimeoutDefinition() program.Definition {
	return program.Definition{
		Questions: []program.QuestionDeclaration{{Name: "Confirm", ResponseType: boolType()}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:          "Main",
				Parameters:    []program.FieldDeclaration{{Name: "player", Type: userType()}},
				ResultType:    stringType(),
				QuestionSlots: []program.QuestionSlotDeclaration{{Name: "Ask", Question: "Confirm"}},
				TimerSlots:    []program.TimerSlotDeclaration{{Name: "Deadline"}},
				InitialState:  "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenQuestionOperation{Slot: "Ask", Recipient: program.ReferenceExpression{Name: "player"}},
									program.ScheduleTimerOperation{Slot: "Deadline", DelayMilliseconds: program.NumberLiteralExpression{Value: "5000"}},
								}},
								Control: program.StayControl{},
							},
							{
								Signal: program.SignalPattern{
									Source:   program.QuestionAnsweredSignalSource{Slot: "Ask"},
									Bindings: []program.SignalBinding{{Field: "answer", Name: "a"}},
								},
								Operations: program.Block{Operations: []program.Operation{program.CancelTimerOperation{Slot: "Deadline"}}},
								Control:    program.CompleteControl{Result: program.StringLiteralExpression{Value: "answered"}},
							},
							{
								Signal:     program.SignalPattern{Source: program.TimerExpiredSignalSource{Slot: "Deadline"}},
								Operations: program.Block{Operations: []program.Operation{program.CloseQuestionOperation{Slot: "Ask"}}},
								Control:    program.CompleteControl{Result: program.StringLiteralExpression{Value: "timed_out"}},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}
}

func TestIntegration_QuestionAnsweredBeforeTimeout(t *testing.T) {
	p, diags := engineservice.Compile(questionWithTimeoutDefinition())
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	snap, startSignal, err := engineservice.NewSnapshot(p, engine.InitializationInput{RootParameters: map[string]engine.Value{"player": engine.UserValue{ID: player}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	commit, err := engineservice.Step(p, snap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commit.Outputs) != 2 { // open question + schedule timer
		t.Fatalf("got %d outputs, want 2: %+v", len(commit.Outputs), commit.Outputs)
	}

	commit, err = engineservice.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindQuestionAnswered, Slot: "Ask", Respondent: player, Answer: engine.BoolValue{Value: true}}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error answering: %v", err)
	}
	if commit.Snapshot.Root.Outcome == nil || commit.Snapshot.Root.Outcome.Result.(engine.StringValue).Value != "answered" {
		t.Fatalf("got outcome %+v, want \"answered\"", commit.Snapshot.Root.Outcome)
	}
	foundCancel := false
	for _, o := range commit.Outputs {
		if _, ok := o.(engine.CancelTimerOutput); ok {
			foundCancel = true
		}
	}
	if !foundCancel {
		t.Fatalf("expected the timer to be cancelled, got outputs %+v", commit.Outputs)
	}
}

func TestIntegration_TimerFiresBeforeAnswer(t *testing.T) {
	p, diags := engineservice.Compile(questionWithTimeoutDefinition())
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	snap, startSignal, err := engineservice.NewSnapshot(p, engine.InitializationInput{RootParameters: map[string]engine.Value{"player": engine.UserValue{ID: player}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	commit, err := engineservice.Step(p, snap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commit, err = engineservice.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindTimerExpired, Slot: "Deadline"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error on timeout: %v", err)
	}
	if commit.Snapshot.Root.Outcome == nil || commit.Snapshot.Root.Outcome.Result.(engine.StringValue).Value != "timed_out" {
		t.Fatalf("got outcome %+v, want \"timed_out\"", commit.Snapshot.Root.Outcome)
	}

	// A late answer must now be rejected — the question was closed.
	_, err = engineservice.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindQuestionAnswered, Slot: "Ask", Respondent: player, Answer: engine.BoolValue{Value: true}}, engine.DefaultLimits())
	if err != engineservice.ErrSignalRejected {
		t.Fatalf("expected the terminated workflow to reject a late answer, got %v", err)
	}
}

// childWorkflowDefinition compiles a "Worker" workflow returning its
// "amount" parameter, spawned and joined by a "Main" root workflow.
func childWorkflowDefinition() program.Definition {
	return program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Worker",
				Parameters:   []program.FieldDeclaration{{Name: "amount", Type: numberType()}},
				ResultType:   numberType(),
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{Name: "S", Transitions: []program.TransitionDeclaration{
						{Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "amount"}}},
					}},
				},
			},
			{
				Name:         "Main",
				ResultType:   numberType(),
				ChildSlots:   []program.ChildWorkflowSlotDeclaration{{Name: "W", Workflow: "Worker"}},
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.SpawnChildWorkflowOperation{Slot: "W", Arguments: []program.CallArgument{{Name: "amount", Value: program.NumberLiteralExpression{Value: "99"}}}},
								}},
								Control: program.StayControl{},
							},
							{
								Signal: program.SignalPattern{
									Source:   program.ChildCompletedSignalSource{Slot: "W"},
									Bindings: []program.SignalBinding{{Field: "result", Name: "r"}},
								},
								Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "r"}},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}
}

func TestIntegration_ChildWorkflowSpawnAndJoin(t *testing.T) {
	p, diags := engineservice.Compile(childWorkflowDefinition())
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	snap, startSignal, err := engineservice.NewSnapshot(p, engine.InitializationInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	commit, err := engineservice.Step(p, snap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error spawning: %v", err)
	}
	if len(commit.InternalSignals) != 1 {
		t.Fatalf("expected one internal WorkflowStarted signal for the child, got %+v", commit.InternalSignals)
	}
	commit, err = engineservice.Step(p, commit.Snapshot, commit.InternalSignals[0], engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error starting the child: %v", err)
	}
	if commit.Snapshot.Root.ChildSlots[0].Child.Outcome == nil {
		t.Fatalf("expected the child to have already completed")
	}

	commit, err = engineservice.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindChildCompleted, Slot: "W"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error joining: %v", err)
	}
	if commit.Snapshot.Root.Outcome == nil || commit.Snapshot.Root.Outcome.Result.(engine.NumberValue).Value != 99 {
		t.Fatalf("got outcome %+v, want 99", commit.Snapshot.Root.Outcome)
	}
}

// askGroupDefinition compiles a "Main" workflow that opens an
// all-responses ask group across two recipients and joins its
// aggregated result.
func askGroupDefinitionForIntegration() program.Definition {
	return program.Definition{
		Questions: []program.QuestionDeclaration{{Name: "Vote", ResponseType: boolType()}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:          "Main",
				Parameters:    []program.FieldDeclaration{{Name: "voters", Type: program.ListTypeReference{Element: userType()}}},
				ResultType:    program.MapTypeReference{Key: userType(), Value: boolType()},
				AskGroupSlots: []program.AskGroupSlotDeclaration{{Name: "Poll", Question: "Vote"}},
				InitialState:  "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenAskGroupOperation{Slot: "Poll", Recipients: program.ReferenceExpression{Name: "voters"}, Completion: program.AskGroupAllResponsesPolicy{}},
								}},
								Control: program.StayControl{},
							},
							{
								Signal: program.SignalPattern{
									Source:   program.AskGroupCompletedSignalSource{Slot: "Poll"},
									Bindings: []program.SignalBinding{{Field: "responses", Name: "results"}},
								},
								Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "results"}},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}
}

func TestIntegration_AskGroupOpenAnswerJoin(t *testing.T) {
	p, diags := engineservice.Compile(askGroupDefinitionForIntegration())
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	snap, startSignal, err := engineservice.NewSnapshot(p, engine.InitializationInput{RootParameters: map[string]engine.Value{
		"voters": engine.ListValue{ElementType: engine.UserType{}, Elements: []engine.Value{engine.UserValue{ID: askAlice}, engine.UserValue{ID: askBob}}},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	commit, err := engineservice.Step(p, snap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error opening: %v", err)
	}
	if len(commit.Outputs) != 2 {
		t.Fatalf("got %d outputs, want 2", len(commit.Outputs))
	}

	commit, err = engineservice.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindAskGroupAnswered, Slot: "Poll", Respondent: askAlice, Answer: engine.BoolValue{Value: true}}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error answering: %v", err)
	}
	commit, err = engineservice.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindAskGroupAnswered, Slot: "Poll", Respondent: askBob, Answer: engine.BoolValue{Value: false}}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error answering: %v", err)
	}

	commit, err = engineservice.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindAskGroupCompleted, Slot: "Poll"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error joining: %v", err)
	}
	results := commit.Snapshot.Root.Outcome.Result.(engine.MapValue).Entries
	if len(results) != 2 {
		t.Fatalf("got %+v", results)
	}
}

// taskGroupDefinitionForIntegration compiles a "Main" workflow that
// begins, seals, and joins a small all-terminal task group of "Worker"
// children.
func taskGroupDefinitionForIntegration() program.Definition {
	return program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Worker",
				Parameters:   []program.FieldDeclaration{{Name: "amount", Type: numberType()}},
				ResultType:   numberType(),
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{Name: "S", Transitions: []program.TransitionDeclaration{
						{Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "amount"}}},
					}},
				},
			},
			{
				Name:           "Main",
				ResultType:     program.MapTypeReference{Key: numberType(), Value: numberType()},
				TaskGroupSlots: []program.TaskGroupSlotDeclaration{{Name: "Workers", Workflow: "Worker", KeyType: numberType()}},
				InitialState:   "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.BeginTaskGroupOperation{Slot: "Workers", Completion: program.TaskGroupAllTerminalPolicy{}},
									program.SpawnTaskGroupChildOperation{Slot: "Workers", Key: program.NumberLiteralExpression{Value: "1"}, Arguments: []program.CallArgument{{Name: "amount", Value: program.NumberLiteralExpression{Value: "10"}}}},
									program.SpawnTaskGroupChildOperation{Slot: "Workers", Key: program.NumberLiteralExpression{Value: "2"}, Arguments: []program.CallArgument{{Name: "amount", Value: program.NumberLiteralExpression{Value: "20"}}}},
									program.SealTaskGroupOperation{Slot: "Workers"},
								}},
								Control: program.StayControl{},
							},
							{
								Signal: program.SignalPattern{
									Source:   program.TaskGroupCompletedSignalSource{Slot: "Workers"},
									Bindings: []program.SignalBinding{{Field: "results", Name: "results"}},
								},
								Control: program.CompleteControl{Result: program.ReferenceExpression{Name: "results"}},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}
}

func TestIntegration_TaskGroupBeginSpawnSealJoin(t *testing.T) {
	p, diags := engineservice.Compile(taskGroupDefinitionForIntegration())
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	snap, startSignal, err := engineservice.NewSnapshot(p, engine.InitializationInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	commit, err := engineservice.Step(p, snap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error beginning: %v", err)
	}
	if len(commit.InternalSignals) != 2 {
		t.Fatalf("expected two internal WorkflowStarted signals, got %+v", commit.InternalSignals)
	}
	snap = commit.Snapshot
	for _, s := range commit.InternalSignals {
		c, err := engineservice.Step(p, snap, s, engine.DefaultLimits())
		if err != nil {
			t.Fatalf("unexpected error starting a task: %v", err)
		}
		snap = c.Snapshot
	}

	group := snap.Root.TaskGroupSlots[0].Group
	if group == nil || group.Phase != engine.TaskGroupPhaseCompleted {
		t.Fatalf("expected the all-terminal group to complete once both tasks finished, got %+v", group)
	}

	commit, err = engineservice.Step(p, snap, engine.Signal{Kind: engine.SignalKindTaskGroupCompleted, Slot: "Workers"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error joining: %v", err)
	}
	results := commit.Snapshot.Root.Outcome.Result.(engine.MapValue).Entries
	if len(results) != 2 {
		t.Fatalf("got %+v", results)
	}
}

// projectionAndPresentationDefinition compiles a "Main" workflow with
// one workflow-level presentation projecting global.score to every
// player.
func projectionAndPresentationDefinition() program.Definition {
	return program.Definition{
		GlobalState: program.StateDeclaration{Fields: []program.StateFieldDeclaration{
			{Name: "score", Type: numberType(), Initializer: program.NumberLiteralExpression{Value: "0"}},
		}},
		PresentationSlots: []program.PresentationSlotDeclaration{{Name: "hud"}},
		Projections: []program.ProjectionDeclaration{
			{Name: "Score", ResultType: numberType(), Body: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "score"}},
		},
		Views: []program.ViewDeclaration{
			{Name: "ScoreView", ModelType: numberType(), Root: program.EmptyElement{}},
		},
		UserIntents: []program.UserIntentDeclaration{{Name: "AddPoint"}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				Parameters:   []program.FieldDeclaration{{Name: "players", Type: program.ListTypeReference{Element: userType()}}},
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				Presentations: []program.PresentationDeclaration{
					{Name: "Hud", Slot: "hud", Targets: program.ReferenceExpression{Name: "players"}, Projection: "Score", View: "ScoreView"},
				},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Control: program.StayControl{}},
							{
								Signal: program.SignalPattern{Source: program.UserIntentSignalSource{Intent: "AddPoint"}},
								Operations: program.Block{Operations: []program.Operation{
									program.SetOperation{
										Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "score"},
										Value: program.BinaryExpression{
											Operator: program.BinaryOperatorAdd,
											Left:     program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "score"},
											Right:    program.NumberLiteralExpression{Value: "1"},
										},
									},
								}},
								Control: program.StayControl{},
							},
						},
					},
				},
			},
		},
		RootWorkflow: "Main",
	}
}

func TestIntegration_ProjectionAndPresentationFlow(t *testing.T) {
	p, diags := engineservice.Compile(projectionAndPresentationDefinition())
	if diags.HasErrors() {
		t.Fatalf("unexpected compile errors: %v", diags)
	}
	snap, startSignal, err := engineservice.NewSnapshot(p, engine.InitializationInput{RootParameters: map[string]engine.Value{
		"players": engine.ListValue{ElementType: engine.UserType{}, Elements: []engine.Value{engine.UserValue{ID: presP1}, engine.UserValue{ID: presP2}}},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	commit, err := engineservice.Step(p, snap, startSignal, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := countOutputs[engine.ActivatePresentationOutput](commit.Outputs); got != 2 {
		t.Fatalf("got %d ActivatePresentationOutput, want 2: %+v", got, commit.Outputs)
	}
	for _, o := range commit.Outputs {
		a := o.(engine.ActivatePresentationOutput)
		if a.Model.(engine.NumberValue).Value != 0 {
			t.Fatalf("got initial model %v, want 0", a.Model)
		}
	}

	commit, err = engineservice.Step(p, commit.Snapshot, engine.Signal{Kind: engine.SignalKindIntent, Intent: "AddPoint"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := countOutputs[engine.UpdatePresentationOutput](commit.Outputs); got != 2 {
		t.Fatalf("got %d UpdatePresentationOutput, want 2: %+v", got, commit.Outputs)
	}
	for _, o := range commit.Outputs {
		u := o.(engine.UpdatePresentationOutput)
		if u.Model.(engine.NumberValue).Value != 1 {
			t.Fatalf("got updated model %v, want 1", u.Model)
		}
	}
}
