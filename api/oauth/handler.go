// Package oauth is the route group of AI applications' OAuth access: the
// discovery documents an MCP client starts from, client registration,
// authorization, tokens and revocation, plus the endpoints the web page a
// User approves a request on, and manages their connections from, calls.
// Each endpoint's handler has its own file.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

//go:generate mockgen -package=oauth -destination=handler_mock_test.go . authorizationServer

// authorizer is Identity's coarse authorization.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// authorizationServer is Identity's OAuth server for AI applications.
type authorizationServer interface {
	RegisterClient(ctx context.Context, metadata identity.AIClientMetadata) (identity.RegisteredAIClient, error)
	StartAuthorization(ctx context.Context, params identity.AIAuthorizationParams) (identity.AIAuthorizationStart, error)
	ReadAuthorization(ctx context.Context, requestID string) (identity.PendingAIAuthorization, error)
	DecideAuthorization(ctx context.Context, userUUID string, requestID string, approve bool) (string, error)
	ExchangeToken(ctx context.Context, params identity.AITokenParams) (identity.AITokens, error)
	Revoke(ctx context.Context, token string, clientID string) error
	ListConnections(ctx context.Context, userUUID string) ([]identity.AIConnection, error)
	RevokeConnection(ctx context.Context, userUUID string, connectionID string) (bool, error)
}

// Config is where the authorization server and its one resource live.
type Config struct {
	// Issuer is the API's public base URL, Resource the MCP endpoint, and
	// Documentation the SDK guide an AI reads.
	Issuer        string
	Resource      string
	Documentation string
}

// Handler exposes the OAuth endpoints.
type Handler struct {
	authorizer authorizer
	server     authorizationServer
	config     Config
}

// NewHandler constructs a Handler.
func NewHandler(authorizer authorizer, server authorizationServer, config Config) *Handler {
	config.Issuer = strings.TrimRight(config.Issuer, "/")
	return &Handler{authorizer: authorizer, server: server, config: config}
}

// registrationLimits bound how often one client address registers
// applications: an application registers once, not per request.
var registrationLimits = routing.RequestLimits{
	PerClient: []ratelimit.Limit{{Count: 5, Per: time.Minute}, {Count: 30, Per: time.Hour}},
}

// RESTRoutes returns this group's endpoints. The OAuth ones are reached by
// applications without a Playhoot session; the consent and connection ones
// by the User's web page.
func (h *Handler) RESTRoutes() []routing.Route {
	connect := auth.Require(h.authorizer, identity.OperationConnectAI)
	return []routing.Route{
		{Pattern: "GET /.well-known/oauth-protected-resource", Handler: h.handleResourceMetadata},
		{Pattern: "GET /.well-known/oauth-protected-resource/mcp", Handler: h.handleResourceMetadata},
		{Pattern: "GET /.well-known/oauth-authorization-server", Handler: h.handleServerMetadata},
		{Pattern: "POST /oauth/register", Handler: h.handleRegister, Limits: &registrationLimits},
		{Pattern: "GET /oauth/authorize", Handler: h.handleAuthorize},
		{Pattern: "POST /oauth/token", Handler: h.handleToken},
		{Pattern: "POST /oauth/revoke", Handler: h.handleRevoke},

		{Pattern: "GET /v1/ai-authorizations/{requestID}", Handler: h.handleReadAuthorization, Authorization: connect},
		{Pattern: "POST /v1/ai-authorizations/{requestID}/approve", Handler: h.handleApprove, Authorization: connect},
		{Pattern: "POST /v1/ai-authorizations/{requestID}/deny", Handler: h.handleDeny, Authorization: connect},
		{Pattern: "GET /v1/ai-connections", Handler: h.handleListConnections, Authorization: connect},
		{Pattern: "POST /v1/ai-connections/{connectionID}/revoke", Handler: h.handleRevokeConnection, Authorization: connect},
	}
}

// WebSocketRoutes returns none.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}

// oauthErrorBody is an OAuth error response (RFC 6749 §5.2).
type oauthErrorBody struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

// writeOAuthError answers err as an OAuth error when it is one, as a server
// error otherwise.
func writeOAuthError(ctx context.Context, w http.ResponseWriter, err error) {
	var refusal *identity.AIAuthError
	if !errors.As(err, &refusal) {
		logging.LogError(ctx, err)
		writeJSON(w, http.StatusInternalServerError, oauthErrorBody{Error: "server_error"})
		return
	}
	logging.LogFields(ctx, logging.Field("oauth_error", refusal.Code))
	status := http.StatusBadRequest
	if refusal.Code == "invalid_client" {
		status = http.StatusUnauthorized
	}
	writeJSON(w, status, oauthErrorBody{Error: refusal.Code, Description: refusal.Description})
}

// writeJSON answers body without letting any cache keep it: OAuth answers
// carry secrets.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// callerUUID returns the authorized caller, or answers itself.
func callerUUID(w http.ResponseWriter, r *http.Request, what string) (string, bool) {
	userUUID, ok := auth.UserUUID(r.Context())
	if !ok {
		logging.LogFields(r.Context(), logging.Field("wiring_error", what+" reached without an authorized caller"))
		httpx.WriteError(w, http.StatusInternalServerError, what+" failed")
		return "", false
	}
	return userUUID, true
}
