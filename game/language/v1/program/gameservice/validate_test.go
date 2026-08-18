package gameservice_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
)

func TestValidate_EmptyDefinition_NoErrors(t *testing.T) {
	errs := gameservice.Validate(program.Definition{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_ArithmeticOnBoolLiteral_IsReported(t *testing.T) {
	def := program.Definition{
		Invariants: []program.InvariantDeclaration{
			{
				Name: "Bad",
				Condition: program.BinaryExpression{
					Operator: program.BinaryOperatorAdd,
					Left:     program.NumberLiteralExpression{Value: "1"},
					Right:    program.BoolLiteralExpression{Value: true},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected at least one error for add(number, bool)")
	}
	found := false
	for _, err := range errs {
		if ve, ok := err.(*gameservice.ValidationError); ok && ve.Path == "$.invariants[0].condition.right" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an error at $.invariants[0].condition.right, got %v", errs)
	}
}

func TestValidate_LogicalOperatorOnNumber_IsReported(t *testing.T) {
	def := program.Definition{
		Invariants: []program.InvariantDeclaration{
			{
				Name: "Bad",
				Condition: program.BinaryExpression{
					Operator: program.BinaryOperatorAnd,
					Left:     program.NumberLiteralExpression{Value: "1"},
					Right:    program.BoolLiteralExpression{Value: true},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for and(number, bool)")
	}
}

func TestValidate_EqualityOnMismatchedKnownTypes_IsReported(t *testing.T) {
	def := program.Definition{
		Invariants: []program.InvariantDeclaration{
			{
				Name: "Bad",
				Condition: program.BinaryExpression{
					Operator: program.BinaryOperatorEqual,
					Left:     program.NumberLiteralExpression{Value: "1"},
					Right:    program.StringLiteralExpression{Value: "1"},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for equal(number, string)")
	}
}

func TestValidate_ValidArithmetic_NoErrors(t *testing.T) {
	def := program.Definition{
		Invariants: []program.InvariantDeclaration{
			{
				Name: "Good",
				Condition: program.BinaryExpression{
					Operator: program.BinaryOperatorGreaterOrEqual,
					Left: program.BinaryExpression{
						Operator: program.BinaryOperatorAdd,
						Left:     program.NumberLiteralExpression{Value: "1"},
						Right:    program.NumberLiteralExpression{Value: "2"},
					},
					Right: program.NumberLiteralExpression{Value: "0"},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_UnknownOperandType_IsNotFlagged(t *testing.T) {
	// A reference's type cannot be statically determined without scope
	// resolution, so this must not be flagged even though it may well be
	// semantically wrong — that diagnosis belongs to the future engine.
	def := program.Definition{
		Invariants: []program.InvariantDeclaration{
			{
				Name: "Unknown",
				Condition: program.BinaryExpression{
					Operator: program.BinaryOperatorAdd,
					Left:     program.ReferenceExpression{Name: "global"},
					Right:    program.NumberLiteralExpression{Value: "1"},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for an unresolved reference operand, got %v", errs)
	}
}

func TestValidate_UnaryOperatorTypeMismatch_IsReported(t *testing.T) {
	def := program.Definition{
		Invariants: []program.InvariantDeclaration{
			{Name: "Bad", Condition: program.UnaryExpression{Operator: program.UnaryOperatorNot, Operand: program.NumberLiteralExpression{Value: "1"}}},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for not(number)")
	}
}

func TestValidate_UnknownOperatorString_IsReported(t *testing.T) {
	def := program.Definition{
		Invariants: []program.InvariantDeclaration{
			{
				Name: "Bad",
				Condition: program.BinaryExpression{
					Operator: program.BinaryOperator("not_a_real_operator"),
					Left:     program.NumberLiteralExpression{Value: "1"},
					Right:    program.NumberLiteralExpression{Value: "1"},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for an unknown binary operator")
	}
}

func TestValidate_UnknownBuiltinType_IsReported(t *testing.T) {
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "R", Type: program.BuiltinTypeReference{Type: program.BuiltinType("not_a_real_type")}, Value: program.NumberLiteralExpression{Value: "1"}},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for an unknown built-in type")
	}
}

func TestValidate_UndeclaredNamedType_IsReported(t *testing.T) {
	def := program.Definition{
		Resources: []program.ResourceDeclaration{
			{Name: "R", Type: program.NamedTypeReference{Name: "DoesNotExist"}, Value: nil},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for a reference to an undeclared type")
	}
}

func TestValidate_DeclaredNamedType_NoError(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.EnumTypeDeclaration{Name: "Color", Values: []program.EnumValueDeclaration{{Name: "RED"}}},
		},
		Resources: []program.ResourceDeclaration{
			{Name: "R", Type: program.NamedTypeReference{Name: "Color"}, Value: program.EnumValueExpression{TypeName: "Color", ValueName: "RED"}},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_DuplicateTopLevelNames_AreReported(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.EnumTypeDeclaration{Name: "Dup"},
			program.EnumTypeDeclaration{Name: "Dup"},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for duplicate type names")
	}
}

func TestValidate_DuplicateEnumValues_AreReported(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.EnumTypeDeclaration{Name: "Color", Values: []program.EnumValueDeclaration{{Name: "RED"}, {Name: "RED"}}},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for duplicate enum values")
	}
}

func TestValidate_DuplicateRecordFields_AreReported(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.RecordTypeDeclaration{
				Name: "P",
				Fields: []program.FieldDeclaration{
					{Name: "id", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeString}},
					{Name: "id", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeString}},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for duplicate record fields")
	}
}

func TestValidate_DuplicateUnionVariants_AreReported(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.UnionTypeDeclaration{
				Name: "U",
				Variants: []program.UnionVariantDeclaration{
					{Name: "A"}, {Name: "A"},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for duplicate union variants")
	}
}

func TestValidate_UnknownUIEventType_IsReported(t *testing.T) {
	def := program.Definition{
		Views: []program.ViewDeclaration{
			{
				Name: "V",
				Root: program.ButtonElement{
					Configuration: program.UIElementConfiguration{
						Events: []program.UIEventHandler{{Event: program.UIEventType("not_a_real_event")}},
					},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for an unknown UI event type")
	}
}

func TestValidate_UnknownLinearLayoutDirection_IsReported(t *testing.T) {
	def := program.Definition{
		Views: []program.ViewDeclaration{
			{
				Name: "V",
				Root: program.ContainerElement{Layout: program.LinearLayout{Direction: program.LinearLayoutDirection("diagonal")}},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for an unknown linear layout direction")
	}
}

func TestValidate_TimerDelayWrongType_IsReported(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "W",
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "t",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{
									Operations: []program.Operation{
										program.ScheduleTimerOperation{Slot: "s", DelayMilliseconds: program.StringLiteralExpression{Value: "soon"}},
									},
								},
								Control: program.StayControl{},
							},
						},
					},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for a non-numeric timer delay")
	}
}

func TestValidate_ConditionalControlNonBoolCondition_IsReported(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "W",
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "t",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Control: program.ConditionalControl{
									Condition: program.NumberLiteralExpression{Value: "1"},
									Then:      program.StayControl{},
									Else:      program.StayControl{},
								},
							},
						},
					},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for a numeric ConditionalControl condition")
	}
}

func TestValidate_DuplicateSignalBindingNames_AreReported(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "W",
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Name: "t",
								Signal: program.SignalPattern{
									Source: program.UserIntentSignalSource{Intent: "PlayCard"},
									Bindings: []program.SignalBinding{
										{Field: "actor", Name: "x"},
										{Field: "card", Name: "x"},
									},
								},
								Control: program.StayControl{},
							},
						},
					},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for duplicate signal binding names")
	}
}

func TestValidate_DuplicateWorkflowStateNames_AreReported(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "W",
				InitialState: "S",
				States: []program.WorkflowStateDeclaration{
					{Name: "S"}, {Name: "S"},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for duplicate workflow state names")
	}
}

func TestValidate_ReferenceResolutionIsNeverPerformed(t *testing.T) {
	// Unknown workflow/state/slot names, unresolved function calls, and
	// unresolved references are all left to the future engine compiler —
	// Validate must not flag any of them.
	def := program.Definition{
		RootWorkflow: "DoesNotExist",
		Functions: []program.FunctionDeclaration{
			{
				Name:       "Recursive",
				ResultType: program.BuiltinTypeReference{Type: program.BuiltinTypeBool},
				Body:       program.CallExpression{Function: "Recursive"},
			},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "W",
				InitialState: "UnknownState",
				QuestionSlots: []program.QuestionSlotDeclaration{
					{Name: "s", Question: "UnknownQuestion"},
				},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "OnlyState",
						Transitions: []program.TransitionDeclaration{
							{
								Name:    "t",
								Signal:  program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Control: program.GotoControl{State: "UnknownTarget"},
							},
						},
					},
				},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) != 0 {
		t.Fatalf("expected no errors from reference-resolution concerns, got %v", errs)
	}
}

func TestValidate_ErrorsAreValidationErrorType(t *testing.T) {
	def := program.Definition{
		Types: []program.TypeDeclaration{
			program.EnumTypeDeclaration{Name: "Dup"},
			program.EnumTypeDeclaration{Name: "Dup"},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected at least one error")
	}
	for _, err := range errs {
		if _, ok := err.(*gameservice.ValidationError); !ok {
			t.Fatalf("expected *gameservice.ValidationError, got %T", err)
		}
	}
}
