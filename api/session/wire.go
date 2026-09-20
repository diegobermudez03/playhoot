package session

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

// joinDeclineResponse is GET /ws's plain-HTTP wire shape for a Join call
// that completed but did not result in an active Participant (a
// deterministic decline, not an error) - the connection is never upgraded
// in that case, so the client learns why over ordinary HTTP instead of a
// WS message.
type joinDeclineResponse struct {
	Outcome string `json:"outcome"`
}

// Inbound WS message types - the client-to-server command envelope's
// "type" discriminator. Joining is the WS handshake itself (GET /ws), not
// a message ridden over an already-open connection; Start and
// AnswerInteraction are the only two.
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
	outboundTypeJoinResult        = "JOIN_RESULT"
	outboundTypeStartResult       = "START_RESULT"
	outboundTypeAnswerResult      = "ANSWER_RESULT"
	outboundTypeInteractionOpened = "INTERACTION_OPENED"
	outboundTypeInteractionClosed = "INTERACTION_CLOSED"
	outboundTypeError             = "ERROR"
)

// outboundMessage is the WS server-to-client envelope, covering both a
// direct command result (JoinResult/StartResult/AnswerResult) and an
// asynchronously fanned-out Event (InteractionOpened/InteractionClosed) -
// the client distinguishes them by Type alone, not by which request they
// answer. SessionUUID is only ever set on JoinResult, for a client that
// joined by join_code alone and does not already know it.
type outboundMessage struct {
	Type          string         `json:"type"`
	Outcome       string         `json:"outcome,omitempty"`
	SessionUUID   string         `json:"session_uuid,omitempty"`
	InteractionID string         `json:"interaction_id,omitempty"`
	Question      string         `json:"question,omitempty"`
	Arguments     map[string]any `json:"arguments,omitempty"`
	Message       string         `json:"message,omitempty"`
}
