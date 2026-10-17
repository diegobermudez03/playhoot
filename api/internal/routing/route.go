// Package routing is the route vocabulary route groups and Server share
// without importing each other.
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
