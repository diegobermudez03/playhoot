package games

import (
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
)

type aiLinkResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type aiLinkStatusResponse struct {
	Active    bool       `json:"active"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// aiPathPrefix is where the routes an AI reaches through its link live.
const aiPathPrefix = "/ai/v1/"

// handleCreateAILink creates the link an AI uses for the game, replacing the
// previous one. The link holds the token, which is answered only here.
func (h *Handler) handleCreateAILink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "creating AI link")
	if !ok {
		return
	}
	noStore(w)
	created, err := h.links.Owner.CreateAILink(ctx, userUUID, gameUUID)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "creating AI link failed")
		return
	}
	if !writeOwnerRefusal(w, created.Access) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, aiLinkResponse{URL: h.links.PublicBaseURL + aiPathPrefix + created.Link.Token, ExpiresAt: created.Link.ExpiresAt})
}

// handleAILinkStatus answers whether the game has a live link for an AI,
// never the link itself.
func (h *Handler) handleAILinkStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "reading AI link")
	if !ok {
		return
	}
	noStore(w)
	read, err := h.links.Status.AILinkStatus(ctx, userUUID, gameUUID)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "reading AI link failed")
		return
	}
	if !writeOwnerRefusal(w, read.Access) {
		return
	}
	response := aiLinkStatusResponse{Active: read.Status.Active}
	if read.Status.Active {
		createdAt, expiresAt := read.Status.CreatedAt, read.Status.ExpiresAt
		response.CreatedAt, response.ExpiresAt = &createdAt, &expiresAt
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

// handleRevokeAILink revokes the game's AI link; with none it does nothing.
func (h *Handler) handleRevokeAILink(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "revoking AI link")
	if !ok {
		return
	}
	access, err := h.links.Owner.RevokeAILink(ctx, userUUID, gameUUID)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "revoking AI link failed")
		return
	}
	if !writeOwnerRefusal(w, access) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeOwnerRefusal answers a caller who does not own the game, returning
// false: Game Management found an editable version, or none open, only for
// its owner.
func writeOwnerRefusal(w http.ResponseWriter, access game.EditableVersionOutcome) bool {
	switch access {
	case game.EditableVersionFound, game.EditableVersionNotEditable:
		return true
	default:
		httpx.WriteError(w, http.StatusNotFound, "game not found")
		return false
	}
}
