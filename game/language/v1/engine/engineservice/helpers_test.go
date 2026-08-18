package engineservice_test

import (
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// This file gathers small local duplicates of fixtures and helpers that
// used to live alongside the tests that needed them, before those
// tests (and their fixtures) moved into engine/internal/compiler and
// engine/internal/runtime. The tests remaining here — genuine
// engineservice-level logic (CheckSnapshotCompatibility) and
// cross-package integration tests exercising the full
// compile->initialize->step pipeline — still need them, but can no
// longer share code with the packages the originals moved to. Per this
// codebase's convention for that exact situation (see
// findInstanceQuestionSlot/findInstanceTimerSlot's history), each side
// keeps its own minimal copy of just what it needs.

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

// childWorkflowProgram builds a hand-assembled engine.Program (bypassing
// engineservice.Compile): a "Worker" workflow taking one number
// parameter "amount" and returning it, reacting to three intents
// ("Succeed", "Fail", "SelfCancel") to reach each of the three terminal
// outcomes; and a "Main" root workflow with one child slot "W" declared
// against "Worker", whose "S" state can spawn into "W", cancel it, join
// each of its three possible outcomes, or terminate itself.
func childWorkflowProgram() engine.Program {
	worker := engine.Workflow{
		Name:         "Worker",
		Parameters:   []engine.FieldType{{Name: "amount", Type: engine.NumberType{}}},
		ResultType:   engine.NumberType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "Started", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "WorkflowStarted"}}, Control: engine.StayControl{}},
					{Name: "Succeed", Signal: engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: "Succeed"}}, Control: engine.CompleteControl{Result: engine.ReferenceExpression{Name: "amount"}}},
					{Name: "Fail", Signal: engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: "Fail"}}, Control: engine.FailControl{Error: engine.StringLiteralExpression{Value: "boom"}}},
					{Name: "SelfCancel", Signal: engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: "SelfCancel"}}, Control: engine.CancelControl{Reason: engine.StringLiteralExpression{Value: "nope"}}},
				},
			},
		},
	}

	spawnOp := engine.SpawnChildWorkflowOperation{Slot: "W", Arguments: []engine.CallArgument{{Name: "amount", Value: engine.NumberLiteralExpression{Value: 42}}}}
	cancelOp := engine.CancelChildWorkflowOperation{Slot: "W", Reason: engine.StringLiteralExpression{Value: "parent cancelled"}}

	main := engine.Workflow{
		Name:         "Main",
		ResultType:   engine.UnitType{},
		ChildSlots:   []engine.ChildWorkflowSlot{{Name: "W", Workflow: "Worker"}},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "Spawn", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Spawn"}}, Operations: engine.Block{Operations: []engine.Operation{spawnOp}}, Control: engine.StayControl{}},
					{Name: "CancelChild", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "CancelChild"}}, Operations: engine.Block{Operations: []engine.Operation{cancelOp}}, Control: engine.StayControl{}},
					{Name: "Terminate", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Terminate"}}, Control: engine.CompleteControl{Result: engine.UnitLiteralExpression{}}},
					{
						Name:    "JoinCompleted",
						Signal:  engine.SignalPattern{Source: engine.ChildCompletedSignalSource{Slot: "W"}, Bindings: []engine.SignalBinding{{Field: "result", Name: "r"}}},
						Control: engine.StayControl{},
					},
					{
						Name:    "JoinFailed",
						Signal:  engine.SignalPattern{Source: engine.ChildFailedSignalSource{Slot: "W"}, Bindings: []engine.SignalBinding{{Field: "error", Name: "e"}}},
						Control: engine.StayControl{},
					},
					{
						Name:    "JoinCancelled",
						Signal:  engine.SignalPattern{Source: engine.ChildCancelledSignalSource{Slot: "W"}, Bindings: []engine.SignalBinding{{Field: "reason", Name: "c"}}},
						Control: engine.StayControl{},
					},
				},
			},
		},
	}

	return engine.Program{
		RootWorkflow: "Main",
		Workflows:    map[string]engine.Workflow{"Main": main, "Worker": worker},
	}
}

func mainSnapshot() engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root: engine.WorkflowInstance{
			Workflow:   "Main",
			State:      "S",
			LocalState: engine.RecordValue{TypeName: "local"},
			ChildSlots: []engine.ChildWorkflowSlotInstance{{Name: "W"}},
		},
	}
}

// spawnAndStart spawns a child in "W" and applies its WorkflowStarted
// internal signal, returning the resulting snapshot.
func spawnAndStart(t *testing.T, p engine.Program, snap engine.Snapshot) engine.Snapshot {
	t.Helper()
	commit, err := engineservice.Step(p, snap, engine.Signal{Name: "Spawn"}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error spawning: %v", err)
	}
	if len(commit.InternalSignals) != 1 {
		t.Fatalf("got %d internal signals, want 1", len(commit.InternalSignals))
	}
	started := commit.InternalSignals[0]
	if started.Name != "WorkflowStarted" || len(started.Path) != 1 || started.Path[0].Slot != "W" {
		t.Fatalf("got %+v", started)
	}
	commit, err = engineservice.Step(p, commit.Snapshot, started, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error starting child: %v", err)
	}
	return commit.Snapshot
}

// taskGroupProgram builds a hand-assembled engine.Program (bypassing
// engineservice.Compile): a "Worker" workflow taking one number
// parameter "amount" and returning it, reacting to three intents
// ("Succeed", "Fail", "SelfCancel") to reach each of the three terminal
// outcomes; and a "Main" root workflow with one number-keyed task-group
// slot "Workers" backed by it, whose "S" state can begin (with the
// given completion policy)+seal a fixed-size batch of tasks in one
// transition, finalize, or cancel the group, and join its completion
// signal with joinControl (bindings "taskKeys"->keys,
// "terminalKeys"->terminal, "results"->results, "failures"->failures,
// "cancellations"->cancellations, "unfinished"->unfinished).
func taskGroupProgram(completion engine.TaskGroupCompletionPolicy, taskAmounts []float64, resultType engine.Type, joinControl engine.WorkflowControl) engine.Program {
	worker := engine.Workflow{
		Name:         "Worker",
		Parameters:   []engine.FieldType{{Name: "amount", Type: engine.NumberType{}}},
		ResultType:   engine.NumberType{},
		InitialState: "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "Started", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "WorkflowStarted"}}, Control: engine.StayControl{}},
					{Name: "Succeed", Signal: engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: "Succeed"}}, Control: engine.CompleteControl{Result: engine.ReferenceExpression{Name: "amount"}}},
					{Name: "Fail", Signal: engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: "Fail"}}, Control: engine.FailControl{Error: engine.StringLiteralExpression{Value: "boom"}}},
					{Name: "SelfCancel", Signal: engine.SignalPattern{Source: engine.UserIntentSignalSource{Intent: "SelfCancel"}}, Control: engine.CancelControl{Reason: engine.StringLiteralExpression{Value: "nope"}}},
				},
			},
		},
	}

	beginOp := engine.BeginTaskGroupOperation{Slot: "Workers", Completion: completion}
	spawnOps := make([]engine.Operation, len(taskAmounts))
	for i, amount := range taskAmounts {
		spawnOps[i] = engine.SpawnTaskGroupChildOperation{
			Slot:      "Workers",
			Key:       engine.NumberLiteralExpression{Value: float64(i + 1)},
			Arguments: []engine.CallArgument{{Name: "amount", Value: engine.NumberLiteralExpression{Value: amount}}},
		}
	}
	sealOp := engine.SealTaskGroupOperation{Slot: "Workers"}
	finalizeOp := engine.FinalizeTaskGroupOperation{Slot: "Workers"}
	cancelOp := engine.CancelTaskGroupOperation{Slot: "Workers", Reason: engine.StringLiteralExpression{Value: "abandoned"}}

	beginAndSealOps := append(append([]engine.Operation{beginOp}, spawnOps...), sealOp)

	main := engine.Workflow{
		Name:           "Main",
		ResultType:     resultType,
		TaskGroupSlots: []engine.TaskGroupSlot{{Name: "Workers", Workflow: "Worker", KeyType: engine.NumberType{}}},
		InitialState:   "S",
		States: []engine.WorkflowState{
			{
				Name: "S",
				Transitions: []engine.Transition{
					{Name: "BeginAndSeal", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "BeginAndSeal"}}, Operations: engine.Block{Operations: beginAndSealOps}, Control: engine.StayControl{}},
					{Name: "Finalize", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "Finalize"}}, Operations: engine.Block{Operations: []engine.Operation{finalizeOp}}, Control: engine.StayControl{}},
					{Name: "CancelGroup", Signal: engine.SignalPattern{Source: engine.NamedSignalSource{Name: "CancelGroup"}}, Operations: engine.Block{Operations: []engine.Operation{cancelOp}}, Control: engine.StayControl{}},
					{
						Name: "Join",
						Signal: engine.SignalPattern{
							Source: engine.TaskGroupCompletedSignalSource{Slot: "Workers"},
							Bindings: []engine.SignalBinding{
								{Field: "taskKeys", Name: "keys"},
								{Field: "terminalKeys", Name: "terminal"},
								{Field: "results", Name: "results"},
								{Field: "failures", Name: "failures"},
								{Field: "cancellations", Name: "cancellations"},
								{Field: "unfinished", Name: "unfinished"},
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
		Workflows:    map[string]engine.Workflow{"Main": main, "Worker": worker},
	}
}

func taskGroupSnapshot() engine.Snapshot {
	return engine.Snapshot{
		GlobalState: engine.RecordValue{TypeName: "global"},
		Root: engine.WorkflowInstance{
			Workflow:       "Main",
			State:          "S",
			LocalState:     engine.RecordValue{TypeName: "local"},
			TaskGroupSlots: []engine.TaskGroupSlotInstance{{Name: "Workers"}},
		},
	}
}

func taskPath(key float64) []engine.PathStep {
	return []engine.PathStep{{Slot: "Workers", TaskKey: engine.NumberValue{Value: key}}}
}

func terminateTask(t *testing.T, p engine.Program, snap engine.Snapshot, key float64, intent string) engine.Snapshot {
	t.Helper()
	commit, err := engineservice.Step(p, snap, engine.Signal{Kind: engine.SignalKindIntent, Path: taskPath(key), Intent: intent}, engine.DefaultLimits())
	if err != nil {
		t.Fatalf("unexpected error terminating task %v via %q: %v", key, intent, err)
	}
	return commit.Snapshot
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
