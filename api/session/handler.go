// Package session is the session workflow's route group: the HTTP/
// WebSocket endpoints for Create, Join, Start, and AnswerInteraction, and
// the fan-out delivered back over a live connection. It depends only on
// play's exported API, never on any `game` package.
package session

import (
	"context"
	"net/http"

	"github.com/diegobermudez03/playhoot/play"
)

// coordinatorAPI is the narrow play.Coordinator contract this package
// depends on - every type it exposes is play-owned/primitive, never a
// `game` type.
type coordinatorAPI interface {
	Create(ctx context.Context, gameUUID, hostUserUUID, idempotencyKey string) (play.CreatedSession, error)
	Join(ctx context.Context, joinCode uint, userUUID, displayName, idempotencyKey string) (play.JoinResult, error)
	Bind(sessionUUID play.SessionUUID, userUUID play.UserUUID, conn play.Conn) (unbind func())
	Start(ctx context.Context, sessionUUID play.SessionUUID, userUUID play.UserUUID, idempotencyKey string) (play.StartOutcome, []play.Event, error)
	AnswerInteraction(ctx context.Context, sessionUUID play.SessionUUID, interactionUUID play.InteractionUUID, userUUID play.UserUUID, answer []byte) (play.AnswerOutcome, []play.Event, error)
	Deliver(sessionUUID play.SessionUUID, events []play.Event)
}

// Handler exposes the session workflow's endpoints: Create and Join as
// ordinary HTTP request/response, and a WebSocket upgrade carrying Start/
// AnswerInteraction and their resulting fan-out.
type Handler struct {
	coord coordinatorAPI
}

// NewHandler constructs a Handler dispatching against coord.
func NewHandler(coord coordinatorAPI) *Handler {
	return &Handler{coord: coord}
}

// Register attaches every session endpoint to mux, satisfying
// api.RouteGroup.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /sessions", h.handleCreateSession)
	mux.HandleFunc("POST /sessions/join", h.handleJoinSession)
	mux.HandleFunc("GET /ws", h.handleWebSocket)
}
