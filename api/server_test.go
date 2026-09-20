package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/api"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/management"
	sessionmigration "github.com/diegobermudez03/playhoot/game/session/migration"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle"
	"github.com/diegobermudez03/playhoot/play"
	"github.com/diegobermudez03/playhoot/play/sessionruntime"
	"github.com/diegobermudez03/playhoot/utils"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var apiTestDB = utils.DisposableTestDB{
	Key:        "session",
	NameSuffix: "api_pkg_test",
	Migrate: func(db *gorm.DB) error {
		return sessionmigration.Migrate(db)
	},
}

const (
	e2eQuestionSlot = "Q"
	e2eQuestionName = "PickNumber"
)

// e2eDefinition opens a Question at players[0] immediately at Start and
// closes it once answered - the minimum shape needed to prove the full
// live-transport path end to end.
func e2eDefinition() program.Definition {
	return program.Definition{
		Metadata:     program.Metadata{ID: "e2e", Name: "E2E"},
		RootWorkflow: "Main",
		Players:      program.PlayerPolicy{Min: 1, Max: 1},
		Questions: []program.QuestionDeclaration{
			{Name: e2eQuestionName, ResponseType: program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}},
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
					{Name: e2eQuestionSlot, Question: e2eQuestionName},
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
										Slot: e2eQuestionSlot,
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
								Signal: program.SignalPattern{Source: program.QuestionAnsweredSignalSource{Slot: e2eQuestionSlot}},
								Operations: program.Block{Operations: []program.Operation{
									program.CloseQuestionOperation{Slot: e2eQuestionSlot},
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

type e2eCurrentGameReader struct{ definition program.Definition }

func (r e2eCurrentGameReader) GetPlayableGameWithCurrentVersion(ctx context.Context, gameUUID string) (*management.Game, error) {
	return &management.Game{UUID: gameUUID, VersionUUID: uuid.NewString(), Definition: r.definition}, nil
}

type e2ePinnedGameReader struct{ definition program.Definition }

func (r e2ePinnedGameReader) GetGameDefinition(ctx context.Context, gameDefinitionUUID string) (*program.Definition, error) {
	return &r.definition, nil
}

// TestLiveTransport_CreateJoinStartAnswer_EndToEnd proves that a real
// client, over a real WebSocket (and real HTTP for Create/Join), can
// Create -> Join -> Start -> receive-the-opened-interaction -> answer ->
// receive-its-resolution against a real sessionlifecycle.Manager backed by
// real Postgres - the actual live path, not just a direct Go call.
func TestLiveTransport_CreateJoinStartAnswer_EndToEnd(t *testing.T) {
	db := apiTestDB.Open(t)
	definition := e2eDefinition()
	manager := sessionlifecycle.New(db, e2eCurrentGameReader{definition: definition}, e2ePinnedGameReader{definition: definition})
	sr := sessionruntime.New(manager, db)
	coord := play.NewCoordinator(sr)
	srv := api.NewServer(coord)

	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	hostUserUUID := uuid.NewString()

	// Create is plain HTTP request/response.
	createBody, err := json.Marshal(map[string]any{
		"game_uuid":       "game-1",
		"host_user_uuid":  hostUserUUID,
		"idempotency_key": "create-" + uuid.NewString(),
	})
	require.NoError(t, err)
	createResp, err := http.Post(ts.URL+"/sessions", "application/json", bytes.NewReader(createBody))
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	var created struct {
		SessionUUID string `json:"session_uuid"`
		JoinCode    uint   `json:"join_code"`
	}
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	require.NotEmpty(t, created.SessionUUID)

	// Join is plain HTTP request/response too.
	joinBody, err := json.Marshal(map[string]any{
		"join_code":       created.JoinCode,
		"user_uuid":       hostUserUUID,
		"display_name":    "Alice",
		"idempotency_key": "join-" + uuid.NewString(),
	})
	require.NoError(t, err)
	joinResp, err := http.Post(ts.URL+"/sessions/join", "application/json", bytes.NewReader(joinBody))
	require.NoError(t, err)
	defer joinResp.Body.Close()
	require.Equal(t, http.StatusOK, joinResp.StatusCode)
	var joined struct {
		Outcome string `json:"outcome"`
	}
	require.NoError(t, json.NewDecoder(joinResp.Body).Decode(&joined))
	require.Equal(t, "JOINED", joined.Outcome)

	// Connect over the live transport, binding this connection to
	// (session, user).
	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?session_uuid=" + created.SessionUUID + "&user_uuid=" + hostUserUUID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Start rides the WebSocket; its own committed RuntimeTurn opens the
	// Question, delivered live to this same connection.
	require.NoError(t, conn.WriteJSON(map[string]any{
		"type":            "START",
		"idempotency_key": "start-" + uuid.NewString(),
	}))

	startResult := readWSMessage(t, conn)
	require.Equal(t, "START_RESULT", startResult["type"])
	require.Equal(t, "STARTED", startResult["outcome"])

	opened := readWSMessage(t, conn)
	require.Equal(t, "INTERACTION_OPENED", opened["type"])
	require.Equal(t, e2eQuestionName, opened["question"])
	interactionID, _ := opened["interaction_id"].(string)
	require.NotEmpty(t, interactionID)

	// Answer rides the WebSocket too, encoded as the engine's own
	// EncodeValue wire format - the wire shape this server expects.
	answerBytes, err := engineservice.EncodeValue(engine.NumberValue{Value: 7})
	require.NoError(t, err)
	require.NoError(t, conn.WriteJSON(map[string]any{
		"type":           "ANSWER_INTERACTION",
		"interaction_id": interactionID,
		"answer":         json.RawMessage(answerBytes),
	}))

	answerResult := readWSMessage(t, conn)
	require.Equal(t, "ANSWER_RESULT", answerResult["type"])
	require.Equal(t, "ANSWERED", answerResult["outcome"])

	closed := readWSMessage(t, conn)
	require.Equal(t, "INTERACTION_CLOSED", closed["type"])
	require.Equal(t, interactionID, closed["interaction_id"])
}

func readWSMessage(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	var msg map[string]any
	require.NoError(t, conn.ReadJSON(&msg))
	return msg
}
