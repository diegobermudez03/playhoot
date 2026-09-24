package gameservice_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
)

func TestValidate_KeyedQuestionSlotKeyType_UndeclaredNamedType_IsReported(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:               "W",
				InitialState:       "S",
				KeyedQuestionSlots: []program.KeyedQuestionSlotDeclaration{{Name: "quiz", Question: "Q", KeyType: program.NamedTypeReference{Name: "DoesNotExist"}}},
				States:             []program.WorkflowStateDeclaration{{Name: "S"}},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) == 0 {
		t.Fatal("expected an error for a keyed question slot's KeyType referencing an undeclared type")
	}
}

func TestValidate_KeyedQuestionSlotKeyType_DeclaredBuiltinType_NoError(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:               "W",
				InitialState:       "S",
				KeyedQuestionSlots: []program.KeyedQuestionSlotDeclaration{{Name: "quiz", Question: "Q", KeyType: program.BuiltinTypeReference{Type: program.BuiltinTypeString}}},
				States:             []program.WorkflowStateDeclaration{{Name: "S"}},
			},
		},
	}
	errs := gameservice.Validate(def)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
}

func TestValidate_KeyedTimerDelayWrongType_IsReported(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:            "W",
				InitialState:    "S",
				KeyedTimerSlots: []program.KeyedTimerSlotDeclaration{{Name: "s", KeyType: program.BuiltinTypeReference{Type: program.BuiltinTypeString}}},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "t",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{
									Operations: []program.Operation{
										program.ScheduleKeyedTimerOperation{
											Slot:              "s",
											Key:               program.StringLiteralExpression{Value: "p1"},
											DelayMilliseconds: program.StringLiteralExpression{Value: "soon"},
										},
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
		t.Fatal("expected an error for a non-numeric keyed timer delay")
	}
}

func TestValidate_KeyedAskGroupQuorumWrongType_IsReported(t *testing.T) {
	def := program.Definition{
		Workflows: []program.WorkflowDeclaration{
			{
				Name:               "W",
				InitialState:       "S",
				KeyedAskGroupSlots: []program.KeyedAskGroupSlotDeclaration{{Name: "team", Question: "Q", KeyType: program.BuiltinTypeReference{Type: program.BuiltinTypeString}}},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "S",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "t",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{
									Operations: []program.Operation{
										program.OpenKeyedAskGroupOperation{
											Slot:       "team",
											Key:        program.StringLiteralExpression{Value: "team_a"},
											Recipients: program.ListExpression{ElementType: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}},
											Completion: program.AskGroupQuorumPolicy{Count: program.StringLiteralExpression{Value: "two"}},
										},
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
		t.Fatal("expected an error for a non-numeric keyed ask-group quorum count")
	}
}
