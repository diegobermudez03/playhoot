package sessionlifecycle

import (
	"context"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/management"
)

// compilableDefinitionForTest is a minimal Game Language definition that
// compiles successfully, reused by integration tests that need Create to
// succeed against a real engineservice.Compile call.
func compilableDefinitionForTest(playersMax int) program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "parques", Name: "Parques"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Max: playersMax},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				States:       []program.WorkflowStateDeclaration{{Name: "Start"}},
			},
		},
	}
}

// uncompilableDefinitionForTest is a Game Language definition that fails
// engineservice.Compile, reused by integration tests that need Create to be
// rejected for a broken definition.
func uncompilableDefinitionForTest() program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "broken", Name: "Broken"},
		RootWorkflow: "does-not-exist",
	}
}

// stubCurrentGameReader satisfies gameCurrentVersionReader with a fixed
// playable Game, so integration tests can exercise the real public
// Manager.Create end to end without depending on Game Management.
type stubCurrentGameReader struct {
	gameUUID    string
	versionUUID string
	definition  program.Definition
}

func (s stubCurrentGameReader) GetPlayableGameWithCurrentVersion(ctx context.Context, gameUUID string) (*management.Game, error) {
	return &management.Game{UUID: gameUUID, VersionUUID: s.versionUUID, Definition: s.definition}, nil
}

// stubPinnedGameReader satisfies gamePinnedDefinitionReader with a fixed
// pinned Definition, so integration tests can exercise the real public
// Manager.Join end to end without depending on Game Management.
type stubPinnedGameReader struct {
	playersMax int
}

func (s stubPinnedGameReader) GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error) {
	return &program.Definition{Players: program.PlayerPolicy{Max: s.playersMax}}, nil
}

// startableDefinition builds a real, engineservice.Compile-able Definition
// declaring the accepted `players: list<user>` root roster parameter
// (game/README.md's Accepted Game Language Root Roster Contract) and a
// root workflow that actually reacts to WorkflowStarted with StayControl -
// unlike testutil_test.go's other fixtures (compilableDefinitionForTest,
// stubPinnedGameReader's minimal stub), which never execute past compile.
// Start's integration tests need a Definition that both compiles and
// executes its first RuntimeTurn successfully.
func startableDefinition(playersMin, playersMax int) program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "startable", Name: "Startable"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Workflows: []program.WorkflowDeclaration{
			{
				Name: "Main",
				Parameters: []program.FieldDeclaration{
					{Name: "players", Type: program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}}},
				},
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{Name: "Started", Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}}, Control: program.StayControl{}},
						},
					},
				},
			},
		},
	}
}

// nonStartableDefinition builds a Definition that compiles successfully
// (so Create could pin it) but whose root workflow declares no transition
// at all for WorkflowStarted - Start's mandatory first Step call is then an
// outright rejection, forcing the pre-first-Turn RUNTIME_EXECUTION_FAILED
// fatal path deterministically, for tests that need to force that path
// against a real database.
func nonStartableDefinition(playersMin, playersMax int) program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "non-startable", Name: "NonStartable"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				States:       []program.WorkflowStateDeclaration{{Name: "Start"}},
			},
		},
	}
}

// stubStartPinnedGameReader satisfies gamePinnedDefinitionReader with a
// fixed, caller-supplied Definition, so Start's/AnswerInteraction's
// integration tests can exercise real
// engineservice.Compile/NewSnapshot/Step execution end to end against
// whichever fixture (startableDefinition/nonStartableDefinition/
// answerableDefinition) a given test case needs.
type stubStartPinnedGameReader struct {
	definition program.Definition
}

func (s stubStartPinnedGameReader) GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error) {
	return &s.definition, nil
}

// answerableQuestionName/answerableSlot/answerableEffect name
// answerableDefinition's own declarations, reused directly by tests that
// need to assert against them (constructing an expected engine.Value
// answer, or asserting a captured interaction's slot).
const (
	answerableQuestionName = "PickNumber"
	answerableSlot         = "Q"
)

// answerableDefinition builds a real, engineservice.Compile-able Definition
// declaring the accepted `players: list<user>` root roster parameter, whose
// root workflow opens a Question at players[0] immediately at Start (Slot
// "Q", Question "PickNumber", a bare number response) and closes it once
// answered - for tests that need a Definition compiling and executing an
// interaction end to end, whether opened at Start or answered afterward.
func answerableDefinition(playersMin, playersMax int) program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "answerable", Name: "Answerable"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Questions: []program.QuestionDeclaration{
			{
				Name:         answerableQuestionName,
				ResponseType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber},
			},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name: "Main",
				Parameters: []program.FieldDeclaration{
					{Name: "players", Type: program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}}},
				},
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				QuestionSlots: []program.QuestionSlotDeclaration{
					{Name: answerableSlot, Question: answerableQuestionName},
				},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "Started",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenQuestionOperation{
										Slot: answerableSlot,
										Recipient: program.IndexExpression{
											Target: program.ReferenceExpression{Name: "players"},
											Index:  program.NumberLiteralExpression{Value: "0"},
										},
									},
								}},
								Control: program.StayControl{},
							},
							{
								Name:   "Answered",
								Signal: program.SignalPattern{Source: program.QuestionAnsweredSignalSource{Slot: answerableSlot}},
								Operations: program.Block{Operations: []program.Operation{
									program.CloseQuestionOperation{Slot: answerableSlot},
								}},
								Control: program.StayControl{},
							},
						},
					},
				},
			},
		},
	}
}

// answerableDefinitionWithFatalAnswer is answerableDefinition's shape,
// except its "Answered" transition deterministically fails (division by
// zero) instead of closing the question - forcing AnswerInteraction's fatal
// RUNTIME_EXECUTION_FAILED path against a real database.
func answerableDefinitionWithFatalAnswer(playersMin, playersMax int) program.Definition {
	d := answerableDefinition(playersMin, playersMax)
	d.Metadata = program.Metadata{ID: "answerable-fatal", Name: "AnswerableFatal"}
	d.GlobalState = program.StateDeclaration{
		Fields: []program.StateFieldDeclaration{
			{Name: "n", Type: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}, Initializer: program.NumberLiteralExpression{Value: "0"}},
		},
	}
	d.Workflows[0].States[0].Transitions[1].Operations = program.Block{Operations: []program.Operation{
		program.SetOperation{
			Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "n"},
			Value: program.BinaryExpression{
				Operator: program.BinaryOperatorDivide,
				Left:     program.NumberLiteralExpression{Value: "1"},
				Right:    program.NumberLiteralExpression{Value: "0"},
			},
		},
	}}
	return d
}

// askGroupAnswerableQuestionName/askGroupAnswerableSlot name
// askGroupAnswerableDefinition's own declarations.
const (
	askGroupAnswerableQuestionName = "PickNumber"
	askGroupAnswerableSlot         = "AG"
)

// askGroupAnswerableDefinition builds a real, engineservice.Compile-able
// Definition whose root workflow opens an ask group at all of `players`
// immediately at Start (Slot "AG", a bare number response, completing once
// every recipient has answered). Answering an ask-group member never
// selects or runs a workflow transition and never produces InternalSignals
// (the engine's own documented behavior for SignalKindInteractionAnswered
// against an Ask Group occurrence), so unlike answerableDefinition this
// fixture needs no transition at all beyond the one that opens the group.
func askGroupAnswerableDefinition(playersMin, playersMax int) program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "ask-group-answerable", Name: "AskGroupAnswerable"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Questions: []program.QuestionDeclaration{
			{
				Name:         askGroupAnswerableQuestionName,
				ResponseType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber},
			},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name: "Main",
				Parameters: []program.FieldDeclaration{
					{Name: "players", Type: program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}}},
				},
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				AskGroupSlots: []program.AskGroupSlotDeclaration{
					{Name: askGroupAnswerableSlot, Question: askGroupAnswerableQuestionName},
				},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "Started",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenAskGroupOperation{
										Slot:       askGroupAnswerableSlot,
										Recipients: program.ReferenceExpression{Name: "players"},
										Completion: program.AskGroupAllResponsesPolicy{},
									},
								}},
								Control: program.StayControl{},
							},
						},
					},
				},
			},
		},
	}
}

// dualQuestionName/dualPrimarySlot/dualSecondarySlot name
// dualQuestionDefinition's own declarations.
const (
	dualQuestionName  = "PickNumber"
	dualPrimarySlot   = "Q1"
	dualSecondarySlot = "Q2"
)

// dualQuestionDefinition builds a real, engineservice.Compile-able
// Definition whose root workflow opens two independent questions at
// players[0] immediately at Start (Q1 and Q2), and whose Q1-answered
// transition explicitly closes the still-pending Q2 via
// CloseQuestionOperation - the one case that actually produces a
// CloseQuestionOutput for a slot other than the one just answered.
func dualQuestionDefinition(playersMin, playersMax int) program.Definition {
	recipient := program.IndexExpression{
		Target: program.ReferenceExpression{Name: "players"},
		Index:  program.NumberLiteralExpression{Value: "0"},
	}
	return program.Definition{
		Metadata:     program.Metadata{ID: "dual-question", Name: "DualQuestion"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Questions: []program.QuestionDeclaration{
			{
				Name:         dualQuestionName,
				ResponseType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber},
			},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name: "Main",
				Parameters: []program.FieldDeclaration{
					{Name: "players", Type: program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}}},
				},
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				QuestionSlots: []program.QuestionSlotDeclaration{
					{Name: dualPrimarySlot, Question: dualQuestionName},
					{Name: dualSecondarySlot, Question: dualQuestionName},
				},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "Started",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenQuestionOperation{Slot: dualPrimarySlot, Recipient: recipient},
									program.OpenQuestionOperation{Slot: dualSecondarySlot, Recipient: recipient},
								}},
								Control: program.StayControl{},
							},
							{
								Name:   "PrimaryAnswered",
								Signal: program.SignalPattern{Source: program.QuestionAnsweredSignalSource{Slot: dualPrimarySlot}},
								Operations: program.Block{Operations: []program.Operation{
									program.CloseQuestionOperation{Slot: dualSecondarySlot},
								}},
								Control: program.StayControl{},
							},
						},
					},
				},
			},
		},
	}
}

// replayObservableQuestionName/replayObservableSlot/replayObservableSlot2/
// replayRandomArgName name replayObservableDefinition's own declarations.
const (
	replayObservableQuestionName = "PickNumber"
	replayObservableSlot         = "Q1"
	replayObservableSlot2        = "Q2"
	replayRandomArgName          = "n"
)

// replayObservableDefinition builds a real, engineservice.Compile-able
// Definition designed specifically so a test can verify replay
// reconstruction against values the live execution actually produced,
// rather than against values re-derived from the same durable rows replay
// itself reads (which would only prove replay is consistent with itself,
// not that it matches live execution).
//
// At Start it draws a random number and exposes it as the first opened
// question's own "n" argument - captured live into
// session_interactions.interaction_payload through the ordinary
// OpenQuestionOutput capture path, a channel entirely independent of
// session_runtime_starts.seed. Once that question is answered, the
// response is stored into global state ("a") and a second question is
// opened; once that one is answered too, its response is stored into
// global state ("b"). A reconstructed Snapshot's global state can then be
// compared directly against the plain Go values a test passed to
// AnswerInteraction, with no decoding of any persisted row on either side -
// and the third Turn only produces a meaningful "b" if replay correctly
// threaded the second Turn's answer through first.
func replayObservableDefinition(playersMin, playersMax int) program.Definition {
	recipient := program.IndexExpression{
		Target: program.ReferenceExpression{Name: "players"},
		Index:  program.NumberLiteralExpression{Value: "0"},
	}
	numberType := program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}
	return program.Definition{
		Metadata:     program.Metadata{ID: "replay-observable", Name: "ReplayObservable"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Questions: []program.QuestionDeclaration{
			{
				Name:         replayObservableQuestionName,
				Parameters:   []program.FieldDeclaration{{Name: replayRandomArgName, Type: numberType}},
				ResponseType: numberType,
			},
		},
		GlobalState: program.StateDeclaration{
			Fields: []program.StateFieldDeclaration{
				{Name: "n", Type: numberType, Initializer: program.NumberLiteralExpression{Value: "0"}},
				{Name: "a", Type: numberType, Initializer: program.NumberLiteralExpression{Value: "0"}},
				{Name: "b", Type: numberType, Initializer: program.NumberLiteralExpression{Value: "0"}},
			},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name: "Main",
				Parameters: []program.FieldDeclaration{
					{Name: "players", Type: program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}}},
				},
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				QuestionSlots: []program.QuestionSlotDeclaration{
					{Name: replayObservableSlot, Question: replayObservableQuestionName},
					{Name: replayObservableSlot2, Question: replayObservableQuestionName},
				},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "Started",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.DrawRandomOperation{
										Name: "drawn",
										Generator: program.RandomIntegerGenerator{
											Minimum: program.NumberLiteralExpression{Value: "1"},
											Maximum: program.NumberLiteralExpression{Value: "1000000"},
										},
									},
									program.SetOperation{
										Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "n"},
										Value:  program.ReferenceExpression{Name: "drawn"},
									},
									program.OpenQuestionOperation{
										Slot:      replayObservableSlot,
										Recipient: recipient,
										Arguments: []program.CallArgument{
											{Name: replayRandomArgName, Value: program.ReferenceExpression{Name: "drawn"}},
										},
									},
								}},
								Control: program.StayControl{},
							},
							{
								Name: "FirstAnswered",
								Signal: program.SignalPattern{
									Source:   program.QuestionAnsweredSignalSource{Slot: replayObservableSlot},
									Bindings: []program.SignalBinding{{Field: "answer", Name: "response"}},
								},
								Operations: program.Block{Operations: []program.Operation{
									program.SetOperation{
										Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "a"},
										Value:  program.ReferenceExpression{Name: "response"},
									},
									program.OpenQuestionOperation{
										Slot:      replayObservableSlot2,
										Recipient: recipient,
										Arguments: []program.CallArgument{
											{Name: replayRandomArgName, Value: program.ReferenceExpression{Name: "response"}},
										},
									},
								}},
								Control: program.StayControl{},
							},
							{
								Name: "SecondAnswered",
								Signal: program.SignalPattern{
									Source:   program.QuestionAnsweredSignalSource{Slot: replayObservableSlot2},
									Bindings: []program.SignalBinding{{Field: "answer", Name: "response2"}},
								},
								Operations: program.Block{Operations: []program.Operation{
									program.SetOperation{
										Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "b"},
										Value:  program.ReferenceExpression{Name: "response2"},
									},
								}},
								Control: program.StayControl{},
							},
						},
					},
				},
			},
		},
	}
}
