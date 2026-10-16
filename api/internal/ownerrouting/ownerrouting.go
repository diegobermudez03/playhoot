// Package ownerrouting routes a live Session request to the instance that
// owns the Session, before the WebSocket upgrade.
package ownerrouting

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

//go:generate mockgen -package=ownerrouting -destination=owner_locator_mock_test.go . ownerLocator

// ownerLocator returns a Session's owner, claiming one when there is none.
type ownerLocator interface {
	LocateOrClaimOwner(ctx context.Context, sessionUUID string) (runtime.OwnerPlacement, error)
}

type ownedSessionContextKey struct{}

type ownedSession struct {
	sessionUUID string
	epoch       int64
}

// WithOwnedSession returns ctx carrying sessionUUID as owned here under epoch.
func WithOwnedSession(ctx context.Context, sessionUUID string, epoch int64) context.Context {
	return context.WithValue(ctx, ownedSessionContextKey{}, ownedSession{sessionUUID: sessionUUID, epoch: epoch})
}

// OwnedSession returns the Session routed here and its ownership epoch, or
// false when the request was not routed by Require.
func OwnedSession(ctx context.Context) (string, int64, bool) {
	owned, ok := ctx.Value(ownedSessionContextKey{}).(ownedSession)
	return owned.sessionUUID, owned.epoch, ok && owned.sessionUUID != ""
}

// Require lets a request through only when this instance owns the Session
// named by the route's pathValue wildcard.
func Require(locator ownerLocator, pathValue string) routing.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			parsed, err := uuid.Parse(r.PathValue(pathValue))
			if err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "session UUID must be a valid UUID")
				return
			}
			sessionUUID := parsed.String()
			logging.LogFields(ctx, logging.Field("session_uuid", sessionUUID))

			placement, err := locator.LocateOrClaimOwner(ctx, sessionUUID)
			if err != nil {
				logging.LogError(ctx, err)
				httpx.WriteError(w, http.StatusInternalServerError, "routing to session failed")
				return
			}
			logging.LogFields(ctx,
				logging.Field("owner_outcome", string(placement.Outcome)),
				logging.Field("owner_epoch", placement.Epoch),
			)

			switch placement.Outcome {
			case runtime.OwnerHere:
				next(w, r.WithContext(WithOwnedSession(ctx, sessionUUID, placement.Epoch)))
			case runtime.OwnerSessionUnavailable:
				httpx.WriteError(w, http.StatusNotFound, "session not found or not joinable")
			case runtime.OwnerElsewhere:
				// Unreachable while only one instance can own Sessions;
				// forwarding to the owner comes with multiple instances.
				logging.LogFields(ctx, logging.Field("routing_error", "session owned by another instance, forwarding not implemented"))
				httpx.WriteError(w, http.StatusServiceUnavailable, "session temporarily unavailable")
			default:
				httpx.WriteError(w, http.StatusInternalServerError, "routing to session failed")
			}
		}
	}
}
