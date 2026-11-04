// Package mcp is the route group of Playhoot's MCP server (Streamable HTTP
// at /mcp): an AI application a User connected through OAuth reads,
// creates and changes that User's games with tools. Every tool calls the
// same games operation as its HTTP route, so both channels accept, refuse
// and answer alike. Publishing is not a tool.
package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/diegobermudez03/playhoot/api/games"
	apiauth "github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

// tokenVerifier is Identity's check of an AI access token.
type tokenVerifier interface {
	VerifyAccessToken(ctx context.Context, token string) (identity.AIAccess, bool, error)
}

// gameOperations are the games operations the tools translate to, the same
// ones the games HTTP handlers call.
type gameOperations interface {
	ListGames(ctx context.Context, userUUID string) ([]games.GameSummaryResponse, *games.Refusal)
	CreateGame(ctx context.Context, userUUID string, req games.CreateGameRequest) (games.CreateGameResponse, *games.Refusal)
	ReadContext(ctx context.Context, userUUID string, gameUUID string, linkToken string) (games.AIContext, *games.Refusal)
	ReadFile(ctx context.Context, userUUID string, gameUUID string, path string, revision *int64) (games.File, *games.Refusal)
	ApplyChanges(ctx context.Context, by games.Caller, gameUUID string, req games.ChangeRequest) (games.ChangeResponse, *games.Refusal)
	RestoreRevision(ctx context.Context, by games.Caller, gameUUID string, req games.RestoreRequest) (games.ChangeResponse, *games.Refusal)
	ListRevisions(ctx context.Context, userUUID string, gameUUID string) ([]games.RevisionResponse, *games.Refusal)
	UploadAsset(ctx context.Context, by games.Caller, gameUUID string, key string, idempotencyKey string, content io.Reader) (games.AssetEditResponse, *games.Refusal)
	RemoveAsset(ctx context.Context, by games.Caller, gameUUID string, key string, idempotencyKey string) (games.AssetEditResponse, *games.Refusal)
	ReadBuild(ctx context.Context, userUUID string, gameUUID string, revision *int64, wait time.Duration) (*games.BuildResponse, *games.Refusal)
	Simulate(ctx context.Context, userUUID string, gameUUID string, req games.ScenarioRequest) (games.SimulationResponse, *games.Refusal)
}

// Config is where the MCP server is reached and what it points AIs to.
type Config struct {
	// ResourceMetadataURL is the Protected Resource Metadata an
	// unauthenticated client is sent to; GuideURL the SDK guide's index.
	ResourceMetadataURL string
	GuideURL            string
}

// maxRequestBytes bounds one MCP request: the largest asset (50 MiB) in
// base64 inside its JSON.
const maxRequestBytes = 72 << 20

// Request deadlines of the MCP endpoint: an upload's body may be large, a
// build wait or a simulation take seconds.
const (
	requestReadTime  = 5 * time.Minute
	requestWriteTime = 2 * time.Minute
)

// Handler exposes the MCP endpoint.
type Handler struct {
	verifier tokenVerifier
	games    gameOperations
	config   Config
	mcp      http.Handler
}

// NewHandler constructs a Handler.
func NewHandler(verifier tokenVerifier, operations gameOperations, config Config) *Handler {
	h := &Handler{verifier: verifier, games: operations, config: config}
	server := sdk.NewServer(&sdk.Implementation{Name: "playhoot", Title: "Playhoot", Version: "1"}, &sdk.ServerOptions{Instructions: instructions(config.GuideURL)})
	h.addTools(server)
	// Stateless: every call is authorized by its own token and needs no
	// session. Every request carries a bearer token, so the SDK's guard for
	// unauthenticated local servers (DNS rebinding) is not needed, and it
	// would refuse a proxy on the same host.
	h.mcp = sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, DisableLocalhostProtection: true, MaxRequestBodyBytes: maxRequestBytes,
	})
	return h
}

// RESTRoutes returns the MCP endpoint. Its authorization (an AI access token
// for this resource with the games scope, standing for its User at account
// level) and the AI rate lane per User apply here, at the entry: the tools
// call the games operations directly, not through the games routes.
func (h *Handler) RESTRoutes() []routing.Route {
	limits := games.AIRequestLimits
	routes := make([]routing.Route, 0, 3)
	for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
		routes = append(routes, routing.Route{Pattern: method + " /mcp", Handler: h.handleMCP, Authorization: h.requireToken, Limits: &limits})
	}
	return routes
}

// WebSocketRoutes returns none.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}

func (h *Handler) handleMCP(w http.ResponseWriter, r *http.Request) {
	controller := http.NewResponseController(w)
	if err := controller.SetReadDeadline(time.Now().Add(requestReadTime)); err != nil {
		logging.LogError(r.Context(), err)
	}
	if err := controller.SetWriteDeadline(time.Now().Add(requestWriteTime)); err != nil {
		logging.LogError(r.Context(), err)
	}
	h.mcp.ServeHTTP(w, r)
}

// requireToken accepts only an AI access token for this resource, with the
// games scope (an unauthenticated request learns where to authorize from
// WWW-Authenticate), and lets the request go on as its User.
func (h *Handler) requireToken(next http.HandlerFunc) http.HandlerFunc {
	verify := func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
		access, valid, err := h.verifier.VerifyAccessToken(ctx, token)
		if err != nil {
			logging.LogError(ctx, err)
			return nil, err
		}
		if !valid {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{Scopes: access.Scopes, Expiration: access.ExpiresAt, UserID: access.UserUUID}, nil
	}
	guard := auth.RequireBearerToken(verify, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: h.config.ResourceMetadataURL, Scopes: []string{identity.AIScopeGames}, ClockSkew: time.Minute,
	})
	return guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := auth.TokenInfoFromContext(r.Context())
		if info == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		logging.LogFields(r.Context(), logging.Field("mcp", true))
		next(w, r.WithContext(apiauth.WithCaller(r.Context(), identity.User{UUID: info.UserID, Level: identity.LevelAccount})))
	})).ServeHTTP
}

// userOf is the User a tool call acts for.
func userOf(req *sdk.CallToolRequest) (string, error) {
	if req.Extra == nil || req.Extra.TokenInfo == nil || req.Extra.TokenInfo.UserID == "" {
		return "", errors.New("the call carries no authorized user")
	}
	return req.Extra.TokenInfo.UserID, nil
}

func instructions(guide string) string {
	return strings.Join([]string{
		"Playhoot hosts multiplayer games written with its TypeScript SDK: authoritative server logic in a sandbox and a React client in an isolated frame.",
		"Before writing code, read the SDK guide: " + guide + " (start there; it lists every page).",
		"Work on the connected user's games: list_my_games or create_game, then get_game_context (current revision, build, assets, recent attempts, notes.md and the sources).",
		"Change a game with apply_changes on the revision you read (base_revision); every revision builds automatically: wait for it with get_build, then try rules with simulate.",
		"Every tool that changes something needs a new random idempotency_key per change; send the same key only to retry that same change.",
		"You cannot publish: the user publishes in Playhoot, and plays the draft there in Probar juego.",
	}, "\n")
}
