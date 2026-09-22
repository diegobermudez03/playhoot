package api

import (
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/routing"
)

// Route and Middleware are this package's own names for
// api/internal/routing's shared vocabulary - see that package's doc
// comment for why a route group and Server share these types through it
// instead of one importing the other directly.
type Route = routing.Route
type Middleware = routing.Middleware

// chain composes mws around final in declared order: mws[0] is the
// outermost wrapper, so it is the first to run on the way in and the last
// to run on the way out - reading the list top-to-bottom reads the same
// way a request actually passes through it.
func chain(mws []Middleware, final http.HandlerFunc) http.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		final = mws[i](final)
	}
	return final
}
