package runtime_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/internal/runtime"
)

const (
	presP1 = engine.UserID("p1")
	presP2 = engine.UserID("p2")
)

// presentationProgram builds a hand-assembled engine.Program (bypassing
// compiler.Compile, mirroring step_test.go's style): a "Score" projection
// exposing global.score to its viewer, a trivial "ScoreView", a "Main"
// workflow with one always-active workflow-level presentation ("Hud",
// slot "hud"), one question slot ("Ask", presented in slot "modal"),
// and two states — "Start" (with its own state-level presentation,
// "StartBanner", slot "banner") and "Playing" (no presentations).
func presentationProgram() engine.Program {
	scoreProjection := engine.Projection{
		Name:       "Score",
		ResultType: engine.NumberType{},
		Body:       engine.FieldExpression{Target: engine.ReferenceExpression{Name: "global"}, Field: "score"},
	}
	scoreView := engine.View{Name: "ScoreView", ModelType: engine.NumberType{}, Root: engine.EmptyElement{}}

	hud := engine.Presentation{Name: "Hud", Slot: "hud", Targets: engine.ReferenceExpression{Name: "players"}, Projection: "Score", View: "ScoreView"}
	startBanner := engine.Presentation{Name: "StartBanner", Slot: "banner", Targets: engine.ReferenceExpression{Name: "players"}, Projection: "Score", View: "ScoreView"}

	incOp := engine.SetOperation{
		Target: engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: "score"},
		Value: engine.BinaryExpression{
			Operator: engine.BinaryOperatorAdd,
			Left:     engine.FieldExpression{Target: engine.ReferenceExpression{Name: "global"}, Field: "score"},
			Right:    engine.NumberLiteralExpression{Value: 1},
		},
	}

	main := engine.Workflow{
		Name:          "Main",
		Parameters:    []engine.FieldType{{Name: "players", Type: engine.ListType{Element: engine.UserType{}}}},
		ResultType:    engine.UnitType{},
		Presentations: []engine.Presentation{hud},
		QuestionSlots: []engine.QuestionSlot{{Name: "Ask", Question: "Confirm", Presentation: &engine.QuestionPresentation{Slot: "modal", Projection: "Score", View: "ScoreView"}}},
		InitialState:  "Start",
		States: []engine.WorkflowState{
			{
				Name:          "Start",
				Presentations: []engine.Presentation{startBanner},
				Transitions: []engine.Transition{
					{Name: "Started", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "WorkflowStarted"}}, Control: engine.StayControl{}},
					{Name: "GoToPlaying", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "GoToPlaying"}}, Control: engine.GotoControl{State: "Playing"}},
					{Name: "Open", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Open"}}, Operations: engine.Block{Operations: []engine.Operation{
						engine.OpenQuestionOperation{Slot: "Ask", Recipient: engine.IndexExpression{Target: engine.ReferenceExpression{Name: "players"}, Index: engine.NumberLiteralExpression{Value: 0}}},
					}}, Control: engine.StayControl{}},
					{Name: "Close", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Close"}}, Operations: engine.Block{Operations: []engine.Operation{
						engine.CloseQuestionOperation{Slot: "Ask"},
					}}, Control: engine.StayControl{}},
				},
			},
			{
				Name: "Playing",
				Transitions: []engine.Transition{
					{Name: "Inc", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Inc"}}, Operations: engine.Block{Operations: []engine.Operation{incOp}}, Control: engine.StayControl{}},
					{Name: "Finish", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Finish"}}, Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}}},
				},
			},
		},
	}

	return engine.Program{
		RootWorkflow: "Main",
		Workflows:    map[string]engine.Workflow{"Main": main},
		Projections:  map[string]engine.Projection{"Score": scoreProjection},
		Views:        map[string]engine.View{"ScoreView": scoreView},
		Questions:    map[string]engine.Question{"Confirm": {Name: "Confirm", ResponseType: engine.BoolType{}}},
	}
}

func presentationSnapshot() engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global", Fields: []engine.FieldValue{{Name: "score", Value: engine.NumberValue{Value: 0}}}},
		Root: engine.WorkflowInstance{
			Workflow: "Main",
			State:    "Start",
			Parameters: []engine.FieldValue{{Name: "players", Value: engine.ListValue{ElementType: engine.UserType{}, Elements: []engine.Value{
				engine.UserValue{ID: presP1}, engine.UserValue{ID: presP2},
			}}}},
			LocalState:    engine.RecordValue{TypeName: "local"},
			QuestionSlots: []engine.QuestionSlotInstance{{Name: "Ask"}},
		},
	}
}

func countOutputs[T engine.Output](outputs []engine.Output) int {
	n := 0
	for _, o := range outputs {
		if _, ok := o.(T); ok {
			n++
		}
	}
	return n
}

func TestExec_WorkflowStartedActivatesWorkflowAndStateLevelPresentations(t *testing.T) {
	p := presentationProgram()
	snap := presentationSnapshot()

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "WorkflowStarted"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	activates := countOutputs[engine.ActivatePresentationOutput](commit.Outputs)
	if activates != 4 { // Hud x2 players + StartBanner x2 players
		t.Fatalf("got %d ActivatePresentationOutput, want 4 (outputs: %+v)", activates, commit.Outputs)
	}
	for _, o := range commit.Outputs {
		a := o.(engine.ActivatePresentationOutput)
		if a.Model.(engine.NumberValue).Value != 0 {
			t.Fatalf("got model %v, want 0", a.Model)
		}
	}
}

func TestExec_StateChangeRemovesStateLevelKeepsWorkflowLevel(t *testing.T) {
	p := presentationProgram()
	commit, _ := runtime.Step(p, presentationSnapshot(), engine.Signal{Name: "WorkflowStarted"}, engine.DefaultLimits())
	snap := commit.Snapshot

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "GoToPlaying"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := countOutputs[engine.UpdatePresentationOutput](commit.Outputs); got != 2 {
		t.Fatalf("got %d UpdatePresentationOutput (Hud persisting), want 2: %+v", got, commit.Outputs)
	}
	if got := countOutputs[engine.RemovePresentationOutput](commit.Outputs); got != 2 {
		t.Fatalf("got %d RemovePresentationOutput (StartBanner gone), want 2: %+v", got, commit.Outputs)
	}
	for _, o := range commit.Outputs {
		if r, ok := o.(engine.RemovePresentationOutput); ok && r.Name != "StartBanner" {
			t.Fatalf("got unexpected removal %+v", r)
		}
	}
}

func TestExec_ProjectionUpdateReflectsNewGlobalState(t *testing.T) {
	p := presentationProgram()
	commit, _ := runtime.Step(p, presentationSnapshot(), engine.Signal{Name: "WorkflowStarted"}, engine.DefaultLimits())
	commit, _ = runtime.Step(p, commit.Snapshot, engine.Signal{Name: "GoToPlaying"}, engine.DefaultLimits())
	snap := commit.Snapshot

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Inc"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updates := 0
	for _, o := range commit.Outputs {
		u, ok := o.(engine.UpdatePresentationOutput)
		if !ok {
			continue
		}
		updates++
		if u.Model.(engine.NumberValue).Value != 1 {
			t.Fatalf("got model %v, want 1 (score incremented)", u.Model)
		}
	}
	if updates != 2 {
		t.Fatalf("got %d updates, want 2", updates)
	}
}

func TestExec_WorkflowTerminationRemovesWorkflowLevelPresentations(t *testing.T) {
	p := presentationProgram()
	commit, _ := runtime.Step(p, presentationSnapshot(), engine.Signal{Name: "WorkflowStarted"}, engine.DefaultLimits())
	commit, _ = runtime.Step(p, commit.Snapshot, engine.Signal{Name: "GoToPlaying"}, engine.DefaultLimits())

	commit, err := runtime.Step(p, commit.Snapshot, engine.Signal{Name: "Finish"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := countOutputs[engine.RemovePresentationOutput](commit.Outputs); got != 2 {
		t.Fatalf("got %d RemovePresentationOutput, want 2 (Hud for both players): %+v", got, commit.Outputs)
	}
}

func TestExec_DuplicateTargetInOneTargetsListIsRejected(t *testing.T) {
	p := presentationProgram()
	main := p.Workflows["Main"]
	// Targets evaluates to [players[0], players[0]] — the same user twice.
	main.Presentations = []engine.Presentation{
		{Name: "Hud", Slot: "hud", Targets: engine.ListExpression{ElementType: engine.UserType{}, Elements: []engine.Expression{
			engine.IndexExpression{Target: engine.ReferenceExpression{Name: "players"}, Index: engine.NumberLiteralExpression{Value: 0}},
			engine.IndexExpression{Target: engine.ReferenceExpression{Name: "players"}, Index: engine.NumberLiteralExpression{Value: 0}},
		}}, Projection: "Score", View: "ScoreView"},
	}
	p.Workflows["Main"] = main

	_, err := runtime.Step(p, presentationSnapshot(), engine.Signal{Name: "WorkflowStarted"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorPresentationSlotOccupied {
		t.Fatalf("expected runtime.ExecutionErrorPresentationSlotOccupied, got %v", err)
	}
}

func TestExec_TwoPresentationsOnSameSlotForSameUserIsRejected(t *testing.T) {
	p := presentationProgram()
	main := p.Workflows["Main"]
	// Both presentations target the same "hud" slot with overlapping recipients.
	main.Presentations = []engine.Presentation{
		{Name: "Hud", Slot: "hud", Targets: engine.ReferenceExpression{Name: "players"}, Projection: "Score", View: "ScoreView"},
		{Name: "HudAgain", Slot: "hud", Targets: engine.ReferenceExpression{Name: "players"}, Projection: "Score", View: "ScoreView"},
	}
	p.Workflows["Main"] = main

	_, err := runtime.Step(p, presentationSnapshot(), engine.Signal{Name: "WorkflowStarted"}, engine.DefaultLimits())
	if e, ok := err.(*runtime.ExecutionError); !ok || e.Code != runtime.ExecutionErrorPresentationSlotOccupied {
		t.Fatalf("expected runtime.ExecutionErrorPresentationSlotOccupied, got %v", err)
	}
}

func TestExec_QuestionPresentationMountLifecycle(t *testing.T) {
	p := presentationProgram()

	commit, _ := runtime.Step(p, presentationSnapshot(), engine.Signal{Name: "WorkflowStarted"}, engine.DefaultLimits())
	snap := commit.Snapshot

	commit, err := runtime.Step(p, snap, engine.Signal{Name: "Open"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error opening: %v", err)
	}
	var found *engine.ActivatePresentationOutput
	for _, o := range commit.Outputs {
		if a, ok := o.(engine.ActivatePresentationOutput); ok && a.Slot == "modal" {
			found = &a
		}
	}
	if found == nil || found.Recipient != presP1 || found.Name != "Ask" {
		t.Fatalf("expected an ActivatePresentationOutput for the question, got %+v", commit.Outputs)
	}
	snap = commit.Snapshot

	commit, err = runtime.Step(p, snap, engine.Signal{Name: "Close"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error closing: %v", err)
	}
	removed := false
	for _, o := range commit.Outputs {
		if r, ok := o.(engine.RemovePresentationOutput); ok && r.Slot == "modal" && r.Recipient == presP1 {
			removed = true
		}
	}
	if !removed {
		t.Fatalf("expected the question presentation to unmount on close, got %+v", commit.Outputs)
	}
}
