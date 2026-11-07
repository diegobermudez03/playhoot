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

//go:generate mockgen -package=games -destination=handler_mock_test.go . gameCreator,draftEditor,draftReader,aiLinkOwner,aiLinkStatusReader,linkResolver,cardManager,publisher,templateReader

// authorizer is Identity's coarse authorization.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// gameCreator creates a game with its version being built, idempotently per
// caller and key.
type gameCreator interface {
	CreateGame(ctx context.Context, userUUID string, name string, idempotencyKey string, templateGameUUID string) (gameauthoring.CreatedGame, error)
}

// draftEditor changes a game's version being built for a user Game
// Management lets edit it. A key reused with another request returns
// sessionpkg.ErrIdempotencyConflict.
type draftEditor interface {
	ApplyDraftChanges(ctx context.Context, userUUID string, gameUUID string, change sessionpkg.DraftChange) (gameauthoring.DraftEdit, error)
	RestoreDraftRevision(ctx context.Context, userUUID string, gameUUID string, revision int64, baseRevision int64, idempotencyKey string, origin string) (gameauthoring.DraftEdit, error)
	UseTemplate(ctx context.Context, userUUID string, gameUUID string, templateGameUUID string, baseRevision int64, idempotencyKey string, origin string) (gameauthoring.DraftEdit, error)
	UploadDraftAsset(ctx context.Context, userUUID string, gameUUID string, key string, content io.Reader, idempotencyKey string, origin string) (gameauthoring.DraftAssetEdit, error)
	RemoveDraftAsset(ctx context.Context, userUUID string, gameUUID string, key string, idempotencyKey string, origin string) (gameauthoring.DraftAssetEdit, error)
	// EnsureDraft creates in Play the version being built that a read found
	// missing there, answering Game Management's edit decision.
	EnsureDraft(ctx context.Context, userUUID string, gameUUID string) (game.EditableVersionOutcome, error)
	StartPreview(ctx context.Context, userUUID string, gameUUID string, revision int64, players int) (gameauthoring.PreviewStart, error)
}

// draftReader reads a game's version being built for a user Game Management
// lets edit it.
type draftReader interface {
	ReadDraft(ctx context.Context, userUUID string, gameUUID string, revision int64, withContent bool) (composergameauthoring.DraftRead, error)
	ListDraftRevisions(ctx context.Context, userUUID string, gameUUID string, before int64, limit int) (composergameauthoring.DraftRevisions, error)
	ReadDraftChanges(ctx context.Context, userUUID string, gameUUID string, since int64, upTo int64) (composergameauthoring.DraftChanges, error)
	ReadDraftDiff(ctx context.Context, userUUID string, gameUUID string, from int64, to int64, onlyPath string) (composergameauthoring.DraftDiff, error)
	ReadDraftBuild(ctx context.Context, userUUID string, gameUUID string, revision int64, wait time.Duration) (composergameauthoring.DraftBuildRead, error)
	SimulateDraft(ctx context.Context, userUUID string, gameUUID string, revision int64, scenario runtime.Scenario) (composergameauthoring.DraftSimulation, error)
	SimulateSavedScenarios(ctx context.Context, userUUID string, gameUUID string, revision int64, path string) (composergameauthoring.DraftSavedSimulations, error)
	ReadSimulationState(ctx context.Context, userUUID string, gameUUID string, id string, atMs *int64, of string, path string) (composergameauthoring.DraftSimulationState, error)
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
// address the API is reached at from outside, PublicWebBaseURL the web app's,
// where the SDK guide is served; GuideBaseURL is derived from it.
type AILinks struct {
	Owner            aiLinkOwner
	Status           aiLinkStatusReader
	Resolver         linkResolver
	Guide            sdkGuide
	PublicBaseURL    string
	PublicWebBaseURL string
	GuideBaseURL     string
}

// Handler exposes the game authoring endpoints.
type Handler struct {
	authorizer authorizer
	creator    gameCreator
	editor     draftEditor
	reader     draftReader
	links      AILinks
	cards      Cards
	templates  templateReader
	now        func() time.Time
}

// NewHandler constructs a Handler.
func NewHandler(authorizer authorizer, creator gameCreator, editor draftEditor, reader draftReader, links AILinks, cards Cards, templates templateReader) *Handler {
	links.PublicBaseURL = strings.TrimRight(links.PublicBaseURL, "/")
	links.GuideBaseURL = strings.TrimRight(links.PublicWebBaseURL, "/") + guidePath
	return &Handler{authorizer: authorizer, creator: creator, editor: editor, reader: reader, links: links, cards: cards, templates: templates, now: time.Now}
}

// RESTRoutes returns this group's request/response endpoints.
func (h *Handler) RESTRoutes() []routing.Route {
	edit := auth.Require(h.authorizer, identity.OperationEditGame)
	create := auth.Require(h.authorizer, identity.OperationCreateGame)
	return []routing.Route{
		{Pattern: "POST /v1/games", Handler: h.handleCreateGame, Authorization: create},
		{Pattern: "GET /v1/games", Handler: h.handleListGames, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}", Handler: h.handleReadCard, Authorization: edit},
		{Pattern: "PATCH /v1/games/{gameUUID}", Handler: h.handleUpdateCard, Authorization: edit},
		{Pattern: "PUT /v1/games/{gameUUID}/logo", Handler: h.handleUploadLogo, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/logo/remove", Handler: h.handleRemoveLogo, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/photos", Handler: h.handleAddPhoto, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/photos/{photoID}/remove", Handler: h.handleRemovePhoto, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/publish", Handler: h.handlePublish, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/editions", Handler: h.handleOpenEdition, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}/draft", Handler: h.handleReadDraft, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/draft/changes", Handler: h.handleApplyDraftChanges, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}/draft/revisions", Handler: h.handleListDraftRevisions, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}/draft/changed", Handler: h.handleReadDraftChanged, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}/draft/diff", Handler: h.handleReadDraftDiff, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}/draft/files", Handler: h.handleReadDraftFiles, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}/draft/build", Handler: h.handleReadDraftBuild, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/draft/restore", Handler: h.handleRestoreDraftRevision, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/draft/template", Handler: h.handleUseTemplate, Authorization: edit},
		// Templates: any creator may find, read and start from them.
		{Pattern: "GET /v1/templates", Handler: h.handleBrowseTemplates, Authorization: create},
		{Pattern: "GET /v1/templates/search", Handler: h.handleSearchTemplates, Authorization: create},
		{Pattern: "GET /v1/templates/{templateGameUUID}", Handler: h.handleInspectTemplate, Authorization: create},
		{Pattern: "GET /v1/templates/{templateGameUUID}/files", Handler: h.handleReadTemplateFiles, Authorization: create},
		{Pattern: "POST /v1/games/{gameUUID}/draft/simulations", Handler: h.handleSimulateDraft, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}/draft/simulations/{resultID}/state", Handler: h.handleReadSimulationState, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/draft/previews", Handler: h.handleStartPreview, Authorization: edit},
		{Pattern: "PUT /v1/games/{gameUUID}/draft/assets/{key}", Handler: h.handleUploadAsset, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/draft/assets/{key}/remove", Handler: h.handleRemoveAsset, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/ai-link", Handler: h.handleCreateAILink, Authorization: edit},
		{Pattern: "GET /v1/games/{gameUUID}/ai-link", Handler: h.handleAILinkStatus, Authorization: edit},
		{Pattern: "POST /v1/games/{gameUUID}/ai-link/revoke", Handler: h.handleRevokeAILink, Authorization: edit},

		// An AI reaches its game through the link token in the path, acting
		// as the game's owner on that game only.
		{Pattern: "GET /ai/v1/{token}", Handler: h.handleAIContext, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/files/{path...}", Handler: h.handleAIFile, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/files", Handler: h.handleReadDraftFiles, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/changed", Handler: h.handleReadDraftChanged, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/diff", Handler: h.handleReadDraftDiff, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/sdk/{page...}", Handler: h.handleAISDKPage, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/revisions", Handler: h.handleListDraftRevisions, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/build", Handler: h.handleReadDraftBuild, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "POST /ai/v1/{token}/changes", Handler: h.handleApplyDraftChanges, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "POST /ai/v1/{token}/restore", Handler: h.handleRestoreDraftRevision, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "POST /ai/v1/{token}/template", Handler: h.handleUseTemplate, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/templates", Handler: h.handleBrowseTemplates, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/templates/search", Handler: h.handleSearchTemplates, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/templates/{templateGameUUID}", Handler: h.handleInspectTemplate, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/templates/{templateGameUUID}/files", Handler: h.handleReadTemplateFiles, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "POST /ai/v1/{token}/simulations", Handler: h.handleSimulateDraft, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
		{Pattern: "GET /ai/v1/{token}/simulations/{resultID}/state", Handler: h.handleReadSimulationState, Authorization: h.aiLinkAccess, Limits: &aiLinkLimits},
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
