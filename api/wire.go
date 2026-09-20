package api

import (
	"encoding/json"
	"time"
)

// createSessionRequest/createSessionResponse are POST /sessions's wire
// shapes.
type createSessionRequest struct {
	GameUUID       string `json:"game_uuid"`
	HostUserUUID   string `json:"host_user_uuid"`
	IdempotencyKey string `json:"idempotency_key"`
}

type createSessionResponse struct {
	SessionUUID    string    `json:"session_uuid"`
	JoinCode       uint      `json:"join_code"`
	LobbyExpiresAt time.Time `json:"lobby_expires_at"`
}

// joinSessionRequest/joinSessionResponse are POST /sessions/join's wire
// shapes.
type joinSessionRequest struct {
	JoinCode       uint   `json:"join_code"`
	UserUUID       string `json:"user_uuid"`
	DisplayName    string `json:"display_name"`
	IdempotencyKey string `json:"idempotency_key"`
}

type joinSessionResponse struct {
	Outcome     string `json:"outcome"`
	SessionUUID string `json:"session_uuid,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// errorResponse is every HTTP handler's error wire shape.
type errorResponse struct {
	Message string `json:"message"`
}

// Inbound WS message types - the client-to-server command envelope's
// "type" discriminator. Only Start and AnswerInteraction ride the
// WebSocket; Create/Join stay ordinary HTTP.
const (
	inboundTypeStart             = "START"
	inboundTypeAnswerInteraction = "ANSWER_INTERACTION"
)

// inboundMessage is the WS client-to-server command envelope. Answer is
// the engine's own EncodeValue wire format, opaque to this package -
// forwarded as-is to play.Coordinator/the SessionRuntime implementation,
// exactly like every other transport-level payload's decoding being the
// caller's responsibility.
type inboundMessage struct {
	Type           string          `json:"type"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	InteractionID  string          `json:"interaction_id,omitempty"`
	Answer         json.RawMessage `json:"answer,omitempty"`
}

// Outbound WS message types - the server-to-client envelope's "type"
// discriminator.
const (
	outboundTypeStartResult       = "START_RESULT"
	outboundTypeAnswerResult      = "ANSWER_RESULT"
	outboundTypeInteractionOpened = "INTERACTION_OPENED"
	outboundTypeInteractionClosed = "INTERACTION_CLOSED"
	outboundTypeError             = "ERROR"
)

// outboundMessage is the WS server-to-client envelope, covering both a
// direct command result (StartResult/AnswerResult) and an asynchronously
// fanned-out Event (InteractionOpened/InteractionClosed) - the client
// distinguishes them by Type alone, not by which request they answer.
type outboundMessage struct {
	Type          string         `json:"type"`
	Outcome       string         `json:"outcome,omitempty"`
	InteractionID string         `json:"interaction_id,omitempty"`
	Question      string         `json:"question,omitempty"`
	Arguments     map[string]any `json:"arguments,omitempty"`
	Message       string         `json:"message,omitempty"`
}
