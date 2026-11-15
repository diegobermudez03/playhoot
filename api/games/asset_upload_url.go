package games

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// resourceGrants is Identity's single-use grants: an upload URL is one.
type resourceGrants interface {
	CreateResourceGrant(ctx context.Context, userUUID string, kind string, resourceID string, scope string) (identity.CreatedResourceGrant, error)
	ClaimResourceGrant(ctx context.Context, token string) (identity.ClaimedResourceGrant, error)
	ReleaseResourceGrant(ctx context.Context, grantUUID string) error
}

const (
	// assetUploadScope starts the scope of an upload grant: "asset_upload
	// <origin> <key>". Keys and origins hold no spaces.
	assetUploadScope = "asset_upload"

	codeUploadURLInvalid = "upload_url_invalid"
	codeUploadURLUsed    = "upload_url_used"
	codeTooManyUploads   = "too_many_upload_urls"
)

// uploadURLLimits bound uploads through grant URLs per client address: the
// URL is the credential, so there is no user to limit by before it is read.
var uploadURLLimits = routing.RequestLimits{
	PerClient: []ratelimit.Limit{{Count: 10, Per: time.Second}, {Count: 120, Per: time.Minute}},
}

// AssetUploadURLResponse is where to send one asset's file, once, without
// passing it through an AI's context: PUT its bytes to URL as the body
// before ExpiresAt. The backend checks them as any upload.
type AssetUploadURLResponse struct {
	URL       string    `json:"upload_url"`
	Method    string    `json:"method"`
	Key       string    `json:"key"`
	Type      string    `json:"type"`
	MaxBytes  int64     `json:"max_bytes"`
	ExpiresAt time.Time `json:"expires_at"`
	How       string    `json:"how"`
}

// handleRequestAssetUpload answers a single-use upload URL for one declared
// asset of the game's version being built.
func (h *Handler) handleRequestAssetUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "requesting an upload URL")
	if !ok {
		return
	}
	noStore(w)
	response, refusal := h.RequestAssetUpload(ctx, Caller{UserUUID: userUUID, Origin: originOf(ctx)}, gameUUID, r.PathValue("key"))
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

// RequestAssetUpload answers a single-use URL to upload key's file of the
// game's version being built, for by: it works for 15 minutes
// and once, and the upload counts as by's, from by's origin.
func (h *Handler) RequestAssetUpload(ctx context.Context, by Caller, rawGameUUID string, key string) (*AssetUploadURLResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return nil, refusal
	}
	logging.LogFields(ctx, logging.Field("asset_key", key))
	read, err := h.reader.ReadDraft(ctx, by.UserUUID, gameUUID, 0, false)
	if err == nil && read.Missing {
		if refusal := h.ensureDraft(ctx, by.UserUUID, gameUUID); refusal != nil {
			return nil, refusal
		}
		read, err = h.reader.ReadDraft(ctx, by.UserUUID, gameUUID, 0, false)
	}
	if err != nil {
		logging.LogError(ctx, err)
		return nil, refuse(http.StatusInternalServerError, "requesting an upload URL failed")
	}
	if refusal := accessRefusal(read.Access); refusal != nil {
		return nil, refusal
	}
	if read.Missing {
		logging.LogFields(ctx, logging.Field("wiring_error", "draft still missing after it was ensured"))
		return nil, refuse(http.StatusInternalServerError, "requesting an upload URL failed")
	}
	var declared *sessionpkg.DraftAsset
	for i, asset := range read.Snapshot.Assets {
		if asset.Key == key && asset.Type != "" {
			declared = &read.Snapshot.Assets[i]
		}
	}
	if declared == nil {
		return nil, refuseCoded(http.StatusNotFound, codeAssetNotDeclared, "game.project.json declares no such asset; declare it first")
	}
	grant, err := h.links.Grants.CreateResourceGrant(ctx, by.UserUUID, game.ResourceKind, gameUUID, strings.Join([]string{assetUploadScope, by.Origin, key}, " "))
	if errors.Is(err, identity.ErrTooManyResourceGrants) {
		return nil, refuseCoded(http.StatusTooManyRequests, codeTooManyUploads, "too many unused upload URLs; use or let some expire first")
	}
	if err != nil {
		logging.LogError(ctx, err)
		return nil, refuse(http.StatusInternalServerError, "requesting an upload URL failed")
	}
	url := h.links.PublicBaseURL + "/v1/asset-uploads/" + grant.Token
	return &AssetUploadURLResponse{
		URL: url, Method: http.MethodPut, Key: key, Type: declared.Type,
		MaxBytes: sessionpkg.MaxDraftAssetBytesOf(declared.Type), ExpiresAt: grant.ExpiresAt,
		How: "Send the file's bytes as the body of one PUT to upload_url (for example: curl -T <file> " + url + "). It works once, until expires_at; the answer is the asset upload's. Never send the file through this conversation.",
	}, nil
}

type grantContextKey struct{}

// grantUploadAccess authorizes an upload by the grant token in its path,
// inside the route (after its rate limit):
// Identity claims it, and the request goes on as the grant's user on its
// game and key, from the origin that asked for it. The token is never
// logged.
func (h *Handler) grantUploadAccess(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")

		claimed, err := h.links.Grants.ClaimResourceGrant(ctx, r.PathValue("token"))
		if err != nil {
			logging.LogError(ctx, err)
			httpx.WriteError(w, http.StatusInternalServerError, "reading the upload URL failed")
			return
		}
		if claimed.Outcome == identity.ResourceGrantUsed {
			httpx.WriteCodedError(w, http.StatusGone, codeUploadURLUsed, "this upload URL was already used: ask for a new one")
			return
		}
		parts := strings.Split(claimed.Scope, " ")
		gameUUID, parseErr := uuid.Parse(claimed.ResourceID)
		if claimed.Outcome != identity.ResourceGrantClaimed || claimed.Kind != game.ResourceKind || parseErr != nil || len(parts) != 3 || parts[0] != assetUploadScope {
			if claimed.Outcome == identity.ResourceGrantClaimed {
				// Not an upload grant: leave it as it was.
				h.release(ctx, claimed.GrantUUID)
			}
			httpx.WriteCodedError(w, http.StatusUnauthorized, codeUploadURLInvalid, "this upload URL does not work or expired: ask for a new one")
			return
		}
		logging.LogFields(ctx, logging.Field("upload_grant_uuid", claimed.GrantUUID))
		ctx = auth.WithCaller(ctx, identity.User{UUID: claimed.UserUUID, Level: identity.LevelAccount})
		ctx = context.WithValue(ctx, originContextKey{}, parts[1])
		ctx = context.WithValue(ctx, grantContextKey{}, claimed.GrantUUID)
		r = r.WithContext(ctx)
		r.SetPathValue("gameUUID", gameUUID.String())
		r.SetPathValue("key", parts[2])
		next(w, r)
	}
}

// handleGrantUpload receives an asset's file through a single-use URL: one
// that attaches it uses the URL up; any other answer (a wrong or too large
// file, a body that broke off, a busy instance) gives it back, so it can be
// sent again until it expires. A refused upload never holds the grant's key.
func (h *Handler) handleGrantUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "uploading through a URL")
	if !ok {
		return
	}
	grantUUID, _ := ctx.Value(grantContextKey{}).(string)
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(assetUploadReadTime)); err != nil {
		logging.LogError(ctx, err)
	}
	body := http.MaxBytesReader(w, r.Body, sessionpkg.MaxDraftAssetBytes+1)
	// The grant is the request's identity: retries of a released URL replay
	// the same attempt.
	response, refusal := h.UploadAsset(ctx, Caller{UserUUID: userUUID, Origin: originOf(ctx)}, gameUUID, r.PathValue("key"), "upload-"+grantUUID, body)
	if refusal != nil {
		h.release(ctx, grantUUID)
	}
	writeAssetEdit(w, response, refusal)
}

// release gives a claimed grant back; failing to is logged, not answered.
func (h *Handler) release(ctx context.Context, grantUUID string) {
	if err := h.links.Grants.ReleaseResourceGrant(ctx, grantUUID); err != nil {
		logging.LogError(ctx, err)
	}
}
