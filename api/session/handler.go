// Package session is the session workflow's route group. POST /sessions
// calls SessionCreator (Orchestrator's CreateSession in production); GET
// /ws's join/message dispatch remains a transport skeleton (see ws.go's
// doc comment) until session-runtime-v1's own live-connection WORK lands.
package session

import (
	"context"

	"github.com/diegobermudez03/playhoot/api/internal/routing"
	sessionpkg "github.com/diegobermudez03/playhoot/session"
)

// SessionCreator is this route group's own narrow dependency on whatever
// coordinates Session creation - Orchestrator in production, satisfied
// structurally rather than imported directly so this package never needs
// to depend on orchestrator's own dependencies.
type SessionCreator interface {
	CreateSession(ctx context.Context, gameUUID sessionpkg.GameUUID, hostUserUUID sessionpkg.UserUUID, idempotencyKey sessionpkg.IdempotencyKey) (sessionpkg.CreatedSession, error)
}

// ContentAccessor is this route group's own narrow dependency on whatever
// grants a participant short-lived access to a Session's pinned frontend
// script and assets - Session Runtime's Manager in production. Both reads
// return a signed URL meant only for Playhoot's own trusted host frontend.
type ContentAccessor interface {
	GetFrontendScriptAccess(ctx context.Context, sessionUUID sessionpkg.SessionUUID, userUUID sessionpkg.UserUUID) (sessionpkg.ContentAccessResult, error)
	GetAssetAccess(ctx context.Context, sessionUUID sessionpkg.SessionUUID, userUUID sessionpkg.UserUUID, key string) (sessionpkg.ContentAccessResult, error)
}

// Handler exposes the session workflow's endpoints.
type Handler struct {
	creator SessionCreator
	content ContentAccessor
}

// NewHandler constructs a Handler backed by creator and content.
func NewHandler(creator SessionCreator, content ContentAccessor) *Handler {
	return &Handler{creator: creator, content: content}
}

// RESTRoutes returns this group's ordinary request/response endpoints,
// satisfying api.routeGroup. It never touches an http.ServeMux and never
// starts its own request log - Server owns both (see api/server.go).
func (h *Handler) RESTRoutes() []routing.Route {
	return []routing.Route{
		{Pattern: "POST /sessions", Handler: h.handleCreateSession},
		{Pattern: "GET /sessions/{session_uuid}/frontend-script", Handler: h.handleGetFrontendScript},
		{Pattern: "GET /sessions/{session_uuid}/assets/{key}", Handler: h.handleGetAsset},
	}
}

// WebSocketRoutes returns this group's WebSocket endpoints, satisfying
// api.routeGroup.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return []routing.Route{
		{Pattern: "GET /ws", Handler: h.handleWebSocket},
	}
}
