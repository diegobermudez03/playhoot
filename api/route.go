package api

import (
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/routing"
)

// chain composes mws around final in declared order: mws[0] is the
// outermost wrapper, so it is the first to run on the way in and the last
// to run on the way out - reading the list top-to-bottom reads the same
// way a request actually passes through it.
func chain(mws []routing.Middleware, final http.HandlerFunc) http.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		final = mws[i](final)
	}
	return final
}
