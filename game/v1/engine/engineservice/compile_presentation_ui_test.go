package engineservice

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// scoreProjection is a minimal projection: ResultType number, Body just
// returns the global "score" field.
func scoreProjection() program.ProjectionDeclaration {
	return program.ProjectionDeclaration{
		Name:       "Score",
		ResultType: numberType(),
		Body:       program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "score"},
	}
}

func withGlobalScoreField(def program.Definition) program.Definition {
	def.GlobalState = program.StateDeclaration{Fields: []program.StateFieldDeclaration{
		{Name: "score", Type: numberType(), Initializer: program.NumberLiteralExpression{Value: "0"}},
	}}
	return def
}

func TestCompile_ProjectionDeclaration(t *testing.T) {
	def := withGlobalScoreField(program.Definition{Projections: []program.ProjectionDeclaration{scoreProjection()}})
	def = withMinimalRootWorkflow(def)
	p, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	proj, ok := p.Projections["Score"]
	if !ok || proj.ResultType == nil {
		t.Fatalf("got %+v", proj)
	}
}

func TestCompile_ProjectionViewerCollisionRejected(t *testing.T) {
	def := program.Definition{Projections: []program.ProjectionDeclaration{
		{Name: "Bad", Parameters: []program.FieldDeclaration{{Name: "viewer", Type: userType()}}, ResultType: numberType(), Body: program.NumberLiteralExpression{Value: "1"}},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a viewer-collision error")
	}
}

func TestCompile_ProjectionCannotReferenceLocal(t *testing.T) {
	def := program.Definition{Projections: []program.ProjectionDeclaration{
		{Name: "Bad", ResultType: numberType(), Body: program.FieldExpression{Target: program.ReferenceExpression{Name: "local"}, Field: "x"}},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an out-of-scope reference error")
	}
}

func TestCompile_ProjectionResultTypeMismatch(t *testing.T) {
	def := program.Definition{Projections: []program.ProjectionDeclaration{
		{Name: "Bad", ResultType: boolType(), Body: program.NumberLiteralExpression{Value: "1"}},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a result-type mismatch error")
	}
}

func TestCompile_DuplicateProjectionName(t *testing.T) {
	def := program.Definition{Projections: []program.ProjectionDeclaration{
		{Name: "Dup", ResultType: numberType(), Body: program.NumberLiteralExpression{Value: "1"}},
		{Name: "Dup", ResultType: numberType(), Body: program.NumberLiteralExpression{Value: "2"}},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a duplicate projection name error")
	}
}

func TestCompile_ViewDeclaration(t *testing.T) {
	def := program.Definition{Views: []program.ViewDeclaration{
		{
			Name:      "Simple",
			ModelType: stringType(),
			Root: program.ContainerElement{
				Layout: program.StackLayout{},
				Children: []program.UIElement{
					program.TextElement{Value: program.ReferenceExpression{Name: "model"}},
				},
			},
		},
	}}
	def = withMinimalRootWorkflow(def)
	p, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	v, ok := p.Views["Simple"]
	if !ok || v.ModelType == nil {
		t.Fatalf("got %+v", v)
	}
	if _, ok := v.Root.(engine.ContainerElement); !ok {
		t.Fatalf("got %T", v.Root)
	}
}

func TestCompile_ViewTextElementValueMustBeString(t *testing.T) {
	def := program.Definition{Views: []program.ViewDeclaration{
		{Name: "Bad", ModelType: numberType(), Root: program.TextElement{Value: program.ReferenceExpression{Name: "model"}}},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a text-value-type error")
	}
}

func TestCompile_ViewCannotReferenceGlobal(t *testing.T) {
	def := program.Definition{Views: []program.ViewDeclaration{
		{Name: "Bad", ModelType: numberType(), Root: program.TextElement{Value: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "x"}}},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an out-of-scope reference error")
	}
}

func TestCompile_ViewLocalStateSetAndConditional(t *testing.T) {
	def := program.Definition{Views: []program.ViewDeclaration{
		{
			Name:      "Toggle",
			ModelType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
			LocalState: program.StateDeclaration{Fields: []program.StateFieldDeclaration{
				{Name: "open", Type: boolType(), Initializer: program.BoolLiteralExpression{Value: false}},
			}},
			Root: program.ConditionalElement{
				Condition: program.FieldExpression{Target: program.ReferenceExpression{Name: "local"}, Field: "open"},
				Then:      program.EmptyElement{},
				Else: program.ButtonElement{
					Configuration: program.UIElementConfiguration{Events: []program.UIEventHandler{
						{Event: program.UIEventTypeClick, Actions: []program.UIAction{
							program.SetLocalStateAction{
								Target: program.FieldTarget{Target: program.NameTarget{Name: "local"}, Field: "open"},
								Value:  program.BoolLiteralExpression{Value: true},
							},
						}},
					}},
				},
			},
		},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
}

func TestCompile_RepeatElementBindsItemAndIndex(t *testing.T) {
	// item (bound from the list<number> model's element type) and idx
	// (always number) are used where only a statically number
	// expression type-checks — proving both are bound with the right
	// types, scoped to Body only.
	def := program.Definition{Views: []program.ViewDeclaration{
		{
			Name:      "ListView",
			ModelType: program.ListTypeReference{Element: numberType()},
			Root: program.RepeatElement{
				Collection: program.ReferenceExpression{Name: "model"},
				ItemName:   "item",
				IndexName:  "idx",
				Body: program.ContainerElement{
					Layout: program.GridLayout{
						Columns: program.ReferenceExpression{Name: "item"},
						RowGap:  program.ReferenceExpression{Name: "idx"},
					},
				},
			},
		},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
}

func TestCompile_RepeatElementBindingsDoNotEscapeBody(t *testing.T) {
	def := program.Definition{Views: []program.ViewDeclaration{
		{
			Name:      "ListView",
			ModelType: program.ListTypeReference{Element: numberType()},
			Root: program.ContainerElement{
				Layout: program.StackLayout{},
				Children: []program.UIElement{
					program.RepeatElement{
						Collection: program.ReferenceExpression{Name: "model"},
						ItemName:   "item",
						Body:       program.EmptyElement{},
					},
					// "item" must not be visible here, outside the RepeatElement's Body.
					program.TextElement{Value: program.ReferenceExpression{Name: "item"}},
				},
			},
		},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undefined-reference error for \"item\" outside its RepeatElement")
	}
}

func TestCompile_SetLocalStateActionMustRootAtLocal(t *testing.T) {
	def := program.Definition{Views: []program.ViewDeclaration{
		{
			Name:      "Bad",
			ModelType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
			Root: program.ButtonElement{
				Configuration: program.UIElementConfiguration{Events: []program.UIEventHandler{
					{Event: program.UIEventTypeClick, Actions: []program.UIAction{
						program.SetLocalStateAction{Target: program.NameTarget{Name: "global"}, Value: program.NumberLiteralExpression{Value: "1"}},
					}},
				}},
			},
		},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a target-must-root-at-local error")
	}
}

func TestCompile_EmitUserIntentActionUndeclaredIntent(t *testing.T) {
	def := program.Definition{Views: []program.ViewDeclaration{
		{
			Name:      "Bad",
			ModelType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
			Root: program.ButtonElement{
				Configuration: program.UIElementConfiguration{Events: []program.UIEventHandler{
					{Event: program.UIEventTypeClick, Actions: []program.UIAction{
						program.EmitUserIntentAction{Intent: "Nope"},
					}},
				}},
			},
		},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared user intent error")
	}
}

func TestCompile_EmitUserIntentActionArgumentMismatch(t *testing.T) {
	def := program.Definition{
		UserIntents: []program.UserIntentDeclaration{
			{Name: "Guess", Parameters: []program.FieldDeclaration{{Name: "value", Type: numberType()}}},
		},
		Views: []program.ViewDeclaration{
			{
				Name:      "Bad",
				ModelType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				Root: program.ButtonElement{
					Configuration: program.UIElementConfiguration{Events: []program.UIEventHandler{
						{Event: program.UIEventTypeClick, Actions: []program.UIAction{
							program.EmitUserIntentAction{Intent: "Guess"}, // missing "value"
						}},
					}},
				},
			},
		},
	}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a missing-argument error")
	}
}

func TestCompile_EmitUserIntentActionValid(t *testing.T) {
	def := program.Definition{
		UserIntents: []program.UserIntentDeclaration{
			{Name: "Guess", Parameters: []program.FieldDeclaration{{Name: "value", Type: numberType()}}},
		},
		Views: []program.ViewDeclaration{
			{
				Name:      "Good",
				ModelType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				Root: program.ButtonElement{
					Configuration: program.UIElementConfiguration{Events: []program.UIEventHandler{
						{Event: program.UIEventTypeClick, Actions: []program.UIAction{
							program.EmitUserIntentAction{Intent: "Guess", Arguments: []program.CallArgument{{Name: "value", Value: program.NumberLiteralExpression{Value: "1"}}}},
						}},
					}},
				},
			},
		},
	}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
}

func TestCompile_InvalidUIEventTypeRejected(t *testing.T) {
	def := program.Definition{Views: []program.ViewDeclaration{
		{
			Name:      "Bad",
			ModelType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
			Root: program.ButtonElement{
				Configuration: program.UIElementConfiguration{Events: []program.UIEventHandler{
					{Event: program.UIEventType("nonsense")},
				}},
			},
		},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an invalid-event-type error")
	}
}

func TestCompile_InvalidLinearLayoutDirectionRejected(t *testing.T) {
	def := program.Definition{Views: []program.ViewDeclaration{
		{
			Name:      "Bad",
			ModelType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
			Root:      program.ContainerElement{Layout: program.LinearLayout{Direction: "diagonal"}},
		},
	}}
	def = withMinimalRootWorkflow(def)
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an invalid-direction error")
	}
}

func TestCompile_PresentationProjectionResultTypeMustMatchViewModelType(t *testing.T) {
	def := program.Definition{
		PresentationSlots: []program.PresentationSlotDeclaration{{Name: "hud"}},
		Projections:       []program.ProjectionDeclaration{{Name: "P", ResultType: numberType(), Body: program.NumberLiteralExpression{Value: "1"}}},
		Views:             []program.ViewDeclaration{{Name: "V", ModelType: boolType(), Root: program.EmptyElement{}}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				Presentations: []program.PresentationDeclaration{
					{Name: "Hud", Slot: "hud", Targets: program.ListExpression{ElementType: userType()}, Projection: "P", View: "V"},
				},
				States: []program.WorkflowStateDeclaration{{Name: "S"}},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a result-type-to-model-type assignability error")
	}
}

func TestCompile_PresentationProjectionArgumentMismatch(t *testing.T) {
	def := program.Definition{
		PresentationSlots: []program.PresentationSlotDeclaration{{Name: "hud"}},
		Projections: []program.ProjectionDeclaration{
			{Name: "P", Parameters: []program.FieldDeclaration{{Name: "label", Type: stringType()}}, ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit}, Body: program.UnitLiteralExpression{}},
		},
		Views: []program.ViewDeclaration{{Name: "V", ModelType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit}, Root: program.EmptyElement{}}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				Presentations: []program.PresentationDeclaration{
					{Name: "Hud", Slot: "hud", Targets: program.ListExpression{ElementType: userType()}, Projection: "P", View: "V"}, // missing "label"
				},
				States: []program.WorkflowStateDeclaration{{Name: "S"}},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a missing-argument error")
	}
}

// answerQuestionView is a view whose Root uses AnswerQuestionAction —
// only valid when mounted through a QuestionPresentation.
func answerQuestionView() program.ViewDeclaration {
	return program.ViewDeclaration{
		Name:      "AnswerView",
		ModelType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
		Root: program.ButtonElement{
			Configuration: program.UIElementConfiguration{Events: []program.UIEventHandler{
				{Event: program.UIEventTypeClick, Actions: []program.UIAction{
					program.AnswerQuestionAction{Value: program.BoolLiteralExpression{Value: true}},
				}},
			}},
		},
	}
}

func TestCompile_AnswerQuestionActionRejectedInPlainPresentation(t *testing.T) {
	def := program.Definition{
		PresentationSlots: []program.PresentationSlotDeclaration{{Name: "hud"}},
		Projections:       []program.ProjectionDeclaration{{Name: "P", ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit}, Body: program.UnitLiteralExpression{}}},
		Views:             []program.ViewDeclaration{answerQuestionView()},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				Presentations: []program.PresentationDeclaration{
					{Name: "Hud", Slot: "hud", Targets: program.ListExpression{ElementType: userType()}, Projection: "P", View: "AnswerView"},
				},
				States: []program.WorkflowStateDeclaration{{Name: "S"}},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an AnswerQuestionAction-in-plain-presentation error")
	}
}

func TestCompile_AnswerQuestionActionAllowedInQuestionPresentation(t *testing.T) {
	def := program.Definition{
		PresentationSlots: []program.PresentationSlotDeclaration{{Name: "modal"}},
		Projections:       []program.ProjectionDeclaration{{Name: "P", ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeUnit}, Body: program.UnitLiteralExpression{}}},
		Views:             []program.ViewDeclaration{answerQuestionView()},
		Questions: []program.QuestionDeclaration{
			{Name: "Confirm", ResponseType: boolType()},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "S",
				QuestionSlots: []program.QuestionSlotDeclaration{
					{Name: "Ask", Question: "Confirm", Presentation: &program.QuestionPresentationDeclaration{Slot: "modal", Projection: "P", View: "AnswerView"}},
				},
				States: []program.WorkflowStateDeclaration{{Name: "S"}},
			},
		},
		RootWorkflow: "Main",
	}
	p, diags := Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	slot := p.Workflows["Main"].QuestionSlots[0]
	if slot.Presentation == nil || slot.Presentation.Slot != "modal" {
		t.Fatalf("got %+v", slot.Presentation)
	}
}
