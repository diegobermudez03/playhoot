package oauth

import (
	"errors"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

const (
	codeAuthorizationNotFound = "authorization_not_found"
	codeConnectionNotFound    = "connection_not_found"
)

type pendingAuthorizationResponse struct {
	ClientName   string    `json:"client_name"`
	ClientURI    string    `json:"client_uri,omitempty"`
	RedirectHost string    `json:"redirect_host"`
	Scopes       []string  `json:"scopes"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type decisionResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// handleReadAuthorization answers what the caller is asked to approve.
func (h *Handler) handleReadAuthorization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := callerUUID(w, r, "reading AI authorization"); !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	pending, err := h.server.ReadAuthorization(ctx, r.PathValue("requestID"))
	if errors.Is(err, identity.ErrAIAuthorizationNotFound) {
		httpx.WriteCodedError(w, http.StatusNotFound, codeAuthorizationNotFound, "this authorization request does not exist, expired or was already answered")
		return
	}
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "reading AI authorization failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, pendingAuthorizationResponse{
		ClientName: pending.ClientName, ClientURI: pending.ClientURI, RedirectHost: pending.RedirectHost, Scopes: pending.Scopes, ExpiresAt: pending.ExpiresAt,
	})
}

func (h *Handler) handleApprove(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, true)
}

func (h *Handler) handleDeny(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, false)
}

// decide records the caller's decision and answers where their browser goes
// back to the application.
func (h *Handler) decide(w http.ResponseWriter, r *http.Request, approve bool) {
	ctx := r.Context()
	userUUID, ok := callerUUID(w, r, "deciding AI authorization")
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	redirectURL, err := h.server.DecideAuthorization(ctx, userUUID, r.PathValue("requestID"), approve)
	if errors.Is(err, identity.ErrAIAuthorizationNotFound) {
		httpx.WriteCodedError(w, http.StatusNotFound, codeAuthorizationNotFound, "this authorization request does not exist, expired or was already answered")
		return
	}
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "deciding AI authorization failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, decisionResponse{RedirectURL: redirectURL})
}

type connectionResponse struct {
	ConnectionID string    `json:"connection_id"`
	ClientName   string    `json:"client_name"`
	CreatedAt    time.Time `json:"created_at"`
	LastUsedAt   time.Time `json:"last_used_at"`
}

// handleListConnections answers the caller's live AI connections.
func (h *Handler) handleListConnections(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, ok := callerUUID(w, r, "listing AI connections")
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	connections, err := h.server.ListConnections(ctx, userUUID)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "listing AI connections failed")
		return
	}
	listed := make([]connectionResponse, len(connections))
	for i, connection := range connections {
		listed[i] = connectionResponse{ConnectionID: connection.ConnectionID, ClientName: connection.ClientName, CreatedAt: connection.CreatedAt, LastUsedAt: connection.LastUsedAt}
	}
	httpx.WriteJSON(w, http.StatusOK, listed)
}

// handleRevokeConnection revokes one of the caller's AI connections.
func (h *Handler) handleRevokeConnection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, ok := callerUUID(w, r, "revoking AI connection")
	if !ok {
		return
	}
	revoked, err := h.server.RevokeConnection(ctx, userUUID, r.PathValue("connectionID"))
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "revoking AI connection failed")
		return
	}
	if !revoked {
		httpx.WriteCodedError(w, http.StatusNotFound, codeConnectionNotFound, "you have no such AI connection")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
