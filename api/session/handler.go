// Package session is the session workflow's route group. It declares no
// endpoints yet: they will be added as the Session Runtime's real-time
// design is accepted.
package session

import "github.com/diegobermudez03/playhoot/api/internal/routing"

// Handler exposes the session workflow's endpoints.
type Handler struct{}

// NewHandler constructs a Handler.
func NewHandler() *Handler {
	return &Handler{}
}

// RESTRoutes returns this group's ordinary request/response endpoints,
// satisfying api.routeGroup. It never touches an http.ServeMux and never
// starts its own request log - Server owns both (see api/server.go).
func (h *Handler) RESTRoutes() []routing.Route {
	return nil
}

// WebSocketRoutes returns this group's WebSocket endpoints, satisfying
// api.routeGroup.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}
