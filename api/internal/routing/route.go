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

// Route is one endpoint a route group exposes: the http.ServeMux pattern
// it registers under (e.g. "POST /sessions"), the handler itself, and any
// middlewares specific to this one route (an authorization check some
// endpoints need and others don't, a rate limiter, and so on). A
// middleware every endpoint needs regardless of route group - today,
// observability - is not declared here; Server applies it to every route
// uniformly.
type Route struct {
	Pattern     string
	Handler     http.HandlerFunc
	Middlewares []Middleware
}
