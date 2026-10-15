// Package auth is the API edge's authentication and coarse authorization
// middleware. It never interprets a token itself: it extracts the bearer
// token from the request and asks Identity who the caller is and whether
// that caller may perform the route's operation.
package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

//go:generate mockgen -package=auth -destination=authorizer_mock_test.go . authorizer

// authorizer is this package's narrow dependency on Identity. It receives
// the raw token and the operation the route performs, and answers with the
// caller's UserUUID and whether the operation is allowed.
//
// A caller who is simply not allowed is reported through the decision, with
// a nil error. A non-nil error means Identity could not evaluate the
// request at all. The returned UserUUID is only meaningful when the
// decision is identity.AccessAllowed.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (string, identity.AccessDecision, error)
}

var (
	decisionLabel = metrics.NewLabel("decision", "allowed", "unauthenticated", "forbidden", "missing_token", "error")

	decisionAllowed         = decisionLabel.Value("allowed")
	decisionUnauthenticated = decisionLabel.Value("unauthenticated")
	decisionForbidden       = decisionLabel.Value("forbidden")
	decisionMissingToken    = decisionLabel.Value("missing_token")
	decisionError           = decisionLabel.Value("error")

	authorizationTotal = metrics.NewCounter(
		"api_authorization_total",
		"Authorization checks performed at the API edge, by outcome.",
		decisionLabel,
	)
	authorizationDuration = metrics.NewHistogram(
		"api_authorization_duration_seconds",
		"Time Identity took to answer an authorization check.",
		metrics.LatencyBuckets(),
		decisionLabel,
	)
)

type userUUIDContextKey struct{}

// WithUserUUID returns a context carrying userUUID as the authorized caller.
// Require uses it once Identity allows a request; it is exported so a handler
// can be exercised without running the middleware in front of it.
func WithUserUUID(ctx context.Context, userUUID string) context.Context {
	return context.WithValue(ctx, userUUIDContextKey{}, userUUID)
}

// UserUUID returns the caller's UserUUID, set by Require. It reports false
// when the request did not pass through Require, which a handler behind it
// must treat as a wiring bug rather than an unauthenticated caller.
func UserUUID(ctx context.Context) (string, bool) {
	userUUID, ok := ctx.Value(userUUIDContextKey{}).(string)
	return userUUID, ok && userUUID != ""
}

// Require returns a middleware that only lets a request through when
// Identity allows its bearer token to perform operation, and makes the
// caller's UserUUID available to the handler through UserUUID.
//
// It assumes the request context already carries a started request log, as
// every route registered by api.Server does.
func Require(authorizer authorizer, operation identity.Operation) routing.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logging.LogFields(ctx, logging.Field("operation", string(operation)))

			token, ok := bearerToken(r)
			if !ok {
				authorizationTotal.Inc(ctx, decisionMissingToken)
				writeUnauthenticated(w, "missing bearer token")
				return
			}

			started := time.Now()
			userUUID, decision, err := authorizer.Authorize(ctx, token, operation)

			result := decisionError
			switch {
			case err != nil:
				logging.LogError(ctx, err)
			case decision == identity.AccessAllowed && userUUID != "":
				result = decisionAllowed
			case decision == identity.AccessUnauthenticated:
				result = decisionUnauthenticated
			case decision == identity.AccessForbidden:
				result = decisionForbidden
			default:
				// Identity allowed the operation without naming the caller,
				// or returned a decision this package does not know: neither
				// is something to grant access on.
				logging.LogFields(ctx, logging.Field("authorization_decision", string(decision)))
			}
			authorizationTotal.Inc(ctx, result)
			authorizationDuration.ObserveDuration(ctx, time.Since(started), result)

			switch result {
			case decisionAllowed:
				logging.LogFields(ctx, logging.Field("user_uuid", userUUID))
				next(w, r.WithContext(WithUserUUID(ctx, userUUID)))
			case decisionUnauthenticated:
				writeUnauthenticated(w, "invalid or expired token")
			case decisionForbidden:
				httpx.WriteError(w, http.StatusForbidden, "operation not allowed")
			default:
				httpx.WriteError(w, http.StatusInternalServerError, "authorization failed")
			}
		}
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header.
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

func writeUnauthenticated(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	httpx.WriteError(w, http.StatusUnauthorized, message)
}
