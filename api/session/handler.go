// Package session is the session workflow's route group: currently an
// HTTP/WebSocket transport skeleton only. It registers the same endpoint
// shapes and keeps the same connection-upgrade mechanics already proven
// out, but calls no domain package - there is nothing to call yet.
//
// This is deliberate, not an oversight: a prior pass built a stateful
// coordinator here on top of Session Runtime output-handling that was
// both incomplete and already known to change. Rather than keep building
// on that foundation, this package was reduced back to its transport
// plumbing until Session Runtime's own engine-Output handling is actually
// complete, at which point a real dispatch layer is rebuilt on top of it
// once, correctly.
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
