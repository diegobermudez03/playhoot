// Package api is the external transport/application edge for the whole
// system: every user-facing HTTP/WebSocket endpoint lives here, decoding/
// encoding wire payloads and forwarding decoded commands to whichever
// capability owns them - a single domain directly, or a cross-domain
// coordination layer when a request spans more than one. It owns no
// business state and no connection registry of its own.
//
// Endpoints are organized into route groups by workflow/feature (the
// session workflow's group lives in api/session) rather than registered
// in one place, since this package is expected to keep accumulating
// endpoints as more workflows are exposed. A route group does not need a
// one-to-one relationship with a single business domain - a group may
// call more than one capability, or a cross-domain coordination layer -
// but each one is still registered and testable on its own. Composing
// every group, and constructing whatever each one depends on, is this
// package's own internal responsibility: a caller constructing a Server
// only supplies infrastructure every group shares (today, a database
// handle), never a pre-built group or the routing mechanism itself.
package api

import (
	"net/http"

	apisession "github.com/diegobermudez03/playhoot/api/session"
	"github.com/diegobermudez03/playhoot/play/sessionruntime"
	"gorm.io/gorm"
)

// routeGroup registers one workflow/feature's endpoints onto a shared mux.
type routeGroup interface {
	Register(mux *http.ServeMux)
}

// Server composes every route group into one HTTP handler.
type Server struct {
	mux *http.ServeMux
}

// NewServer constructs a Server exposing every endpoint every route group
// this package knows about registers, building each group's own
// dependency itself from db.
func NewServer(db *gorm.DB) *Server {
	coordinator := sessionruntime.NewCoordinator(db)

	mux := http.NewServeMux()
	groups := []routeGroup{
		apisession.NewHandler(coordinator),
	}
	for _, group := range groups {
		group.Register(mux)
	}
	return &Server{mux: mux}
}

// Routes returns Server's HTTP handler.
func (s *Server) Routes() http.Handler {
	return s.mux
}
