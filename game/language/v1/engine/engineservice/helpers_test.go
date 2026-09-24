package engineservice_test

import (
	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// This file holds small fixtures and helpers used only by this
// package's tests: minimal program.TypeReference builders, and
// hand-built program.Definition/engine.Program fixtures supporting
// CheckSnapshotCompatibility and the cross-package
// compile->initialize->step integration tests. They are local, rather
// than shared with engine/internal/compiler's or engine/internal/runtime's
// own test fixtures of similar shape, because engineservice cannot
// import those internal packages' test files.

func numberType() program.TypeReference {
	return program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}
}
func boolType() program.TypeReference {
	return program.BuiltinTypeReference{Type: program.BuiltinTypeBool}
}
func stringType() program.TypeReference {
	return program.BuiltinTypeReference{Type: program.BuiltinTypeString}
}
func userType() program.TypeReference {
	return program.BuiltinTypeReference{Type: program.BuiltinTypeUser}
}

const (
	askAlice = engine.UserID("alice")
	askBob   = engine.UserID("bob")
)

const player = engine.UserID("player-1")

const (
	presP1 = engine.UserID("p1")
	presP2 = engine.UserID("p2")
)

func countOutputs[T engine.Output](outputs []engine.Output) int {
	n := 0
	for _, o := range outputs {
		if _, ok := o.(T); ok {
			n++
		}
	}
	return n
}

// globalCountField is the AssignmentTarget for global.count, reused by
// counterProgram below.
var globalCountField = engine.FieldTarget{Target: engine.NameTarget{Name: "global"}, Field: "count"}

// counterProgram builds a hand-assembled engine.Program (bypassing
// engineservice.Compile): a "Counter" workflow that increments
// global.count on "Increment" and completes with it once at least 3 on
// "Finish", with a global "Abort" transition that cancels.
func counterProgram() engine.Program {
	incrementOp := engine.SetOperation{
		Target: globalCountField,
		Value: engine.BinaryExpression{
			Operator: engine.BinaryOperatorAdd,
			Left:     engine.FieldExpression{Target: engine.ReferenceExpression{Name: "global"}, Field: "count"},
			Right:    engine.NumberLiteralExpression{Value: 1},
		},
	}
	finishGuard := engine.BinaryExpression{
		Operator: engine.BinaryOperatorGreaterOrEqual,
		Left:     engine.FieldExpression{Target: engine.ReferenceExpression{Name: "global"}, Field: "count"},
		Right:    engine.NumberLiteralExpression{Value: 3},
	}

	workflow := engine.Workflow{
		Name:         "Counter",
		ResultType:   engine.NumberType{},
		InitialState: "Running",
		States: []engine.WorkflowState{
			{
				Name: "Running",
				Transitions: []engine.Transition{
					{
						Name:       "Increment",
						Signal:     engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Increment"}},
						Operations: engine.Block{Operations: []engine.Operation{incrementOp}},
						Control:    engine.StayControl{},
					},
					{
						Name:    "Finish",
						Signal:  engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Finish"}},
						Guard:   finishGuard,
						Control: engine.CompleteControl{Result: engine.FieldExpression{Target: engine.ReferenceExpression{Name: "global"}, Field: "count"}},
					},
				},
			},
		},
		GlobalTransitions: []engine.Transition{
			{
				Name:    "Abort",
				Signal:  engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Abort"}},
				Control: engine.CancelControl{Reason: engine.StringLiteralExpression{Value: "aborted"}},
			},
		},
	}

	return engine.Program{
		RootWorkflow: "Counter",
		Workflows:    map[string]engine.Workflow{"Counter": workflow},
	}
}

func counterSnapshot(count float64) engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global", Fields: []engine.FieldValue{{Name: "count", Value: engine.NumberValue{Value: count}}}},
		Root: engine.WorkflowInstance{
			Workflow:   "Counter",
			State:      "Running",
			LocalState: engine.RecordValue{TypeName: "local"},
		},
	}
}

// askGroupProgram builds a hand-assembled engine.Program (bypassing
// engineservice.Compile): a "Main" workflow taking a list<user>
// parameter "recipients", with one ask-group slot "Ask" backed by a
// bool-answering question "Confirm", able to open (with the given
// completion policy), finalize, or cancel the group, and join its
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
	return engineservice.Step(p, snap, engine.Signal{Kind: engine.SignalKindAskGroupAnswered, Slot: "Ask", Respondent: respondent, Answer: engine.BoolValue{Value: answer}}, engine.DefaultLimits())
}

// questionDemoProgram builds a hand-assembled engine.Program (bypassing
// engineservice.Compile) for a single workflow "QDemo" with one
// question slot ("Ask", backed by question "Confirm") and one timer
// slot ("Deadline"). Its "S" state has transitions to open/close the
// question, schedule/cancel the timer, emit an effect, and a
// QuestionAnswered/TimerExpired-sourced transition
// ("Answered"/"Expired") that completes the workflow so dispatch can be
// observed.
func questionDemoProgram() engine.Program {
	openOp := engine.OpenQuestionOperation{
		Slot:      "Ask",
		Recipient: engine.ReferenceExpression{Name: "player"},
		Arguments: []engine.CallArgument{{Name: "prompt", Value: engine.StringLiteralExpression{Value: "Ready?"}}},
	}
	closeOp := engine.CloseQuestionOperation{Slot: "Ask"}
	scheduleOp := engine.ScheduleTimerOperation{Slot: "Deadline", DelayMilliseconds: engine.NumberLiteralExpression{Value: 5000}}
	cancelOp := engine.CancelTimerOperation{Slot: "Deadline"}
	emitOp := engine.EmitEffectOperation{
		Effect:     "Confetti",
		Recipients: engine.ReferenceExpression{Name: "recipients"},
		Arguments:  []engine.CallArgument{{Name: "amount", Value: engine.NumberLiteralExpression{Value: 10}}},
	}

	workflow := engine.Workflow{
		Name:          "QDemo",
		Parameters:    []engine.FieldType{{Name: "player", Type: engine.UserType{}}, {Name: "recipients", Type: engine.ListType{Element: engine.UserType{}}}},
		ResultType:    engine.UnitType{},
		QuestionSlots: []engine.QuestionSlot{{Name: "Ask", Question: "Confirm"}},
		TimerSlots:    []string{"Deadline"},
		InitialState:  "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "Open", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Open"}}, Operations: engine.Block{Operations: []engine.Operation{openOp}}, Control: engine.StayControl{}},
					{Name: "Close", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Close"}}, Operations: engine.Block{Operations: []engine.Operation{closeOp}}, Control: engine.StayControl{}},
					{Name: "Schedule", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Schedule"}}, Operations: engine.Block{Operations: []engine.Operation{scheduleOp}}, Control: engine.StayControl{}},
					{Name: "Cancel", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Cancel"}}, Operations: engine.Block{Operations: []engine.Operation{cancelOp}}, Control: engine.StayControl{}},
					{Name: "Emit", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Emit"}}, Operations: engine.Block{Operations: []engine.Operation{emitOp}}, Control: engine.StayControl{}},
					{
						Name:    "Answered",
						Signal:  engine.SignalPattern{Source: engine.QuestionAnsweredSignalSource{Slot: "Ask"}, Bindings: []engine.SignalBinding{{Field: "answer", Name: "a"}}},
						Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}},
					},
					{
						Name:    "Expired",
						Signal:  engine.SignalPattern{Source: engine.TimerExpiredSignalSource{Slot: "Deadline"}},
						Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}},
					},
				},
			},
		},
	}

	return engine.Program{
		RootWorkflow: "QDemo",
		Workflows:    map[string]engine.Workflow{"QDemo": workflow},
		Questions: map[string]engine.Question{
			"Confirm": {
				Name:         "Confirm",
				Parameters:   []engine.FieldType{{Name: "prompt", Type: engine.StringType{}}},
				ResponseType: engine.BoolType{},
			},
		},
		Effects: map[string]engine.Effect{
			"Confetti": {Name: "Confetti", Parameters: []engine.FieldType{{Name: "amount", Type: engine.NumberType{}}}},
		},
	}
}

func questionDemoSnapshot() engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root: engine.WorkflowInstance{
			Workflow: "QDemo",
			State:    "S",
			Parameters: []engine.FieldValue{
				{Name: "player", Value: engine.UserValue{ID: player}},
				{Name: "recipients", Value: engine.ListValue{ElementType: engine.UserType{}, Elements: []engine.Value{engine.UserValue{ID: player}}}},
			},
			LocalState:    engine.RecordValue{TypeName: "local"},
			QuestionSlots: []engine.QuestionSlotInstance{{Name: "Ask"}},
			TimerSlots:    []engine.TimerSlotInstance{{Name: "Deadline"}},
		},
	}
}
