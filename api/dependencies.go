package api

import (
	"context"
	"io"
	"strings"
	"time"

	apigames "github.com/diegobermudez03/playhoot/api/games"
	composergameauthoring "github.com/diegobermudez03/playhoot/composer/gameauthoring"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// guestCreator is Identity's guest creation.
type guestCreator interface {
	CreateGuest(ctx context.Context, displayName string, idempotencyKey string) (identity.CreateGuestResult, error)
}

// operationLister is Identity's catalog of operations and the minimum
// authentication level each requires.
type operationLister interface {
	ListOperations(ctx context.Context) []identity.OperationLevel
}

// credentialRefresher is Identity's credential renewal.
type credentialRefresher interface {
	Refresh(ctx context.Context, refreshToken string) (identity.RefreshResult, error)
}

// sessionEnder is Identity's sign-out.
type sessionEnder interface {
	EndSession(ctx context.Context, refreshToken string) error
}

// loginStarter is Identity's start of an account sign-in.
type loginStarter interface {
	StartLogin(ctx context.Context, provider string, codeChallenge string, guestUserUUID string) (identity.StartLoginResult, error)
}

// loginCallbackHandler is Identity's handling of a provider's answer.
type loginCallbackHandler interface {
	HandleCallback(ctx context.Context, provider string, state string, code string, providerError string) (identity.LoginCallbackResult, error)
}

// loginCompleter is Identity's completion of a sign-in.
type loginCompleter interface {
	CompleteLogin(ctx context.Context, code string, codeVerifier string, replacesRefreshToken string) (identity.CompleteLoginResult, error)
}

// providerLister is Identity's list of enabled login providers.
type providerLister interface {
	Enabled() []identity.Provider
}

// localPages is Identity's local provider page, when enabled.
type localPages interface {
	LocalAuthorizePage(state string) (string, bool)
}

// Logins is what account sign-in needs.
type Logins struct {
	Starter    loginStarter
	Callbacks  loginCallbackHandler
	Completer  loginCompleter
	Providers  providerLister
	LocalPages localPages
	// CompleteURL is the client's sign-in completion route every provider
	// callback redirects to.
	CompleteURL string
}

// displayNameSetter is Identity's naming of a User.
type displayNameSetter interface {
	SetDisplayName(ctx context.Context, userUUID string, displayName string) (identity.SetDisplayNameResult, error)
}

// authorizer is Identity's coarse authorization.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// sessionCreator coordinates Session creation.
type sessionCreator interface {
	CreateSession(ctx context.Context, userUUID string, gameUUID string, idempotencyKey string) (sessionpkg.CreateSessionResult, error)
}

// ownerLocator is the Runtime's live Session ownership.
type ownerLocator interface {
	LocateOrClaimOwner(ctx context.Context, sessionUUID string) (runtime.OwnerPlacement, error)
}

// adminAttacher is the Runtime's admission of admin connections.
type adminAttacher interface {
	AttachAdmin(ctx context.Context, sessionUUID string, userUUID string, ownerEpoch int64) (runtime.AttachAdminResult, error)
}

// playerAttacher is the Runtime's admission of player connections.
type playerAttacher interface {
	AttachPlayer(ctx context.Context, sessionUUID string, player runtime.Player, ownerEpoch int64) (runtime.AttachPlayerResult, error)
}

// joinCodeResolver is the durable Session's join code lookup.
type joinCodeResolver interface {
	ResolveJoinCode(ctx context.Context, joinCode string) (sessionpkg.JoinCodeResolution, error)
}

// gameCreator is game authoring's creation of a game with its version being
// built.
type gameCreator interface {
	CreateGame(ctx context.Context, userUUID string, name string, idempotencyKey string) (gameauthoring.CreatedGame, error)
}

// draftEditor is game authoring's changes to a game's version being built.
type draftEditor interface {
	ApplyDraftChanges(ctx context.Context, userUUID string, gameUUID string, change sessionpkg.DraftChange) (gameauthoring.DraftEdit, error)
	RestoreDraftRevision(ctx context.Context, userUUID string, gameUUID string, revision int64, baseRevision int64, idempotencyKey string, origin string) (gameauthoring.DraftEdit, error)
	UploadDraftAsset(ctx context.Context, userUUID string, gameUUID string, key string, content io.Reader, idempotencyKey string, origin string) (gameauthoring.DraftAssetEdit, error)
	RemoveDraftAsset(ctx context.Context, userUUID string, gameUUID string, key string, idempotencyKey string, origin string) (gameauthoring.DraftAssetEdit, error)
	EnsureDraft(ctx context.Context, userUUID string, gameUUID string) (game.EditableVersionOutcome, error)
	StartPreview(ctx context.Context, userUUID string, gameUUID string, revision int64, players int) (gameauthoring.PreviewStart, error)
}

// draftReader is game authoring's reads of a game's version being built.
type draftReader interface {
	ReadDraft(ctx context.Context, userUUID string, gameUUID string, revision int64, withContent bool) (composergameauthoring.DraftRead, error)
	ListDraftRevisions(ctx context.Context, userUUID string, gameUUID string, before int64, limit int) (composergameauthoring.DraftRevisions, error)
	ReadDraftChanges(ctx context.Context, userUUID string, gameUUID string, since int64, upTo int64) (composergameauthoring.DraftChanges, error)
	ReadDraftBuild(ctx context.Context, userUUID string, gameUUID string, revision int64, wait time.Duration) (composergameauthoring.DraftBuildRead, error)
	SimulateDraft(ctx context.Context, userUUID string, gameUUID string, revision int64, scenario runtime.Scenario) (composergameauthoring.DraftSimulation, error)
}

// Games is what game authoring needs, AILinks included: the links an AI uses
// to read and change one game.
type Games struct {
	Previews previewAttacher
	Creator  gameCreator
	Editor   draftEditor
	Reader   draftReader
	AILinks  apigames.AILinks
	Cards    apigames.Cards
}

// previewAttacher attaches a draft owner's connection to their preview.
type previewAttacher interface {
	AttachPreview(ctx context.Context, previewUUID string, ownerUUID string) (runtime.AttachPreviewResult, error)
}

// mcpPath is where the MCP server is reached, the resource AI tokens are
// for.
const mcpPath = "/mcp"

// MCPResource is the MCP resource identifier for an API reached at base.
func MCPResource(base string) string {
	return strings.TrimRight(base, "/") + mcpPath
}

// AI is what AI applications' OAuth access and the MCP server need:
// Identity's authorization server, which also verifies their tokens.
type AI struct {
	Server aiAuthorizationServer
}

// aiAuthorizationServer is Identity's OAuth server for AI applications.
type aiAuthorizationServer interface {
	RegisterClient(ctx context.Context, metadata identity.AIClientMetadata) (identity.RegisteredAIClient, error)
	StartAuthorization(ctx context.Context, params identity.AIAuthorizationParams) (identity.AIAuthorizationStart, error)
	ReadAuthorization(ctx context.Context, requestID string) (identity.PendingAIAuthorization, error)
	DecideAuthorization(ctx context.Context, userUUID string, requestID string, approve bool) (string, error)
	ExchangeToken(ctx context.Context, params identity.AITokenParams) (identity.AITokens, error)
	Revoke(ctx context.Context, token string, clientID string) error
	ListConnections(ctx context.Context, userUUID string) ([]identity.AIConnection, error)
	RevokeConnection(ctx context.Context, userUUID string, connectionID string) (bool, error)
	VerifyAccessToken(ctx context.Context, token string) (identity.AIAccess, bool, error)
}
