package identity

import (
	"encoding/json"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
)

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

const maxLogoutBodyBytes = 2 << 10

// handleLogout ends the session of the caller's refresh token, so it can no
// longer renew. Answered 204 whether or not a session was active.
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req logoutRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLogoutBodyBytes)).Decode(&req); err != nil || req.RefreshToken == "" {
		httpx.WriteError(w, http.StatusBadRequest, "a refresh_token is required")
		return
	}
	if err := h.sessions.EndSession(ctx, req.RefreshToken); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "signing out failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
