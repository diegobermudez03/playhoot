// Package session is the session workflow's route group: currently an
// HTTP/WebSocket transport skeleton only. It registers real endpoints
// with real connection-upgrade mechanics, but calls no domain package -
// there is nothing to call yet, so every handler answers a fixed
// placeholder response until a real dispatch layer exists behind it.
package session

import (
	"github.com/diegobermudez03/playhoot/api/internal/routing"
)

// Handler exposes the session workflow's endpoints. It holds no
// dependency today - handleCreateSession and handleWebSocket are
// currently stubs (see http.go/ws.go).
type Handler struct{}

// NewHandler constructs a Handler.
func NewHandler() *Handler {
	return &Handler{}
}

// RESTRoutes returns this group's ordinary request/response endpoints,
// satisfying api.routeGroup. It never touches an http.ServeMux and never
// starts its own request log - Server owns both (see api/server.go).
func (h *Handler) RESTRoutes() []routing.Route {
	return []routing.Route{
		{Pattern: "POST /sessions", Handler: h.handleCreateSession},
	}
}

// WebSocketRoutes returns this group's WebSocket endpoints, satisfying
// api.routeGroup.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return []routing.Route{
		{Pattern: "GET /ws", Handler: h.handleWebSocket},
	}
}
