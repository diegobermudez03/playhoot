// Package routing declares the shared vocabulary a route group (e.g.
// api/session) and Server (api/server.go) both need to hand endpoints
// back and forth without either importing the other - a route group
// lives under api/, and api itself composes every route group, so
// neither can import the other directly without a cycle.
package routing

import "net/http"

// Middleware wraps a handler, producing a new handler that may run
// behavior before and/or after calling it.
type Middleware func(http.HandlerFunc) http.HandlerFunc

// Route is one endpoint a route group exposes: its http.ServeMux pattern, its
// handler and its route-specific middlewares. Middlewares every route needs
// are applied by Server, not declared here.
type Route struct {
	Pattern     string
	Handler     http.HandlerFunc
	Middlewares []Middleware
}
