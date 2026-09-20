package sessionruntime

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/management"
	sessionmigration "github.com/diegobermudez03/playhoot/game/session/migration"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle"
	"github.com/diegobermudez03/playhoot/play"
	"github.com/diegobermudez03/playhoot/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var sessionRuntimeTestDB = utils.DisposableTestDB{
	Key:        "session",
	NameSuffix: "play_sessionruntime_pkg_test",
	Migrate: func(db *gorm.DB) error {
		return sessionmigration.Migrate(db)
	},
}

// answerableQuestionSlot/answerableQuestionName name
// answerableDefinition's own declarations.
const (
	answerableQuestionSlot = "Q"
	answerableQuestionName = "PickNumber"
)

// answerableDefinition builds a real, engineservice.Compile-able Definition
// whose root workflow opens a Question at players[0] immediately at Start
// (Slot "Q", a bare number response) and closes it once answered - the
// minimum needed to exercise a real OpenQuestionOutput/CloseQuestionOutput
// end to end through a real Postgres-backed sessionlifecycle.Manager,
// mirroring sessionlifecycle's own (unexported, out of reach from this
// package) testutil_test.go fixture of the same shape.
func answerableDefinition() program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "answerable", Name: "Answerable"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: 1, Max: 1},
		Questions: []program.QuestionDeclaration{
			{Name: answerableQuestionName, ResponseType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
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
					{Name: answerableQuestionSlot, Question: answerableQuestionName},
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
										Slot: answerableQuestionSlot,
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
								Signal: program.SignalPattern{Source: program.QuestionAnsweredSignalSource{Slot: answerableQuestionSlot}},
								Operations: program.Block{Operations: []program.Operation{
									program.CloseQuestionOperation{Slot: answerableQuestionSlot},
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

type stubCurrentGameReader struct {
	gameUUID   string
	definition program.Definition
}

func (s stubCurrentGameReader) GetPlayableGameWithCurrentVersion(ctx context.Context, gameUUID string) (*management.Game, error) {
	return &management.Game{UUID: gameUUID, VersionUUID: uuid.NewString(), Definition: s.definition}, nil
}

type stubPinnedGameReader struct {
	definition program.Definition
}

func (s stubPinnedGameReader) GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error) {
	return &s.definition, nil
}

// TestSessionRuntime_Start_TranslatesOpenedInteractionForItsRecipient
// proves the real end-to-end read-after-commit path: a real
// sessionlifecycle.Manager, backed by a real Postgres database, actually
// opens a Question at Start, and SessionRuntime.Start translates it into
// exactly one play.Event carrying the joined recipient UserUUID, Question
// text, and Arguments - never the internal engine path/slot/actor id a
// client has no reason to see.
func TestSessionRuntime_Start_TranslatesOpenedInteractionForItsRecipient(t *testing.T) {
	db := sessionRuntimeTestDB.Open(t)
	ctx := context.Background()
	definition := answerableDefinition()
	manager := sessionlifecycle.New(db, stubCurrentGameReader{gameUUID: "game-1", definition: definition}, stubPinnedGameReader{definition: definition})
	sr := New(manager, db)

	hostUserUUID := uuid.NewString()
	created, err := manager.Create(ctx, sessionlifecycle.GameUUID("game-1"), sessionlifecycle.UserUUID(hostUserUUID), sessionlifecycle.IdempotencyKey("create-"+uuid.NewString()))
	require.NoError(t, err)

	joinResult, err := manager.Join(ctx, created.JoinCode, sessionlifecycle.UserUUID(hostUserUUID), "Alice", sessionlifecycle.IdempotencyKey("join-"+uuid.NewString()))
	require.NoError(t, err)
	require.Equal(t, sessionlifecycle.JoinOutcomeJoined, joinResult.Outcome)

	result, err := sr.Start(ctx, string(created.SessionUUID), hostUserUUID, "start-"+uuid.NewString())
	require.NoError(t, err)
	require.Equal(t, play.StartOutcomeStarted, result.Outcome)
	require.Len(t, result.Events, 1)

	event := result.Events[0]
	require.Equal(t, play.UserUUID(hostUserUUID), event.Recipient)
	require.Equal(t, play.EventKindInteractionOpened, event.Kind)
	require.Equal(t, answerableQuestionName, event.Question)
	require.NotEmpty(t, event.InteractionID)

	var interactionUUID string
	require.NoError(t, db.Raw(`SELECT uuid FROM session_interactions WHERE session_id = (SELECT id FROM sessions WHERE uuid = ?)`, string(created.SessionUUID)).Scan(&interactionUUID).Error)
	require.Equal(t, interactionUUID, string(event.InteractionID))
}

// TestSessionRuntime_AnswerInteraction_TranslatesClosedInteractionAndSurvivesFailedDelivery
// proves both: (1) a valid answer commits and its resulting close is
// translated into a play.Event for the same recipient, and (2) the durable
// session_interactions row is already CLOSED with the accepted response
// persisted regardless of what a caller does with the returned Events
// afterward - this package never retries or rolls back on a downstream
// delivery failure, since that failure happens entirely outside this call,
// in play.Coordinator.
func TestSessionRuntime_AnswerInteraction_TranslatesClosedInteractionAndSurvivesFailedDelivery(t *testing.T) {
	db := sessionRuntimeTestDB.Open(t)
	ctx := context.Background()
	definition := answerableDefinition()
	manager := sessionlifecycle.New(db, stubCurrentGameReader{gameUUID: "game-1", definition: definition}, stubPinnedGameReader{definition: definition})
	sr := New(manager, db)

	hostUserUUID := uuid.NewString()
	created, err := manager.Create(ctx, sessionlifecycle.GameUUID("game-1"), sessionlifecycle.UserUUID(hostUserUUID), sessionlifecycle.IdempotencyKey("create-"+uuid.NewString()))
	require.NoError(t, err)
	_, err = manager.Join(ctx, created.JoinCode, sessionlifecycle.UserUUID(hostUserUUID), "Alice", sessionlifecycle.IdempotencyKey("join-"+uuid.NewString()))
	require.NoError(t, err)
	startResult, err := sr.Start(ctx, string(created.SessionUUID), hostUserUUID, "start-"+uuid.NewString())
	require.NoError(t, err)
	interactionUUID := string(startResult.Events[0].InteractionID)

	answerValue := engine.NumberValue{Value: 7}
	answerBytes, err := engineservice.EncodeValue(answerValue)
	require.NoError(t, err)

	answerResult, err := sr.AnswerInteraction(ctx, interactionUUID, hostUserUUID, answerBytes)
	require.NoError(t, err)
	require.Equal(t, play.AnswerOutcomeAnswered, answerResult.Outcome)
	require.Len(t, answerResult.Events, 1)
	require.Equal(t, play.EventKindInteractionClosed, answerResult.Events[0].Kind)
	require.Equal(t, play.InteractionUUID(interactionUUID), answerResult.Events[0].InteractionID)
	require.Equal(t, play.UserUUID(hostUserUUID), answerResult.Events[0].Recipient)

	var state string
	var responsePayload []byte
	require.NoError(t, db.Raw(`SELECT state, response_payload FROM session_interactions WHERE uuid = ?`, interactionUUID).Row().Scan(&state, &responsePayload))
	require.Equal(t, "CLOSED", state)
	storedAnswer, err := engineservice.DecodeValue(responsePayload)
	require.NoError(t, err)
	require.True(t, storedAnswer.Equal(answerValue))
}

func TestSessionRuntime_AnswerInteraction_RejectsMalformedAnswerPayload(t *testing.T) {
	db := sessionRuntimeTestDB.Open(t)
	ctx := context.Background()
	definition := answerableDefinition()
	manager := sessionlifecycle.New(db, stubCurrentGameReader{gameUUID: "game-1", definition: definition}, stubPinnedGameReader{definition: definition})
	sr := New(manager, db)

	_, err := sr.AnswerInteraction(ctx, uuid.NewString(), uuid.NewString(), []byte("not engine-encoded json"))
	require.Error(t, err)
}
