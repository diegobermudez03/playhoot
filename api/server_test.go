package api_test

import (
	"bytes"
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
	"github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
	managementmigration "github.com/diegobermudez03/playhoot/game/management/migration"
	sessionmigration "github.com/diegobermudez03/playhoot/game/session/migration"
	"github.com/diegobermudez03/playhoot/utils"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var apiTestDB = utils.DisposableTestDB{
	Key:        "api",
	NameSuffix: "api_pkg_test",
	Migrate: func(db *gorm.DB) error {
		if err := managementmigration.Migrate(db); err != nil {
			return err
		}
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

type gameSeedInsert struct {
	ID                  uint   `gorm:"column:id"`
	UUID                string `gorm:"column:uuid"`
	Name                string `gorm:"column:name"`
	Description         string `gorm:"column:description"`
	OwnerUUID           string `gorm:"column:owner_uuid"`
	CurrentDefinitionID *uint  `gorm:"column:current_definition_id"`
	LogoImageURL        string `gorm:"column:logo_image_url"`
	Visibility          string `gorm:"column:visibility"`
}

func (gameSeedInsert) TableName() string { return "games" }

type gameDefinitionSeedInsert struct {
	ID            uint   `gorm:"column:id"`
	UUID          string `gorm:"column:uuid"`
	GameID        uint   `gorm:"column:game_id"`
	VersionNumber uint   `gorm:"column:version_number"`
	Script        []byte `gorm:"column:script"`
}

func (gameDefinitionSeedInsert) TableName() string { return "game_definitions" }

// seedGame inserts a playable games/game_definitions pair for definition
// and returns the game's public UUID - the one piece of Game Management
// state api.NewServer(db)'s own real getgame/getgamedefinition readers
// need to resolve Create/Start against, now that api builds them itself
// from db rather than accepting stub readers from its caller.
func seedGame(t *testing.T, db *gorm.DB, definition program.Definition) string {
	t.Helper()

	script, err := gameservice.EncodeJSON(definition)
	require.NoError(t, err)

	game := gameSeedInsert{
		UUID:         uuid.NewString(),
		Name:         "E2E",
		Description:  "End-to-end live transport test game",
		OwnerUUID:    uuid.NewString(),
		LogoImageURL: "https://example.com/logo.png",
		Visibility:   "public",
	}
	require.NoError(t, db.Create(&game).Error)

	def := gameDefinitionSeedInsert{
		UUID:          uuid.NewString(),
		GameID:        game.ID,
		VersionNumber: 1,
		Script:        script,
	}
	require.NoError(t, db.Create(&def).Error)

	require.NoError(t, db.Exec(`UPDATE games SET current_definition_id = ? WHERE id = ?`, def.ID, game.ID).Error)

	return game.UUID
}

// TestLiveTransport_CreateJoinStartAnswer_EndToEnd proves that a real
// client, over a real WebSocket (and real HTTP for Create/Join), can
// Create -> Join -> Start -> receive-the-opened-interaction -> answer ->
// receive-its-resolution against api.NewServer's own real, fully-wired
// stack - the actual live path, not just a direct Go call.
func TestLiveTransport_CreateJoinStartAnswer_EndToEnd(t *testing.T) {
	db := apiTestDB.Open(t)
	gameUUID := seedGame(t, db, e2eDefinition())

	srv := api.NewServer(db)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	hostUserUUID := uuid.NewString()

	// Create is plain HTTP request/response.
	createBody, err := json.Marshal(map[string]any{
		"game_uuid":       gameUUID,
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
