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

// Reasons a join is refused by the market matrix, on the wire.
const (
	// rejectMarketRestricted means the service does not offer playing where
	// the person is.
	rejectMarketRestricted = "market_restricted"

	// rejectDeclarationRequired means the person must say where they live
	// and that they have the age it asks for.
	rejectDeclarationRequired = "declaration_required"
)

// requireMarketAccess lets a live request through only when the caller may
// use Playhoot interactively where they are. A guest brings no declaration,
// so only the approximate country speaks for them; where it is unknown the
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
			err := gate.Require(ctx, userUUID, compliance.FunctionInteractiveUse, geo.Country(ctx))
			if err == nil {
				next(w, r)
				return
			}
			var declaration *marketgate.DeclarationRequiredError
			var restricted *marketgate.RestrictedError
			switch {
			case errors.As(err, &declaration):
				logging.LogFields(ctx, logging.Field("market_access_outcome", rejectDeclarationRequired))
				refuse(w, r, rejectedMessage{Reason: rejectDeclarationRequired})
			case errors.As(err, &restricted):
				logging.LogFields(ctx, logging.Field("market_access_outcome", rejectMarketRestricted))
				refuse(w, r, rejectedMessage{Reason: rejectMarketRestricted})
			default:
				logging.LogError(ctx, err)
				httpx.WriteError(w, http.StatusInternalServerError, "checking market access failed")
			}
		}
	}
}
