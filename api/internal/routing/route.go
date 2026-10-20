// Package routing is the route vocabulary route groups and Server share
// without importing each other.
package routing

import (
	"net/http"

	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// Middleware wraps a handler, producing a new handler that may run
// behavior before and/or after calling it.
type Middleware func(http.HandlerFunc) http.HandlerFunc

// Route is one endpoint a route group exposes: its http.ServeMux pattern, its
// handler, who may call it, how often, and its route-specific middlewares.
// Middlewares every route needs are applied by Server, not declared here.
type Route struct {
	Pattern string
	Handler http.HandlerFunc
	// Authorization, when set, decides who may call the route; it runs before
	// rate limiting, so an authorized caller is limited as a user.
	Authorization Middleware
	// Limits replaces Server's default request limits for this route.
	Limits      *RequestLimits
	Middlewares []Middleware
}

// RequestLimits bound how often one caller may call a route. Every limit in a
// list applies at once, such as at most 5 a second and 30 a minute.
type RequestLimits struct {
	// PerUser bound an authorized caller, by UserUUID.
	PerUser []ratelimit.Limit
	// PerClient bound a caller the route did not authorize, by client address.
	PerClient []ratelimit.Limit
}
