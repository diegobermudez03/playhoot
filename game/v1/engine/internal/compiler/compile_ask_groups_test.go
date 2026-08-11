package compiler_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/engine/internal/compiler"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// askGroupDef builds a definition with a "Confirm" bool-answering
// question and a "Main" root workflow declaring an ask-group slot "Ask"
// for it (recipients supplied via a list<user> parameter "recipients"),
// whose one transition runs ops.
func askGroupDef(ops []program.Operation) program.Definition {
	return program.Definition{
		Questions: []program.QuestionDeclaration{
			{Name: "Confirm", ResponseType: boolType()},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:          "Main",
				Parameters:    []program.FieldDeclaration{{Name: "recipients", Type: program.ListTypeReference{Element: userType()}}},
				ResultType:    program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				AskGroupSlots: []program.AskGroupSlotDeclaration{{Name: "Ask", Question: "Confirm"}},
				InitialState:  "S",
				States: []program.WorkflowStateDeclaration{
					{Name: "S", Transitions: []program.TransitionDeclaration{
						{
							Signal:     program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
							Operations: program.Block{Operations: ops},
							Control:    program.StayControl{},
						},
					}},
				},
			},
		},
		RootWorkflow: "Main",
	}
}

func TestCompile_OpenAskGroupOperation(t *testing.T) {
	def := askGroupDef([]program.Operation{
		program.OpenAskGroupOperation{
			Slot:       "Ask",
			Recipients: program.ReferenceExpression{Name: "recipients"},
			Completion: program.AskGroupAllResponsesPolicy{},
		},
	})
	p, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.OpenAskGroupOperation); !ok {
		t.Fatalf("got %+v", ops)
	}
}

func TestCompile_OpenAskGroupUndeclaredSlot(t *testing.T) {
	def := askGroupDef([]program.Operation{
		program.OpenAskGroupOperation{Slot: "Nope", Recipients: program.ReferenceExpression{Name: "recipients"}, Completion: program.AskGroupAllResponsesPolicy{}},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared ask-group slot error")
	}
}

func TestCompile_OpenAskGroupRecipientsMustBeListUser(t *testing.T) {
	def := askGroupDef([]program.Operation{
		program.OpenAskGroupOperation{Slot: "Ask", Recipients: program.NumberLiteralExpression{Value: "1"}, Completion: program.AskGroupAllResponsesPolicy{}},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a recipients-type error")
	}
}

func TestCompile_OpenAskGroupArgumentMismatch(t *testing.T) {
	def := program.Definition{
		Questions: []program.QuestionDeclaration{
			{Name: "Confirm", Parameters: []program.FieldDeclaration{{Name: "prompt", Type: stringType()}}, ResponseType: boolType()},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:          "Main",
				Parameters:    []program.FieldDeclaration{{Name: "recipients", Type: program.ListTypeReference{Element: userType()}}},
				ResultType:    program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				AskGroupSlots: []program.AskGroupSlotDeclaration{{Name: "Ask", Question: "Confirm"}},
				InitialState:  "S",
				States: []program.WorkflowStateDeclaration{
					{Name: "S", Transitions: []program.TransitionDeclaration{
						{
							Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
							Operations: program.Block{Operations: []program.Operation{
								program.OpenAskGroupOperation{Slot: "Ask", Recipients: program.ReferenceExpression{Name: "recipients"}, Completion: program.AskGroupAllResponsesPolicy{}}, // missing "prompt"
							}},
							Control: program.StayControl{},
						},
					}},
				},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a missing-argument error")
	}
}

func TestCompile_OpenAskGroupQuorumCountMustBeNumber(t *testing.T) {
	def := askGroupDef([]program.Operation{
		program.OpenAskGroupOperation{
			Slot:       "Ask",
			Recipients: program.ReferenceExpression{Name: "recipients"},
			Completion: program.AskGroupQuorumPolicy{Count: program.StringLiteralExpression{Value: "two"}},
		},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected a quorum-count-type error")
	}
}

func TestCompile_FinalizeAskGroupOperation(t *testing.T) {
	def := askGroupDef([]program.Operation{program.FinalizeAskGroupOperation{Slot: "Ask"}})
	p, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.FinalizeAskGroupOperation); !ok {
		t.Fatalf("got %+v", ops)
	}
}

func TestCompile_FinalizeAskGroupUndeclaredSlot(t *testing.T) {
	def := askGroupDef([]program.Operation{program.FinalizeAskGroupOperation{Slot: "Nope"}})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared ask-group slot error")
	}
}

func TestCompile_CancelAskGroupOperation(t *testing.T) {
	def := askGroupDef([]program.Operation{program.CancelAskGroupOperation{Slot: "Ask"}})
	p, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	ops := p.Workflows["Main"].States[0].Transitions[0].Operations.Operations
	if _, ok := ops[0].(engine.CancelAskGroupOperation); !ok {
		t.Fatalf("got %+v", ops)
	}
}

func TestCompile_CancelAskGroupUndeclaredSlot(t *testing.T) {
	def := askGroupDef([]program.Operation{program.CancelAskGroupOperation{Slot: "Nope"}})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared ask-group slot error")
	}
}

func TestCompile_AskGroupCompletedSignalSchema(t *testing.T) {
	def := program.Definition{
		Questions: []program.QuestionDeclaration{{Name: "Confirm", ResponseType: boolType()}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:          "Main",
				ResultType:    program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				AskGroupSlots: []program.AskGroupSlotDeclaration{{Name: "Ask", Question: "Confirm"}},
				InitialState:  "S",
				States: []program.WorkflowStateDeclaration{
					{Name: "S", Transitions: []program.TransitionDeclaration{
						{
							Signal: program.SignalPattern{
								Source: program.AskGroupCompletedSignalSource{Slot: "Ask"},
								Bindings: []program.SignalBinding{
									{Field: "responses", Name: "r"},
									{Field: "respondents", Name: "resp"},
									{Field: "missing", Name: "m"},
								},
							},
							Control: program.StayControl{},
						},
					}},
				},
			},
		},
		RootWorkflow: "Main",
	}
	_, diags := compiler.Compile(def)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
}

func TestCompile_AskGroupCompletedSignalUndeclaredSlot(t *testing.T) {
	def := workflowWithOneTransition(program.TransitionDeclaration{
		Signal:  program.SignalPattern{Source: program.AskGroupCompletedSignalSource{Slot: "Nope"}},
		Control: program.StayControl{},
	})
	_, diags := compiler.Compile(def)
	if !diags.HasErrors() {
		t.Fatal("expected an undeclared ask-group slot error")
	}
}
