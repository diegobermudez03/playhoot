package sessionlifecycle

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/management"
	"github.com/diegobermudez03/playhoot/game/session/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// numberAnswer encodes a bare engine.NumberValue in AnswerInteraction's own
// wire shape, for tests submitting a plain numeric response - callers of
// the real Manager never construct an engine.Value themselves.
func numberAnswer(t *testing.T, value float64) json.RawMessage {
	t.Helper()
	encoded, err := engineservice.EncodeValue(engine.NumberValue{Value: value})
	require.NoError(t, err)
	return encoded
}

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

// presentationEffectQuestionName/presentationEffectSlot/
// presentationEffectHudSlot/presentationEffectName name
// presentationEffectDefinition's own declarations.
const (
	presentationEffectQuestionName = "PickNumber"
	presentationEffectSlot         = "Q"
	presentationEffectHudSlot      = "hud"
	presentationEffectName         = "Celebrate"
)

// presentationEffectDefinition builds a real, engineservice.Compile-able
// Definition declaring the accepted `players: list<user>` root roster
// parameter, whose root workflow mounts a workflow-level presentation (slot
// "hud") for every player, projecting global "score", and opens a Question
// at players[0] immediately at Start. Answering it sets "score" to the
// answer (causing every mounted Hud presentation to recompute and report an
// UpdatePresentationOutput) and emits a client-facing effect addressed to
// every player - for tests that need a Definition whose committed Turns
// produce Effect/Presentation Outputs alongside an ordinary question.
func presentationEffectDefinition(playersMin, playersMax int) program.Definition {
	numberType := program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}
	return program.Definition{
		Metadata:     program.Metadata{ID: "presentation-effect", Name: "PresentationEffect"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		GlobalState: program.StateDeclaration{
			Fields: []program.StateFieldDeclaration{
				{Name: "score", Type: numberType, Initializer: program.NumberLiteralExpression{Value: "0"}},
			},
		},
		Questions: []program.QuestionDeclaration{
			{Name: presentationEffectQuestionName, ResponseType: numberType},
		},
		Effects: []program.EffectDeclaration{
			{Name: presentationEffectName},
		},
		PresentationSlots: []program.PresentationSlotDeclaration{{Name: presentationEffectHudSlot}},
		Projections: []program.ProjectionDeclaration{
			{Name: "Score", ResultType: numberType, Body: program.FieldExpression{Target: program.ReferenceExpression{Name: "global"}, Field: "score"}},
		},
		Views: []program.ViewDeclaration{
			{Name: "ScoreView", ModelType: numberType, Root: program.EmptyElement{}},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name: "Main",
				Parameters: []program.FieldDeclaration{
					{Name: "players", Type: program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}}},
				},
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				Presentations: []program.PresentationDeclaration{
					{Name: "Hud", Slot: presentationEffectHudSlot, Targets: program.ReferenceExpression{Name: "players"}, Projection: "Score", View: "ScoreView"},
				},
				QuestionSlots: []program.QuestionSlotDeclaration{
					{Name: presentationEffectSlot, Question: presentationEffectQuestionName},
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
										Slot: presentationEffectSlot,
										Recipient: program.IndexExpression{
											Target: program.ReferenceExpression{Name: "players"},
											Index:  program.NumberLiteralExpression{Value: "0"},
										},
									},
								}},
								Control: program.StayControl{},
							},
							{
								Name: "Answered",
								Signal: program.SignalPattern{
									Source:   program.QuestionAnsweredSignalSource{Slot: presentationEffectSlot},
									Bindings: []program.SignalBinding{{Field: "answer", Name: "response"}},
								},
								Operations: program.Block{Operations: []program.Operation{
									program.SetOperation{
										Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "score"},
										Value:  program.ReferenceExpression{Name: "response"},
									},
									program.EmitEffectOperation{
										Effect:     presentationEffectName,
										Recipients: program.ReferenceExpression{Name: "players"},
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

// terminationControlQuestionName/terminationControlSlot/
// terminationControlBystanderSlot name
// answerTriggersTerminationDefinition's own declarations.
const (
	terminationControlQuestionName  = "PickNumber"
	terminationControlSlot          = "Q1"
	terminationControlBystanderSlot = "Q2"
)

// answerTriggersTerminationDefinition builds a real, engineservice.Compile-able
// Definition declaring the accepted `players: list<user>` root roster
// parameter, whose root workflow opens a Question at players[0] (slot "Q1")
// and a second, independent Question at players[1] (slot "Q2", a bystander
// whose own interaction is never itself answered or explicitly closed) at
// Start, and whose "Q1 answered" transition applies control - never an
// ordinary CloseQuestionOperation - for tests proving Manager detects the
// resulting engine.RunCompletedOutput, terminalizes the Session, and closes
// the bystander's still-ACTIVE Q2 through terminal cleanup rather than
// ordinary gameplay closure.
func answerTriggersTerminationDefinition(control program.WorkflowControl, playersMin, playersMax int) program.Definition {
	recipient0 := program.IndexExpression{Target: program.ReferenceExpression{Name: "players"}, Index: program.NumberLiteralExpression{Value: "0"}}
	recipient1 := program.IndexExpression{Target: program.ReferenceExpression{Name: "players"}, Index: program.NumberLiteralExpression{Value: "1"}}
	return program.Definition{
		Metadata:     program.Metadata{ID: "answer-triggers-termination", Name: "AnswerTriggersTermination"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Questions: []program.QuestionDeclaration{
			{Name: terminationControlQuestionName, ResponseType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
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
					{Name: terminationControlSlot, Question: terminationControlQuestionName},
					{Name: terminationControlBystanderSlot, Question: terminationControlQuestionName},
				},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "Started",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.OpenQuestionOperation{Slot: terminationControlSlot, Recipient: recipient0},
									program.OpenQuestionOperation{Slot: terminationControlBystanderSlot, Recipient: recipient1},
								}},
								Control: program.StayControl{},
							},
							{
								Name:    "Answered",
								Signal:  program.SignalPattern{Source: program.QuestionAnsweredSignalSource{Slot: terminationControlSlot}},
								Control: control,
							},
						},
					},
				},
			},
		},
	}
}

// startTriggersTerminationDefinition builds a real, engineservice.Compile-able
// Definition declaring the accepted `players: list<user>` root roster
// parameter, whose root workflow applies control immediately on its very
// first (`WorkflowStarted`) transition, opening no question at all - for
// tests proving Manager.Start's own first RuntimeTurn detects an immediate
// engine.RunCompletedOutput and terminalizes the Session.
func startTriggersTerminationDefinition(control program.WorkflowControl, playersMin, playersMax int) program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "start-triggers-termination", Name: "StartTriggersTermination"},
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
							{
								Name:    "Started",
								Signal:  program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Control: control,
							},
						},
					},
				},
			},
		},
	}
}

// startedTerminationControlSession seeds a LOBBY Session with two active
// Participants (host plus one bystander), starts it against
// answerTriggersTerminationDefinition(control, ...) (opening Q1 for the
// host and Q2 for the bystander), and returns the resulting Manager, the
// Session's/host's identities, the primary (host-answerable) interaction's
// public UUID, and the bystander's own interaction UUID - the common setup
// every termination-control AnswerInteraction test below builds on.
func startedTerminationControlSession(t *testing.T, db *gorm.DB, control program.WorkflowControl) (m *Manager, sessionUUID SessionUUID, hostUUID UserUUID, primaryInteractionUUID, bystanderInteractionUUID InteractionUUID) {
	t.Helper()

	m = New(db, nil, stubStartPinnedGameReader{definition: answerTriggersTerminationDefinition(control, 2, 4)})

	fx := testfixtures.SeedLobbySession(t, db, time.Now().Add(10*time.Minute))
	hostUUIDStr := uuid.NewString()
	hostActorID := testfixtures.SeedActor(t, db, fx.SessionID, hostUUIDStr)
	require.NoError(t, db.Exec(`UPDATE sessions SET host_actor_id = ? WHERE id = ?`, hostActorID, fx.SessionID).Error)
	testfixtures.SeedParticipantForActor(t, db, hostActorID, "Host")
	testfixtures.SeedActiveParticipant(t, db, fx.SessionID, uuid.NewString(), "Bystander")

	startResult, err := m.Start(context.Background(), SessionUUID(fx.SessionUUID), UserUUID(hostUUIDStr), IdempotencyKey(uuid.NewString()))
	require.NoError(t, err)
	require.Equal(t, StartOutcomeStarted, startResult.Outcome)

	var primaryUUIDStr, bystanderUUIDStr string
	require.NoError(t, db.Raw(`SELECT uuid FROM session_interactions WHERE session_id = ? AND session_actor_id = ?`, fx.SessionID, hostActorID).Scan(&primaryUUIDStr).Error)
	require.NotEmpty(t, primaryUUIDStr, "Start's own first Turn must open Q1 for the host")
	require.NoError(t, db.Raw(`SELECT uuid FROM session_interactions WHERE session_id = ? AND session_actor_id != ?`, fx.SessionID, hostActorID).Scan(&bystanderUUIDStr).Error)
	require.NotEmpty(t, bystanderUUIDStr, "Start's own first Turn must open Q2 for the bystander")

	return m, SessionUUID(fx.SessionUUID), UserUUID(hostUUIDStr), InteractionUUID(primaryUUIDStr), InteractionUUID(bystanderUUIDStr)
}

// replayObservableQuestionName/replayObservableSlot/replayObservableSlot2/
// replayRandomArgName name replayObservableDefinition's own declarations.
const (
	replayObservableQuestionName = "PickNumber"
	replayObservableSlot         = "Q1"
	replayObservableSlot2        = "Q2"
	replayObservableSlot3        = "Q3"
	replayRandomArgName          = "n"
)

// replayObservableDefinition builds a real, engineservice.Compile-able
// Definition designed specifically so a test can verify AdvanceTurn's
// internal replay against values the live execution actually produced,
// rather than against values re-derived from the same durable rows replay
// itself reads (which would only prove replay is consistent with itself,
// not that it matches live execution) - entirely through Outputs, since
// engineservice never hands a caller an engine.Snapshot to inspect
// directly.
//
// At Start it draws a random number and exposes it as the first opened
// question's own "n" argument - captured live into
// session_interactions.interaction_payload through the ordinary
// OpenQuestionOutput capture path, a channel entirely independent of
// session_runtime_starts.seed. Once that question is answered, the
// response is stored into global state ("a") and a second question is
// opened, exposing "a" as its own argument. Once that one is answered too,
// its response is stored into global state ("b") and a third question is
// opened, exposing "b" as its own argument - the only way a test can
// observe "b" without reading global state directly, and only reachable
// if AdvanceTurn's internal replay correctly threaded the second Turn's
// answer through first.
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
				{Name: "c", Type: numberType, Initializer: program.NumberLiteralExpression{Value: "0"}},
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
					{Name: replayObservableSlot3, Question: replayObservableQuestionName},
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
									program.OpenQuestionOperation{
										Slot:      replayObservableSlot3,
										Recipient: recipient,
										Arguments: []program.CallArgument{
											{Name: replayRandomArgName, Value: program.ReferenceExpression{Name: "response2"}},
										},
									},
								}},
								Control: program.StayControl{},
							},
							{
								Name: "ThirdAnswered",
								Signal: program.SignalPattern{
									Source:   program.QuestionAnsweredSignalSource{Slot: replayObservableSlot3},
									Bindings: []program.SignalBinding{{Field: "answer", Name: "response3"}},
								},
								Operations: program.Block{Operations: []program.Operation{
									program.SetOperation{
										Target: program.FieldTarget{Target: program.NameTarget{Name: "global"}, Field: "c"},
										Value:  program.ReferenceExpression{Name: "response3"},
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

// timerSlotName/timerEffectName name timerDefinition's own declarations.
const (
	timerSlotName   = "T"
	timerEffectName = "TimerFired"
)

// timerDefinition builds a real, engineservice.Compile-able Definition
// declaring the accepted `players: list<user>` root roster parameter, whose
// root workflow schedules an ordinary TimerSlot ("T", delayMilliseconds)
// immediately at Start and emits a client-facing effect for every player
// once it expires - for tests proving a scheduled timer obligation is
// durably persisted, and its expiration (Manager.ExpireTimer) drives a new
// RuntimeTurn.
func timerDefinition(playersMin, playersMax int, delayMilliseconds int) program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "timer", Name: "Timer"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Effects:      []program.EffectDeclaration{{Name: timerEffectName}},
		Workflows: []program.WorkflowDeclaration{
			{
				Name: "Main",
				Parameters: []program.FieldDeclaration{
					{Name: "players", Type: program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}}},
				},
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				TimerSlots:   []program.TimerSlotDeclaration{{Name: timerSlotName}},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "Started",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.ScheduleTimerOperation{
										Slot:              timerSlotName,
										DelayMilliseconds: program.NumberLiteralExpression{Value: strconv.Itoa(delayMilliseconds)},
									},
								}},
								Control: program.StayControl{},
							},
							{
								Name:   "TimerFired",
								Signal: program.SignalPattern{Source: program.TimerExpiredSignalSource{Slot: timerSlotName}},
								Operations: program.Block{Operations: []program.Operation{
									program.EmitEffectOperation{
										Effect:     timerEffectName,
										Recipients: program.ReferenceExpression{Name: "players"},
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

// timerCancelledOnAnswerDefinition builds a real, engineservice.Compile-able
// Definition whose root workflow schedules TimerSlot "T" and opens Question
// "Q" (reusing answerableQuestionName/answerableSlot) both at Start;
// answering Q explicitly cancels "T" via CancelTimerOperation instead of
// letting it expire - for tests proving a cancelled timer obligation is
// durably transitioned to CANCELLED and no longer expirable.
func timerCancelledOnAnswerDefinition(playersMin, playersMax int) program.Definition {
	recipient := program.IndexExpression{Target: program.ReferenceExpression{Name: "players"}, Index: program.NumberLiteralExpression{Value: "0"}}
	return program.Definition{
		Metadata:     program.Metadata{ID: "timer-cancelled-on-answer", Name: "TimerCancelledOnAnswer"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Questions: []program.QuestionDeclaration{
			{Name: answerableQuestionName, ResponseType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name: "Main",
				Parameters: []program.FieldDeclaration{
					{Name: "players", Type: program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}}},
				},
				ResultType:    program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState:  "Start",
				TimerSlots:    []program.TimerSlotDeclaration{{Name: timerSlotName}},
				QuestionSlots: []program.QuestionSlotDeclaration{{Name: answerableSlot, Question: answerableQuestionName}},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "Started",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.ScheduleTimerOperation{Slot: timerSlotName, DelayMilliseconds: program.NumberLiteralExpression{Value: "5000"}},
									program.OpenQuestionOperation{Slot: answerableSlot, Recipient: recipient},
								}},
								Control: program.StayControl{},
							},
							{
								Name:   "Answered",
								Signal: program.SignalPattern{Source: program.QuestionAnsweredSignalSource{Slot: answerableSlot}},
								Operations: program.Block{Operations: []program.Operation{
									program.CloseQuestionOperation{Slot: answerableSlot},
									program.CancelTimerOperation{Slot: timerSlotName},
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

// timerActiveAtTerminationDefinition builds a real, engineservice.Compile-able
// Definition whose root workflow schedules TimerSlot "T" and opens Question
// "Q" (reusing answerableQuestionName/answerableSlot) both at Start;
// answering Q applies control (terminating the game) without ever cancelling
// "T" - for tests proving terminal cleanup cancels a still-ACTIVE timer
// obligation exactly like it already does for a still-ACTIVE interaction, so
// a TERMINAL Session never retains an obligation that could still fire.
func timerActiveAtTerminationDefinition(control program.WorkflowControl, playersMin, playersMax int) program.Definition {
	recipient := program.IndexExpression{Target: program.ReferenceExpression{Name: "players"}, Index: program.NumberLiteralExpression{Value: "0"}}
	return program.Definition{
		Metadata:     program.Metadata{ID: "timer-active-at-termination", Name: "TimerActiveAtTermination"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Questions: []program.QuestionDeclaration{
			{Name: answerableQuestionName, ResponseType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name: "Main",
				Parameters: []program.FieldDeclaration{
					{Name: "players", Type: program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}}},
				},
				ResultType:    program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState:  "Start",
				TimerSlots:    []program.TimerSlotDeclaration{{Name: timerSlotName}},
				QuestionSlots: []program.QuestionSlotDeclaration{{Name: answerableSlot, Question: answerableQuestionName}},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "Started",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.ScheduleTimerOperation{Slot: timerSlotName, DelayMilliseconds: program.NumberLiteralExpression{Value: "5000"}},
									program.OpenQuestionOperation{Slot: answerableSlot, Recipient: recipient},
								}},
								Control: program.StayControl{},
							},
							{
								Name:    "Answered",
								Signal:  program.SignalPattern{Source: program.QuestionAnsweredSignalSource{Slot: answerableSlot}},
								Control: control,
							},
						},
					},
				},
			},
		},
	}
}

// doubleScheduleTimerDefinition builds a real, engineservice.Compile-able
// Definition whose root workflow schedules TimerSlot "T" twice into the same
// slot within one transition - the engine's own atomic occupied-slot
// execution error (ScheduleTimerOperation's documented contract) - for tests
// proving the whole Start Turn fails atomically and no
// session_timer_obligations row is left behind.
func doubleScheduleTimerDefinition(playersMin, playersMax int) program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "double-schedule-timer", Name: "DoubleScheduleTimer"},
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
				TimerSlots:   []program.TimerSlotDeclaration{{Name: timerSlotName}},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "Started",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.ScheduleTimerOperation{Slot: timerSlotName, DelayMilliseconds: program.NumberLiteralExpression{Value: "1000"}},
									program.ScheduleTimerOperation{Slot: timerSlotName, DelayMilliseconds: program.NumberLiteralExpression{Value: "2000"}},
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

// keyedTimerSlotName/keyedTimerKeyArgName/keyedTimerEffectName name
// keyedTimerDefinition's own declarations.
const (
	keyedTimerSlotName   = "KT"
	keyedTimerKeyArgName = "key"
	keyedTimerEffectName = "KeyedTimerFired"
)

// keyedTimerDefinition builds a real, engineservice.Compile-able Definition
// whose root workflow schedules a string-keyed KeyedTimerSlot ("KT")
// independently for two keys ("P0", "P1") at Start, and emits a
// client-facing effect carrying which key expired once a keyed timer fires -
// for tests proving two independent keyed timer obligations coexist under
// the same slot and each expiration threads its own authored key correctly
// through persistence and replay.
func keyedTimerDefinition(playersMin, playersMax int) program.Definition {
	stringType := program.BuiltinTypeReference{Type: program.BuiltinTypeString}
	return program.Definition{
		Metadata:     program.Metadata{ID: "keyed-timer", Name: "KeyedTimer"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: playersMin, Max: playersMax},
		Effects: []program.EffectDeclaration{
			{Name: keyedTimerEffectName, Parameters: []program.FieldDeclaration{{Name: keyedTimerKeyArgName, Type: stringType}}},
		},
		Workflows: []program.WorkflowDeclaration{
			{
				Name: "Main",
				Parameters: []program.FieldDeclaration{
					{Name: "players", Type: program.ListTypeReference{Element: program.BuiltinTypeReference{Type: program.BuiltinTypeUser}}},
				},
				ResultType:      program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState:    "Start",
				KeyedTimerSlots: []program.KeyedTimerSlotDeclaration{{Name: keyedTimerSlotName, KeyType: stringType}},
				States: []program.WorkflowStateDeclaration{
					{
						Name: "Start",
						Transitions: []program.TransitionDeclaration{
							{
								Name:   "Started",
								Signal: program.SignalPattern{Source: program.NamedSignalSource{Name: "WorkflowStarted"}},
								Operations: program.Block{Operations: []program.Operation{
									program.ScheduleKeyedTimerOperation{
										Slot:              keyedTimerSlotName,
										Key:               program.StringLiteralExpression{Value: "P0"},
										DelayMilliseconds: program.NumberLiteralExpression{Value: "5000"},
									},
									program.ScheduleKeyedTimerOperation{
										Slot:              keyedTimerSlotName,
										Key:               program.StringLiteralExpression{Value: "P1"},
										DelayMilliseconds: program.NumberLiteralExpression{Value: "5000"},
									},
								}},
								Control: program.StayControl{},
							},
							{
								Name: "KeyedTimerFired",
								Signal: program.SignalPattern{
									Source:   program.KeyedTimerExpiredSignalSource{Slot: keyedTimerSlotName},
									Bindings: []program.SignalBinding{{Field: "key", Name: "k"}},
								},
								Operations: program.Block{Operations: []program.Operation{
									program.EmitEffectOperation{
										Effect:     keyedTimerEffectName,
										Recipients: program.ReferenceExpression{Name: "players"},
										Arguments: []program.CallArgument{
											{Name: keyedTimerKeyArgName, Value: program.ReferenceExpression{Name: "k"}},
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
	}
}
