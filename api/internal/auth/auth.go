// Package auth is the API edge's authorization middleware: it extracts the
// caller's bearer token and asks Identity who they are and whether they may
// perform the route's operation.
package auth

import (
	"context"
	"encoding/base64"
	"errors"
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

// authorizer resolves a token to the caller's UserUUID and decides whether
// they may perform operation. A refusal is a decision, not an error.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (string, identity.AccessDecision, error)
}

var (
	decisionLabel = metrics.NewLabel("decision", "allowed", "unauthenticated", "forbidden", "missing_token", "malformed_token", "error")

	decisionAllowed         = decisionLabel.Value("allowed")
	decisionUnauthenticated = decisionLabel.Value("unauthenticated")
	decisionForbidden       = decisionLabel.Value("forbidden")
	decisionMissingToken    = decisionLabel.Value("missing_token")
	decisionMalformedToken  = decisionLabel.Value("malformed_token")
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

// WithUserUUID returns ctx carrying userUUID as the authorized caller.
func WithUserUUID(ctx context.Context, userUUID string) context.Context {
	return context.WithValue(ctx, userUUIDContextKey{}, userUUID)
}

// UserUUID returns the authorized caller, or false when the request was not
// authorized by this package's middleware.
func UserUUID(ctx context.Context) (string, bool) {
	userUUID, ok := ctx.Value(userUUIDContextKey{}).(string)
	return userUUID, ok && userUUID != ""
}

// webSocketBearerProtocolPrefix carries a bearer token as a WebSocket
// subprotocol (the token follows, as unpadded base64url), because browsers
// cannot set headers on a WebSocket handshake. The same scheme as Kubernetes.
const webSocketBearerProtocolPrefix = "base64url.bearer.authorization.playhoot."

var (
	errMissingToken   = errors.New("missing bearer token")
	errMalformedToken = errors.New("malformed bearer token")
)

// tokenSource extracts a request's bearer token.
type tokenSource func(r *http.Request) (string, error)

// Require lets a request through only when Identity allows its bearer token
// (Authorization header) to perform operation.
func Require(authorizer authorizer, operation identity.Operation) routing.Middleware {
	return requireToken(authorizer, operation, headerToken)
}

// RequireWebSocket is Require for a WebSocket handshake: the token comes from
// the Authorization header or, for browsers, the bearer token subprotocol
// "base64url.bearer.authorization.playhoot.<base64url token>".
func RequireWebSocket(authorizer authorizer, operation identity.Operation) routing.Middleware {
	return requireToken(authorizer, operation, webSocketToken)
}

func requireToken(authorizer authorizer, operation identity.Operation, tokenFrom tokenSource) routing.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logging.LogFields(ctx, logging.Field("operation", string(operation)))

			token, err := tokenFrom(r)
			if errors.Is(err, errMalformedToken) {
				authorizationTotal.Inc(ctx, decisionMalformedToken)
				writeUnauthenticated(w, "malformed bearer token")
				return
			}
			if err != nil {
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

func headerToken(r *http.Request) (string, error) {
	token, ok := bearerToken(r)
	if !ok {
		return "", errMissingToken
	}
	return token, nil
}

// webSocketToken prefers the Authorization header over the subprotocol.
func webSocketToken(r *http.Request) (string, error) {
	if token, ok := bearerToken(r); ok {
		return token, nil
	}
	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, protocol := range strings.Split(header, ",") {
			encoded, found := strings.CutPrefix(strings.TrimSpace(protocol), webSocketBearerProtocolPrefix)
			if !found {
				continue
			}
			token, err := base64.RawURLEncoding.DecodeString(encoded)
			if err != nil || strings.TrimSpace(string(token)) == "" {
				return "", errMalformedToken
			}
			return string(token), nil
		}
	}
	return "", errMissingToken
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
