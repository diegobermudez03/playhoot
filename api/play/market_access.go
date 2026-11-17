package play

import (
	"errors"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/geolocation"
	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/orchestrator/marketgate"
)

// rejectMarketRestricted is the reason a join is refused by the market
// matrix, on the wire: the service does not offer playing where the person is.
const rejectMarketRestricted = "market_restricted"

// requireMarketAccess lets a live request through unless playing is
// restricted where the caller is. Joining asks nothing: without a declaration
// only the approximate country speaks for them, and where it is unknown the
// gate is open. It runs before the Session is named, so a refusal reveals
// nothing about it.
func requireMarketAccess(gate *marketgate.Gate, geo geolocation.Resolver, refuse joinRefuser) routing.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			userUUID := ""
			if caller, ok := auth.Caller(ctx); ok {
				userUUID = caller.UUID
			}
			err := gate.Admit(ctx, userUUID, compliance.FunctionInteractiveUse, geo.Country(ctx))
			if err == nil {
				next(w, r)
				return
			}
			var restricted *marketgate.RestrictedError
			if errors.As(err, &restricted) {
				logging.LogFields(ctx, logging.Field("market_access_outcome", rejectMarketRestricted))
				refuse(w, r, rejectedMessage{Reason: rejectMarketRestricted})
				return
			}
			logging.LogError(ctx, err)
			httpx.WriteError(w, http.StatusInternalServerError, "checking market access failed")
		}
	}
}
