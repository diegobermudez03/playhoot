// Package games is the route group of game authoring: creating a game and
// reading and changing the version being built for it. Each endpoint's
// handler has its own file.
package games

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	composergameauthoring "github.com/diegobermudez03/playhoot/composer/gameauthoring"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=games -destination=handler_mock_test.go . gameCreator,draftEditor,draftReader,aiLinkOwner,aiLinkStatusReader,linkResolver

// authorizer is Identity's coarse authorization.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// gameCreator creates a game with its version being built, idempotently per
// caller and key.
type gameCreator interface {
	CreateGame(ctx context.Context, userUUID string, idempotencyKey string) (gameauthoring.CreatedGame, error)
}

// draftEditor changes a game's version being built for a user Game
// Management lets edit it. A key reused with another request returns
// sessionpkg.ErrIdempotencyConflict.
type draftEditor interface {
	ApplyDraftChanges(ctx context.Context, userUUID string, gameUUID string, change sessionpkg.DraftChange) (gameauthoring.DraftEdit, error)
	RestoreDraftRevision(ctx context.Context, userUUID string, gameUUID string, revision int64, baseRevision int64, idempotencyKey string, origin string) (gameauthoring.DraftEdit, error)
	UploadDraftAsset(ctx context.Context, userUUID string, gameUUID string, key string, content io.Reader, idempotencyKey string, origin string) (gameauthoring.DraftAssetEdit, error)
	RemoveDraftAsset(ctx context.Context, userUUID string, gameUUID string, key string, idempotencyKey string, origin string) (gameauthoring.DraftAssetEdit, error)
	// EnsureDraft creates in Play the version being built that a read found
	// missing there, answering Game Management's edit decision.
	EnsureDraft(ctx context.Context, userUUID string, gameUUID string) (game.EditableVersionOutcome, error)
}

// draftReader reads a game's version being built for a user Game Management
// lets edit it.
type draftReader interface {
	ReadDraft(ctx context.Context, userUUID string, gameUUID string, revision int64, withContent bool) (composergameauthoring.DraftRead, error)
	ListDraftRevisions(ctx context.Context, userUUID string, gameUUID string) (composergameauthoring.DraftRevisions, error)
	ReadDraftBuild(ctx context.Context, userUUID string, gameUUID string, revision int64, wait time.Duration) (composergameauthoring.DraftBuildRead, error)
	SimulateDraft(ctx context.Context, userUUID string, gameUUID string, revision int64, scenario runtime.Scenario) (composergameauthoring.DraftSimulation, error)
}

// aiLinkOwner creates and revokes the link an AI uses for a game, for its
// owner only.
type aiLinkOwner interface {
	CreateAILink(ctx context.Context, userUUID string, gameUUID string) (gameauthoring.AILinkCreation, error)
	RevokeAILink(ctx context.Context, userUUID string, gameUUID string) (game.EditableVersionOutcome, error)
}

// aiLinkStatusReader says whether a game has a live AI link, for its owner
// only.
type aiLinkStatusReader interface {
	AILinkStatus(ctx context.Context, userUUID string, gameUUID string) (composergameauthoring.AILinkStatusRead, error)
}

// linkResolver is Identity's answer to whom a link token acts for and on
// what resource.
type linkResolver interface {
	ResolveResourceLink(ctx context.Context, token string) (identity.ResolvedResourceLink, error)
}

// AILinks is what the links an AI uses for a game need. PublicBaseURL is the
// address the API is reached at from outside, with no trailing slash.
type AILinks struct {
	Owner         aiLinkOwner
	Status        aiLinkStatusReader
	Resolver      linkResolver
	PublicBaseURL string
}

// Handler exposes the game authoring endpoints.
type Handler struct {
	authorizer authorizer
	creator    gameCreator
	editor     draftEditor
	reader     draftReader
	links      AILinks
	now        func() time.Time
}

// NewHandler constructs a Handler.
func NewHandler(authorizer authorizer, creator gameCreator, editor draftEditor, reader draftReader, links AILinks) *Handler {
	links.PublicBaseURL = strings.TrimRight(links.PublicBaseURL, "/")
	return &Handler{authorizer: authorizer, creator: creator, editor: editor, reader: reader, links: links, now: time.Now}
}

// RESTRoutes returns this group's request/response endpoints.
func (h *Handler) RESTRoutes() []routing.Route {
	edit := auth.Require(h.authorizer, identity.OperationEditGame)
	return []routing.Route{
		{Pattern: "POST /v1/games", Handler: h.handleCreateGame, Authorization: auth.Require(h.authorizer, identity.OperationCreateGame)},
		{Pattern: "GET /v1/games/{gameUUID}/draft", Handler: h.handleReadDraft, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/draft/changes", Handler: h.handleApplyDraftChanges, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}/draft/revisions", Handler: h.handleListDraftRevisions, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}/draft/build", Handler: h.handleReadDraftBuild, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/draft/restore", Handler: h.handleRestoreDraftRevision, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/draft/simulations", Handler: h.handleSimulateDraft, Authorization: edit},
		{Pattern: "PUT /v1/games/{gameUUID}/draft/assets/{key}", Handler: h.handleUploadAsset, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/draft/assets/{key}/remove", Handler: h.handleRemoveAsset, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/ai-link", Handler: h.handleCreateAILink, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}/ai-link", Handler: h.handleAILinkStatus, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/ai-link/revoke", Handler: h.handleRevokeAILink, Authorization: edit},

		// An AI reaches its game through the link token in the path, acting
		// as the game's owner on that game only.
		{Pattern: "GET /ai/v1/{token}", Handler: h.handleAIContext, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/files/{path...}", Handler: h.handleAIFile, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/build", Handler: h.handleReadDraftBuild, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "POST /ai/v1/{token}/changes", Handler: h.handleApplyDraftChanges, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "POST /ai/v1/{token}/restore", Handler: h.handleRestoreDraftRevision, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "POST /ai/v1/{token}/simulations", Handler: h.handleSimulateDraft, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "PUT /ai/v1/{token}/assets/{key}", Handler: h.handleUploadAsset, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "POST /ai/v1/{token}/assets/{key}/remove", Handler: h.handleRemoveAsset, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
	}
}

// WebSocketRoutes returns none: authoring has no live endpoint.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}

const (
	maxIdempotencyKeyLength = 128

	codeNotEditable         = "not_editable"
	codeRevisionConflict    = "revision_conflict"
	codeRevisionNotFound    = "revision_not_found"
	codeInvalidChange       = "invalid_change"
	codeIdempotencyConflict = "idempotency_conflict"
)

// caller returns the authorized caller and the path's game, or answers the
// request itself and returns false.
func caller(w http.ResponseWriter, r *http.Request, what string) (userUUID string, gameUUID string, ok bool) {
	ctx := r.Context()
	userUUID, ok = auth.UserUUID(ctx)
	if !ok {
		// Only possible if the route was registered without its middleware.
		logging.LogFields(ctx, logging.Field("wiring_error", what+" reached without an authorized caller"))
		httpx.WriteError(w, http.StatusInternalServerError, what+" failed")
		return "", "", false
	}
	if raw := r.PathValue("gameUUID"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			// Not a game anyone has: answered like an unknown one.
			httpx.WriteError(w, http.StatusNotFound, "game not found")
			return "", "", false
		}
		gameUUID = parsed.String()
		logging.LogFields(ctx, logging.Field("game_uuid", gameUUID))
	}
	return userUUID, gameUUID, true
}

// noStore marks a response that must not be kept by any cache: a game's
// sources are private. A stricter policy already set (an AI link's) stays.
func noStore(w http.ResponseWriter) {
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
}
